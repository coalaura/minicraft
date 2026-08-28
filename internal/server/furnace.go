package server

import (
	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

const (
	furnaceInputSlot  = 0
	furnaceFuelSlot   = 1
	furnaceResultSlot = 2
)

type runtimeFurnace struct {
	position  game.BlockPosition
	entity    game.BlockEntity
	viewers   map[*Session]struct{}
	lastInput game.ItemStack
}

func newRuntimeFurnace(position game.BlockPosition, entity game.BlockEntity) RuntimeBlockEntity {
	furnace := &runtimeFurnace{position: position, entity: entity}

	data, valid := entity.Data.(*game.FurnaceBlockEntityData)
	if valid && len(data.Items) == game.FurnaceSlotCount {
		furnace.lastInput = data.Items[furnaceInputSlot].Clone()
	}

	return furnace
}

func (furnace *runtimeFurnace) BlockEntityType() game.BlockEntityType {
	return furnace.entity.Type
}

func (furnace *runtimeFurnace) BlockPosition() game.BlockPosition {
	return furnace.position
}

func (furnace *runtimeFurnace) ContainsPosition(position game.BlockPosition) bool {
	return furnace.position == position
}

func (furnace *runtimeFurnace) InteractBlock(runtime *Runtime, session *Session) error {
	return runtime.openFurnaceLocked(session, furnace)
}

func (furnace *runtimeFurnace) Attach(_ *Runtime, session *Session) {
	if furnace.viewers == nil {
		furnace.viewers = make(map[*Session]struct{})
	}

	furnace.viewers[session] = struct{}{}
}

func (furnace *runtimeFurnace) Detach(_ *Runtime, session *Session) {
	delete(furnace.viewers, session)
}

func (furnace *runtimeFurnace) Changed(runtime *Runtime, actor *Session, _ []int) {
	data, valid := furnace.entity.Data.(*game.FurnaceBlockEntityData)
	if valid && len(data.Items) == game.FurnaceSlotCount {
		input := data.Items[furnaceInputSlot]
		if !sameFurnaceInput(input, furnace.lastInput) {
			data.CookingProgress = 0
			data.CookingTotalTime = 200

			recipe, present := game.CookingRecipeFor(furnace.cookingRecipeType(), input)
			if present {
				data.CookingTotalTime = recipe.CookingTime()
			}
		}

		furnace.lastInput = input.Clone()
	}

	runtime.World.SetBlockEntity(furnace.position, furnace.entity)

	furnace.synchronizeInventory(runtime, actor)
}

func (furnace *runtimeFurnace) StillValid(runtime *Runtime, session *Session) bool {
	return containerBlockEntityStillValid(runtime, session, furnace)
}

func (furnace *runtimeFurnace) Tick(runtime *Runtime, _ *ActiveChunk) {
	data, valid := furnace.entity.Data.(*game.FurnaceBlockEntityData)
	if !valid || len(data.Items) != game.FurnaceSlotCount {
		return
	}

	wasLit := data.LitTimeRemaining > 0
	stateChanged := false
	inventoryChanged := false

	if wasLit {
		data.LitTimeRemaining--
		stateChanged = true
	}

	input := &data.Items[furnaceInputSlot]
	fuel := &data.Items[furnaceFuelSlot]

	recipe, hasRecipe := game.CookingRecipeFor(furnace.cookingRecipeType(), *input)

	canBurn := hasRecipe && furnaceCanBurn(recipe, data.Items)
	if canBurn && data.CookingTotalTime <= 0 {
		data.CookingTotalTime = recipe.CookingTime()
		stateChanged = true
	}

	if data.LitTimeRemaining > 0 || !fuel.Empty() && !input.Empty() {
		if data.LitTimeRemaining <= 0 && canBurn {
			burnDuration := furnace.burnDuration(fuel.Item)

			data.LitTimeRemaining = burnDuration
			data.LitTotalTime = burnDuration

			stateChanged = true

			if burnDuration > 0 {
				fuelItem := fuel.Item

				fuel.Count--

				normalizeStack(fuel)

				if fuel.Empty() {
					remainder, present := game.CraftingRemainder(fuelItem)
					if present {
						*fuel = game.ItemStack{Item: remainder, Count: 1}
					}
				}

				inventoryChanged = true
			}
		}

		if data.LitTimeRemaining > 0 && canBurn {
			data.CookingProgress++
			stateChanged = true

			if data.CookingProgress == data.CookingTotalTime {
				data.CookingProgress = 0
				data.CookingTotalTime = recipe.CookingTime()

				furnaceBurn(recipe, data.Items)

				if data.RecipesUsed == nil {
					data.RecipesUsed = make(map[string]int32)
				}

				data.RecipesUsed[recipe.Name()]++
				inventoryChanged = true
			}
		} else if data.CookingProgress != 0 {
			data.CookingProgress = 0
			stateChanged = true
		}
	} else if data.CookingProgress > 0 {
		data.CookingProgress = max(0, data.CookingProgress-2)
		stateChanged = true
	}

	isLit := data.LitTimeRemaining > 0
	if wasLit != isLit {
		runtime.setFurnaceLitStateLocked(furnace, isLit)
	}

	if stateChanged || inventoryChanged {
		runtime.World.SetBlockEntity(furnace.position, furnace.entity)
	}

	if inventoryChanged {
		furnace.synchronizeInventory(runtime, nil)
	}

	furnace.lastInput = data.Items[furnaceInputSlot].Clone()
}

func (furnace *runtimeFurnace) cookingRecipeType() game.CookingRecipeType {
	switch furnace.entity.Type {
	case game.BlockEntityTypeSmoker:
		return game.CookingRecipeSmoking
	case game.BlockEntityTypeBlastFurnace:
		return game.CookingRecipeBlasting
	default:
		return game.CookingRecipeSmelting
	}
}

func (furnace *runtimeFurnace) burnDuration(item game.Item) int32 {
	duration := game.FuelDuration(item)
	if furnace.entity.Type == game.BlockEntityTypeSmoker || furnace.entity.Type == game.BlockEntityTypeBlastFurnace {
		duration /= 2
	}

	return duration
}

func (furnace *runtimeFurnace) synchronizeInventory(_ *Runtime, actor *Session) {
	for viewer := range furnace.viewers {
		if viewer == actor {
			continue
		}

		current := viewer.activeMenu()
		if current.backing != furnace {
			continue
		}

		current.incrementStateID()
		err := viewer.sendMenuSnapshot(current.snapshot())
		if err != nil && viewer.Log != nil {
			viewer.Log.Warnf("[play] failed to synchronize furnace: %v\n", err)
			continue
		}

		err = viewer.sendChangedMenuData(current, false)
		if err != nil && viewer.Log != nil {
			viewer.Log.Warnf("[play] failed to synchronize furnace data: %v\n", err)
		}
	}
}

func furnaceCanBurn(recipe game.CookingRecipe, items []game.ItemStack) bool {
	if len(items) != game.FurnaceSlotCount || items[furnaceInputSlot].Empty() {
		return false
	}

	produced := recipe.Result()
	if produced.Empty() {
		return false
	}

	result := items[furnaceResultSlot]
	if result.Empty() {
		return true
	}
	if !result.SameItem(produced) {
		return false
	}

	resultDefinition, resultValid := result.Item.Definition()
	producedDefinition, producedValid := produced.Item.Definition()

	if !resultValid || !producedValid {
		return false
	}

	if result.Count < 64 && result.Count < resultDefinition.StackSize {
		return true
	}

	return result.Count < producedDefinition.StackSize
}

func furnaceBurn(recipe game.CookingRecipe, items []game.ItemStack) {
	input := &items[furnaceInputSlot]
	result := &items[furnaceResultSlot]

	produced := recipe.Result()

	if result.Empty() {
		*result = produced
	} else {
		result.Count += produced.Count
	}

	if input.Item == game.ItemWetSponge && items[furnaceFuelSlot].Item == game.ItemBucket {
		items[furnaceFuelSlot] = game.ItemStack{Item: game.ItemWaterBucket, Count: 1}
	}

	input.Count--

	normalizeStack(input)
}

func (r *Runtime) openFurnaceLocked(session *Session, furnace *runtimeFurnace) error {
	data, valid := furnace.entity.Data.(*game.FurnaceBlockEntityData)
	if !valid || len(data.Items) != game.FurnaceSlotCount {
		return nil
	}

	r.closeMenuLocked(session, false)

	menu := newFurnaceMenu(session.allocateWindowID(), furnace, data, &session.Player.Inventory)

	menu.backing = furnace

	session.containerMenu = menu

	furnace.Attach(r, session)

	menuType, title := furnaceMenuIdentity(furnace.entity.Type)

	menu.protocolMenuType = menuType

	err := session.writePacket(protocol.ClientboundOpenScreenID, protocol.OpenScreen{
		ContainerID: menu.windowID,
		MenuType:    menu.protocolMenuType,
		Title:       game.TranslatableText(title),
	})

	if err != nil {
		r.closeMenuLocked(session, false)
		return err
	}

	err = session.sendMenuSnapshot(menu.snapshot())
	if err != nil {
		return err
	}

	return session.sendChangedMenuData(menu, true)
}

func newFurnaceMenu(windowID int32, furnace *runtimeFurnace, data *game.FurnaceBlockEntityData, inventory *game.PlayerInventory) *menu {
	slots := make([]menuSlot, 0, 39)

	slots = append(slots,
		menuSlot{stack: &data.Items[furnaceInputSlot], limit: 64, storage: menuStorageBacking},
		menuSlot{stack: &data.Items[furnaceFuelSlot], limit: 64, storage: menuStorageBacking, accepts: furnaceFuelAccepts},
		menuSlot{stack: &data.Items[furnaceResultSlot], role: menuSlotResult, limit: 64, storage: menuStorageBacking},
	)

	for playerSlot := 9; playerSlot <= 44; playerSlot++ {
		slots = append(slots, menuSlot{
			stack: inventory.Slot(playerSlot), limit: 64, playerSlot: playerSlot,
			hasPlayerSlot: true, storage: menuStoragePlayer,
		})
	}

	current := &menu{
		windowID: windowID, slots: slots, hiddenOffhand: inventory.Slot(45),
		quickMove: quickMoveFurnace(furnace), containerSlots: game.FurnaceSlotCount,
		data: []*int32{&data.LitTimeRemaining, &data.LitTotalTime, &data.CookingProgress, &data.CookingTotalTime},
	}

	for hotbar := range game.HotbarSlotCount {
		current.hotbarSlots[hotbar] = 30 + hotbar
		current.hasHotbarSlots[hotbar] = true
	}

	return current
}

func furnaceFuelAccepts(candidate *menuCandidate, slot int, stack game.ItemStack) bool {
	if game.IsFuel(stack.Item) {
		return true
	}

	return stack.Item == game.ItemBucket && candidate.slots[slot].Item != game.ItemBucket
}

func sameFurnaceInput(first, second game.ItemStack) bool {
	if first.Empty() || second.Empty() {
		return first.Empty() && second.Empty()
	}

	return first.SameItem(second)
}

func quickMoveFurnace(furnace *runtimeFurnace) menuQuickMove {
	return func(candidate *menuCandidate, slot int) {
		remaining := candidate.slots[slot].Clone()

		switch {
		case slot == furnaceResultSlot:
			moveIntoSlots(candidate, &remaining, reverseSlotRange(3, 38))
		case slot == furnaceInputSlot || slot == furnaceFuelSlot:
			moveIntoSlots(candidate, &remaining, slotRange(3, 38))
		case slot >= 3 && slot <= 38:
			if _, cookable := game.CookingRecipeFor(furnace.cookingRecipeType(), remaining); cookable {
				moveIntoSlots(candidate, &remaining, []int{furnaceInputSlot})
			} else if game.IsFuel(remaining.Item) {
				moveIntoSlots(candidate, &remaining, []int{furnaceFuelSlot})
			} else if slot <= 29 {
				moveIntoSlots(candidate, &remaining, slotRange(30, 38))
			} else {
				moveIntoSlots(candidate, &remaining, slotRange(3, 29))
			}
		}

		candidate.slots[slot] = remaining

		normalizeStack(&candidate.slots[slot])
	}
}

func furnaceMenuIdentity(entityType game.BlockEntityType) (int32, string) {
	switch entityType {
	case game.BlockEntityTypeSmoker:
		return protocol.MenuSmoker, "container.smoker"
	case game.BlockEntityTypeBlastFurnace:
		return protocol.MenuBlastFurnace, "container.blast_furnace"
	default:
		return protocol.MenuFurnace, "container.furnace"
	}
}

func (r *Runtime) setFurnaceLitStateLocked(furnace *runtimeFurnace, lit bool) {
	block := r.World.BlockAt(furnace.position)
	if game.BlockEntityTypeForBlock(block) != furnace.entity.Type {
		return
	}

	replacement := withBlockProperties(block, game.BlockPropertyValue{Name: "lit", Value: boolProperty(lit)})
	if replacement == block {
		return
	}

	change := game.BlockChange{Position: furnace.position, Replacement: replacement}

	result, delivery, err := r.mutateBlocksLocked(nil, BlockMutationInteract, []game.BlockChange{change}, 1, true, false, true, false)
	if err != nil || !result.Changed {
		return
	}

	r.runtimeBlockMutations = append(r.runtimeBlockMutations, queuedBlockMutation{result: result, delivery: delivery})
}
