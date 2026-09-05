package server

import (
	"slices"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

const noMenuSlot = -1

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

type menuSlotRole uint8

type menuStorage uint8

type menuBacking interface {
	ContainsPosition(game.BlockPosition) bool
	Attach(*Runtime, *Session)
	Detach(*Runtime, *Session)
	Changed(*Runtime, *Session, []int)
	StillValid(*Runtime, *Session) bool
}

type menuInventoryBacking struct {
	items []game.ItemStack
	index int
}

type menuSlot struct {
	stack         *game.ItemStack
	role          menuSlotRole
	derived       bool
	armorIndex    int
	limit         int32
	playerSlot    int
	hasPlayerSlot bool
	storage       menuStorage
	backingIndex  int
	onTake        menuResultTake
	accepts       menuSlotAccepts
}

type menuQuickMove func(*menuCandidate, int)

type menuResultTake func(*menuCandidate, int, game.ItemStack)

type menuSlotAccepts func(*menuCandidate, int, game.ItemStack) bool

type menuDerive func(*menuCandidate)

type menuRemoved func(*menuCandidate, bool)

type menu struct {
	windowID         int32
	protocolMenuType int32
	stateID          int32
	carried          game.ItemStack
	drag             inventoryDragState
	slots            []menuSlot

	hotbarSlots     [game.HotbarSlotCount]int
	hasHotbarSlots  [game.HotbarSlotCount]bool
	offhandSlot     int
	hasOffhandSlot  bool
	hiddenOffhand   *game.ItemStack
	quickMove       menuQuickMove
	derive          menuDerive
	removed         menuRemoved
	backing         menuBacking
	containerSlots  int
	data            []*int32
	lastData        []int32
	dataInitialized bool
}

type menuCandidate struct {
	menu          *menu
	slots         []game.ItemStack
	carried       game.ItemStack
	hiddenOffhand game.ItemStack
	selected      int
	dropped       []game.ItemStack
}

type menuSnapshot struct {
	windowID int32
	stateID  int32
	items    []game.ItemStack
	carried  game.ItemStack
}

func (candidate *menuCandidate) changedBackings() []int {
	seen := make(map[int]struct{})

	var changed []int

	for slot := range candidate.slots {
		definition := candidate.menu.slots[slot]
		if definition.storage != menuStorageBacking || definition.stack.Equal(candidate.slots[slot]) {
			continue
		}

		if _, present := seen[definition.backingIndex]; present {
			continue
		}

		seen[definition.backingIndex] = struct{}{}
		changed = append(changed, definition.backingIndex)
	}

	return changed
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
	if menuSlot.accepts != nil {
		return menuSlot.accepts(candidate, slot, stack)
	}

	switch menuSlot.role {
	case menuSlotResult:
		return false
	case menuSlotArmor:
		return armorSlotForItem(stack) == menuSlot.armorIndex
	default:
		return true
	}
}

func canRemoveFromArmorSlot(mode game.GameMode, slot int, stack game.ItemStack) bool {
	if mode == game.GameModeCreative || slot < 5 || slot > 8 {
		return true
	}

	return !stack.PreventsArmorChange()
}

func (candidate *menuCandidate) take(slot int, amount int32) game.ItemStack {
	target := candidate.slot(slot)
	if target == nil || target.Empty() || amount <= 0 {
		return game.ItemStack{}
	}

	taken := target.Clone()

	taken.Count = min(taken.Count, amount)
	target.Count -= taken.Count

	normalizeStack(target)

	definition := candidate.menu.slots[slot]
	if definition.onTake != nil {
		definition.onTake(candidate, slot, taken)
	}

	return taken
}

func (candidate *menuCandidate) deriveSlots() {
	if candidate.menu.derive != nil {
		candidate.menu.derive(candidate)
	}
}

func (candidate *menuCandidate) appendDrop(stack game.ItemStack) {
	if stack.Empty() {
		return
	}

	candidate.dropped = append(candidate.dropped, stack.Clone())
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

func (candidate *menuCandidate) playerStack(playerSlot int) *game.ItemStack {
	if playerSlot == 45 && candidate.menu.hiddenOffhand != nil {
		return &candidate.hiddenOffhand
	}

	for slot, definition := range candidate.menu.slots {
		if definition.hasPlayerSlot && definition.playerSlot == playerSlot {
			return &candidate.slots[slot]
		}
	}

	return nil
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
	slots[0].derived = true
	slots[0].onTake = takeCraftingResult

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
		derive:         derivePlayerCraftingResult,
	}

	for hotbar := range game.HotbarSlotCount {
		playerMenu.hotbarSlots[hotbar] = 36 + hotbar
		playerMenu.hasHotbarSlots[hotbar] = true
	}

	candidate := playerMenu.candidate()

	candidate.deriveSlots()

	playerMenu.commit(candidate)

	return playerMenu
}

func newGenericContainerMenu(windowID int32, rows int, container []game.ItemStack, inventory *game.PlayerInventory) *menu {
	return newComposedGenericContainerMenu(windowID, rows, []menuInventoryBacking{{items: container}}, inventory)
}

func newComposedGenericContainerMenu(windowID int32, rows int, backings []menuInventoryBacking, inventory *game.PlayerInventory) *menu {
	containerSlots := rows * 9

	menuType, validMenuType := protocol.Generic9xMenuType(rows)
	if !validMenuType {
		return nil
	}

	slots := make([]menuSlot, 0, containerSlots+36)

	for _, backing := range backings {
		for slot := range backing.items {
			slots = append(slots, menuSlot{
				stack:        &backing.items[slot],
				limit:        64,
				storage:      menuStorageBacking,
				backingIndex: backing.index,
			})
		}
	}

	if len(slots) != containerSlots {
		return nil
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

func quickMoveCraftingTable(candidate *menuCandidate, slot int) {
	remaining := candidate.slots[slot].Clone()

	switch {
	case slot == 0:
		moveIntoSlots(candidate, &remaining, reverseSlotRange(10, 45))
	case slot >= 1 && slot <= 9:
		moveIntoSlots(candidate, &remaining, slotRange(10, 45))
	case slot >= 10 && slot <= 45:
		before := remaining.Clone()

		moveIntoSlots(candidate, &remaining, slotRange(1, 9))

		if remaining.Equal(before) {
			if slot <= 36 {
				moveIntoSlots(candidate, &remaining, slotRange(37, 45))
			} else {
				moveIntoSlots(candidate, &remaining, slotRange(10, 36))
			}
		}
	}

	candidate.slots[slot] = remaining

	normalizeStack(&candidate.slots[slot])
}

func derivePlayerCraftingResult(candidate *menuCandidate) {
	deriveCraftingResult(candidate, 2, 2, slotRange(1, 4), 0)
}

func deriveCraftingResult(candidate *menuCandidate, width, height int, inputSlots []int, resultSlot int) {
	inputs := make([]game.ItemStack, len(inputSlots))

	for index, slot := range inputSlots {
		inputs[index] = candidate.slots[slot].Clone()
	}

	recipe, matched := game.MatchCrafting(width, height, inputs)
	if !matched {
		candidate.slots[resultSlot] = game.ItemStack{}

		return
	}

	candidate.slots[resultSlot] = recipe.Result()
}

func takeCraftingResult(candidate *menuCandidate, _ int, _ game.ItemStack) {
	takeCraftingInputs(candidate, slotRange(1, 4))

	candidate.deriveSlots()
}

func deriveCraftingTableResult(candidate *menuCandidate) {
	deriveCraftingResult(candidate, 3, 3, slotRange(1, 9), 0)
}

func takeCraftingTableResult(candidate *menuCandidate, _ int, _ game.ItemStack) {
	takeCraftingInputs(candidate, slotRange(1, 9))

	candidate.deriveSlots()
}

func removeCraftingTableItems(candidate *menuCandidate, disconnected bool) {
	for slot := 1; slot <= 9; slot++ {
		stack := &candidate.slots[slot]
		if stack.Empty() {
			continue
		}

		if !disconnected {
			moveIntoPlayerInventory(candidate, stack)
		}

		candidate.appendDrop(*stack)

		*stack = game.ItemStack{}
	}

	candidate.slots[0] = game.ItemStack{}
}

func takeCraftingInputs(candidate *menuCandidate, inputSlots []int) {
	for _, slot := range inputSlots {
		input := &candidate.slots[slot]
		if input.Empty() {
			continue
		}

		remainderItem, hasRemainder := game.CraftingRemainder(input.Item)

		input.Count--

		normalizeStack(input)

		if !hasRemainder {
			continue
		}

		remainder := game.ItemStack{Item: remainderItem, Count: 1}

		if input.Empty() {
			*input = remainder

			continue
		}

		if input.SameItem(remainder) {
			input.Count++

			continue
		}

		moveIntoPlayerInventory(candidate, &remainder)
		candidate.appendDrop(remainder)
	}
}

func moveIntoPlayerInventory(candidate *menuCandidate, stack *game.ItemStack) {
	mergeSlots := []int{36 + candidate.selected, 45}
	mergeSlots = append(mergeSlots, slotRange(36, 44)...)
	mergeSlots = append(mergeSlots, slotRange(9, 35)...)

	for _, playerSlot := range mergeSlots {
		target := candidate.playerStack(playerSlot)
		if target == nil || target.Empty() || !target.SameItem(*stack) {
			continue
		}

		capacity := stackLimit(*target) - target.Count
		moved := min(capacity, stack.Count)

		target.Count += moved
		stack.Count -= moved

		if stack.Count <= 0 {
			*stack = game.ItemStack{}

			return
		}
	}

	emptySlots := append(slotRange(36, 44), slotRange(9, 35)...)

	for _, playerSlot := range emptySlots {
		target := candidate.playerStack(playerSlot)
		if target == nil || !target.Empty() {
			continue
		}

		moved := min(stackLimit(*stack), stack.Count)

		*target = stack.Clone()

		target.Count = moved
		stack.Count -= moved

		if stack.Count <= 0 {
			*stack = game.ItemStack{}

			return
		}
	}
}
