package crypto

import (
	"crypto/cipher"
)

type CFB8 struct {
	block    cipher.Block
	iv       []byte
	outBlock []byte
	decrypt  bool
}

func NewCFB8(block cipher.Block, iv []byte, decrypt bool) cipher.Stream {
	return &CFB8{
		block:    block,
		iv:       append([]byte{}, iv...),
		outBlock: make([]byte, block.BlockSize()),
		decrypt:  decrypt,
	}
}

func (x *CFB8) XORKeyStream(dst, src []byte) {
	for i := range src {
		x.block.Encrypt(x.outBlock, x.iv)

		c := src[i] ^ x.outBlock[0]

		dst[i] = c

		if x.decrypt {
			copy(x.iv, x.iv[1:])

			x.iv[len(x.iv)-1] = src[i]
		} else {
			copy(x.iv, x.iv[1:])

			x.iv[len(x.iv)-1] = c
		}
	}
}
