package ready

import (
	"bigbro2/bot/command/replay"
	"bigbro2/bot/discord/handler"
	"bigbro2/bot/tasks/unopened"
	"github.com/bwmarrin/discordgo"
	"go.uber.org/zap"
)

type Session struct{ *discordgo.Session }

func NewReadySession(
	session unopened.Session,
	ch handler.ReadyChannel,
	logger *zap.Logger,
	_ replay.InteractionCreateHandler,
) Session {
	logger.Debug("waiting for discord client to be ready")
	<-ch
	logger.Info("discord client is ready")
	return Session{session.Session}
}
