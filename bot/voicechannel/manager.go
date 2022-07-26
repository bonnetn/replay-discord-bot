package voicechannel

import (
	"bigbro2/bot/circular"
	"bigbro2/bot/cleanup"
	"context"
	"fmt"
	"github.com/bwmarrin/discordgo"
	"go.uber.org/zap"
	"sync"
	"time"
)

type Manager struct {
	sync.RWMutex
	logger             *zap.Logger
	guildID            string
	session            *discordgo.Session
	audioBuffer        *circular.Buffer
	voiceChannelToJoin chan *string
	stopListenersCh    chan struct{}
}

type CreateManager = func(context.Context) (*Manager, cleanup.Func, error)

func NewManagerFactory(logger *zap.Logger, guildID string, session *discordgo.Session, audioBuffer *circular.Buffer) CreateManager {
	return func(ctx context.Context) (*Manager, cleanup.Func, error) {
		m := &Manager{
			logger:             logger,
			guildID:            guildID,
			session:            session,
			audioBuffer:        audioBuffer,
			voiceChannelToJoin: make(chan *string),
		}

		doneCh := make(chan struct{})

		go func() {
			err := m.run(doneCh)
			if err != nil {
				logger.Panic("voice channel manager failed", zap.Error(err))
			}
		}()

		cleanupFunc := func() error {
			close(doneCh)
			return nil
		}

		return m, cleanupFunc, nil
	}
}

func (m *Manager) JoinChannel(channelID *string) {
	m.logger.Debug("asking to join channel", zap.Stringp("channel", channelID))
	m.voiceChannelToJoin <- channelID
}

func (m *Manager) run(doneCh <-chan struct{}) error {
	defer m.cleanupVoiceChannel()

	for {
		select {
		case <-doneCh:
			return nil

		case channelID := <-m.voiceChannelToJoin:
			err := m.handleJoinRequest(channelID)
			if err != nil {
				m.logger.Error(
					"failed to handle join request",
					zap.Stringp("channel", channelID),
					zap.Error(err),
				)
				return err
			}

		}
	}
}
func (m *Manager) CurrentChannel() *discordgo.VoiceConnection {
	m.session.RLock()
	defer m.session.RUnlock()

	voice, _ := m.session.VoiceConnections[m.guildID]
	if voice == nil {
		return nil
	}

	return voice
}

func (m *Manager) CurrentChannelID() *string {
	voice := m.CurrentChannel()
	if voice == nil {
		return nil
	}

	voice.RLock()
	defer voice.RUnlock()

	channelID := voice.ChannelID
	return &channelID
}

func (m *Manager) handleJoinRequest(channelID *string) error {

	m.Lock()
	defer m.Unlock()

	m.logger.Debug("request to join a voice channel received", zap.Stringp("channel", channelID))
	if channelID != nil {
		if m.CurrentChannelID() == nil {
			return m.connectToNewVoiceChannel(*channelID)
		} else {
			return m.changeChannel(*channelID)
		}
	} else {
		return m.disconnectFromChannel()
	}
}

func (m *Manager) connectToNewVoiceChannel(channelID string) error {
	m.logger.Debug("connecting bot to new voice channel")

	// The recording should not include data from previous channels.
	m.audioBuffer.Reset()

	// Join the new channel.
	c, err := m.session.ChannelVoiceJoin(m.guildID, channelID, true, false)
	if err != nil {
		return fmt.Errorf("could not join voice channel: %w", err)
	}

	m.logger.Debug("bot joined the voice channel")

	// Create listeners that will put raw audio data in the buffer.
	m.stopListenersCh = make(chan struct{})
	go func() {
		for {
			select {
			case pkt := <-c.OpusRecv:
				m.audioBuffer.Add(time.Now(), *pkt)
			case <-m.stopListenersCh:
				m.logger.Debug("closing voice channel listener")
				return
			}
		}
	}()
	return nil
}

func (m *Manager) changeChannel(channelID string) error {
	logger := m.logger.With(zap.String("channel", channelID))
	chanID := m.CurrentChannelID()

	if chanID != nil && *chanID == channelID {
		logger.Debug("bot is already in the voice channel")
		return nil
	}

	logger.Debug("moving bot to another voice channel")

	// The recording should not include data from previous channels.
	m.audioBuffer.Reset()

	// Move the bot.
	err := m.CurrentChannel().ChangeChannel(channelID, true, false)
	if err != nil {
		return fmt.Errorf("could not change voice channel: %w", err)
	}

	return nil
}

func (m *Manager) disconnectFromChannel() error {
	if m.CurrentChannel() == nil {
		m.logger.Debug("bot is already disconnected from voice channel")
		return nil
	}

	m.logger.Debug("disconnecting bot from voice channel")

	// Close the listeners.
	close(m.stopListenersCh)
	m.stopListenersCh = nil

	// Disconnect from actual channel.
	if err := m.CurrentChannel().Disconnect(); err != nil {
		return fmt.Errorf("could not disconnect from channel: %w", err)
	}

	m.logger.Debug("disconnected")
	return nil
}

func (m *Manager) cleanupVoiceChannel() {
	if m.CurrentChannel() == nil {
		return
	}

	m.logger.Debug(
		"disconnecting bot from voice channel",
		zap.Stringp("channel", m.CurrentChannelID()),
	)
	err := m.CurrentChannel().Disconnect()
	if err != nil {
		m.logger.Warn(
			"could not disconnect from voice channel",
			zap.String("channel", m.CurrentChannel().ChannelID),
			zap.Error(err),
		)
	}
}
