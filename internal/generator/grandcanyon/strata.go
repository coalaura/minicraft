package grandcanyon

import "github.com/coalaura/minicraft/internal/game"

type blockPalette struct {
	redSand             game.Block
	redSandstone        game.Block
	sandstone           game.Block
	smoothSandstone     game.Block
	cutSandstone        game.Block
	terracotta          game.Block
	orangeTerracotta    game.Block
	yellowTerracotta    game.Block
	redTerracotta       game.Block
	brownTerracotta     game.Block
	whiteTerracotta     game.Block
	lightGrayTerracotta game.Block
	grayTerracotta      game.Block
	greenTerracotta     game.Block
	coarseDirt          game.Block
	mud                 game.Block
	granite             game.Block
	deepslate           game.Block
	cobbledDeepslate    game.Block
	tuff                game.Block
	andesite            game.Block
	cobblestone         game.Block
	gravel              game.Block
	cactus              game.Block
	deadBush            game.Block
}

var palette = blockPalette{
	redSand:             mustBlock("red_sand"),
	redSandstone:        mustBlock("red_sandstone"),
	sandstone:           mustBlock("sandstone"),
	smoothSandstone:     mustBlock("smooth_sandstone"),
	cutSandstone:        mustBlock("cut_sandstone"),
	terracotta:          mustBlock("terracotta"),
	orangeTerracotta:    mustBlock("orange_terracotta"),
	yellowTerracotta:    mustBlock("yellow_terracotta"),
	redTerracotta:       mustBlock("red_terracotta"),
	brownTerracotta:     mustBlock("brown_terracotta"),
	whiteTerracotta:     mustBlock("white_terracotta"),
	lightGrayTerracotta: mustBlock("light_gray_terracotta"),
	grayTerracotta:      mustBlock("gray_terracotta"),
	greenTerracotta:     mustBlock("green_terracotta"),
	coarseDirt:          mustBlock("coarse_dirt"),
	mud:                 mustBlock("mud"),
	granite:             mustBlock("granite"),
	deepslate:           mustBlock("deepslate"),
	cobbledDeepslate:    mustBlock("cobbled_deepslate"),
	tuff:                mustBlock("tuff"),
	andesite:            mustBlock("andesite"),
	cobblestone:         mustBlock("cobblestone"),
	gravel:              mustBlock("gravel"),
	cactus:              mustBlock("cactus"),
	deadBush:            mustBlock("dead_bush"),
}

func strataBlockAt(y int32, sublayer uint64) game.Block {
	switch {
	case y < 18:
		return palette.deepslate

	case y < 24:
		if sublayer%5 == 0 {
			return palette.granite
		}

		if sublayer%3 == 0 {
			return palette.cobbledDeepslate
		}

		return palette.deepslate

	case y < 34:
		if sublayer%4 == 0 {
			return palette.granite
		}

		if sublayer%5 == 0 {
			return palette.tuff
		}

		return palette.deepslate

	case y < 44:
		if sublayer%3 == 0 {
			return palette.granite
		}

		if sublayer%7 == 0 {
			return palette.andesite
		}

		return game.Stone

	case y < 54:
		if sublayer%4 == 0 {
			return palette.granite
		}

		return palette.deepslate

	case y < 58:
		return palette.brownTerracotta

	case y < 66:
		if sublayer%5 == 0 {
			return palette.brownTerracotta
		}

		return palette.redSandstone

	case y < 70:
		if sublayer%4 == 0 {
			return palette.greenTerracotta
		}

		return palette.brownTerracotta

	case y < 74:
		if sublayer%3 == 0 {
			return palette.tuff
		}

		return palette.terracotta

	case y < 78:
		if sublayer%6 == 0 {
			return palette.greenTerracotta
		}

		return palette.brownTerracotta

	case y < 82:
		return palette.grayTerracotta

	case y < 86:
		if sublayer%4 == 0 {
			return palette.sandstone
		}

		return palette.lightGrayTerracotta

	case y < 88:
		return palette.terracotta

	case y < 94:
		if sublayer%5 == 0 {
			return palette.orangeTerracotta
		}

		return palette.redTerracotta

	case y < 100:
		if sublayer%6 == 0 {
			return palette.granite
		}

		return palette.terracotta

	case y < 107:
		if sublayer%4 == 0 {
			return palette.terracotta
		}

		return palette.redTerracotta

	case y < 114:
		if sublayer%5 == 0 {
			return palette.redSandstone
		}

		return palette.orangeTerracotta

	case y < 118:
		return palette.yellowTerracotta

	case y < 124:
		if sublayer%3 == 0 {
			return palette.redSandstone
		}

		return palette.redTerracotta

	case y < 128:
		return palette.orangeTerracotta

	case y < 132:
		if sublayer%4 == 0 {
			return palette.yellowTerracotta
		}

		return palette.redSandstone

	case y < 136:
		return palette.brownTerracotta

	case y < 142:
		if sublayer%5 == 0 {
			return palette.terracotta
		}

		return palette.redTerracotta

	case y < 148:
		return palette.brownTerracotta

	case y < 152:
		return palette.whiteTerracotta

	case y < 158:
		if sublayer%4 == 0 {
			return palette.smoothSandstone
		}

		return palette.sandstone

	case y < 162:
		return palette.whiteTerracotta

	case y < 166:
		if sublayer%3 == 0 {
			return palette.cutSandstone
		}

		return palette.sandstone

	case y < 172:
		if sublayer%4 == 0 {
			return palette.orangeTerracotta
		}

		return palette.yellowTerracotta

	case y < 178:
		if sublayer%5 == 0 {
			return palette.cutSandstone
		}

		return palette.sandstone

	case y < 184:
		if sublayer%4 == 0 {
			return palette.smoothSandstone
		}

		return palette.yellowTerracotta

	case y < 192:
		if sublayer%3 == 0 {
			return palette.cutSandstone
		}

		return palette.sandstone

	default:
		return palette.sandstone
	}
}

func mustBlock(name string) game.Block {
	block, ok := game.BlockByName(name)
	if !ok {
		panic("grand-canyon: missing generated block " + name)
	}

	return block
}
