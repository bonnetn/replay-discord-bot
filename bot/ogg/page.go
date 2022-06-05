package ogg

import (
	"bytes"
	"fmt"
	"io"
)

const (
	maxSegmentLength = 255 // bytes
	continuedFlag    = 1 << 0
	firstPageFlag    = 1 << 1
	lastPageFlag     = 1 << 2
)

// From: https://github.com/pion/webrtc/blob/67826b19141ec9e6f1002a2267008a016a118934/pkg/media/oggwriter/oggwriter.go#L245-L261
func crcChecksum() *[256]uint32 {
	var table [256]uint32
	const poly = 0x04c11db7

	for i := range table {
		r := uint32(i) << 24
		for j := 0; j < 8; j++ {
			if (r & 0x80000000) != 0 {
				r = (r << 1) ^ poly
			} else {
				r <<= 1
			}
			table[i] = r & 0xffffffff
		}
	}
	return &table
}

var crcTable = crcChecksum()

// page represents an OGG page.
type page struct {
	Header   pageHeader
	Segments []byte
}

// pageHeader represents the header data of a page.
type pageHeader struct {
	Continued             bool
	FirstPage             bool
	LastPage              bool
	GranulePosition       int64
	BitstreamSerialNumber uint32
	PageSequenceNumber    uint32
	SegmentTable          []uint8
}

// EncodeWithCRC encodes the pageHeader with a given CRC.
func (h *pageHeader) EncodeWithCRC(writer io.Writer, crc uint32) error {
	/*
		 0                   1                   2                   3
		 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1| Byte
		+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
		| capture_pattern: Magic number for page start "OggS"           | 0-3
		+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
		| version       | header_type   | granule_position              | 4-7
		+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
		|                                                               | 8-11
		+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
		|                               | bitstream_serial_number       | 12-15
		+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
		|                               | page_sequence_number          | 16-19
		+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
		|                               | CRC_checksum                  | 20-23
		+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
		|                               |page_segments  | segment_table | 24-27
		+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
		| ...                                                           | 28-
		+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	*/

	var headerType uint8
	if h.Continued {
		headerType |= continuedFlag
	}
	if h.FirstPage {
		headerType |= firstPageFlag
	}
	if h.LastPage {
		headerType |= lastPageFlag
	}

	w := errWriter{w: writer}

	w.write([4]uint8{'O', 'g', 'g', 'S'}) // Capture pattern.
	w.write(uint8(0))                     // Version
	w.write(headerType)
	w.write(h.GranulePosition)
	w.write(h.BitstreamSerialNumber)
	w.write(h.PageSequenceNumber)
	w.write(crc)
	w.write(uint8(len(h.SegmentTable)))
	for _, segment := range h.SegmentTable {
		w.write(segment)
	}

	if w.err != nil {
		return fmt.Errorf("could not encode page header: %w", w.err)
	}
	return nil
}

// Encode encodes the OGG page.
func (p *page) Encode(w io.Writer) error {
	var buf bytes.Buffer
	if err := p.EncodeWithCRC(&buf, 0); err != nil {
		return fmt.Errorf("failed to encode page for CRC: %w", err)
	}

	var checksum uint32
	for _, b := range buf.Bytes() {
		checksum = (checksum << 8) ^ crcTable[byte(checksum>>24)^b]
	}

	return p.EncodeWithCRC(w, checksum)
}

// EncodeWithCRC encodes the OGG page with a given pageHeader.
func (p *page) EncodeWithCRC(w io.Writer, crc uint32) error {
	if err := p.Header.EncodeWithCRC(w, crc); err != nil {
		return err
	}

	if _, err := w.Write(p.Segments); err != nil {
		return fmt.Errorf("failed to write segments: %w", err)
	}

	return nil
}

// AddSegment add a segment to the page.
// This function panics if the segment is more than 255 bytes long of if the page is full.
func (p *page) AddSegment(segment []byte) {
	n := len(segment)
	if n > maxSegmentLength {
		panic("segment length is greater than max length")
	}

	if len(p.Header.SegmentTable) == 255 {
		panic("page is full")
	}

	p.Segments = append(p.Segments, segment...)
	p.Header.SegmentTable = append(p.Header.SegmentTable, uint8(n))
}
