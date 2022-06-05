package ogg

import (
	"fmt"
	"io"
)

const BitstreamSerialNumber = 1

// bitstreamEncoder encodes a physical OGG bitstream.
// It it NOT safe for concurrent use.
// Note: The implementation is simplified for the purpose of this discord bot:
// - This only encodes ONE logical bitstream.
// - Every packet has its own page.
type bitstreamEncoder struct {
	writer         io.Writer
	firstPage      bool
	sequenceNumber uint32
}

func newBitstreamEncoder(writer io.Writer) bitstreamEncoder {
	return bitstreamEncoder{
		writer:         writer,
		firstPage:      true,
		sequenceNumber: 1,
	}
}

// Encode adds a packet to the bitstream in a new page.
// It is sub-optimal (as we could have several packets in 1 page), but it is easier to implementat.
func (s *bitstreamEncoder) Encode(packetData []byte, granulePosition int64) error {
	page := page{
		Header: pageHeader{
			Continued: false, // Will never be continued, as we follow the convention 1 packet <=> 1 page.

			FirstPage: s.firstPage,

			// NOTE: We never set this flag to true.
			// According to the RFC: "implementations need to be prepared to deal with truncated streams that do not
			// have a page marked 'end of stream'.".
			// For simplicity, I decided not to set it.
			LastPage: false,

			GranulePosition:       granulePosition,
			BitstreamSerialNumber: BitstreamSerialNumber,
			PageSequenceNumber:    s.sequenceNumber,
			SegmentTable:          nil,
		},
		Segments: nil,
	}

	for i := 0; i < len(packetData); i += maxSegmentLength {
		end := i + maxSegmentLength
		if end > len(packetData) {
			end = len(packetData)
		}
		page.AddSegment(packetData[i:end])
	}

	if len(packetData)%maxSegmentLength == 0 {
		// It means we wrote (len(packetData) / 255) segments, completely full.
		// According to the RFC, we need to insert a 0-length segment to signal the end of a packet.
		page.AddSegment(nil)
	}

	if err := page.Encode(s.writer); err != nil {
		return fmt.Errorf("failed to encode page: %w", err)
	}

	s.sequenceNumber++
	s.firstPage = false
	return nil
}
