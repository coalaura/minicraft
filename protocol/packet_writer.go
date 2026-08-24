package protocol

import "bytes"

type PacketWriter struct {
	bytes.Buffer
	err error
}

func (w *PacketWriter) VarInt(value int32) {
	if w.err != nil {
		return
	}

	w.err = WriteVarInt(&w.Buffer, value)
}

func (w *PacketWriter) Int(value int32) {
	if w.err != nil {
		return
	}

	w.err = WriteInt(&w.Buffer, value)
}

func (w *PacketWriter) Long(value int64) {
	if w.err != nil {
		return
	}

	w.err = WriteLong(&w.Buffer, value)
}

func (w *PacketWriter) Double(value float64) {
	if w.err != nil {
		return
	}

	w.err = WriteDouble(&w.Buffer, value)
}

func (w *PacketWriter) Float(value float32) {
	if w.err != nil {
		return
	}

	w.err = WriteFloat(&w.Buffer, value)
}

func (w *PacketWriter) Bool(value bool) {
	if w.err != nil {
		return
	}

	w.err = WriteBool(&w.Buffer, value)
}

func (w *PacketWriter) String(value string) {
	if w.err != nil {
		return
	}

	w.err = WriteString(&w.Buffer, value)
}

func (w *PacketWriter) Byte(value byte) {
	if w.err != nil {
		return
	}

	w.err = w.WriteByte(value)
}

func (w *PacketWriter) Err() error {
	return w.err
}
