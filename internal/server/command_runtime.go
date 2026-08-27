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

		changes = append(changes, commandInventoryChange{session: session, before: before, after: after})
	}

	for index := range changes {
		change := &changes[index]

		change.session.updatePlayerState(func(player *game.Player) bool {
			player.Inventory = change.after

			return true
		})
	}

	for _, change := range changes {
		err := change.session.synchronizePlayerInventoryMutation(change.before)
		if err != nil {
			return fmt.Errorf("synchronize inventory: %w", err)
		}
	}

	return nil
}

func giveItemToInventory(inventory *game.PlayerInventory, item game.Item, count int32) error {
	stack := game.ItemStack{Item: item, Count: count}

	playerMenu := newPlayerInventoryMenu(inventory)

	candidate := playerMenu.candidate()

	moveIntoSlots(candidate, &stack, slotRange(36, 44))
	moveIntoSlots(candidate, &stack, slotRange(9, 35))

	if !stack.Empty() {
		return errInventoryFull
	}

	playerMenu.commit(candidate)

	return nil
}

func (r *Runtime) ClearItems(session *Session, item *game.Item, maximum int32) (int32, error) {
	var removed int32

	err := r.mutateCommandInventory(session, func(inventory *game.PlayerInventory, currentMenu *menu) error {
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

		if !currentMenu.carried.Empty() && (item == nil || currentMenu.carried.Item == *item) && (maximum < 0 || removed < maximum || maximum == 0) {
			count := currentMenu.carried.Count
			if maximum > 0 {
				count = min(count, maximum-removed)
			}

			removed += count

			if maximum != 0 {
				currentMenu.carried.Count -= count
				if currentMenu.carried.Count == 0 {
					currentMenu.carried = game.ItemStack{}
				}
			}
		}

		return nil
	})

	return removed, err
}

func (r *Runtime) mutateCommandInventory(session *Session, mutate func(*game.PlayerInventory, *menu) error) error {
	r.lifecycleMu.Lock()
	defer r.lifecycleMu.Unlock()

	before := session.snapshotPlayer()
	afterInventory := before.Inventory.Clone()

	currentMenu := session.activeMenu()
	beforeCarried := currentMenu.carried.Clone()

	err := mutate(&afterInventory, currentMenu)
	if err != nil {
		currentMenu.carried = beforeCarried

		return err
	}

	if len(inventoryChanges(before.Inventory, afterInventory)) == 0 && beforeCarried.Equal(currentMenu.carried) {
		return nil
	}

	session.updatePlayerState(func(player *game.Player) bool {
		player.Inventory = afterInventory

		return true
	})

	err = session.synchronizePlayerInventoryMutation(before.Inventory)
	if err != nil {
		return fmt.Errorf("synchronize inventory: %w", err)
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
