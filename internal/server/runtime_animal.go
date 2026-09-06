package server

import (
	"math"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

const (
	animalFlyingSpeed          = float32(0.02)
	animalGroundFriction       = float32(0.91)
	animalAirFriction          = float32(0.91)
	animalGravity              = 0.08
	animalVerticalDrag         = 0.98
	animalJumpStrength         = 0.42
	animalStepHeight           = 0.6
	animalStepDistanceScale    = 0.6
	animalMoveMaximumTurn      = float32(90)
	animalIdleLookMaximumYaw   = float32(10)
	animalIdleLookMaximumPitch = float32(40)
	animalMaximumHeadYaw       = float32(75)
	animalTrackingRangeChunks  = 10
	animalTrackingInterval     = 3
	animalFollowRange          = 16
	animalLootPickupDelay      = 10
	animalAmbientSoundInterval = 120
	chickenEggMinimumTicks     = 6000
	chickenEggRandomTicks      = 6000
	sheepShearedMask           = byte(0x10)
)

type animalDrop struct {
	Raw     game.Item
	Cooked  game.Item
	Minimum int32
	Random  int32
}

type runtimeAnimalSpec struct {
	EntityType    game.EntityType
	MaxHealth     float32
	MovementSpeed float32
	EyeHeight     float64
	PanicSpeed    float64
	AmbientSound  game.SoundEvent
	HurtSound     game.SoundEvent
	DeathSound    game.SoundEvent
	StepSound     game.SoundEvent
	TemptItems    map[game.Item]struct{}
	Drops         []animalDrop
}

type runtimeAnimal struct {
	State  RuntimeEntityState
	Living RuntimeLivingState
	RuntimeMobState
	Rotation game.Rotation

	Navigation  groundNavigationState
	MoveControl groundMoveControlState
	LookControl groundLookControlState
	BodyControl groundBodyRotationState
	Goals       runtimeGoalSelector
	Spec        *runtimeAnimalSpec

	TickCount        int32
	AmbientSoundTime int32
	LootDropped      bool
}

type runtimeCowEntity struct {
	runtimeAnimal
}

type runtimeSheepEntity struct {
	runtimeAnimal
	WoolColor   byte
	Sheared     bool
	WoolDropped bool
}

type runtimeChickenEntity struct {
	runtimeAnimal
	EggTime          int32
	Flap             float32
	FlapSpeed        float32
	Flapping         float32
	NextFlap         float32
	ChickenFlap      float32
	ChickenFlapSpeed float32
}

var cowAnimalSpec = runtimeAnimalSpec{
	EntityType:    game.EntityCow,
	MaxHealth:     10,
	MovementSpeed: 0.2,
	EyeHeight:     1.3,
	PanicSpeed:    2,
	AmbientSound:  game.SoundEntityCowAmbient,
	HurtSound:     game.SoundEntityCowHurt,
	DeathSound:    game.SoundEntityCowDeath,
	StepSound:     game.SoundEntityCowStep,
	TemptItems:    map[game.Item]struct{}{game.ItemWheat: {}},
	Drops: []animalDrop{
		{Raw: game.ItemLeather, Minimum: 0, Random: 3},
		{Raw: game.ItemBeef, Cooked: game.ItemCookedBeef, Minimum: 1, Random: 3},
	},
}

var sheepAnimalSpec = runtimeAnimalSpec{
	EntityType:    game.EntitySheep,
	MaxHealth:     8,
	MovementSpeed: 0.23,
	EyeHeight:     1.235,
	PanicSpeed:    1.25,
	AmbientSound:  game.SoundEntitySheepAmbient,
	HurtSound:     game.SoundEntitySheepHurt,
	DeathSound:    game.SoundEntitySheepDeath,
	StepSound:     game.SoundEntitySheepStep,
	TemptItems:    map[game.Item]struct{}{game.ItemWheat: {}},
	Drops: []animalDrop{
		{Raw: game.ItemMutton, Cooked: game.ItemCookedMutton, Minimum: 1, Random: 2},
	},
}

var chickenAnimalSpec = runtimeAnimalSpec{
	EntityType:    game.EntityChicken,
	MaxHealth:     4,
	MovementSpeed: 0.25,
	EyeHeight:     0.644,
	PanicSpeed:    1.4,
	AmbientSound:  game.SoundEntityChickenAmbient,
	HurtSound:     game.SoundEntityChickenHurt,
	DeathSound:    game.SoundEntityChickenDeath,
	StepSound:     game.SoundEntityChickenStep,
	TemptItems: map[game.Item]struct{}{
		game.ItemWheatSeeds:       {},
		game.ItemMelonSeeds:       {},
		game.ItemPumpkinSeeds:     {},
		game.ItemBeetrootSeeds:    {},
		game.ItemTorchflowerSeeds: {},
		game.ItemPitcherPod:       {},
	},
	Drops: []animalDrop{
		{Raw: game.ItemFeather, Minimum: 0, Random: 3},
		{Raw: game.ItemChicken, Cooked: game.ItemCookedChicken, Minimum: 1, Random: 1},
	},
}

func (entity *runtimeAnimal) RuntimeEntityState() *RuntimeEntityState {
	return &entity.State
}

func (entity *runtimeAnimal) RuntimeLivingState() *RuntimeLivingState {
	return &entity.Living
}

func (entity *runtimeAnimal) RuntimeMob() *RuntimeMobState {
	return &entity.RuntimeMobState
}

func (entity *runtimeAnimal) RuntimeMobDespawnConfig() RuntimeMobDespawnConfig {
	return RuntimeMobDespawnConfig{
		NoDespawnDistance: runtimeMobNoDespawnDistance,
		DespawnDistance:   runtimeMobDespawnDistance,
		AllowedInPeaceful: true,
	}
}

func (entity *runtimeAnimal) RuntimeMobRemoveWhenFarAway(float64) bool {
	return false
}

func (entity *runtimeAnimal) RuntimeMobRequiresCustomPersistence() bool {
	return false
}

func (entity *runtimeAnimal) RuntimeEntityTrackingConfig() RuntimeEntityTrackingConfig {
	return RuntimeEntityTrackingConfig{ClientRangeChunks: animalTrackingRangeChunks, UpdateInterval: animalTrackingInterval, TrackDeltas: true}
}

func (entity *runtimeAnimal) RuntimeEntityView() runtimeEntityView {
	entity.State.mu.RLock()
	defer entity.State.mu.RUnlock()

	return entity.runtimeEntityViewLocked()
}

func (entity *runtimeAnimal) EntityMetadata() []protocol.EntityMetadataEntry {
	entity.State.mu.RLock()
	defer entity.State.mu.RUnlock()

	return runtimeMobMetadata(entity.Living.EntityFlags(), entity.Living.Health, 0)
}

func (entity *runtimeAnimal) EntityVelocity() game.Velocity {
	entity.State.mu.RLock()
	defer entity.State.mu.RUnlock()

	return entity.Living.Velocity
}

func (entity *runtimeAnimal) AddEntityPacket(snapshot runtimeEntitySpawnSnapshot) protocol.AddEntity {
	return protocol.AddEntity{
		EntityID:  snapshot.ID,
		UUID:      snapshot.UUID,
		Type:      int32(entity.Spec.EntityType),
		X:         snapshot.Position.X,
		Y:         snapshot.Position.Y,
		Z:         snapshot.Position.Z,
		VelocityX: snapshot.Velocity.X,
		VelocityY: snapshot.Velocity.Y,
		VelocityZ: snapshot.Velocity.Z,
		Yaw:       snapshot.Yaw,
		Pitch:     snapshot.Pitch,
		HeadYaw:   snapshot.HeadYaw,
	}
}

func (entity *runtimeAnimal) RuntimeLivingDamageSound(died bool) (game.SoundEvent, float32, float32) {
	if died {
		return entity.Spec.DeathSound, 1, 1
	}

	entity.State.mu.Lock()
	entity.AmbientSoundTime = -animalAmbientSoundInterval
	entity.State.mu.Unlock()

	return entity.Spec.HurtSound, 1, 1
}

func (entity *runtimeAnimal) RuntimeEntitySoundSource() int32 {
	return protocol.SoundSourceNeutral
}

func (entity *runtimeAnimal) RuntimeLivingDied(runtime *Runtime) {
	entity.State.mu.Lock()

	if entity.LootDropped {
		entity.State.mu.Unlock()

		return
	}

	entity.LootDropped = true

	position := entity.State.Position
	burning := entity.Living.RemainingFireTicks > 0

	entity.State.mu.Unlock()

	for _, drop := range entity.Spec.Drops {
		count := drop.Minimum

		if drop.Random > 1 {
			count += int32(runtime.nextEntityRandom() * float32(drop.Random))
		}

		item := drop.Raw

		if burning && drop.Cooked != game.ItemAir {
			item = drop.Cooked
		}

		spawnAnimalDrop(runtime, position, item, count)
	}
}

func (entity *runtimeCowEntity) Tick(runtime *Runtime, _ *ActiveChunk) {
	runtime.tickAnimal(entity, &entity.runtimeAnimal, false)
}

func (entity *runtimeCowEntity) RuntimeEntityInteract(runtime *Runtime, session *Session, interaction RuntimeEntityInteraction) bool {
	return runtime.milkCow(session, entity, interaction.Hand)
}

func (entity *runtimeSheepEntity) Tick(runtime *Runtime, _ *ActiveChunk) {
	runtime.tickAnimal(entity, &entity.runtimeAnimal, false)
}

func (entity *runtimeSheepEntity) EntityMetadata() []protocol.EntityMetadataEntry {
	entity.State.mu.RLock()
	defer entity.State.mu.RUnlock()

	metadata := runtimeMobMetadata(entity.Living.EntityFlags(), entity.Living.Health, 0)

	wool := entity.WoolColor & 0x0f

	if entity.Sheared {
		wool |= sheepShearedMask
	}

	return append(metadata, protocol.EntityMetadataEntry{Index: protocol.SheepWoolMetadataIndex, Type: protocol.MetadataTypeByte, Value: protocol.MetadataByte(wool)})
}

func (entity *runtimeSheepEntity) RuntimeEntityInteract(runtime *Runtime, session *Session, interaction RuntimeEntityInteraction) bool {
	return runtime.shearSheep(session, entity, interaction.Hand)
}

func (entity *runtimeSheepEntity) RuntimeLivingDied(runtime *Runtime) {
	entity.runtimeAnimal.RuntimeLivingDied(runtime)

	entity.State.mu.Lock()
	position := entity.State.Position
	dropWool := !entity.Sheared && !entity.WoolDropped

	entity.WoolDropped = true
	entity.State.mu.Unlock()

	if dropWool {
		spawnAnimalDrop(runtime, position, game.ItemWhiteWool, 1)
	}
}

func (entity *runtimeChickenEntity) Tick(runtime *Runtime, _ *ActiveChunk) {
	runtime.tickChicken(entity)
}

func (entity *runtimeAnimal) runtimeEntityViewLocked() runtimeEntityView {
	return runtimeEntityView{
		ID:       entity.State.ID,
		UUID:     entity.State.UUID,
		Position: entity.State.Position,
		Chunk:    entity.State.Chunk,
		Removed:  entity.State.Removed,
		Velocity: entity.Living.Velocity,
		Rotation: entity.Rotation,
		OnGround: entity.Living.OnGround,
	}
}

func (runtime *Runtime) SpawnCow(position game.Position) *runtimeCowEntity {
	entity := &runtimeCowEntity{}
	if !runtime.initializeAnimal(&entity.runtimeAnimal, &cowAnimalSpec) {
		return nil
	}

	runtime.configureAnimalGoals(&entity.runtimeAnimal)
	runtime.registerRuntimeEntity(entity, position)

	return entity
}

func (runtime *Runtime) SpawnSheep(position game.Position) *runtimeSheepEntity {
	entity := &runtimeSheepEntity{}
	if !runtime.initializeAnimal(&entity.runtimeAnimal, &sheepAnimalSpec) {
		return nil
	}

	runtime.configureAnimalGoals(&entity.runtimeAnimal)
	runtime.registerRuntimeEntity(entity, position)

	return entity
}

func (runtime *Runtime) SpawnChicken(position game.Position) *runtimeChickenEntity {
	entity := &runtimeChickenEntity{
		EggTime:  chickenEggMinimumTicks + int32(runtime.nextEntityRandom()*chickenEggRandomTicks),
		Flapping: 1,
		NextFlap: 1,
	}

	if !runtime.initializeAnimal(&entity.runtimeAnimal, &chickenAnimalSpec) {
		return nil
	}

	runtime.configureAnimalGoals(&entity.runtimeAnimal)
	runtime.registerRuntimeEntity(entity, position)

	return entity
}

func (runtime *Runtime) initializeAnimal(entity *runtimeAnimal, specification *runtimeAnimalSpec) bool {
	definition, valid := specification.EntityType.Definition()
	if !valid {
		return false
	}

	entity.Spec = specification
	entity.Living = RuntimeLivingState{
		Width:               definition.Width,
		Height:              definition.Height,
		KnockbackResistance: 0,
		Armor:               0,
		ArmorToughness:      0,
	}

	entity.Living.Reset(specification.MaxHealth)

	return true
}

func (runtime *Runtime) tickAnimal(concrete RuntimeEntity, entity *runtimeAnimal, fallDamageImmune bool) {
	livingEntity := concrete.(RuntimeLivingEntity)

	entity.State.mu.RLock()
	removed := entity.State.Removed
	dead := entity.Living.Dead
	entity.State.mu.RUnlock()

	if removed {
		return
	}

	if dead {
		runtime.tickRuntimeLivingEntity(livingEntity)

		return
	}

	mob := concrete.(RuntimeMobEntity)
	if runtime.checkRuntimeMobDespawn(mob) {
		return
	}

	entity.State.mu.Lock()
	entity.TickCount++
	entity.NoActionTime++
	entity.State.mu.Unlock()

	runtime.tickRuntimeLivingBaseEnvironment(livingEntity)

	if runtimeLivingDead(livingEntity) {
		runtime.tickRuntimeLivingEntity(livingEntity)
		runtime.synchronizeRuntimeEntity(concrete)

		return
	}

	entity.Goals.Tick(runtime)

	configuration := animalGroundControlConfig(entity.Spec)

	fallDamage, step := runtime.tickGroundMobMovement(concrete, &entity.Living, &entity.Rotation, &entity.Navigation, &entity.MoveControl, &entity.LookControl, &entity.BodyControl, configuration, fallDamageImmune)
	if fallDamage > 0 {
		runtime.applyRuntimeLivingEnvironmentDamage(livingEntity, game.Damage{Type: game.DamageFall, Amount: fallDamage})
	}

	runtime.tickRuntimeLivingBlockEnvironment(livingEntity)

	if step {
		runtime.broadcastRuntimeEntitySound(concrete, entity.Spec.StepSound, 0.15, 1)
	}

	runtime.tickAnimalAmbientSound(concrete, entity)
	runtime.tickRuntimeLivingEntity(livingEntity)
	runtime.synchronizeRuntimeEntity(concrete)
}

func (runtime *Runtime) tickChicken(entity *runtimeChickenEntity) {
	entity.State.mu.RLock()
	inactive := entity.State.Removed || entity.Living.Dead
	entity.State.mu.RUnlock()

	if inactive {
		runtime.tickAnimal(entity, &entity.runtimeAnimal, true)

		return
	}

	entity.State.mu.Lock()
	airborne := !entity.Living.OnGround

	if airborne && entity.Living.Velocity.Y < 0 {
		entity.Living.Velocity.Y *= 0.6
	}

	entity.Flap += entity.Flapping * 2

	flapChange := func() float32 {
		if entity.Living.OnGround {
			return -0.3
		}

		return 1.2
	}

	entity.Flapping += flapChange()
	entity.Flapping = min(max(entity.Flapping, 0), 1)
	entity.NextFlap *= 0.9

	if !entity.Living.OnGround && entity.NextFlap < 1 {
		entity.NextFlap = 1
	}

	entity.EggTime--
	layEgg := entity.EggTime <= 0 && !entity.Living.Dead

	if layEgg {
		entity.EggTime = chickenEggMinimumTicks + int32(runtime.nextEntityRandom()*chickenEggRandomTicks)
	}

	entity.State.mu.Unlock()

	runtime.tickAnimal(entity, &entity.runtimeAnimal, true)

	if layEgg && !runtimeLivingDead(entity) {
		entity.State.mu.RLock()
		position := entity.State.Position
		removed := entity.State.Removed
		entity.State.mu.RUnlock()

		if removed {
			return
		}

		runtime.broadcastRuntimeEntitySound(entity, game.SoundEntityChickenEgg, 1, 1)
		spawnAnimalDrop(runtime, position, game.ItemEgg, 1)
	}
}

func (runtime *Runtime) tickAnimalAmbientSound(concrete RuntimeEntity, entity *runtimeAnimal) {
	selection := runtimeMobRandomInt(runtime, 1000)

	entity.State.mu.Lock()
	ambientSoundTime := entity.AmbientSoundTime
	entity.AmbientSoundTime++
	play := selection < int(ambientSoundTime)

	if play {
		entity.AmbientSoundTime = -animalAmbientSoundInterval
	}

	entity.State.mu.Unlock()

	if play {
		runtime.broadcastRuntimeEntitySound(concrete, entity.Spec.AmbientSound, 1, 1)
	}
}

func (runtime *Runtime) milkCow(session *Session, entity *runtimeCowEntity, hand int32) bool {
	entity.State.mu.RLock()
	alive := !entity.Living.Dead && !entity.State.Removed
	entity.State.mu.RUnlock()

	if !alive {
		return false
	}

	_, changed := session.updatePlayerState(func(player *game.Player) bool {
		held, valid := heldItemPointer(player, hand)
		if !valid || held.Item != game.ItemBucket || held.Count <= 0 {
			return false
		}

		if player.GameMode == game.GameModeCreative {
			inserted := insertAnimalInteractionItem(&player.Inventory, game.ItemStack{Item: game.ItemMilkBucket, Count: 1})

			return inserted
		}

		if held.Count == 1 {
			*held = game.ItemStack{Item: game.ItemMilkBucket, Count: 1}

			return true
		}

		held.Count--

		inserted := insertAnimalInteractionItem(&player.Inventory, game.ItemStack{Item: game.ItemMilkBucket, Count: 1})

		return inserted
	})

	if changed {
		runtime.broadcastRuntimeEntitySound(entity, game.SoundEntityCowMilk, 1, 1)
	}

	return changed
}

func (runtime *Runtime) shearSheep(session *Session, entity *runtimeSheepEntity, hand int32) bool {
	player := session.snapshotPlayer()

	held, valid := heldItemPointer(&player, hand)
	if !valid || held.Item != game.ItemShears {
		return false
	}

	entity.State.mu.Lock()

	if entity.Sheared || entity.Living.Dead || entity.State.Removed {
		entity.State.mu.Unlock()

		return false
	}

	entity.Sheared = true
	entity.State.metadataDirty = true

	position := entity.State.Position

	entity.State.mu.Unlock()

	count := int32(1 + runtimeMobRandomInt(runtime, 3))

	spawnAnimalDrop(runtime, position, game.ItemWhiteWool, count)

	runtime.broadcastRuntimeEntitySoundFromSource(entity, game.SoundEntitySheepShear, protocol.SoundSourcePlayer, 1, 1)

	if player.GameMode == game.GameModeCreative {
		return false
	}

	changed := false

	session.updatePlayerState(func(current *game.Player) bool {
		stack, stackValid := heldItemPointer(current, hand)
		if !stackValid || !stack.SameItem(*held) {
			return false
		}

		definition, defined := stack.Item.Definition()
		if !defined {
			return false
		}

		_, changed = runtime.damageItemStack(stack, 1, definition.MaxDurability)

		return changed
	})

	return changed
}

func runtimeMobMetadata(entityFlags byte, health float32, mobFlags byte) []protocol.EntityMetadataEntry {
	return []protocol.EntityMetadataEntry{
		{Index: protocol.EntityFlagsMetadataIndex, Type: protocol.MetadataTypeByte, Value: protocol.MetadataByte(entityFlags)},
		{Index: protocol.LivingHealthMetadataIndex, Type: protocol.MetadataTypeFloat, Value: protocol.MetadataFloat(health)},
		{Index: protocol.MobFlagsMetadataIndex, Type: protocol.MetadataTypeByte, Value: protocol.MetadataByte(mobFlags)},
	}
}

func animalGroundControlConfig(specification *runtimeAnimalSpec) groundMobControlConfig {
	return groundMobControlConfig{
		MovementSpeed:        specification.MovementSpeed,
		FlyingSpeed:          animalFlyingSpeed,
		GroundFriction:       animalGroundFriction,
		AirFriction:          animalAirFriction,
		Gravity:              animalGravity,
		VerticalDrag:         animalVerticalDrag,
		JumpStrength:         animalJumpStrength,
		StepHeight:           animalStepHeight,
		StepDistanceScale:    animalStepDistanceScale,
		EyeHeight:            specification.EyeHeight,
		MoveMaximumTurn:      animalMoveMaximumTurn,
		IdleLookMaximumYaw:   animalIdleLookMaximumYaw,
		IdleLookMaximumPitch: animalIdleLookMaximumPitch,
		MaximumHeadYaw:       animalMaximumHeadYaw,
	}
}

func spawnAnimalDrop(runtime *Runtime, position game.Position, item game.Item, count int32) {
	if count <= 0 {
		return
	}

	runtime.SpawnItemEntity(game.ItemStack{Item: item, Count: count}, position, game.Velocity{}, animalLootPickupDelay)
}

func insertAnimalInteractionItem(inventory *game.PlayerInventory, stack game.ItemStack) bool {
	for slot := 9; slot <= 44; slot++ {
		candidate := inventory.Slot(slot)

		if candidate.Empty() {
			*candidate = stack

			return true
		}
	}

	return false
}

func animalRandomStrollPath(runtime *Runtime, entity *runtimeAnimal) []game.Position {
	position := entity.State.Position

	for range 10 {
		offsetX := runtimeMobRandomInt(runtime, 21) - 10
		offsetY := runtimeMobRandomInt(runtime, 15) - 7
		offsetZ := runtimeMobRandomInt(runtime, 21) - 10

		goal := game.Position{X: position.X + float64(offsetX), Y: position.Y + float64(offsetY), Z: position.Z + float64(offsetZ)}

		path := runtime.findGroundPath(position, goal, entity.Living.Width, entity.Living.Height, animalFollowRange)
		if len(path) > 0 {
			return path
		}
	}

	return nil
}

func animalEyePosition(entity *runtimeAnimal) game.Position {
	position := entity.State.Position
	position.Y += entity.Spec.EyeHeight

	return position
}

func animalLookDirection(runtime *Runtime) (float64, float64) {
	angle := float64(runtime.nextEntityRandom()) * math.Pi * 2

	return math.Cos(angle), math.Sin(angle)
}
