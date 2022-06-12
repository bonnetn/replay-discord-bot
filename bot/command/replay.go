package command

import (
	"bigbro2/bot/circular"
	"bigbro2/bot/replayfile"
	"context"
	"fmt"
	"github.com/bwmarrin/discordgo"
	"go.uber.org/zap"
	"os"
	"time"
)

type Replay struct {
	logger      *zap.Logger
	creator     *replayfile.Creator
	session     *discordgo.Session
	audioBuffer *circular.Buffer
}

func NewReplay(logger *zap.Logger, creator *replayfile.Creator, session *discordgo.Session, audioBuffer *circular.Buffer) *Replay {
	return &Replay{
		logger:      logger,
		creator:     creator,
		session:     session,
		audioBuffer: audioBuffer,
	}
}

func (r *Replay) Run(ctx context.Context, duration time.Duration, i *discordgo.Interaction) error {
	var path string
	defer func() {
		if err := os.Remove(path); err != nil {
			r.logger.Warn("could not delete file", zap.Error(err))
		}

		r.logger.Debug("deleted file", zap.String("path", path))
	}()

	err := r.createTemporaryFile(&path)
	if err != nil {
		return err
	}

	err = r.creator.Create(ctx, r.audioBuffer, path, duration)
	if err == replayfile.NoAudioDataErr {
		_, err = r.session.InteractionResponseEdit(i, &discordgo.WebhookEdit{Content: "No audio data."})
		if err != nil {
			return fmt.Errorf("failed to send message: %w", err)
		}
		return nil
	}
	if err != nil {
		return err
	}

	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			r.logger.Warn("failed to close file", zap.Error(err))
		}
	}()

	_, err = r.session.InteractionResponseEdit(i, &discordgo.WebhookEdit{
		Content: fmt.Sprintf("Last %d seconds.", int(duration.Seconds())),
		Files: []*discordgo.File{{
			Name:        fmt.Sprintf("recording-%s.ogg", time.Now().Format(time.RFC3339)),
			ContentType: "audio/ogg; codecs=opus",
			Reader:      f,
		}},
	})
	if err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}

	return nil
}

func (r *Replay) createTemporaryFile(path *string) error {
	f, err := os.CreateTemp("", "*.opus")
	if err != nil {
		return fmt.Errorf("failed to create temporay file: %w", err)
	}

	defer func() {
		if err := f.Close(); err != nil {
			r.logger.Warn("failed to close temporary file", zap.Error(err))
		}
	}()

	*path = f.Name()
	return nil
}
