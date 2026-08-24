package server

import (
	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

const (
	creativeHotbarSlotStart = 36
	creativeOffhandSlot     = 45
)

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
		return
	}

	s.Runtime.lifecycleMu.Lock()
	defer s.Runtime.lifecycleMu.Unlock()

	equipmentSlot := byte(0xFF)

	player, changed := s.updatePlayerState(func(player *game.Player) bool {
		if player.GameMode != game.GameModeCreative {
			return false
		}

		switch {
		case update.Slot >= creativeHotbarSlotStart && update.Slot < creativeHotbarSlotStart+game.HotbarSlotCount:
			index := int(update.Slot - creativeHotbarSlotStart)
			if player.Hotbar[index] == stack {
				return false
			}

			player.Hotbar[index] = stack
			if index == player.SelectedHotbarSlot {
				equipmentSlot = protocol.EquipmentSlotMainHand
			}

			return true
		case update.Slot == creativeOffhandSlot:
			if player.Offhand == stack {
				return false
			}

			player.Offhand = stack
			equipmentSlot = protocol.EquipmentSlotOffHand

			return true
		default:
			return false
		}
	})

	if changed && equipmentSlot != 0xFF {
		s.Runtime.broadcastPlayerEquipment(s, player, equipmentSlot)
	}
}

func (s *Session) handleDropHeldItem(dropAll bool) {
	s.Runtime.lifecycleMu.Lock()
	defer s.Runtime.lifecycleMu.Unlock()

	player, changed := s.updatePlayerState(func(player *game.Player) bool {
		if player.GameMode != game.GameModeCreative || player.SelectedHotbarSlot < 0 || player.SelectedHotbarSlot >= game.HotbarSlotCount {
			return false
		}

		stack := &player.Hotbar[player.SelectedHotbarSlot]
		if stack.Empty() {
			return false
		}

		if dropAll || stack.Count == 1 {
			*stack = game.ItemStack{}
		} else {
			stack.Count--
		}

		return true
	})

	if changed {
		s.Runtime.broadcastPlayerEquipment(s, player, protocol.EquipmentSlotMainHand)
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
		if player.SelectedHotbarSlot < 0 || player.SelectedHotbarSlot >= game.HotbarSlotCount {
			return game.ItemStack{}, false
		}

		return player.Hotbar[player.SelectedHotbarSlot], true
	case protocol.OffHand:
		return player.Offhand, true
	default:
		return game.ItemStack{}, false
	}
}

func creativeItemStack(item protocol.UntrustedSlot) (game.ItemStack, bool) {
	if item.ItemCount == 0 {
		return game.ItemStack{}, true
	}

	if item.ItemCount < 0 || item.ItemID < 0 || item.ItemID > int32(game.MaxItemID) {
		return game.ItemStack{}, false
	}

	itemID := game.Item(item.ItemID)
	definition, valid := itemID.Definition()

	if !valid || item.ItemCount > definition.StackSize {
		return game.ItemStack{}, false
	}

	stack := game.ItemStack{Item: itemID, Count: item.ItemCount}
	if stack.Empty() {
		return game.ItemStack{}, false
	}

	return stack, true
}
