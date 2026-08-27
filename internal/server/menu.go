package server

import (
	"slices"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

const noMenuSlot = -1

type menuSlotRole uint8

type menuStorage uint8

const (
	menuSlotNormal menuSlotRole = iota
	menuSlotResult
	menuSlotArmor
)

const (
	menuStorageNone menuStorage = iota
	menuStoragePlayer
	menuStorageBacking
)

type menuBacking interface {
	Position() game.BlockPosition
	Attach(*Runtime, *Session)
	Detach(*Runtime, *Session)
	Changed(*Runtime, *Session)
	StillValid(*Runtime, *Session) bool
}

type menuSlot struct {
	stack         *game.ItemStack
	role          menuSlotRole
	armorIndex    int
	limit         int32
	playerSlot    int
	hasPlayerSlot bool
	storage       menuStorage
}

type menuQuickMove func(*menuCandidate, int)

type menu struct {
	windowID         int32
	protocolMenuType int32
	stateID          int32
	carried          game.ItemStack
	drag             inventoryDragState
	slots            []menuSlot

	hotbarSlots    [game.HotbarSlotCount]int
	hasHotbarSlots [game.HotbarSlotCount]bool
	offhandSlot    int
	hasOffhandSlot bool
	hiddenOffhand  *game.ItemStack
	quickMove      menuQuickMove
	backing        menuBacking
	containerSlots int
}

type menuCandidate struct {
	menu          *menu
	slots         []game.ItemStack
	carried       game.ItemStack
	hiddenOffhand game.ItemStack
}

type menuSnapshot struct {
	windowID int32
	stateID  int32
	items    []game.ItemStack
	carried  game.ItemStack
}

func newPlayerInventoryMenu(inventory *game.PlayerInventory) *menu {
	slots := make([]menuSlot, game.PlayerInventorySlots)

	for slot := range slots {
		slots[slot] = menuSlot{
			stack:         inventory.Slot(slot),
			limit:         64,
			playerSlot:    slot,
			hasPlayerSlot: true,
			storage:       menuStoragePlayer,
		}
	}

	slots[0].role = menuSlotResult

	for slot := 5; slot <= 8; slot++ {
		slots[slot].role = menuSlotArmor
		slots[slot].armorIndex = slot
		slots[slot].limit = 1
	}

	playerMenu := &menu{
		windowID:       playerInventoryWindowID,
		slots:          slots,
		offhandSlot:    45,
		hasOffhandSlot: true,
		quickMove:      quickMovePlayerInventory,
	}

	for hotbar := range game.HotbarSlotCount {
		playerMenu.hotbarSlots[hotbar] = 36 + hotbar
		playerMenu.hasHotbarSlots[hotbar] = true
	}

	return playerMenu
}

func newGenericContainerMenu(windowID int32, rows int, container []game.ItemStack, inventory *game.PlayerInventory) *menu {
	containerSlots := rows * 9

	menuType, validMenuType := protocol.Generic9xMenuType(rows)
	if !validMenuType || len(container) != containerSlots {
		return nil
	}

	slots := make([]menuSlot, 0, containerSlots+36)

	for slot := range container {
		slots = append(slots, menuSlot{stack: &container[slot], limit: 64, storage: menuStorageBacking})
	}

	for playerSlot := 9; playerSlot <= 44; playerSlot++ {
		slots = append(slots, menuSlot{
			stack:         inventory.Slot(playerSlot),
			limit:         64,
			playerSlot:    playerSlot,
			hasPlayerSlot: true,
			storage:       menuStoragePlayer,
		})
	}

	containerMenu := &menu{
		windowID:         windowID,
		protocolMenuType: menuType,
		slots:            slots,
		hiddenOffhand:    inventory.Slot(45),
		quickMove:        quickMoveGenericContainer,
		containerSlots:   containerSlots,
	}

	for hotbar := range game.HotbarSlotCount {
		containerMenu.hotbarSlots[hotbar] = containerSlots + 27 + hotbar
		containerMenu.hasHotbarSlots[hotbar] = true
	}

	return containerMenu
}

func (candidate *menuCandidate) backingChanged() bool {
	for slot := range candidate.slots {
		definition := candidate.menu.slots[slot]
		if definition.storage == menuStorageBacking && !definition.stack.Equal(candidate.slots[slot]) {
			return true
		}
	}

	return false
}

func (current *menu) candidate() *menuCandidate {
	candidate := &menuCandidate{
		menu:    current,
		slots:   make([]game.ItemStack, len(current.slots)),
		carried: current.carried.Clone(),
	}

	for slot := range current.slots {
		candidate.slots[slot] = current.slots[slot].stack.Clone()
	}

	if current.hiddenOffhand != nil {
		candidate.hiddenOffhand = current.hiddenOffhand.Clone()
	}

	return candidate
}

func (current *menu) snapshot() menuSnapshot {
	candidate := current.candidate()

	return menuSnapshot{
		windowID: current.windowID,
		stateID:  current.stateID,
		items:    candidate.slots,
		carried:  candidate.carried,
	}
}

func (current *menu) commit(candidate *menuCandidate) {
	for slot := range current.slots {
		*current.slots[slot].stack = candidate.slots[slot].Clone()
	}

	if current.hiddenOffhand != nil {
		*current.hiddenOffhand = candidate.hiddenOffhand.Clone()
	}

	current.carried = candidate.carried.Clone()
}

func (current *menu) incrementStateID() {
	current.stateID = nextMenuStateID(current.stateID)
}

func (current *menu) exposesPlayerSlots(changed []int) bool {
	for _, menuSlot := range current.slots {
		if menuSlot.hasPlayerSlot && slices.Contains(changed, menuSlot.playerSlot) {
			return true
		}
	}

	return false
}

func (current *menu) resetDrag() {
	current.drag = inventoryDragState{}
}

func (candidate *menuCandidate) slot(slot int) *game.ItemStack {
	if slot < 0 || slot >= len(candidate.slots) {
		return nil
	}

	return &candidate.slots[slot]
}

func (candidate *menuCandidate) accepts(slot int, stack game.ItemStack) bool {
	if stack.Empty() || slot < 0 || slot >= len(candidate.menu.slots) {
		return false
	}

	menuSlot := candidate.menu.slots[slot]

	switch menuSlot.role {
	case menuSlotResult:
		return false
	case menuSlotArmor:
		return armorSlotForItem(stack) == menuSlot.armorIndex
	default:
		return true
	}
}

func (candidate *menuCandidate) stackLimit(slot int, stack game.ItemStack) int32 {
	limit := stackLimit(stack)
	if slot < 0 || slot >= len(candidate.menu.slots) {
		return limit
	}

	slotLimit := candidate.menu.slots[slot].limit
	if slotLimit > 0 {
		limit = min(limit, slotLimit)
	}

	return limit
}

func (candidate *menuCandidate) changedSlots() []int {
	var changed []int

	for slot := range candidate.slots {
		if !candidate.menu.slots[slot].stack.Equal(candidate.slots[slot]) {
			changed = append(changed, slot)
		}
	}

	return changed
}

func quickMovePlayerInventory(candidate *menuCandidate, slot int) {
	remaining := candidate.slots[slot].Clone()

	if slot >= 9 && slot <= 44 {
		armorSlot := armorSlotForItem(remaining)
		if armorSlot >= 0 && candidate.slots[armorSlot].Empty() {
			moveIntoSlots(candidate, &remaining, []int{armorSlot})
		}
	}

	switch {
	case slot >= 9 && slot <= 35:
		moveIntoSlots(candidate, &remaining, slotRange(36, 44))
	case slot >= 36 && slot <= 44:
		moveIntoSlots(candidate, &remaining, slotRange(9, 35))
	default:
		moveIntoSlots(candidate, &remaining, slotRange(9, 44))
	}

	candidate.slots[slot] = remaining

	normalizeStack(&candidate.slots[slot])
}

func quickMoveGenericContainer(candidate *menuCandidate, slot int) {
	remaining := candidate.slots[slot].Clone()

	containerSlots := candidate.menu.containerSlots

	if slot < containerSlots {
		moveIntoSlots(candidate, &remaining, reverseSlotRange(containerSlots, len(candidate.slots)-1))
	} else {
		moveIntoSlots(candidate, &remaining, slotRange(0, containerSlots-1))
	}

	candidate.slots[slot] = remaining

	normalizeStack(&candidate.slots[slot])
}
