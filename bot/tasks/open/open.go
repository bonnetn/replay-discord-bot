package open

import (
	"bigbro2/bot/discord/handler"
	"bigbro2/bot/tasks/unopened"
	"fmt"
	"github.com/bwmarrin/discordgo"
	"go.uber.org/zap"
)

type Session struct{ unopened.Session }

func NewOpenSession(
	session unopened.Session,
	logger *zap.Logger,
	_ handler.VoiceStateUpdateHandler,
	_ handler.ReadyChannel,
) (Session, func(), error) {
	logger.Debug("opening discord session")
	session.Identify.Intents = discordgo.IntentGuilds | discordgo.IntentGuildMembers | discordgo.IntentGuildVoiceStates

	if err := session.Open(); err != nil {
		return Session{}, nil, fmt.Errorf("could not open discord session: %w", err)
	}

	cleanupFunc := func() {
		logger.Debug("closing discord session")
		if err := session.Close(); err != nil {
			logger.Error("could not close session", zap.Error(err))
		}
	}

	return Session{session}, cleanupFunc, nil
}
