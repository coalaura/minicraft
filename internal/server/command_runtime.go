package server

import (
	"errors"
	"fmt"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

const gameEventChangeGameMode = 3

var errInventoryFull = errors.New("inventory does not have enough space")

func (r *Runtime) ChangeGameMode(session *Session, mode game.GameMode) error {
	r.lifecycleMu.Lock()

	player, changed := session.updatePlayerState(func(player *game.Player) bool {
		if player.GameMode == mode {
			return false
		}

		player.GameMode = mode

		return true
	})

	r.lifecycleMu.Unlock()

	if !changed {
		return nil
	}

	err := session.writePacket(protocol.ClientboundGameEventID, protocol.GameEvent{
		Event: gameEventChangeGameMode,
		Value: float32(mode),
	})

	if err != nil {
		return fmt.Errorf("synchronize game mode: %w", err)
	}

	update := protocol.PlayerInfoUpdate{
		Actions: protocol.PlayerInfoActionUpdateGameMode,
		Players: []protocol.PlayerInfo{{
			UUID:     player.UUID,
			GameMode: int32(player.GameMode),
		}},
	}

	for _, other := range r.snapshotSessions() {
		err = other.writePacket(protocol.ClientboundPlayerInfoUpdateID, update)
		if err != nil {
			return fmt.Errorf("synchronize player game mode: %w", err)
		}
	}

	return nil
}

func (r *Runtime) GiveItem(session *Session, item game.Item, count int32) error {
	if !item.Valid() || item == game.ItemAir || count <= 0 {
		return errors.New("invalid item or count")
	}

	return r.mutateCommandInventory(session, func(inventory *game.PlayerInventory) error {
		stack := game.ItemStack{Item: item, Count: count}

		moveIntoSlots(inventory, &stack, slotRange(36, 44))
		moveIntoSlots(inventory, &stack, slotRange(9, 35))

		if !stack.Empty() {
			return errInventoryFull
		}

		return nil
	})
}

func (r *Runtime) ClearItems(session *Session, item *game.Item) (int32, error) {
	var removed int32

	err := r.mutateCommandInventory(session, func(inventory *game.PlayerInventory) error {
		for slot := 1; slot < game.PlayerInventorySlots; slot++ {
			stack := inventory.Slot(slot)
			if stack.Empty() || item != nil && stack.Item != *item {
				continue
			}

			removed += stack.Count

			*stack = game.ItemStack{}
		}

		if !inventory.Carried.Empty() && (item == nil || inventory.Carried.Item == *item) {
			removed += inventory.Carried.Count

			inventory.Carried = game.ItemStack{}
		}

		return nil
	})

	return removed, err
}

func (r *Runtime) mutateCommandInventory(session *Session, mutate func(*game.PlayerInventory) error) error {
	r.lifecycleMu.Lock()
	defer r.lifecycleMu.Unlock()

	before := session.snapshotPlayer()
	afterInventory := before.Inventory.Clone()

	err := mutate(&afterInventory)
	if err != nil {
		return err
	}

	if len(inventoryChanges(before.Inventory, afterInventory)) == 0 && before.Inventory.Carried.Equal(afterInventory.Carried) {
		return nil
	}

	afterInventory.StateID = nextInventoryStateID(before.Inventory.StateID)

	after, _ := session.updatePlayerState(func(player *game.Player) bool {
		player.Inventory = afterInventory

		return true
	})

	err = session.sendPlayerInventorySnapshot(after.Inventory)
	if err != nil {
		return fmt.Errorf("synchronize inventory: %w", err)
	}

	equipment := changedEquipmentSlots(before.Inventory, after.Inventory, after.SelectedHotbarSlot)
	if len(equipment) != 0 {
		r.broadcastPlayerEquipment(session, after, equipment...)
	}

	return nil
}

func (r *Runtime) TeleportPlayer(session *Session, position game.Position) error {
	if !validPlayerPosition(position.X, position.Y, position.Z) {
		return errors.New("invalid teleport destination")
	}

	r.updatePlayerMovement(session, func(player *game.Player) {
		player.Position = position
		player.Velocity = game.Velocity{}
		player.OnGround = false
	})

	err := session.updatePlayerChunks()
	if err != nil {
		return fmt.Errorf("update teleport chunks: %w", err)
	}

	err = session.sendPlayerPosition()
	if err != nil {
		return fmt.Errorf("synchronize teleport: %w", err)
	}

	return nil
}
