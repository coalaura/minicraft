package game

type SoundEvent string

const (
	BlockSoundEmpty BlockSoundType = iota
	BlockSoundStone
	BlockSoundGrass
	BlockSoundWood
	BlockSoundGlass
	BlockSoundWool
	BlockSoundSnow
	BlockSoundMetal
	BlockSoundCandle
)

const (
	SoundBlockBarrelClose SoundEvent = "minecraft:block.barrel.close"
	SoundBlockBarrelOpen  SoundEvent = "minecraft:block.barrel.open"

	SoundBlockBambooWoodDoorClose      SoundEvent = "minecraft:block.bamboo_wood_door.close"
	SoundBlockBambooWoodDoorOpen       SoundEvent = "minecraft:block.bamboo_wood_door.open"
	SoundBlockBambooWoodTrapdoorClose  SoundEvent = "minecraft:block.bamboo_wood_trapdoor.close"
	SoundBlockBambooWoodTrapdoorOpen   SoundEvent = "minecraft:block.bamboo_wood_trapdoor.open"
	SoundBlockBambooWoodButtonClickOff SoundEvent = "minecraft:block.bamboo_wood_button.click_off"
	SoundBlockBambooWoodButtonClickOn  SoundEvent = "minecraft:block.bamboo_wood_button.click_on"
	SoundBlockBambooWoodFenceGateClose SoundEvent = "minecraft:block.bamboo_wood_fence_gate.close"
	SoundBlockBambooWoodFenceGateOpen  SoundEvent = "minecraft:block.bamboo_wood_fence_gate.open"

	SoundBlockCandleBreak SoundEvent = "minecraft:block.candle.break"
	SoundBlockCandleFall  SoundEvent = "minecraft:block.candle.fall"
	SoundBlockCandleHit   SoundEvent = "minecraft:block.candle.hit"
	SoundBlockCandlePlace SoundEvent = "minecraft:block.candle.place"
	SoundBlockCandleStep  SoundEvent = "minecraft:block.candle.step"

	SoundBlockCherryWoodDoorClose      SoundEvent = "minecraft:block.cherry_wood_door.close"
	SoundBlockCherryWoodDoorOpen       SoundEvent = "minecraft:block.cherry_wood_door.open"
	SoundBlockCherryWoodTrapdoorClose  SoundEvent = "minecraft:block.cherry_wood_trapdoor.close"
	SoundBlockCherryWoodTrapdoorOpen   SoundEvent = "minecraft:block.cherry_wood_trapdoor.open"
	SoundBlockCherryWoodButtonClickOff SoundEvent = "minecraft:block.cherry_wood_button.click_off"
	SoundBlockCherryWoodButtonClickOn  SoundEvent = "minecraft:block.cherry_wood_button.click_on"
	SoundBlockCherryWoodFenceGateClose SoundEvent = "minecraft:block.cherry_wood_fence_gate.close"
	SoundBlockCherryWoodFenceGateOpen  SoundEvent = "minecraft:block.cherry_wood_fence_gate.open"

	SoundBlockFenceGateClose SoundEvent = "minecraft:block.fence_gate.close"
	SoundBlockFenceGateOpen  SoundEvent = "minecraft:block.fence_gate.open"

	SoundBlockGlassBreak SoundEvent = "minecraft:block.glass.break"
	SoundBlockGlassFall  SoundEvent = "minecraft:block.glass.fall"
	SoundBlockGlassHit   SoundEvent = "minecraft:block.glass.hit"
	SoundBlockGlassPlace SoundEvent = "minecraft:block.glass.place"
	SoundBlockGlassStep  SoundEvent = "minecraft:block.glass.step"

	SoundBlockGrassBreak SoundEvent = "minecraft:block.grass.break"
	SoundBlockGrassFall  SoundEvent = "minecraft:block.grass.fall"
	SoundBlockGrassHit   SoundEvent = "minecraft:block.grass.hit"
	SoundBlockGrassPlace SoundEvent = "minecraft:block.grass.place"
	SoundBlockGrassStep  SoundEvent = "minecraft:block.grass.step"

	SoundBlockMetalBreak SoundEvent = "minecraft:block.metal.break"
	SoundBlockMetalFall  SoundEvent = "minecraft:block.metal.fall"
	SoundBlockMetalHit   SoundEvent = "minecraft:block.metal.hit"
	SoundBlockMetalPlace SoundEvent = "minecraft:block.metal.place"
	SoundBlockMetalStep  SoundEvent = "minecraft:block.metal.step"

	SoundBlockNetherWoodDoorClose      SoundEvent = "minecraft:block.nether_wood_door.close"
	SoundBlockNetherWoodDoorOpen       SoundEvent = "minecraft:block.nether_wood_door.open"
	SoundBlockNetherWoodTrapdoorClose  SoundEvent = "minecraft:block.nether_wood_trapdoor.close"
	SoundBlockNetherWoodTrapdoorOpen   SoundEvent = "minecraft:block.nether_wood_trapdoor.open"
	SoundBlockNetherWoodButtonClickOff SoundEvent = "minecraft:block.nether_wood_button.click_off"
	SoundBlockNetherWoodButtonClickOn  SoundEvent = "minecraft:block.nether_wood_button.click_on"
	SoundBlockNetherWoodFenceGateClose SoundEvent = "minecraft:block.nether_wood_fence_gate.close"
	SoundBlockNetherWoodFenceGateOpen  SoundEvent = "minecraft:block.nether_wood_fence_gate.open"

	SoundBlockSnowBreak SoundEvent = "minecraft:block.snow.break"
	SoundBlockSnowFall  SoundEvent = "minecraft:block.snow.fall"
	SoundBlockSnowHit   SoundEvent = "minecraft:block.snow.hit"
	SoundBlockSnowPlace SoundEvent = "minecraft:block.snow.place"
	SoundBlockSnowStep  SoundEvent = "minecraft:block.snow.step"

	SoundBlockStoneBreak          SoundEvent = "minecraft:block.stone.break"
	SoundBlockStoneButtonClickOff SoundEvent = "minecraft:block.stone_button.click_off"
	SoundBlockStoneButtonClickOn  SoundEvent = "minecraft:block.stone_button.click_on"
	SoundBlockStoneFall           SoundEvent = "minecraft:block.stone.fall"
	SoundBlockStoneHit            SoundEvent = "minecraft:block.stone.hit"
	SoundBlockStonePlace          SoundEvent = "minecraft:block.stone.place"
	SoundBlockStoneStep           SoundEvent = "minecraft:block.stone.step"

	SoundBlockWoodenDoorClose      SoundEvent = "minecraft:block.wooden_door.close"
	SoundBlockWoodenDoorOpen       SoundEvent = "minecraft:block.wooden_door.open"
	SoundBlockWoodenTrapdoorClose  SoundEvent = "minecraft:block.wooden_trapdoor.close"
	SoundBlockWoodenTrapdoorOpen   SoundEvent = "minecraft:block.wooden_trapdoor.open"
	SoundBlockWoodenButtonClickOff SoundEvent = "minecraft:block.wooden_button.click_off"
	SoundBlockWoodenButtonClickOn  SoundEvent = "minecraft:block.wooden_button.click_on"

	SoundBlockWoodBreak SoundEvent = "minecraft:block.wood.break"
	SoundBlockWoodFall  SoundEvent = "minecraft:block.wood.fall"
	SoundBlockWoodHit   SoundEvent = "minecraft:block.wood.hit"
	SoundBlockWoodPlace SoundEvent = "minecraft:block.wood.place"
	SoundBlockWoodStep  SoundEvent = "minecraft:block.wood.step"

	SoundBlockWoolBreak SoundEvent = "minecraft:block.wool.break"
	SoundBlockWoolFall  SoundEvent = "minecraft:block.wool.fall"
	SoundBlockWoolHit   SoundEvent = "minecraft:block.wool.hit"
	SoundBlockWoolPlace SoundEvent = "minecraft:block.wool.place"
	SoundBlockWoolStep  SoundEvent = "minecraft:block.wool.step"
)

type BlockSound struct {
	Volume float32
	Pitch  float32
	Break  SoundEvent
	Step   SoundEvent
	Place  SoundEvent
	Hit    SoundEvent
	Fall   SoundEvent
}

var blockSounds = [...]BlockSound{
	BlockSoundEmpty:  {},
	BlockSoundStone:  {Volume: 1, Pitch: 1, Break: SoundBlockStoneBreak, Step: SoundBlockStoneStep, Place: SoundBlockStonePlace, Hit: SoundBlockStoneHit, Fall: SoundBlockStoneFall},
	BlockSoundGrass:  {Volume: 1, Pitch: 1, Break: SoundBlockGrassBreak, Step: SoundBlockGrassStep, Place: SoundBlockGrassPlace, Hit: SoundBlockGrassHit, Fall: SoundBlockGrassFall},
	BlockSoundWood:   {Volume: 1, Pitch: 1, Break: SoundBlockWoodBreak, Step: SoundBlockWoodStep, Place: SoundBlockWoodPlace, Hit: SoundBlockWoodHit, Fall: SoundBlockWoodFall},
	BlockSoundGlass:  {Volume: 1, Pitch: 1, Break: SoundBlockGlassBreak, Step: SoundBlockGlassStep, Place: SoundBlockGlassPlace, Hit: SoundBlockGlassHit, Fall: SoundBlockGlassFall},
	BlockSoundWool:   {Volume: 1, Pitch: 1, Break: SoundBlockWoolBreak, Step: SoundBlockWoolStep, Place: SoundBlockWoolPlace, Hit: SoundBlockWoolHit, Fall: SoundBlockWoolFall},
	BlockSoundSnow:   {Volume: 1, Pitch: 1, Break: SoundBlockSnowBreak, Step: SoundBlockSnowStep, Place: SoundBlockSnowPlace, Hit: SoundBlockSnowHit, Fall: SoundBlockSnowFall},
	BlockSoundMetal:  {Volume: 1, Pitch: 1.5, Break: SoundBlockMetalBreak, Step: SoundBlockMetalStep, Place: SoundBlockMetalPlace, Hit: SoundBlockMetalHit, Fall: SoundBlockMetalFall},
	BlockSoundCandle: {Volume: 1, Pitch: 1, Break: SoundBlockCandleBreak, Step: SoundBlockCandleStep, Place: SoundBlockCandlePlace, Hit: SoundBlockCandleHit, Fall: SoundBlockCandleFall},
}
