package main

import (
	"bigbro2/bot"
	"bigbro2/bot/circular"
	"bigbro2/bot/command"
	"bigbro2/bot/replayfile"
	"bigbro2/bot/voicechannel"
	"context"
	"errors"
	"fmt"
	"github.com/bwmarrin/discordgo"
	"go.uber.org/zap"
	"os"
	"os/signal"
	"time"
)

const (
	DiscordToken   = "DISCORD_TOKEN"
	DiscordGuildId = "DISCORD_GUILD_ID"
	DiscordChannel = "DISCORD_CHANNEL"
	Development    = "DEVELOPMENT"
)

// Required permissions:
// * bot
// * applications.commands
// * Send Messages
// * Attach Files
// * Connect

func run() error {
	token, err := getEnvVar(DiscordToken)
	if err != nil {
		return err
	}

	guildID, err := getEnvVar(DiscordGuildId)
	if err != nil {
		return err
	}

	channelID, err := getEnvVar(DiscordChannel)
	if err != nil {
		return err
	}

	dev := false
	devStr := os.Getenv(Development)
	if devStr == "true" {
		dev = true
	}

	var loggerFunc func() (*zap.Logger, error)
	if dev {
		loggerFunc = func() (*zap.Logger, error) {
			return zap.NewDevelopment()
		}
	} else {
		loggerFunc = func() (*zap.Logger, error) {
			return zap.NewProduction()
		}
	}
	logger, err := loggerFunc()
	if err != nil {
		return fmt.Errorf("could not create logger: %w", err)
	}

	session, err := discordgo.New("Bot " + token)
	if err != nil {
		return fmt.Errorf("could not instantiate discord client: %w", err)
	}

	var (
		audioBuffer    = circular.Buffer{}
		replayCreator  = replayfile.NewCreator(logger, time.Now)
		replayCmd      = command.NewReplay(logger, replayCreator, session, &audioBuffer, channelID)
		managerFactory = voicechannel.NewManagerFactory(logger, guildID, session, &audioBuffer)
		botInstance    = bot.NewBot(logger, session, guildID, managerFactory, replayCmd)
	)

	ctx := context.Background()
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt)
	defer stop()

	for {
		err = runBot(ctx, botInstance, logger)
		if err != nil && err != panicError {
			return err
		}
	}
}

var panicError = errors.New("panic")

func runBot(ctx context.Context, botInstance *bot.Bot, logger *zap.Logger) (err error) {
	defer func() {
		if r := recover(); r != nil {
			logger.DPanic("panicked", zap.Any("reason", r))
			err = panicError
		}
	}()
	return botInstance.Run(ctx)
}

func main() {
	err := run()

	var userError UserError
	switch {
	case err == nil:
		os.Exit(0)

	case errors.Is(err, context.Canceled):
		os.Exit(0)

	case errors.As(err, &userError):
		fmt.Fprintf(os.Stderr, "error: %s\n", userError.Error())
		os.Exit(1)

	default:
		fmt.Fprintf(os.Stderr, "unexpected error: %s\n", err.Error())
		os.Exit(2)
	}
}

func getEnvVar(key string) (string, error) {
	envVar := os.Getenv(key)
	if envVar == "" {
		return "", UserError{fmt.Sprintf("environment variable %q is not set", key)}
	}
	return envVar, nil
}

type UserError struct{ Reason string }

func (e UserError) Error() string { return e.Reason }
