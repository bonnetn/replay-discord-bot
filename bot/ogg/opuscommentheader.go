package ogg

import (
	"bytes"
	"fmt"
	"io"
)

type opusCommentHeader struct {
	VendorString []byte
}

func (h *opusCommentHeader) Encode(writer io.Writer) error {
	/*

	    0                   1                   2                   3
	    0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
	   +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	   |      'O'      |      'p'      |      'u'      |      's'      |
	   +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	   |      'T'      |      'a'      |      'g'      |      's'      |
	   +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	   |                     Vendor String Length                      |
	   +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	   |                                                               |
	   :                        Vendor String...                       :
	   |                                                               |
	   +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	   |                   User Comment List Length                    |
	   +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	   |                 User Comment #0 String Length                 |
	   +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	   |                                                               |
	   :                   User Comment #0 String...                   :
	   |                                                               |
	   +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	   |                 User Comment #1 String Length                 |
	   +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	*/
	w := errWriter{w: writer}

	w.write([]uint8{'O', 'p', 'u', 's', 'T', 'a', 'g', 's'}) // Magic signature.
	w.write(uint32(len(h.VendorString)))                     // Vendor string.
	w.write(h.VendorString)                                  // Vendor string length.
	w.write(uint32(0))                                       // User comment list

	if w.err != nil {
		return fmt.Errorf("failed to write opus comment header: %w", w.err)
	}
	return nil
}

func (h *opusCommentHeader) Bytes() []byte {
	var b bytes.Buffer
	if err := h.Encode(&b); err != nil {
		panic("failed to serialize opus comment header")
	}

	return b.Bytes()
}
