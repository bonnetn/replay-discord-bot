package bot

import (
	"bigbro2/bot/cleanup"
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

type (
	Bot struct {
		logger                    *zap.Logger
		session                   *discordgo.Session
		guildID                   string
		createVoiceChannelManager voicechannel.CreateManager
		replayCmd                 *command.Replay
	}
	readyChannel              = <-chan struct{}
	interactionCreateCallback = func(ctx context.Context, i *discordgo.InteractionCreate) error
)

func NewBot(
	logger *zap.Logger,
	session *discordgo.Session,
	guildID string,
	withManager voicechannel.CreateManager,
	replayCmd *command.Replay,
) *Bot {
	return &Bot{
		session:                   session,
		guildID:                   guildID,
		logger:                    logger,
		createVoiceChannelManager: withManager,
		replayCmd:                 replayCmd,
	}
}

func (b *Bot) Run(ctx context.Context) error {
	manager, cleanupManager, err := b.createVoiceChannelManager(ctx)
	if err != nil {
		return fmt.Errorf("failed to create voice connection manager: %w", err)
	}
	defer b.cleanup("voice channel manager", cleanupManager)

	onReadyChan, cleanupOnReadyHandler := b.registerOnReadyHandler()
	defer b.cleanup("onReady handler", cleanupOnReadyHandler)

	cleanupVoiceStateUpdateHandler := b.registerVoiceStateUpdateHandler(manager)
	defer b.cleanup("handler", cleanupVoiceStateUpdateHandler)

	cleanupSession, err := b.openDiscordSession()
	if err != nil {
		return fmt.Errorf("failed to open session: %w", err)
	}
	defer b.cleanup("discord session", cleanupSession)

	replayCommandID, cleanupApplicationCommand, err := b.createReplayCommand()
	if err != nil {
		return err
	}
	defer b.cleanup("application command", cleanupApplicationCommand)

	cleanupReplayCommandHandler := b.registerInteractionCreateHandler(ctx, func(ctx context.Context, i *discordgo.InteractionCreate) error {
		if i.ID != replayCommandID {
			return nil
		}
		return b.handleReplayCommand(ctx, manager, i)
	})
	defer b.cleanup("replay command handler", cleanupReplayCommandHandler)

	b.waitToBeReady(onReadyChan)

	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error { return b.joinVoiceChannel(manager) })
	g.Go(func() error {
		b.logger.Info("bot is running")
		<-ctx.Done()
		return nil
	})

	return g.Wait()
}

func (b *Bot) registerOnReadyHandler() (readyChannel, cleanup.Func) {
	onReadyCh := make(chan struct{})

	b.logger.Debug("registering on ready handler")
	removeReady := b.session.AddHandler(func(_ *discordgo.Session, i *discordgo.Ready) {
		close(onReadyCh)
	})
	cleanupFunc := func() error {
		b.logger.Debug("unregistering onReady update handler")
		removeReady()
		return nil
	}

	return onReadyCh, cleanupFunc
}

func (b *Bot) registerInteractionCreateHandler(ctx context.Context, cb interactionCreateCallback) cleanup.Func {
	b.logger.Debug("registering interaction create handler")
	removeInteractionUpdate := b.session.AddHandler(func(_ *discordgo.Session, i *discordgo.InteractionCreate) {
		err := cb(ctx, i)
		if err != nil {
			b.logger.Error("could not handle interaction create", zap.Error(err))
		}
	})
	cleanupFunc := func() error {
		b.logger.Debug("unregistering interaction update handler")
		removeInteractionUpdate()
		return nil
	}
	return cleanupFunc
}

func (b *Bot) registerVoiceStateUpdateHandler(manager *voicechannel.Manager) cleanup.Func {
	b.logger.Debug("registering voice state update handler")
	removeVoiceStateUpdate := b.session.AddHandler(func(_ *discordgo.Session, u *discordgo.VoiceStateUpdate) {
		err := b.joinVoiceChannel(manager)
		if err != nil {
			b.logger.Error("could not handle voice state update", zap.Error(err))
		}
	})
	cleanupFunc := func() error {
		b.logger.Debug("unregistering voice state handler")
		removeVoiceStateUpdate()
		return nil
	}

	return cleanupFunc
}

func (b *Bot) openDiscordSession() (cleanup.Func, error) {
	b.logger.Debug("opening discord session")
	b.session.Identify.Intents = discordgo.IntentGuilds | discordgo.IntentGuildMembers | discordgo.IntentGuildVoiceStates

	if err := b.session.Open(); err != nil {
		return nil, fmt.Errorf("could not open discord session: %w", err)
	}

	cleanupFunc := func() error {
		b.logger.Debug("closing discord session")
		if err := b.session.Close(); err != nil {
			return fmt.Errorf("could not close discord session: %w", err)
		}
		return nil
	}

	return cleanupFunc, nil
}

func (b *Bot) waitToBeReady(ch <-chan struct{}) {
	b.logger.Debug("waiting for discord client to be ready")
	<-ch
	b.logger.Info("discord client is ready")
}

func (b *Bot) createReplayCommand() (string, cleanup.Func, error) {
	if b.session == nil {
		return "", nil, errors.New("nil session")
	}
	if b.session.State == nil {
		return "", nil, errors.New("nil state")
	}
	if b.session.State.User == nil {
		return "", nil, errors.New("nil user")
	}
	userID := b.session.State.User.ID

	b.logger.Debug("creating discord application command")
	minValue := float64(2)
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
		return "", nil, fmt.Errorf("could not register application command: %w", err)
	}
	cleanupFunc := func() error {
		b.logger.Debug("deleting application command", zap.String("id", cmd.ApplicationID))
		err := b.session.ApplicationCommandDelete(userID, b.guildID, cmd.ID)
		if err != nil {
			b.logger.Debug("could not unregister application command", zap.Error(err))
			return err
		}
		return nil
	}

	return cmd.ID, cleanupFunc, nil
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

func (b *Bot) isInVoiceChannel(voiceChannelID, userID string) (bool, error) {
	guild, err := b.session.State.Guild(b.guildID)
	if err != nil {
		return false, fmt.Errorf("could not fetch guild: %w", err)
	}

	for _, vs := range guild.VoiceStates {
		if vs.ChannelID == voiceChannelID && vs.UserID == userID {
			return true, nil
		}
	}
	return false, nil
}

func (b *Bot) handleReplayCommand(ctx context.Context, manager *voicechannel.Manager, i *discordgo.InteractionCreate) error {
	logger := b.logger.With(
		zap.String("interaction_id", i.ID),
		zap.Uint8("interaction_type", uint8(i.Type)),
		zap.String("guild_id", i.GuildID),
		zap.String("channel_id", i.ChannelID),
	)

	logger.Debug("received interaction create")
	if i.GuildID != b.guildID {
		logger.Debug("interaction from wrong guild discarded")
		return nil
	}

	data, ok := i.Data.(discordgo.ApplicationCommandInteractionData)
	if !ok {
		return fmt.Errorf("wrong interaction data type: %T", i.Data)
	}

	logger = logger.With(
		zap.String("interaction_data_id", data.ID),
		zap.String("interaction_data_name", data.Name),
	)

	// A user should not be able to ask for a replay if they are not in the channel.
	// NOTE: There is a race condition: the channel may change while we are checking if the user is in it.
	// But this is fine as the audio buffer is cleaned every time the channel is changed so the user may use this to
	// record other channels.
	currentChannel := manager.CurrentChannelID()
	if currentChannel == nil {
		logger.Info("rejecting request as bot is not connected to the voice channel")
		return b.session.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "❌ Bot is not connected to any voice channel."},
		})
	}

	member := i.Member
	if member == nil {
		logger.Info("rejecting request as it is not a guild message")
		return b.session.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "❌ Can only be invoked in a server."},
		})
	}
	logger = logger.With(zap.String("member_nick", member.Nick))

	user := member.User
	if user == nil {
		return errors.New("user is nil")
	}

	logger = logger.With(
		zap.String("user_id", user.ID),
		zap.String("user_username", user.Username),
		zap.String("user_discriminator", user.Discriminator),
		zap.Bool("user_bot", user.Bot),
	)
	if user.Bot {
		logger.Info("discarding request as it was made by a bot")
		return nil
	}

	inVoiceChannel, err := b.isInVoiceChannel(*currentChannel, user.ID)
	if err != nil {
		return fmt.Errorf("could not check if bot is in voice channel of the user: %w", err)
	}

	if !inVoiceChannel {
		logger.Info("rejecting request as the user is not in same the voice channel as the bot")
		return b.session.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "❌ You are not in the voice channel."},
		})
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
	logger = logger.With(zap.Duration("duration", duration))

	err = b.session.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
	if err != nil {
		return fmt.Errorf("could not respond to interaction: %w", err)
	}

	err = b.replayCmd.Run(ctx, duration, i.Interaction)
	if err != nil {
		return fmt.Errorf("could not create replay: %w", err)
	}

	logger.Info("created replay")
	return nil
}

// cleanup is a helper function to clean up resource and log failures.
func (b *Bot) cleanup(name string, f cleanup.Func) {
	err := f()
	if err != nil {
		b.logger.Warn(fmt.Sprintf("failed to cleanup %s", name), zap.Error(err))
	}
}
