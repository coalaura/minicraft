package server

import (
	"slices"
	"strings"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

const (
	playerInventoryWindowID = 0
	outsideInventorySlot    = -999
	creativeDropSlot        = -1
	maxItemComponentType    = 103
)

const (
	clickModePickup int32 = iota
	clickModeQuickMove
	clickModeSwap
	clickModeClone
	clickModeThrow
	clickModeQuickCraft
	clickModePickupAll
)

type inventoryDragState struct {
	active bool
	button int8
	slots  []int
}

type equipmentSlotChange struct {
	before *game.ItemStack
	after  *game.ItemStack
	slot   byte
}

func (s *Session) handleSetHeldItem(selection protocol.SetHeldItem) {
	if selection.Slot < 0 || selection.Slot >= game.HotbarSlotCount {
		return
	}

	s.Runtime.lifecycleMu.Lock()
	defer s.Runtime.lifecycleMu.Unlock()

	player, changed := s.updatePlayerState(func(player *game.Player) bool {
		selected := int(selection.Slot)
		if player.SelectedHotbarSlot == selected {
			return false
		}

		player.SelectedHotbarSlot = selected

		return true
	})

	if changed {
		s.Runtime.broadcastPlayerEquipment(s, player, protocol.EquipmentSlotMainHand)
	}
}

func (s *Session) handleSetCreativeModeSlot(update protocol.SetCreativeModeSlot) {
	stack, valid := creativeItemStack(update.Item)
	if !valid {
		s.resynchronizePlayerInventory()

		return
	}

	if update.Slot == creativeDropSlot {
		return
	}

	if update.Slot <= 0 || update.Slot >= game.PlayerInventorySlots {
		s.resynchronizePlayerInventory()

		return
	}

	s.Runtime.lifecycleMu.Lock()
	defer s.Runtime.lifecycleMu.Unlock()

	var equipment []byte

	player, changed := s.updatePlayerState(func(player *game.Player) bool {
		if player.GameMode != game.GameModeCreative {
			return false
		}

		slot := player.Inventory.Slot(int(update.Slot))
		if slot == nil || slot.Equal(stack) {
			return false
		}

		before := player.Inventory.Clone()
		*slot = stack.Clone()

		player.Inventory.StateID = nextInventoryStateID(player.Inventory.StateID)

		equipment = changedEquipmentSlots(before, player.Inventory, player.SelectedHotbarSlot)

		return true
	})

	if !changed {
		if player.GameMode != game.GameModeCreative {
			s.resynchronizePlayerInventory()
		}

		return
	}

	err := s.sendPlayerInventorySnapshot(player.Inventory)
	if err != nil {
		s.Log.Warnf("[play] failed to synchronize creative inventory: %v\n", err)
	}

	if len(equipment) > 0 {
		s.Runtime.broadcastPlayerEquipment(s, player, equipment...)
	}
}

func (s *Session) handleContainerClick(click protocol.ContainerClick) error {
	s.Runtime.lifecycleMu.Lock()
	defer s.Runtime.lifecycleMu.Unlock()

	var (
		equipment []byte
		valid     bool
	)

	player, changed := s.updatePlayerState(func(player *game.Player) bool {
		if click.WindowID != playerInventoryWindowID || click.StateID != player.Inventory.StateID {
			return false
		}

		before := player.Inventory.Clone()
		candidate := before.Clone()

		if !s.applyInventoryClick(&candidate, player.GameMode, click) {
			return false
		}

		changedSlots := inventoryChanges(before, candidate)
		if !validPredictedInventory(click, candidate, changedSlots) {
			return false
		}

		valid = true
		if len(changedSlots) == 0 && before.Carried.Equal(candidate.Carried) {
			return false
		}

		candidate.StateID = nextInventoryStateID(before.StateID)

		player.Inventory = candidate

		equipment = changedEquipmentSlots(before, candidate, player.SelectedHotbarSlot)

		return true
	})

	if !valid {
		s.inventoryDrag = inventoryDragState{}

		return s.sendPlayerInventorySnapshot(player.Inventory)
	}

	if changed {
		err := s.sendPlayerInventorySnapshot(player.Inventory)
		if err != nil {
			return err
		}

		if len(equipment) > 0 {
			s.Runtime.broadcastPlayerEquipment(s, player, equipment...)
		}
	}

	return nil
}

func (s *Session) applyInventoryClick(inventory *game.PlayerInventory, mode game.GameMode, click protocol.ContainerClick) bool {
	switch click.Mode {
	case clickModePickup:
		s.inventoryDrag = inventoryDragState{}

		return applyPickup(inventory, int(click.Slot), click.MouseButton)
	case clickModeQuickMove:
		s.inventoryDrag = inventoryDragState{}

		return applyQuickMove(inventory, int(click.Slot), click.MouseButton)
	case clickModeSwap:
		s.inventoryDrag = inventoryDragState{}

		return applySwap(inventory, int(click.Slot), click.MouseButton)
	case clickModeClone:
		s.inventoryDrag = inventoryDragState{}

		return applyClone(inventory, mode, int(click.Slot), click.MouseButton)
	case clickModeThrow:
		s.inventoryDrag = inventoryDragState{}

		return applyThrow(inventory, int(click.Slot), click.MouseButton)
	case clickModeQuickCraft:
		return s.applyQuickCraft(inventory, mode, int(click.Slot), click.MouseButton)
	case clickModePickupAll:
		s.inventoryDrag = inventoryDragState{}

		return applyPickupAll(inventory, int(click.Slot), click.MouseButton)
	default:
		s.inventoryDrag = inventoryDragState{}

		return false
	}
}

func applyPickup(inventory *game.PlayerInventory, slot int, button int8) bool {
	if button != 0 && button != 1 {
		return false
	}

	if slot == outsideInventorySlot {
		if inventory.Carried.Empty() {
			return true
		}

		if button == 0 || inventory.Carried.Count == 1 {
			inventory.Carried = game.ItemStack{}
		} else {
			inventory.Carried.Count--
		}

		return true
	}

	target := inventory.Slot(slot)
	if target == nil {
		return false
	}

	if button == 0 {
		return applyLeftPickup(inventory, slot, target)
	}

	return applyRightPickup(inventory, slot, target)
}

func applyLeftPickup(inventory *game.PlayerInventory, slot int, target *game.ItemStack) bool {
	carried := &inventory.Carried

	if carried.Empty() {
		*carried = target.Clone()
		*target = game.ItemStack{}

		return true
	}

	if target.Empty() {
		if slotAcceptsItem(slot, *carried) {
			*target = carried.Clone()
			*carried = game.ItemStack{}
		}

		return true
	}

	if target.SameItem(*carried) {
		capacity := stackLimit(*target) - target.Count
		moved := min(capacity, carried.Count)

		if moved > 0 && slotAcceptsItem(slot, *carried) {
			target.Count += moved
			carried.Count -= moved

			normalizeStack(carried)
		}

		return true
	}

	if slotAcceptsItem(slot, *carried) {
		*target, *carried = carried.Clone(), target.Clone()
	}

	return true
}

func applyRightPickup(inventory *game.PlayerInventory, slot int, target *game.ItemStack) bool {
	carried := &inventory.Carried

	if carried.Empty() {
		if target.Empty() {
			return true
		}

		amount := (target.Count + 1) / 2
		*carried = target.Clone()

		carried.Count = amount
		target.Count -= amount

		normalizeStack(target)

		return true
	}

	if target.Empty() {
		if slotAcceptsItem(slot, *carried) {
			*target = carried.Clone()

			target.Count = 1
			carried.Count--

			normalizeStack(carried)
		}

		return true
	}

	if target.SameItem(*carried) {
		if target.Count < stackLimit(*target) && slotAcceptsItem(slot, *carried) {
			target.Count++
			carried.Count--

			normalizeStack(carried)
		}

		return true
	}

	if slotAcceptsItem(slot, *carried) {
		*target, *carried = carried.Clone(), target.Clone()
	}

	return true
}

func applyQuickMove(inventory *game.PlayerInventory, slot int, button int8) bool {
	if button != 0 && button != 1 {
		return false
	}

	source := inventory.Slot(slot)
	if source == nil {
		return false
	}

	if source.Empty() {
		return true
	}

	remaining := source.Clone()

	if slot >= 9 && slot <= 44 {
		armorSlot := armorSlotForItem(remaining)
		if armorSlot >= 0 && inventory.Slot(armorSlot).Empty() {
			moveIntoSlots(inventory, &remaining, []int{armorSlot})
		}
	}

	switch {
	case slot >= 9 && slot <= 35:
		moveIntoSlots(inventory, &remaining, slotRange(36, 44))
	case slot >= 36 && slot <= 44:
		moveIntoSlots(inventory, &remaining, slotRange(9, 35))
	default:
		moveIntoSlots(inventory, &remaining, slotRange(9, 44))
	}

	*source = remaining

	normalizeStack(source)

	return true
}

func applySwap(inventory *game.PlayerInventory, slot int, button int8) bool {
	target := inventory.Slot(slot)
	if target == nil {
		return false
	}

	var swapSlot int

	switch {
	case button >= 0 && button < game.HotbarSlotCount:
		swapSlot = 36 + int(button)
	case button == 40:
		swapSlot = 45
	default:
		return false
	}

	other := inventory.Slot(swapSlot)
	if slot == swapSlot {
		return true
	}

	if !target.Empty() && !slotAcceptsItem(swapSlot, *target) || !other.Empty() && !slotAcceptsItem(slot, *other) {
		return true
	}

	*target, *other = other.Clone(), target.Clone()

	return true
}

func applyClone(inventory *game.PlayerInventory, mode game.GameMode, slot int, button int8) bool {
	if mode != game.GameModeCreative || button != 2 {
		return false
	}

	target := inventory.Slot(slot)
	if target == nil {
		return false
	}

	if !target.Empty() {
		inventory.Carried = target.Clone()

		inventory.Carried.Count = stackLimit(inventory.Carried)
	}

	return true
}

func applyThrow(inventory *game.PlayerInventory, slot int, button int8) bool {
	if button != 0 && button != 1 {
		return false
	}

	target := inventory.Slot(slot)
	if target == nil {
		return false
	}

	if target.Empty() {
		return true
	}

	if button == 1 || target.Count == 1 {
		*target = game.ItemStack{}
	} else {
		target.Count--
	}

	return true
}

func (s *Session) applyQuickCraft(inventory *game.PlayerInventory, mode game.GameMode, slot int, button int8) bool {
	stage := button & 3
	dragButton := button >> 2

	if dragButton < 0 || dragButton > 2 || dragButton == 2 && mode != game.GameModeCreative {
		s.inventoryDrag = inventoryDragState{}

		return false
	}

	switch stage {
	case 0:
		if slot != outsideInventorySlot || inventory.Carried.Empty() {
			s.inventoryDrag = inventoryDragState{}

			return false
		}

		s.inventoryDrag = inventoryDragState{active: true, button: dragButton}

		return true
	case 1:
		if !s.inventoryDrag.active || s.inventoryDrag.button != dragButton || slot < 0 || slot >= game.PlayerInventorySlots {
			return false
		}

		target := inventory.Slot(slot)
		if slotAcceptsItem(slot, inventory.Carried) && (target.Empty() || target.SameItem(inventory.Carried)) && target.Count < stackLimit(inventory.Carried) && !containsSlot(s.inventoryDrag.slots, slot) {
			s.inventoryDrag.slots = append(s.inventoryDrag.slots, slot)
		}

		return true
	case 2:
		if slot != outsideInventorySlot || !s.inventoryDrag.active || s.inventoryDrag.button != dragButton || len(s.inventoryDrag.slots) == 0 {
			s.inventoryDrag = inventoryDragState{}

			return false
		}

		slots := append([]int(nil), s.inventoryDrag.slots...)

		s.inventoryDrag = inventoryDragState{}

		applyDragDistribution(inventory, slots, dragButton)

		return true
	default:
		s.inventoryDrag = inventoryDragState{}

		return false
	}
}

func applyDragDistribution(inventory *game.PlayerInventory, slots []int, button int8) {
	original := inventory.Carried.Clone()

	remaining := original.Count

	for _, slot := range slots {
		target := inventory.Slot(slot)

		capacity := stackLimit(original)
		if !target.Empty() {
			capacity -= target.Count
		}

		var amount int32

		switch button {
		case 0:
			amount = original.Count / int32(len(slots))
		case 1:
			amount = 1
		case 2:
			amount = capacity
		}

		amount = min(amount, capacity)
		if button != 2 {
			amount = min(amount, remaining)
		}

		if amount <= 0 {
			continue
		}

		if target.Empty() {
			*target = original.Clone()

			target.Count = amount
		} else {
			target.Count += amount
		}

		if button != 2 {
			remaining -= amount
		}
	}

	if button == 2 {
		inventory.Carried = original
	} else {
		inventory.Carried.Count = remaining

		normalizeStack(&inventory.Carried)
	}
}

func applyPickupAll(inventory *game.PlayerInventory, slot int, button int8) bool {
	if button != 0 || slot < 0 || slot >= game.PlayerInventorySlots {
		return false
	}

	if inventory.Carried.Empty() {
		return true
	}

	limit := stackLimit(inventory.Carried)

	for current := range game.PlayerInventorySlots {
		stack := inventory.Slot(current)
		if stack.Empty() || !stack.SameItem(inventory.Carried) {
			continue
		}

		moved := min(stack.Count, limit-inventory.Carried.Count)

		inventory.Carried.Count += moved
		stack.Count -= moved

		normalizeStack(stack)

		if inventory.Carried.Count == limit {
			break
		}
	}

	return true
}

func moveIntoSlots(inventory *game.PlayerInventory, stack *game.ItemStack, slots []int) {
	for _, slot := range slots {
		target := inventory.Slot(slot)
		if target.Empty() || !target.SameItem(*stack) || !slotAcceptsItem(slot, *stack) {
			continue
		}

		moved := min(stackLimit(*target)-target.Count, stack.Count)
		if moved <= 0 {
			continue
		}

		target.Count += moved
		stack.Count -= moved

		if stack.Count == 0 {
			return
		}
	}

	for _, slot := range slots {
		target := inventory.Slot(slot)
		if !target.Empty() || !slotAcceptsItem(slot, *stack) {
			continue
		}

		moved := min(stackLimit(*stack), stack.Count)
		*target = stack.Clone()

		target.Count = moved
		stack.Count -= moved

		if stack.Count == 0 {
			return
		}
	}
}

func (s *Session) handleDropHeldItem(dropAll bool) {
	s.Runtime.lifecycleMu.Lock()
	defer s.Runtime.lifecycleMu.Unlock()

	player, changed := s.updatePlayerState(func(player *game.Player) bool {
		stack := player.Inventory.Held(player.SelectedHotbarSlot)
		if stack == nil || stack.Empty() {
			return false
		}

		if dropAll || stack.Count == 1 {
			*stack = game.ItemStack{}
		} else {
			stack.Count--
		}

		player.Inventory.StateID = nextInventoryStateID(player.Inventory.StateID)

		return true
	})

	if changed {
		err := s.sendPlayerInventorySnapshot(player.Inventory)
		if err != nil {
			s.Log.Warnf("[play] failed to synchronize dropped held item: %v\n", err)
		}

		s.Runtime.broadcastPlayerEquipment(s, player, protocol.EquipmentSlotMainHand)
	}
}

func (s *Session) handleSwapWithOffhand() {
	s.Runtime.lifecycleMu.Lock()
	defer s.Runtime.lifecycleMu.Unlock()

	player, changed := s.updatePlayerState(func(player *game.Player) bool {
		held := player.Inventory.Held(player.SelectedHotbarSlot)
		if held == nil {
			return false
		}

		*held, player.Inventory.Offhand = player.Inventory.Offhand.Clone(), held.Clone()

		player.Inventory.StateID = nextInventoryStateID(player.Inventory.StateID)

		return true
	})

	if changed {
		err := s.sendPlayerInventorySnapshot(player.Inventory)
		if err != nil {
			s.Log.Warnf("[play] failed to synchronize offhand swap: %v\n", err)
		}

		s.Runtime.broadcastPlayerEquipment(s, player, protocol.EquipmentSlotMainHand, protocol.EquipmentSlotOffHand)
	}
}

func (s *Session) sendPlayerInventory() error {
	return s.sendPlayerInventorySnapshot(s.snapshotPlayer().Inventory)
}

func (s *Session) sendPlayerInventorySnapshot(inventory game.PlayerInventory) error {
	return s.writePacket(protocol.ClientboundContainerSetContentID, protocol.ContainerSetContent{
		WindowID:    playerInventoryWindowID,
		StateID:     inventory.StateID,
		Items:       inventory.Contents(),
		CarriedItem: inventory.Carried,
	})
}

func (s *Session) resynchronizePlayerInventory() {
	err := s.sendPlayerInventory()
	if err != nil {
		s.Log.Warnf("[play] failed to resynchronize player inventory: %v\n", err)
	}
}

func (r *Runtime) broadcastPlayerEquipment(session *Session, player game.Player, slots ...byte) {
	r.mu.RLock()
	_, active := r.sessions[session]
	r.mu.RUnlock()

	if !active {
		return
	}

	for _, other := range r.snapshotSessions() {
		if other == session || !playersVisible(other.snapshotPlayer(), player, other.renderDistance()) {
			continue
		}

		err := other.sendPlayerEquipment(player, slots...)
		if err != nil {
			other.Log.Warnf("[play] failed to update player equipment: %v\n", err)
		}
	}
}

func (s *Session) heldItem(hand int32) (game.ItemStack, bool) {
	player := s.snapshotPlayer()

	switch hand {
	case protocol.MainHand:
		stack := player.Inventory.Held(player.SelectedHotbarSlot)
		if stack == nil {
			return game.ItemStack{}, false
		}

		return stack.Clone(), true
	case protocol.OffHand:
		return player.Inventory.Offhand.Clone(), true
	default:
		return game.ItemStack{}, false
	}
}

func creativeItemStack(item protocol.UntrustedSlot) (game.ItemStack, bool) {
	if item.ItemCount == 0 {
		return game.ItemStack{}, len(item.RemovedComponents) == 0
	}

	if item.ItemCount < 0 || item.ItemID < 0 || item.ItemID > int32(game.MaxItemID) {
		return game.ItemStack{}, false
	}

	itemID := game.Item(item.ItemID)
	definition, valid := itemID.Definition()

	if !valid || item.ItemCount > definition.StackSize {
		return game.ItemStack{}, false
	}

	removed := make([]int32, len(item.RemovedComponents))
	seen := make(map[int32]struct{}, len(removed))

	for index, componentType := range item.RemovedComponents {
		if componentType < 0 || componentType > maxItemComponentType {
			return game.ItemStack{}, false
		}

		if _, exists := seen[componentType]; exists {
			return game.ItemStack{}, false
		}

		seen[componentType] = struct{}{}
		removed[index] = componentType
	}

	stack := game.ItemStack{Item: itemID, Count: item.ItemCount, RemovedComponents: removed}
	if stack.Empty() {
		return game.ItemStack{}, false
	}

	return stack, true
}

func validPredictedInventory(click protocol.ContainerClick, inventory game.PlayerInventory, changed []int) bool {
	if len(click.ChangedSlots) != len(changed) || !hashedSlotMatches(click.CursorItem, inventory.Carried) {
		return false
	}

	expected := make(map[int]struct{}, len(changed))

	for _, slot := range changed {
		expected[slot] = struct{}{}
	}

	seen := make(map[int]struct{}, len(click.ChangedSlots))

	for _, prediction := range click.ChangedSlots {
		slot := int(prediction.Location)
		if _, ok := expected[slot]; !ok {
			return false
		}

		if _, duplicate := seen[slot]; duplicate {
			return false
		}

		seen[slot] = struct{}{}
		if !hashedSlotMatches(prediction.Item, *inventory.Slot(slot)) {
			return false
		}
	}

	return true
}

func hashedSlotMatches(hashed protocol.HashedSlot, stack game.ItemStack) bool {
	if stack.Empty() {
		return !hashed.Present
	}

	if !hashed.Present || hashed.ItemID != int32(stack.Item) || hashed.ItemCount != stack.Count || len(hashed.Components) != len(stack.Components) || len(hashed.RemovedComponents) != len(stack.RemovedComponents) {
		return false
	}

	if len(stack.Components) != 0 {
		return false
	}

	for index, componentType := range stack.RemovedComponents {
		if hashed.RemovedComponents[index] != componentType {
			return false
		}
	}

	return true
}

func inventoryChanges(before, after game.PlayerInventory) []int {
	var changed []int

	for slot := range game.PlayerInventorySlots {
		if !before.Slot(slot).Equal(*after.Slot(slot)) {
			changed = append(changed, slot)
		}
	}

	return changed
}

func changedEquipmentSlots(before, after game.PlayerInventory, selected int) []byte {
	var changed []byte

	heldBefore := before.Held(selected)
	heldAfter := after.Held(selected)

	if heldBefore != nil && heldAfter != nil && !heldBefore.Equal(*heldAfter) {
		changed = append(changed, protocol.EquipmentSlotMainHand)
	}

	equipment := [...]equipmentSlotChange{
		{before.Slot(45), after.Slot(45), protocol.EquipmentSlotOffHand},
		{before.Slot(8), after.Slot(8), protocol.EquipmentSlotFeet},
		{before.Slot(7), after.Slot(7), protocol.EquipmentSlotLegs},
		{before.Slot(6), after.Slot(6), protocol.EquipmentSlotChest},
		{before.Slot(5), after.Slot(5), protocol.EquipmentSlotHead},
	}

	for _, entry := range equipment {
		if !entry.before.Equal(*entry.after) {
			changed = append(changed, entry.slot)
		}
	}

	return changed
}

func slotAcceptsItem(slot int, stack game.ItemStack) bool {
	if stack.Empty() || slot == 0 {
		return false
	}

	if slot >= 5 && slot <= 8 {
		return armorSlotForItem(stack) == slot
	}

	return slot > 0 && slot < game.PlayerInventorySlots
}

func armorSlotForItem(stack game.ItemStack) int {
	definition, valid := stack.Item.Definition()
	if !valid {
		return -1
	}

	switch {
	case strings.HasSuffix(definition.Name, "_helmet"), definition.Name == "carved_pumpkin":
		return 5
	case strings.HasSuffix(definition.Name, "_chestplate"), definition.Name == "elytra":
		return 6
	case strings.HasSuffix(definition.Name, "_leggings"):
		return 7
	case strings.HasSuffix(definition.Name, "_boots"):
		return 8
	default:
		return -1
	}
}

func stackLimit(stack game.ItemStack) int32 {
	definition, valid := stack.Item.Definition()
	if !valid || definition.StackSize <= 0 {
		return 0
	}

	return definition.StackSize
}

func normalizeStack(stack *game.ItemStack) {
	if stack.Count <= 0 {
		*stack = game.ItemStack{}
	}
}

func slotRange(first, last int) []int {
	slots := make([]int, 0, last-first+1)

	for slot := first; slot <= last; slot++ {
		slots = append(slots, slot)
	}

	return slots
}

func containsSlot(slots []int, target int) bool {
	return slices.Contains(slots, target)
}

func nextInventoryStateID(stateID int32) int32 {
	return (stateID + 1) & 32767
}
