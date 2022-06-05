package replayfile

import (
	"bigbro2/bot/circular"
	"bigbro2/bot/ogg"
	"context"
	"errors"
	"fmt"
	"go.uber.org/zap"
	"os"
	"os/exec"
	"time"
)

const (
	FrameLengthNs = 20 * 1e6
	SampleRate    = 48_000
	FrameSize     = FrameLengthNs * SampleRate / 1e9
)

var (
	silentFrame    = []byte{0xF8, 0xFF, 0xFE}
	NoAudioDataErr = errors.New("no audio data")
)

type Creator struct {
	logger *zap.Logger
	now    func() time.Time
}

func NewCreator(logger *zap.Logger, now func() time.Time) *Creator {
	return &Creator{
		logger: logger,
		now:    now,
	}
}

// Create creates a new Opus file containing the packets from the audio buffer.
// It creates N temporary opus files (one for each voice stream) and mixes them together using ffmpeg.
func (c *Creator) Create(ctx context.Context, audioBuffer *circular.Buffer, path string, recordingDuration time.Duration) error {
	return audioBuffer.WithIterator(func(iterator *circular.Iterator) error {
		return c.create(ctx, iterator, path, recordingDuration)
	})
}

func (c *Creator) create(ctx context.Context, iterator *circular.Iterator, path string, recordingDuration time.Duration) error {
	var files []string
	defer func() {
		for _, fileName := range files {
			if err := os.Remove(fileName); err != nil {
				c.logger.Warn("failed to remove file", zap.Error(err))
			}
			c.logger.Debug("removed file", zap.String("path", fileName))
		}
	}()

	err := c.createStreamFiles(iterator, &files, recordingDuration)
	if err != nil {
		return fmt.Errorf("failed to create temporary stream files: %w", err)
	}

	if len(files) == 0 {
		return NoAudioDataErr
	}

	// Now that we have N files, we need to mix them all into one single file.
	if err := c.mixFiles(ctx, path, files); err != nil {
		return fmt.Errorf("failed to mix files together: %w", err)
	}

	return nil
}

// createStreamFiles
// Takes a pointer to slice as argument to make sure we always delete them with defer.
func (c *Creator) createStreamFiles(iterator *circular.Iterator, files *[]string, recordingDuration time.Duration) error {
	streams := map[uint32]*streamState{}

	var streamStartTime *time.Time
	for iterator.HasNext() {
		pkt := iterator.Next()
		// Discard packets that too old.
		if c.now().Sub(pkt.Time) >= recordingDuration {
			continue
		}

		// This is the first packet we process, since the packets are ordered we can extract the time the replay
		//starts.
		if streamStartTime == nil {
			c.logger.Debug("stream start time", zap.Time("time", pkt.Time))
			streamStartTime = &pkt.Time
		}

		ssrc := pkt.SSRC

		// We haven't encountered this voice stream before, we need to create a new file & encoder for it.
		if _, ok := streams[ssrc]; !ok {
			f, err := os.CreateTemp("", "*.opus")
			if err != nil {
				return fmt.Errorf("failed to create temporary file: %w", err)
			}
			defer func(f *os.File) {
				if err := f.Close(); err != nil {
					c.logger.Warn("failed to close file", zap.Error(err))
				}
			}(f)

			c.logger.Debug("created new file for stream",
				zap.Uint32("ssrc", ssrc),
				zap.String("path", f.Name()),
			)

			// Create an encoder for this particular file.
			encoder, err := ogg.NewEncoder(c.logger, f)
			if err != nil {
				return fmt.Errorf("failed to create ogg encoder: %w", err)
			}

			// Since the voice stream don't all start at the same time, we need to pad the beginning of the stream
			// with silent data so the voices are synchronized.
			// We pretend the last packet was at the beginning of the stream so it pads it correctly.
			timeRelativeStartStream := pkt.Time.Sub(*streamStartTime)
			pcmSamplesToPad := timeRelativeStartStream.Nanoseconds() * SampleRate / 1e9

			streams[ssrc] = &streamState{
				encoder:      encoder,
				lastPCMIndex: int64(pkt.PCMIndex) - pcmSamplesToPad,
			}
			*files = append(*files, f.Name())
		}

		stream := streams[ssrc]

		// OGG file readers by default skip time discontinuities.
		// We compute the difference between the *start* of the *current* frame and the *end* of the previous frame.
		// This will give us the number of silent packets we need to insert.
		pcmSamplesToPad := int64(pkt.PCMIndex) - (stream.lastPCMIndex + FrameSize)
		packetsToPad := pcmSamplesToPad / FrameSize
		for i := int64(0); i < packetsToPad; i++ {
			if err := stream.encoder.Encode(silentFrame, stream.lastPCMIndex+(i+1)*FrameSize); err != nil {
				return fmt.Errorf("failed to encode silent padding frame: %w", err)
			}
		}

		// Now we can encode the actual opus data.
		if err := stream.encoder.Encode(pkt.Opus, int64(pkt.PCMIndex)); err != nil {
			return fmt.Errorf("failed to encode opus data: %w", err)
		}

		streams[ssrc].lastPCMIndex = int64(pkt.PCMIndex)
	}
	return nil
}

func (c *Creator) mixFiles(ctx context.Context, path string, files []string) error {
	var args []string
	args = append(args, "-y") // Overwrite output file.

	// Input files.
	for _, fileName := range files {
		args = append(args, "-i", fileName)
	}

	// Mix files together.
	args = append(args, "-filter_complex", fmt.Sprintf("amix=inputs=%d:duration=longest", len(files)))

	// Output path.
	args = append(args, path)
	if err := exec.CommandContext(ctx, "ffmpeg", args...).Run(); err != nil {
		return fmt.Errorf("ffmpeg errored: %w", err)
	}
	return nil
}

type streamState struct {
	encoder      *ogg.Encoder
	lastPCMIndex int64
}
