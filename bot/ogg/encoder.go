package ogg

import (
	"fmt"
	"go.uber.org/zap"
	"io"
)

const (
	ChannelCount   = 2      // (from discord).
	PreSkip        = 3840   // Value recommended by the RFC.
	SamplingRateHz = 48_000 // 48kHz (from discord).
	Gain           = 0      // 0dB
	MappingFamily  = 0
)

// Encoder allows writing OGG files from opus data received from Discord.
// Very little conversion is needed as OGG file support Opus encoded data.
type Encoder struct {
	logger    *zap.Logger
	bitstream bitstreamEncoder
}

func NewEncoder(logger *zap.Logger, writer io.Writer) (*Encoder, error) {
	enc := &Encoder{
		logger:    logger,
		bitstream: newBitstreamEncoder(writer),
	}

	idHeader := opusIdentificationHeader{
		ChannelCount:    ChannelCount,
		PreSkip:         PreSkip,
		InputSampleRate: SamplingRateHz,
		OutputGain:      Gain,
		MappingFamily:   MappingFamily,
	}
	// TODO: We could get rid of the intermediate encoding set .Bytes() and directly encode into the writer.
	if err := enc.bitstream.Encode(idHeader.Bytes(), 0); err != nil {
		return nil, fmt.Errorf("could not write the opus header page: %w", err)
	}

	commentHeader := opusCommentHeader{
		VendorString: []byte("discord-replay"),
	}
	if err := enc.bitstream.Encode(commentHeader.Bytes(), 0); err != nil {
		return nil, fmt.Errorf("could not write the opus comment page: %w", err)
	}

	return enc, nil
}

func (e *Encoder) Encode(opusData []byte, pcmSampleIndex int64) error {
	if err := e.bitstream.Encode(opusData, pcmSampleIndex); err != nil {
		return fmt.Errorf("failed to write packet to bitstream: %w", err)
	}
	return nil
}
