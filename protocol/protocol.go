package protocol

import "github.com/coalaura/plain"

var log *plain.Plain

func SetLogger(l *plain.Plain) {
	log = l
}
