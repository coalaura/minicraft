package game

type SoundEvent int32

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
	SoundBlockBambooWoodDoorClose      SoundEvent = 112
	SoundBlockBambooWoodDoorOpen       SoundEvent = 113
	SoundBlockBambooWoodTrapdoorClose  SoundEvent = 114
	SoundBlockBambooWoodTrapdoorOpen   SoundEvent = 115
	SoundBlockBambooWoodButtonClickOff SoundEvent = 116
	SoundBlockBambooWoodButtonClickOn  SoundEvent = 117
	SoundBlockBambooWoodFenceGateClose SoundEvent = 120
	SoundBlockBambooWoodFenceGateOpen  SoundEvent = 121

	SoundBlockCandleBreak SoundEvent = 250
	SoundBlockCandleFall  SoundEvent = 252
	SoundBlockCandleHit   SoundEvent = 253
	SoundBlockCandlePlace SoundEvent = 254
	SoundBlockCandleStep  SoundEvent = 255

	SoundBlockCherryWoodDoorClose      SoundEvent = 296
	SoundBlockCherryWoodDoorOpen       SoundEvent = 297
	SoundBlockCherryWoodTrapdoorClose  SoundEvent = 298
	SoundBlockCherryWoodTrapdoorOpen   SoundEvent = 299
	SoundBlockCherryWoodButtonClickOff SoundEvent = 300
	SoundBlockCherryWoodButtonClickOn  SoundEvent = 301
	SoundBlockCherryWoodFenceGateClose SoundEvent = 304
	SoundBlockCherryWoodFenceGateOpen  SoundEvent = 305

	SoundBlockFenceGateClose SoundEvent = 572
	SoundBlockFenceGateOpen  SoundEvent = 573

	SoundBlockGlassBreak SoundEvent = 665
	SoundBlockGlassFall  SoundEvent = 666
	SoundBlockGlassHit   SoundEvent = 667
	SoundBlockGlassPlace SoundEvent = 668
	SoundBlockGlassStep  SoundEvent = 669

	SoundBlockGrassBreak SoundEvent = 698
	SoundBlockGrassFall  SoundEvent = 699
	SoundBlockGrassHit   SoundEvent = 700
	SoundBlockGrassPlace SoundEvent = 701
	SoundBlockGrassStep  SoundEvent = 702

	SoundBlockMetalBreak SoundEvent = 894
	SoundBlockMetalFall  SoundEvent = 895
	SoundBlockMetalHit   SoundEvent = 896
	SoundBlockMetalPlace SoundEvent = 897
	SoundBlockMetalStep  SoundEvent = 900

	SoundBlockNetherWoodDoorClose      SoundEvent = 970
	SoundBlockNetherWoodDoorOpen       SoundEvent = 971
	SoundBlockNetherWoodTrapdoorClose  SoundEvent = 972
	SoundBlockNetherWoodTrapdoorOpen   SoundEvent = 973
	SoundBlockNetherWoodButtonClickOff SoundEvent = 974
	SoundBlockNetherWoodButtonClickOn  SoundEvent = 975
	SoundBlockNetherWoodFenceGateClose SoundEvent = 978
	SoundBlockNetherWoodFenceGateOpen  SoundEvent = 979

	SoundBlockSnowBreak SoundEvent = 1385
	SoundBlockSnowFall  SoundEvent = 1386
	SoundBlockSnowHit   SoundEvent = 1392
	SoundBlockSnowPlace SoundEvent = 1393
	SoundBlockSnowStep  SoundEvent = 1394

	SoundBlockStoneBreak          SoundEvent = 1413
	SoundBlockStoneButtonClickOff SoundEvent = 1414
	SoundBlockStoneButtonClickOn  SoundEvent = 1415
	SoundBlockStoneFall           SoundEvent = 1416
	SoundBlockStoneHit            SoundEvent = 1417
	SoundBlockStonePlace          SoundEvent = 1418
	SoundBlockStoneStep           SoundEvent = 1421

	SoundBlockWoodenDoorClose      SoundEvent = 1604
	SoundBlockWoodenDoorOpen       SoundEvent = 1605
	SoundBlockWoodenTrapdoorClose  SoundEvent = 1606
	SoundBlockWoodenTrapdoorOpen   SoundEvent = 1607
	SoundBlockWoodenButtonClickOff SoundEvent = 1608
	SoundBlockWoodenButtonClickOn  SoundEvent = 1609

	SoundBlockWoodBreak SoundEvent = 1612
	SoundBlockWoodFall  SoundEvent = 1613
	SoundBlockWoodHit   SoundEvent = 1614
	SoundBlockWoodPlace SoundEvent = 1615
	SoundBlockWoodStep  SoundEvent = 1616

	SoundBlockWoolBreak SoundEvent = 1617
	SoundBlockWoolFall  SoundEvent = 1618
	SoundBlockWoolHit   SoundEvent = 1619
	SoundBlockWoolPlace SoundEvent = 1620
	SoundBlockWoolStep  SoundEvent = 1621
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
