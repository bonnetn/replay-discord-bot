package ogg

import (
	"bytes"
	"fmt"
	"io"
)

type opusIdentificationHeader struct {
	ChannelCount    uint8
	PreSkip         uint16
	InputSampleRate uint32
	OutputGain      uint16
	MappingFamily   byte
}

func (h *opusIdentificationHeader) Encode(writer io.Writer) error {
	/*
	    0                   1                   2                   3
	    0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
	   +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	   |      'O'      |      'p'      |      'u'      |      's'      |
	   +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	   |      'H'      |      'e'      |      'a'      |      'd'      |
	   +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	   |  Version = 1  | Channel Count |           Pre-skip            |
	   +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	   |                     Input Sample Rate (Hz)                    |
	   +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	   |   Output Gain (Q7.8 in dB)    | Mapping Family|               |
	   +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+               :
	   |                                                               |
	   :               Optional Channel Mapping Table...               :
	   |                                                               |
	   +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	*/
	w := errWriter{w: writer}

	w.write([8]uint8{'O', 'p', 'u', 's', 'H', 'e', 'a', 'd'}) // Magic signature.
	w.write(uint8(1))                                         // Version.
	w.write(h.ChannelCount)                                   // Channel count.
	w.write(h.PreSkip)                                        // Pre skip.
	w.write(h.InputSampleRate)                                // Input sample rate.
	w.write(h.OutputGain)                                     // Output gain.
	w.write(uint8(0))                                         // Channel mapping family.

	if w.err != nil {
		return fmt.Errorf("failed to write opus identification header: %w", w.err)
	}
	return nil
}

func (h *opusIdentificationHeader) Bytes() []byte {
	var b bytes.Buffer
	if err := h.Encode(&b); err != nil {
		panic("failed to serialize opus identification header")
	}

	return b.Bytes()
}
