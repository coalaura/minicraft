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

	var before game.PlayerInventory

	player, changed := s.updatePlayerState(func(player *game.Player) bool {
		if player.GameMode != game.GameModeCreative {
			return false
		}

		slot := player.Inventory.Slot(int(update.Slot))
		if slot == nil || slot.Equal(stack) {
			return false
		}

		before = player.Inventory.Clone()
		*slot = stack.Clone()

		return true
	})

	if !changed {
		if player.GameMode != game.GameModeCreative {
			s.resynchronizePlayerInventory()
		}

		return
	}

	playerSlot := int(update.Slot)

	err := s.synchronizePlayerInventoryMutationSlot(before, &playerSlot)

	if err != nil {
		s.Log.Warnf("[play] failed to synchronize creative inventory slot: %v\n", err)
	}

}

func (s *Session) handlePickItemFromBlock(pick protocol.PickItemFromBlock) {
	if !s.hasLoadedBlock(pick.Position) {
		return
	}

	block := s.Runtime.World.BlockAt(pick.Position)

	item, valid := game.ItemForBlock(block)
	if !valid {
		return
	}

	definition, valid := item.Definition()
	if !valid || definition.StackSize <= 0 {
		return
	}

	pickedStack := game.ItemStack{Item: item, Count: 1}

	s.Runtime.lifecycleMu.Lock()
	defer s.Runtime.lifecycleMu.Unlock()

	var (
		before           game.PlayerInventory
		inventoryChanged bool
		selectionChanged bool
	)

	player, changed := s.updatePlayerState(func(player *game.Player) bool {
		if player.GameMode != game.GameModeCreative {
			return false
		}

		for slot := range player.Inventory.Hotbar {
			if !player.Inventory.Hotbar[slot].SameItem(pickedStack) {
				continue
			}

			if player.SelectedHotbarSlot == slot {
				return false
			}

			player.SelectedHotbarSlot = slot

			selectionChanged = true

			return true
		}

		selectedSlot := player.SelectedHotbarSlot
		targetSlot := selectedSlot

		for offset := range game.HotbarSlotCount {
			slot := (selectedSlot + offset) % game.HotbarSlotCount
			if player.Inventory.Hotbar[slot].Empty() {
				targetSlot = slot

				break
			}
		}

		held := player.Inventory.Held(targetSlot)
		if held == nil {
			return false
		}

		before = player.Inventory.Clone()

		*held = pickedStack

		player.SelectedHotbarSlot = targetSlot

		inventoryChanged = true
		selectionChanged = targetSlot != selectedSlot

		return true
	})

	if !changed {
		return
	}

	if selectionChanged {
		err := s.writePacket(protocol.ClientboundSetHeldSlotID, protocol.SetHeldSlot{Slot: int32(player.SelectedHotbarSlot)})
		if err != nil {
			s.Log.Warnf("[play] failed to synchronize picked hotbar slot: %v\n", err)
		}
	}

	if inventoryChanged {
		err := s.synchronizePlayerInventoryMutation(before)
		if err != nil {
			s.Log.Warnf("[play] failed to synchronize picked block item: %v\n", err)
		}
	}

	if selectionChanged && !inventoryChanged {
		s.Runtime.broadcastPlayerEquipment(s, player, protocol.EquipmentSlotMainHand)
	}
}

func (s *Session) handleContainerClick(click protocol.ContainerClick) error {
	s.Runtime.worldMutationMu.Lock()
	defer s.Runtime.worldMutationMu.Unlock()

	s.Runtime.lifecycleMu.Lock()
	defer s.Runtime.lifecycleMu.Unlock()

	currentMenu := s.activeMenu()
	if currentMenu.backing != nil && !currentMenu.backing.StillValid(s.Runtime, s) {
		s.Runtime.closeMenuLocked(s, true)

		return nil
	}

	var (
		changedBackings []int
		equipment       []byte
		valid           bool
	)

	player, changed := s.updatePlayerState(func(player *game.Player) bool {
		if click.WindowID != currentMenu.windowID || click.StateID != currentMenu.stateID {
			return false
		}

		beforeInventory := player.Inventory.Clone()

		candidate := currentMenu.candidate()
		beforeCarried := candidate.carried.Clone()

		if !applyMenuClick(candidate, player.GameMode, click) {
			return false
		}

		changedSlots := candidate.changedSlots()
		if !validPredictedMenu(click, candidate, changedSlots) {
			return false
		}

		valid = true
		if len(changedSlots) == 0 && beforeCarried.Equal(candidate.carried) {
			return false
		}

		changedBackings = candidate.changedBackings()

		currentMenu.commit(candidate)

		s.drainPreservedCarriedLocked(player)

		currentMenu.incrementStateID()

		equipment = changedEquipmentSlots(beforeInventory, player.Inventory, player.SelectedHotbarSlot)

		return true
	})

	if !valid {
		currentMenu.resetDrag()

		return s.sendMenuSnapshot(currentMenu.snapshot())
	}

	if changed {
		if len(changedBackings) != 0 && currentMenu.backing != nil {
			currentMenu.backing.Changed(s.Runtime, s, changedBackings)
		}

		err := s.sendMenuSnapshot(currentMenu.snapshot())
		if err != nil {
			return err
		}

		if len(equipment) > 0 {
			s.Runtime.broadcastPlayerEquipment(s, player, equipment...)
		}
	}

	return nil
}

func applyMenuClick(candidate *menuCandidate, mode game.GameMode, click protocol.ContainerClick) bool {
	switch click.Mode {
	case clickModePickup:
		candidate.menu.resetDrag()

		return applyPickup(candidate, int(click.Slot), click.MouseButton)
	case clickModeQuickMove:
		candidate.menu.resetDrag()

		return applyQuickMove(candidate, int(click.Slot), click.MouseButton)
	case clickModeSwap:
		candidate.menu.resetDrag()

		return applySwap(candidate, int(click.Slot), click.MouseButton)
	case clickModeClone:
		candidate.menu.resetDrag()

		return applyClone(candidate, mode, int(click.Slot), click.MouseButton)
	case clickModeThrow:
		candidate.menu.resetDrag()

		return applyThrow(candidate, int(click.Slot), click.MouseButton)
	case clickModeQuickCraft:
		return applyQuickCraft(candidate, mode, int(click.Slot), click.MouseButton)
	case clickModePickupAll:
		candidate.menu.resetDrag()

		return applyPickupAll(candidate, int(click.Slot), click.MouseButton)
	default:
		candidate.menu.resetDrag()

		return false
	}
}

func applyPickup(candidate *menuCandidate, slot int, button int8) bool {
	if button != 0 && button != 1 {
		return false
	}

	if slot == outsideInventorySlot {
		if candidate.carried.Empty() {
			return true
		}

		if button == 0 || candidate.carried.Count == 1 {
			candidate.carried = game.ItemStack{}
		} else {
			candidate.carried.Count--
		}

		return true
	}
	if slot < 0 {
		return true
	}

	target := candidate.slot(slot)
	if target == nil {
		return false
	}

	if button == 0 {
		return applyLeftPickup(candidate, slot, target)
	}

	return applyRightPickup(candidate, slot, target)
}

func applyLeftPickup(candidate *menuCandidate, slot int, target *game.ItemStack) bool {
	carried := &candidate.carried

	if carried.Empty() {
		*carried = target.Clone()
		*target = game.ItemStack{}

		return true
	}

	if target.Empty() {
		if candidate.accepts(slot, *carried) {
			*target = carried.Clone()
			*carried = game.ItemStack{}
		}

		return true
	}

	if target.SameItem(*carried) {
		capacity := candidate.stackLimit(slot, *target) - target.Count
		moved := min(capacity, carried.Count)

		if moved > 0 && candidate.accepts(slot, *carried) {
			target.Count += moved
			carried.Count -= moved

			normalizeStack(carried)
		}

		return true
	}

	if candidate.accepts(slot, *carried) {
		*target, *carried = carried.Clone(), target.Clone()
	}

	return true
}

func applyRightPickup(candidate *menuCandidate, slot int, target *game.ItemStack) bool {
	carried := &candidate.carried

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
		if candidate.accepts(slot, *carried) {
			*target = carried.Clone()

			target.Count = 1
			carried.Count--

			normalizeStack(carried)
		}

		return true
	}

	if target.SameItem(*carried) {
		if target.Count < candidate.stackLimit(slot, *target) && candidate.accepts(slot, *carried) {
			target.Count++
			carried.Count--

			normalizeStack(carried)
		}

		return true
	}

	if candidate.accepts(slot, *carried) {
		*target, *carried = carried.Clone(), target.Clone()
	}

	return true
}

func applyQuickMove(candidate *menuCandidate, slot int, button int8) bool {
	if button != 0 && button != 1 {
		return false
	}

	source := candidate.slot(slot)
	if source == nil {
		return false
	}

	if source.Empty() {
		return true
	}

	if candidate.menu.quickMove == nil {
		return true
	}

	candidate.menu.quickMove(candidate, slot)

	return true
}

func applySwap(candidate *menuCandidate, slot int, button int8) bool {
	target := candidate.slot(slot)
	if target == nil {
		return false
	}

	swapSlot := noMenuSlot

	var other *game.ItemStack

	switch {
	case button >= 0 && int(button) < len(candidate.menu.hotbarSlots) && candidate.menu.hasHotbarSlots[button]:
		swapSlot = candidate.menu.hotbarSlots[button]
		other = candidate.slot(candidate.menu.hotbarSlots[button])
	case button == 40:
		if candidate.menu.hasOffhandSlot {
			swapSlot = candidate.menu.offhandSlot
			other = candidate.slot(candidate.menu.offhandSlot)
		} else if candidate.menu.hiddenOffhand != nil {
			other = &candidate.hiddenOffhand
		}
	default:
		return false
	}

	if other == nil {
		return false
	}

	if target == other {
		return true
	}

	if !target.Empty() && swapSlot != noMenuSlot && !candidate.accepts(swapSlot, *target) || !other.Empty() && !candidate.accepts(slot, *other) {
		return true
	}

	*target, *other = other.Clone(), target.Clone()

	return true
}

func applyClone(candidate *menuCandidate, mode game.GameMode, slot int, _ int8) bool {
	if mode != game.GameModeCreative {
		return false
	}
	if slot < 0 {
		return true
	}

	target := candidate.slot(slot)
	if target == nil {
		return false
	}

	if candidate.carried.Empty() && !target.Empty() {
		candidate.carried = target.Clone()

		candidate.carried.Count = stackLimit(candidate.carried)
	}

	return true
}

func applyThrow(candidate *menuCandidate, slot int, button int8) bool {
	if button != 0 && button != 1 {
		return false
	}

	if slot < 0 {
		return true
	}

	target := candidate.slot(slot)
	if target == nil {
		return false
	}

	if !candidate.carried.Empty() || target.Empty() {
		return true
	}

	if button == 1 || target.Count == 1 {
		*target = game.ItemStack{}
	} else {
		target.Count--
	}

	return true
}

func applyQuickCraft(candidate *menuCandidate, mode game.GameMode, slot int, button int8) bool {
	stage := button & 3
	dragButton := button >> 2

	if dragButton < 0 || dragButton > 2 || dragButton == 2 && mode != game.GameModeCreative {
		candidate.menu.resetDrag()

		return false
	}

	switch stage {
	case 0:
		if slot != outsideInventorySlot || candidate.carried.Empty() {
			candidate.menu.resetDrag()

			return false
		}

		candidate.menu.drag = inventoryDragState{active: true, button: dragButton}

		return true
	case 1:
		if !candidate.menu.drag.active || candidate.menu.drag.button != dragButton || candidate.slot(slot) == nil {
			return false
		}

		target := candidate.slot(slot)
		if candidate.accepts(slot, candidate.carried) && (target.Empty() || target.SameItem(candidate.carried)) && (dragButton == 2 || candidate.carried.Count > int32(len(candidate.menu.drag.slots))) && !containsSlot(candidate.menu.drag.slots, slot) {
			candidate.menu.drag.slots = append(candidate.menu.drag.slots, slot)
		}

		return true
	case 2:
		if slot != outsideInventorySlot || !candidate.menu.drag.active || candidate.menu.drag.button != dragButton || len(candidate.menu.drag.slots) == 0 {
			candidate.menu.resetDrag()

			return false
		}

		slots := append([]int(nil), candidate.menu.drag.slots...)

		candidate.menu.resetDrag()
		if len(slots) == 1 {
			if dragButton == 0 || dragButton == 1 {
				return applyPickup(candidate, slots[0], dragButton)
			}

			return true
		}

		applyDragDistribution(candidate, slots, dragButton)

		return true
	default:
		candidate.menu.resetDrag()

		return false
	}
}

func applyDragDistribution(candidate *menuCandidate, slots []int, button int8) {
	original := candidate.carried.Clone()

	remaining := original.Count

	for _, slot := range slots {
		target := candidate.slot(slot)

		capacity := candidate.stackLimit(slot, original)
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

		remaining -= amount
	}

	candidate.carried.Count = remaining

	normalizeStack(&candidate.carried)
}

func applyPickupAll(candidate *menuCandidate, slot int, button int8) bool {
	if button != 0 && button != 1 || candidate.slot(slot) == nil {
		return false
	}

	if candidate.carried.Empty() || !candidate.slots[slot].Empty() {
		return true
	}

	limit := stackLimit(candidate.carried)
	first := 0
	last := len(candidate.slots)
	step := 1
	if button == 1 {
		first = len(candidate.slots) - 1
		last = -1
		step = -1
	}

	for pass := 0; pass < 2 && candidate.carried.Count < limit; pass++ {
		for current := first; current != last && candidate.carried.Count < limit; current += step {
			stack := candidate.slot(current)
			if stack.Empty() || !stack.SameItem(candidate.carried) || pass == 0 && stack.Count == candidate.stackLimit(current, *stack) {
				continue
			}

			moved := min(stack.Count, limit-candidate.carried.Count)

			candidate.carried.Count += moved
			stack.Count -= moved

			normalizeStack(stack)
		}
	}

	return true
}

func moveIntoSlots(candidate *menuCandidate, stack *game.ItemStack, slots []int) {
	for _, slot := range slots {
		target := candidate.slot(slot)
		if target == nil || target.Empty() || !target.SameItem(*stack) || !candidate.accepts(slot, *stack) {
			continue
		}

		moved := min(candidate.stackLimit(slot, *target)-target.Count, stack.Count)
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
		target := candidate.slot(slot)
		if target == nil || !target.Empty() || !candidate.accepts(slot, *stack) {
			continue
		}

		moved := min(candidate.stackLimit(slot, *stack), stack.Count)
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

	var before game.PlayerInventory

	_, changed := s.updatePlayerState(func(player *game.Player) bool {
		stack := player.Inventory.Held(player.SelectedHotbarSlot)
		if stack == nil || stack.Empty() {
			return false
		}

		before = player.Inventory.Clone()

		if dropAll || stack.Count == 1 {
			*stack = game.ItemStack{}
		} else {
			stack.Count--
		}

		return true
	})

	if changed {
		err := s.synchronizePlayerInventoryMutation(before)
		if err != nil {
			s.Log.Warnf("[play] failed to synchronize dropped held item: %v\n", err)
		}
	}

}

func (s *Session) handleSwapWithOffhand() {
	s.Runtime.lifecycleMu.Lock()
	defer s.Runtime.lifecycleMu.Unlock()

	var before game.PlayerInventory

	_, changed := s.updatePlayerState(func(player *game.Player) bool {
		held := player.Inventory.Held(player.SelectedHotbarSlot)
		if held == nil {
			return false
		}

		before = player.Inventory.Clone()
		*held, player.Inventory.Offhand = player.Inventory.Offhand.Clone(), held.Clone()

		return true
	})

	if changed {
		err := s.synchronizePlayerInventoryMutation(before)
		if err != nil {
			s.Log.Warnf("[play] failed to synchronize offhand swap: %v\n", err)
		}
	}

}

func (s *Session) sendPlayerInventory() error {
	return s.sendMenuSnapshot(s.activeMenu().snapshot())
}

func (s *Session) sendMenuSnapshot(snapshot menuSnapshot) error {
	return s.writePacket(protocol.ClientboundContainerSetContentID, protocol.ContainerSetContent{
		WindowID:    snapshot.windowID,
		StateID:     snapshot.stateID,
		Items:       snapshot.items,
		CarriedItem: snapshot.carried,
	})
}

func (s *Session) synchronizePlayerInventoryMutation(before game.PlayerInventory) error {
	return s.synchronizePlayerInventoryMutationSlot(before, nil)
}

func (s *Session) synchronizePlayerInventoryMutationSlot(before game.PlayerInventory, preferredPlayerSlot *int) error {
	s.updatePlayerState(func(player *game.Player) bool {
		return s.drainPreservedCarriedLocked(player)
	})

	player := s.snapshotPlayer()

	changedSlots := inventoryChanges(before, player.Inventory)

	currentMenu := s.activeMenu()

	equipment := changedEquipmentSlots(before, player.Inventory, player.SelectedHotbarSlot)

	if len(changedSlots) > 0 && !currentMenu.exposesPlayerSlots(changedSlots) {
		if len(equipment) > 0 {
			s.Runtime.broadcastPlayerEquipment(s, player, equipment...)
		}

		return nil
	}

	currentMenu.incrementStateID()

	var err error

	if preferredPlayerSlot != nil && len(changedSlots) == 1 {
		menuSlot := noMenuSlot
		for slot, definition := range currentMenu.slots {
			if definition.hasPlayerSlot && definition.playerSlot == *preferredPlayerSlot {
				menuSlot = slot

				break
			}
		}

		if menuSlot != noMenuSlot {
			err = s.writePacket(protocol.ClientboundContainerSetSlotID, protocol.ContainerSetSlot{
				WindowID: currentMenu.windowID,
				StateID:  currentMenu.stateID,
				Slot:     int16(menuSlot),
				Item:     currentMenu.slots[menuSlot].stack.Clone(),
			})
		} else {
			err = s.sendMenuSnapshot(currentMenu.snapshot())
		}
	} else {
		err = s.sendMenuSnapshot(currentMenu.snapshot())
	}

	if err != nil {
		return err
	}

	if len(equipment) > 0 {
		s.Runtime.broadcastPlayerEquipment(s, player, equipment...)
	}

	return nil
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

func validPredictedMenu(click protocol.ContainerClick, candidate *menuCandidate, changed []int) bool {
	if len(click.ChangedSlots) != len(changed) || !hashedSlotMatches(click.CursorItem, candidate.carried) {
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
		stack := candidate.slot(slot)
		if stack == nil || !hashedSlotMatches(prediction.Item, *stack) {
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

func reverseSlotRange(first, last int) []int {
	slots := make([]int, 0, last-first+1)

	for slot := last; slot >= first; slot-- {
		slots = append(slots, slot)
	}

	return slots
}

func containsSlot(slots []int, target int) bool {
	return slices.Contains(slots, target)
}

func nextMenuStateID(stateID int32) int32 {
	return (stateID + 1) & 32767
}
