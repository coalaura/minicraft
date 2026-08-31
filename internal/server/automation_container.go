package server

import (
	"math"
	"slices"

	"github.com/coalaura/minicraft/internal/game"
)

const (
	containerFaceDown containerFace = iota
	containerFaceUp
	containerFaceNorth
	containerFaceSouth
	containerFaceWest
	containerFaceEast
)

type containerFace uint8

type automatedContainer struct {
	slots          []*game.ItemStack
	backingIndices []int
	backing        menuBacking
	furnace        *runtimeFurnace
	hopper         *runtimeHopper
}

func (r *Runtime) automatedContainerAt(position game.BlockPosition) (*automatedContainer, bool) {
	block := r.World.BlockAt(position)
	if block.Behavior() == game.BlockBehaviorChest {
		backing, valid := r.chestBackingAt(position)
		if !valid {
			return nil, false
		}

		container := &automatedContainer{backing: backing}

		for index, chest := range backing.chests {
			items, inventory := chest.entity.Inventory()
			if !inventory || len(items) != game.ChestSlotCount {
				return nil, false
			}

			for slot := range items {
				container.slots = append(container.slots, &items[slot])
				container.backingIndices = append(container.backingIndices, index)
			}
		}

		return container, true
	}

	entity, present := r.authoritativeRuntimeBlockEntityAt(position, block)
	if !present {
		return nil, false
	}

	container := &automatedContainer{}

	switch current := entity.(type) {
	case *runtimeBarrel:
		container.backing = current
	case *runtimeFurnace:
		container.backing = current
		container.furnace = current
	case *runtimeHopper:
		container.backing = current
		container.hopper = current
	default:
		return nil, false
	}

	var items []game.ItemStack

	switch current := entity.(type) {
	case *runtimeBarrel:
		items, present = current.entity.Inventory()
	case *runtimeFurnace:
		items, present = current.entity.Inventory()
	case *runtimeHopper:
		items, present = current.entity.Inventory()
	}

	if !present {
		return nil, false
	}

	for slot := range items {
		container.slots = append(container.slots, &items[slot])
		container.backingIndices = append(container.backingIndices, 0)
	}

	return container, true
}

func (container *automatedContainer) availableSlots(face containerFace) []int {
	if container.furnace == nil {
		slots := make([]int, len(container.slots))

		for slot := range slots {
			slots[slot] = slot
		}

		return slots
	}

	switch face {
	case containerFaceUp:
		return []int{furnaceInputSlot}
	case containerFaceDown:
		return []int{furnaceResultSlot, furnaceFuelSlot}
	default:
		return []int{furnaceFuelSlot}
	}
}

func (container *automatedContainer) canInsert(slot int, stack game.ItemStack, face containerFace) bool {
	if stack.Empty() || slot < 0 || slot >= len(container.slots) || !slices.Contains(container.availableSlots(face), slot) {
		return false
	}

	if container.furnace == nil {
		return true
	}

	switch slot {
	case furnaceInputSlot:
		return true
	case furnaceFuelSlot:
		if game.IsFuel(stack.Item) {
			return true
		}

		return stack.Item == game.ItemBucket && container.slots[slot].Item != game.ItemBucket
	default:
		return false
	}
}

func (container *automatedContainer) canExtract(slot int, stack game.ItemStack, face containerFace) bool {
	if stack.Empty() || slot < 0 || slot >= len(container.slots) || !slices.Contains(container.availableSlots(face), slot) {
		return false
	}

	if container.furnace != nil && face == containerFaceDown && slot == furnaceFuelSlot {
		return stack.Item == game.ItemWaterBucket || stack.Item == game.ItemBucket
	}

	return true
}

func (container *automatedContainer) changed(runtime *Runtime, slots ...int) {
	changed := make([]int, 0, len(slots))

	for _, slot := range slots {
		if slot < 0 || slot >= len(container.backingIndices) {
			continue
		}

		index := container.backingIndices[slot]
		if !slices.Contains(changed, index) {
			changed = append(changed, index)
		}
	}

	container.backing.Changed(runtime, nil, changed)
}

func (container *automatedContainer) insert(stack *game.ItemStack, face containerFace) []int {
	if stack.Empty() {
		return nil
	}

	var changed []int

	for _, slot := range container.availableSlots(face) {
		target := container.slots[slot]
		if target.Empty() || !target.SameItem(*stack) || !container.canInsert(slot, *stack, face) {
			continue
		}

		limit := stackLimit(*target)

		moved := min(limit-target.Count, stack.Count)
		if moved <= 0 {
			continue
		}

		target.Count += moved
		stack.Count -= moved

		changed = append(changed, slot)

		if stack.Empty() {
			normalizeStack(stack)

			return changed
		}
	}

	for _, slot := range container.availableSlots(face) {
		target := container.slots[slot]
		if !target.Empty() || !container.canInsert(slot, *stack, face) {
			continue
		}

		moved := min(stackLimit(*stack), stack.Count)

		*target = stack.Clone()

		target.Count = moved
		stack.Count -= moved

		changed = append(changed, slot)

		if stack.Empty() {
			normalizeStack(stack)

			return changed
		}
	}

	return changed
}

func offsetContainerPosition(position game.BlockPosition, face containerFace) (game.BlockPosition, bool) {
	switch face {
	case containerFaceDown:
		if position.Y == math.MinInt32 {
			return game.BlockPosition{}, false
		}

		position.Y--
	case containerFaceUp:
		if position.Y == math.MaxInt32 {
			return game.BlockPosition{}, false
		}

		position.Y++
	case containerFaceNorth:
		if position.Z == math.MinInt32 {
			return game.BlockPosition{}, false
		}

		position.Z--
	case containerFaceSouth:
		if position.Z == math.MaxInt32 {
			return game.BlockPosition{}, false
		}

		position.Z++
	case containerFaceWest:
		if position.X == math.MinInt32 {
			return game.BlockPosition{}, false
		}

		position.X--
	case containerFaceEast:
		if position.X == math.MaxInt32 {
			return game.BlockPosition{}, false
		}

		position.X++
	}

	return position, true
}

func oppositeContainerFace(face containerFace) containerFace {
	return [...]containerFace{
		containerFaceUp,
		containerFaceDown,
		containerFaceSouth,
		containerFaceNorth,
		containerFaceEast,
		containerFaceWest,
	}[face]
}

func containerFaceFromName(name string) (containerFace, bool) {
	switch name {
	case "down":
		return containerFaceDown, true
	case "up":
		return containerFaceUp, true
	case "north":
		return containerFaceNorth, true
	case "south":
		return containerFaceSouth, true
	case "west":
		return containerFaceWest, true
	case "east":
		return containerFaceEast, true
	default:
		return 0, false
	}
}
