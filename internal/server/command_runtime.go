package server

import (
	"errors"
	"fmt"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

const gameEventChangeGameMode = 3

var errInventoryFull = errors.New("inventory does not have enough space")

type commandInventoryChange struct {
	session *Session
	before  game.PlayerInventory
	after   game.PlayerInventory
	player  game.Player
}

func (r *Runtime) ChangeGameMode(session *Session, mode game.GameMode) (bool, error) {
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
		return false, nil
	}

	err := session.writePacket(protocol.ClientboundGameEventID, protocol.GameEvent{
		Event: gameEventChangeGameMode,
		Value: float32(mode),
	})

	if err != nil {
		return false, fmt.Errorf("synchronize game mode: %w", err)
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
			return false, fmt.Errorf("synchronize player game mode: %w", err)
		}
	}

	return true, nil
}

func (r *Runtime) GiveItem(session *Session, item game.Item, count int32) error {
	return r.GiveItems([]*Session{session}, item, count)
}

func (r *Runtime) GiveItems(sessions []*Session, item game.Item, count int32) error {
	if !item.Valid() || item == game.ItemAir || count <= 0 {
		return errors.New("invalid item or count")
	}

	r.lifecycleMu.Lock()
	defer r.lifecycleMu.Unlock()

	changes := make([]commandInventoryChange, 0, len(sessions))

	for _, session := range sessions {
		before := session.snapshotPlayer().Inventory
		after := before.Clone()

		err := giveItemToInventory(&after, item, count)
		if err != nil {
			return fmt.Errorf("give to %s: %w", session.snapshotPlayer().Name, err)
		}

		after.StateID = nextInventoryStateID(before.StateID)

		changes = append(changes, commandInventoryChange{session: session, before: before, after: after})
	}

	for index := range changes {
		change := &changes[index]

		change.player, _ = change.session.updatePlayerState(func(player *game.Player) bool {
			player.Inventory = change.after

			return true
		})
	}

	for _, change := range changes {
		err := change.session.sendPlayerInventorySnapshot(change.after)
		if err != nil {
			return fmt.Errorf("synchronize inventory: %w", err)
		}

		equipment := changedEquipmentSlots(change.before, change.after, change.player.SelectedHotbarSlot)
		if len(equipment) != 0 {
			r.broadcastPlayerEquipment(change.session, change.player, equipment...)
		}
	}

	return nil
}

func giveItemToInventory(inventory *game.PlayerInventory, item game.Item, count int32) error {
	stack := game.ItemStack{Item: item, Count: count}

	moveIntoSlots(inventory, &stack, slotRange(36, 44))
	moveIntoSlots(inventory, &stack, slotRange(9, 35))

	if !stack.Empty() {
		return errInventoryFull
	}

	return nil
}

func (r *Runtime) ClearItems(session *Session, item *game.Item, maximum int32) (int32, error) {
	var removed int32

	err := r.mutateCommandInventory(session, func(inventory *game.PlayerInventory) error {
		for slot := 1; slot < game.PlayerInventorySlots; slot++ {
			stack := inventory.Slot(slot)
			if stack.Empty() || item != nil && stack.Item != *item {
				continue
			}

			if maximum == 0 {
				removed += stack.Count

				continue
			}

			count := stack.Count
			if maximum > 0 {
				count = min(count, maximum-removed)
			}

			removed += count
			stack.Count -= count

			if stack.Count == 0 {
				*stack = game.ItemStack{}
			}

			if maximum > 0 && removed == maximum {
				break
			}
		}

		if !inventory.Carried.Empty() && (item == nil || inventory.Carried.Item == *item) && (maximum < 0 || removed < maximum || maximum == 0) {
			count := inventory.Carried.Count
			if maximum > 0 {
				count = min(count, maximum-removed)
			}

			removed += count

			if maximum != 0 {
				inventory.Carried.Count -= count
				if inventory.Carried.Count == 0 {
					inventory.Carried = game.ItemStack{}
				}
			}
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

func (r *Runtime) TeleportPlayer(session *Session, position game.Position, rotation *game.Rotation) error {
	if !validPlayerPosition(position.X, position.Y, position.Z) {
		return errors.New("invalid teleport destination")
	}

	r.updatePlayerMovement(session, func(player *game.Player) {
		player.Position = position
		player.Velocity.Y = 0
		player.OnGround = true

		if rotation != nil {
			player.Rotation = *rotation
		}
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
