package ogg

import (
	"encoding/binary"
	"io"
)

type errWriter struct {
	w   io.Writer
	err error
}

func (ew *errWriter) write(v any) {
	if ew.err != nil {
		return
	}
	ew.err = binary.Write(ew.w, binary.LittleEndian, v)
}
