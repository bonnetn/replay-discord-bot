package bot

import (
	"bigbro2/bot/command"
	"bigbro2/bot/voicechannel"
	"context"
	"errors"
	"fmt"
	"github.com/bwmarrin/discordgo"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	"time"
)

const (
	defaultDuration = 30 * time.Second
	maxDuration     = time.Minute
)

type Bot struct {
	logger          *zap.Logger
	session         *discordgo.Session
	guildID         string
	replayCommandID *string // NOTE: This field will be set once the application command is registered.
	withManager     voicechannel.ManagerFactory
	replayCmd       *command.Replay
}

func NewBot(
	logger *zap.Logger,
	session *discordgo.Session,
	guildID string,
	withManager voicechannel.ManagerFactory,
	replayCmd *command.Replay,
) *Bot {
	return &Bot{
		session:     session,
		guildID:     guildID,
		logger:      logger,
		withManager: withManager,
		replayCmd:   replayCmd,
	}
}

func (b *Bot) Run(ctx context.Context) error {
	if b.session == nil {
		return errors.New("session is nil")
	}

	return b.withManager(ctx, func(manager *voicechannel.Manager) error {
		return b.withHandlersRegistered(ctx, manager, func(onReadyCh <-chan struct{}) error {
			return b.withOpenedSession(func() error {
				return b.withApplicationCommand(func() error {
					b.waitToBeReady(onReadyCh)

					g, ctx := errgroup.WithContext(ctx)

					g.Go(func() error { return b.joinVoiceChannel(manager) })

					g.Go(func() error {
						b.logger.Info("bot is running")
						<-ctx.Done()
						return nil
					})

					return g.Wait()
				})
			})
		})
	})
}

func (b *Bot) waitToBeReady(ch <-chan struct{}) {
	b.logger.Debug("waiting for discord client to be ready")
	<-ch
	b.logger.Info("discord client is ready")
}

func (b *Bot) withHandlersRegistered(ctx context.Context, manager *voicechannel.Manager, cb func(<-chan struct{}) error) error {
	onReadyCh := make(chan struct{})

	b.logger.Debug("registering handlers")
	removeInteractionUpdate := b.session.AddHandler(func(_ *discordgo.Session, i *discordgo.InteractionCreate) {
		err := b.handleInteractionCreate(ctx, i)
		if err != nil {
			b.logger.Error("could not handle interaction create", zap.Error(err))
		}
	})
	defer func() {
		b.logger.Debug("unregistering interaction update handler")
		removeInteractionUpdate()
	}()

	removeVoiceStateUpdate := b.session.AddHandler(func(_ *discordgo.Session, u *discordgo.VoiceStateUpdate) {
		err := b.joinVoiceChannel(manager)
		if err != nil {
			b.logger.Error("could not handle interaction create", zap.Error(err))
		}
	})
	defer func() {
		b.logger.Debug("unregistering voice state handler")
		removeVoiceStateUpdate()
	}()

	removeReady := b.session.AddHandler(func(_ *discordgo.Session, i *discordgo.Ready) {
		close(onReadyCh)
	})
	defer func() {
		b.logger.Debug("unregistering onReady update handler")
		removeReady()
	}()

	return cb(onReadyCh)
}

func (b *Bot) withOpenedSession(cb func() error) error {
	b.logger.Debug("opening discord session")
	b.session.Identify.Intents = discordgo.IntentGuilds | discordgo.IntentGuildMembers | discordgo.IntentGuildVoiceStates

	if err := b.session.Open(); err != nil {
		return fmt.Errorf("could not open discord session: %w", err)
	}
	defer func(session *discordgo.Session) {
		b.logger.Debug("closing discord session")
		err := session.Close()
		if err != nil {
			b.logger.Error("could not close discord session", zap.Error(err))
		}
	}(b.session)

	return cb()
}

func (b *Bot) withApplicationCommand(cb func() error) error {
	if b.session == nil {
		return errors.New("nil session")
	}
	if b.session.State == nil {
		return errors.New("nil state")
	}
	if b.session.State.User == nil {
		return errors.New("nil user")
	}
	userID := b.session.State.User.ID

	b.logger.Debug("creating discord application command")
	minValue := float64(1)
	cmd, err := b.session.ApplicationCommandCreate(userID, b.guildID, &discordgo.ApplicationCommand{
		Name:        "replay",
		Description: "Save the last minute",
		Options: []*discordgo.ApplicationCommandOption{{
			Type:        discordgo.ApplicationCommandOptionInteger,
			Name:        "seconds",
			Description: "number of seconds to capture",
			MinValue:    &minValue,
			MaxValue:    maxDuration.Seconds(),
		}},
	})

	if err != nil {
		return fmt.Errorf("could not register application command: %w", err)
	}
	defer func() {
		b.logger.Debug("deleting discord application command")
		err := b.session.ApplicationCommandDelete(userID, b.guildID, cmd.ID)
		if err != nil {
			b.logger.Debug("could not unregister application command", zap.Error(err))
		}
	}()

	b.replayCommandID = &cmd.ID
	return cb()
}

func (b *Bot) joinVoiceChannel(m *voicechannel.Manager) error {
	b.logger.Debug("finding channel with most members")
	chanID, err := b.findChannelToJoin()
	if err != nil {
		return fmt.Errorf("could not get the channel with most members: %w", err)
	}

	m.JoinChannel(chanID)
	return nil
}

// findChannelToJoin returns the channel that the bot should join.
func (b *Bot) findChannelToJoin() (*string, error) {
	guild, err := b.session.State.Guild(b.guildID)
	if err != nil {
		return nil, fmt.Errorf("could not fetch guild: %w", err)
	}

	channelMembers := map[string]int{}
	for _, vs := range guild.VoiceStates {
		if vs.SelfMute || vs.SelfDeaf {
			// We do not account for people on mute, we want to join the channel with the most people that can speak.
			continue
		}
		n, _ := channelMembers[vs.ChannelID]
		channelMembers[vs.ChannelID] = n + 1
	}

	var result *string
	var maxCount int
	for channelID, memberCount := range channelMembers {
		if memberCount > maxCount {
			cID := channelID // Copy because channelID is an iterator.
			result = &cID
			maxCount = memberCount
		}
	}
	return result, nil
}
func (b *Bot) handleInteractionCreate(ctx context.Context, i *discordgo.InteractionCreate) error {
	if i.GuildID != b.guildID {
		return nil
	}

	data, ok := i.Data.(discordgo.ApplicationCommandInteractionData)
	if !ok {
		return fmt.Errorf("wrong interaction data type: %T", i.Data)
	}

	if b.replayCommandID != nil && data.ID != *b.replayCommandID {
		return fmt.Errorf("unknown interaction: %q", data.Name)
	}

	duration := defaultDuration
	if len(data.Options) == 1 {
		opt := data.Options[0]
		v, ok := opt.Value.(float64)
		if !ok {
			return errors.New("unexpected type for value")
		}

		duration = time.Duration(1e9 * int64(v))
		if duration > maxDuration {
			duration = maxDuration
		}
	}

	err := b.session.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Content: "✅"},
	})
	if err != nil {
		return fmt.Errorf("could not respond to interaction: %w", err)
	}

	b.logger.Debug("creating replay", zap.Duration("duration", duration))
	return b.replayCmd.Run(ctx, duration)
}
