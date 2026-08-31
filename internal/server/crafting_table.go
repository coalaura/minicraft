package server

import (
	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

type craftingTableBacking struct {
	position game.BlockPosition
	result   game.ItemStack
	inputs   [9]game.ItemStack
}

func (table *craftingTableBacking) ContainsPosition(position game.BlockPosition) bool {
	return table.position == position
}

func (*craftingTableBacking) Attach(*Runtime, *Session) {}

func (*craftingTableBacking) Detach(*Runtime, *Session) {}

func (*craftingTableBacking) Changed(*Runtime, *Session, []int) {}

func (table *craftingTableBacking) StillValid(runtime *Runtime, session *Session) bool {
	if runtime.World.BlockAt(table.position) != game.CraftingTable {
		return false
	}

	player := session.snapshotPlayer()
	return containerWithinRange(player, table.position)
}

func (r *Runtime) openCraftingTableLocked(session *Session, position game.BlockPosition) error {
	r.closeMenuLocked(session, false)

	table := &craftingTableBacking{position: position}

	menu := newCraftingTableMenu(session.allocateWindowID(), table, &session.Player.Inventory)

	session.containerMenu = menu

	table.Attach(r, session)

	err := session.writePacket(protocol.ClientboundOpenScreenID, protocol.OpenScreen{
		ContainerID: menu.windowID,
		MenuType:    menu.protocolMenuType,
		Title:       game.TranslatableText("container.crafting"),
	})

	if err != nil {
		r.closeMenuLocked(session, false)

		return err
	}

	return session.sendMenuSnapshot(menu.snapshot())
}
func newCraftingTableMenu(windowID int32, table *craftingTableBacking, inventory *game.PlayerInventory) *menu {
	slots := make([]menuSlot, 0, 46)

	slots = append(slots, menuSlot{
		stack:        &table.result,
		role:         menuSlotResult,
		derived:      true,
		limit:        64,
		storage:      menuStorageBacking,
		backingIndex: 0,
		onTake:       takeCraftingTableResult,
	})

	for slot := range table.inputs {
		slots = append(slots, menuSlot{
			stack:        &table.inputs[slot],
			limit:        64,
			storage:      menuStorageBacking,
			backingIndex: 0,
		})
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

	craftingMenu := &menu{
		windowID:         windowID,
		protocolMenuType: protocol.MenuCrafting,
		slots:            slots,
		hiddenOffhand:    inventory.Slot(45),
		quickMove:        quickMoveCraftingTable,
		derive:           deriveCraftingTableResult,
		removed:          removeCraftingTableItems,
		backing:          table,
		containerSlots:   10,
	}

	for hotbar := range game.HotbarSlotCount {
		craftingMenu.hotbarSlots[hotbar] = 37 + hotbar
		craftingMenu.hasHotbarSlots[hotbar] = true
	}

	candidate := craftingMenu.candidate()

	candidate.deriveSlots()

	craftingMenu.commit(candidate)

	return craftingMenu
}
