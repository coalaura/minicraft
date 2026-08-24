package protocol

type StatusResponse struct {
	JSON string
}

type StatusPong struct {
	Payload int64
}

func (p StatusResponse) Encode(wr *PacketWriter) {
	wr.String(p.JSON)
}

func (p StatusPong) Encode(wr *PacketWriter) {
	wr.Long(p.Payload)
}
