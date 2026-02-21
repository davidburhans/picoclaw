package channels

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/logger"
	"github.com/sipeed/picoclaw/pkg/utils"
	"github.com/sipeed/picoclaw/pkg/voice"
)

const (
	transcriptionTimeout    = 30 * time.Second
	sendTimeout             = 10 * time.Second
	maxDiscordMessageLength = 1950
)

type DiscordChannel struct {
	*BaseChannel
	session              *discordgo.Session
	config               config.DiscordConfig
	transcriber          voice.Transcriber
	synthesizer          voice.Synthesizer
	includeTextWithVoice bool
	ctx                  context.Context
	wg                   sync.WaitGroup
	typingMu             sync.Mutex
	typingStop           map[string]chan struct{} // chatID → stop signal

	reactionMu      sync.Mutex
	activeReactions map[string]string // messageID → emoji (currently added position emoji)
	botUserID       string            // stored for mention checking
}

func NewDiscordChannel(cfg config.DiscordConfig, bus *bus.MessageBus) (*DiscordChannel, error) {
	session, err := discordgo.New("Bot " + cfg.Token)
	if err != nil {
		return nil, fmt.Errorf("failed to create discord session: %w", err)
	}

	base := NewBaseChannel("discord", cfg, bus, cfg.AllowFrom)

	return &DiscordChannel{
		BaseChannel:     base,
		session:         session,
		config:          cfg,
		transcriber:     nil,
		ctx:             context.Background(),
		typingStop:      make(map[string]chan struct{}),
		activeReactions: make(map[string]string),
	}, nil
}

func (c *DiscordChannel) SetTranscriber(transcriber voice.Transcriber) {
	c.transcriber = transcriber
}

func (c *DiscordChannel) SetSynthesizer(synthesizer voice.Synthesizer) {
	c.synthesizer = synthesizer
}

func (c *DiscordChannel) SetIncludeTextWithVoice(include bool) {
	c.includeTextWithVoice = include
}

func (c *DiscordChannel) getContext() context.Context {
	if c.ctx == nil {
		return context.Background()
	}
	return c.ctx
}

func (c *DiscordChannel) Start(ctx context.Context) error {
	logger.InfoC("discord", "Starting Discord bot")

	c.ctx = ctx

	// Get bot user ID before opening session to avoid race condition
	botUser, err := c.session.User("@me")
	if err != nil {
		return fmt.Errorf("failed to get bot user: %w", err)
	}
	c.botUserID = botUser.ID

	c.session.AddHandler(c.handleMessage)

	if err := c.session.Open(); err != nil {
		return fmt.Errorf("failed to open discord session: %w", err)
	}

	c.setRunning(true)

	logger.InfoCF("discord", "Discord bot connected", map[string]any{
		"username": botUser.Username,
		"user_id":  botUser.ID,
	})

	return nil
}

func (c *DiscordChannel) Stop(ctx context.Context) error {
	logger.InfoC("discord", "Stopping Discord bot")
	c.setRunning(false)

	// Stop all typing goroutines before closing session
	c.typingMu.Lock()
	for chatID, stop := range c.typingStop {
		close(stop)
		delete(c.typingStop, chatID)
	}
	c.typingMu.Unlock()

	if err := c.session.Close(); err != nil {
		return fmt.Errorf("failed to close discord session: %w", err)
	}
	c.wg.Wait()

	return nil
}

func (c *DiscordChannel) Send(ctx context.Context, msg bus.OutboundMessage) error {
	c.stopTyping(msg.ChatID)

	if !c.IsRunning() {
		return fmt.Errorf("discord bot not running")
	}

	channelID := msg.ChatID
	if channelID == "" {
		return fmt.Errorf("channel ID is empty")
	}

	if msg.Type == bus.MessageTypeTyping {
		if err := c.session.ChannelTyping(channelID); err != nil {
			logger.ErrorCF("discord", "Failed to send typing indicator", map[string]any{
				"error": err.Error(),
			})
			return err
		}
		return nil
	}

	if msg.Type == bus.MessageTypeReaction {
		return c.handleReaction(msg)
	}

	// Handle audio message type
	if msg.Type == bus.MessageTypeAudio || msg.Audio != "" {
		logger.InfoCF("discord", "Received audio message, processing TTS", map[string]any{
			"has_audio_path": msg.Audio != "",
			"has_content":    msg.Content != "",
			"content_len":    len(msg.Content),
		})
		audioPath := msg.Audio
		if audioPath == "" {
			// Generate TTS if synthesizer is available and text is provided
			if c.synthesizer != nil && c.synthesizer.IsAvailable() && msg.Content != "" {
				var err error
				audioPath, err = c.synthesizer.Synthesize(ctx, msg.Content)
				if err != nil {
					logger.ErrorCF("discord", "TTS synthesis failed", map[string]any{"error": err})
					// Fall back to text message
				}
			} else {
				logger.WarnCF("discord", "TTS not available", map[string]any{
					"synthesizer_nil":       c.synthesizer == nil,
					"synthesizer_available": c.synthesizer != nil && c.synthesizer.IsAvailable(),
				})
			}
		}

		if audioPath != "" {
			defer os.Remove(audioPath)
			f, err := os.Open(audioPath)
			if err != nil {
				logger.ErrorCF("discord", "Failed to open audio file", map[string]any{"error": err})
				return err
			}
			defer f.Close()
			file := &discordgo.File{
				Name:        "voice.wav",
				ContentType: "audio/wav",
				Reader:      f,
			}

			// Include text content along with audio if enabled
			msgSend := &discordgo.MessageSend{
				Files: []*discordgo.File{file},
			}
			if c.includeTextWithVoice && msg.Content != "" {
				msgSend.Content = msg.Content
			}

			_, err = c.session.ChannelMessageSendComplex(channelID, msgSend)
			if err != nil {
				logger.ErrorCF("discord", "Failed to send audio message", map[string]any{"error": err})
				return err
			}
			return nil
		}
	}

	runes := []rune(msg.Content)
	if len(runes) == 0 {
		return nil
	}

	chunks := utils.SplitMessage(msg.Content, 2000) // Split messages into chunks, Discord length limit: 2000 chars

	for _, chunk := range chunks {
		if err := c.sendChunk(ctx, channelID, chunk); err != nil {
			return err
		}
	}

	return nil
}

func (c *DiscordChannel) handleReaction(msg bus.OutboundMessage) error {
	msgID := msg.Metadata["message_id"]
	if msgID == "" {
		return nil
	}
	action := msg.Metadata["action"]
	emoji := msg.Metadata["emoji"]

	switch action {
	case "add":
		if emoji == "queue" {
			// Special handling for queue position
			pos := 0
			fmt.Sscanf(msg.Content, "%d", &pos)

			// Determine current position emoji
			var newEmoji string
			numberEmojisOnly := []string{"0️⃣", "1️⃣", "2️⃣", "3️⃣", "4️⃣", "5️⃣", "6️⃣", "7️⃣", "8️⃣", "9️⃣", "🔟"}
			if pos > 0 {
				if pos <= 10 {
					newEmoji = numberEmojisOnly[pos]
				} else {
					newEmoji = "🔟" // Using 🔟 for 10+ for now, can be improved
				}
			}

			c.reactionMu.Lock()
			oldEmoji := c.activeReactions[msgID]

			// Only update if it changed
			if newEmoji != oldEmoji {
				// Remove old one if exists
				if oldEmoji != "" {
					c.session.MessageReactionRemove(msg.ChatID, msgID, oldEmoji, "@me")
				}
				// Add new one
				if newEmoji != "" {
					c.session.MessageReactionAdd(msg.ChatID, msgID, newEmoji)
					if pos > 10 {
						c.session.MessageReactionAdd(msg.ChatID, msgID, "➕")
					}
				}
				c.activeReactions[msgID] = newEmoji
			}
			c.reactionMu.Unlock()

			// Add queue indicator (Hourglass) - Discord API will ignore if already there
			return c.session.MessageReactionAdd(msg.ChatID, msgID, "⌛")
		}
		return c.session.MessageReactionAdd(msg.ChatID, msgID, emoji)

	case "remove":
		if emoji == "⚙️" || emoji == "🛠️" {
			// Cleanup tracking when core reactions are removed?
			// Actually just clear them for clear_queue.
		}
		return c.session.MessageReactionRemove(msg.ChatID, msgID, emoji, "@me")

	case "clear_queue":
		c.reactionMu.Lock()
		oldEmoji := c.activeReactions[msgID]
		delete(c.activeReactions, msgID)
		c.reactionMu.Unlock()

		// Remove hourglass, and the specific number emoji + plus sign
		c.session.MessageReactionRemove(msg.ChatID, msgID, "⌛", "@me")
		c.session.MessageReactionRemove(msg.ChatID, msgID, "➕", "@me")
		if oldEmoji != "" {
			c.session.MessageReactionRemove(msg.ChatID, msgID, oldEmoji, "@me")
		}

		// Fallback: cleaning up common ones just in case tracking missed one (e.g. restart)
		// But let's avoid the loop if we have tracking.
		return nil
	}

	return nil
}

func (c *DiscordChannel) sendChunk(ctx context.Context, channelID, content string) error {
	// Use the passed ctx for timeout control
	sendCtx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, err := c.session.ChannelMessageSend(channelID, content)
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("failed to send discord message: %w", err)
		}
	case <-sendCtx.Done():
		return fmt.Errorf("send message timeout: %w", sendCtx.Err())
	}

	return nil
}

// appendContent safely appends content to existing text
func appendContent(content, suffix string) string {
	if content == "" {
		return suffix
	}
	return content + "\n" + suffix
}

func (c *DiscordChannel) handleMessage(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m == nil || m.Author == nil {
		return
	}

	if m.Author.ID == s.State.User.ID {
		return
	}

	// Check allowlist first to avoid downloading attachments and transcribing for rejected users
	if !c.IsAllowed(m.Author.ID) {
		logger.DebugCF("discord", "Message rejected by allowlist", map[string]any{
			"user_id": m.Author.ID,
		})
		return
	}

	// Reactions will be added by the AgentLoop via the message bus
	// to provide accurate status (queueing, processing, etc.)

	// If configured to only respond to mentions, check if bot is mentioned
	// Skip this check for DMs (GuildID is empty) - DMs should always be responded to
	if c.config.MentionOnly && m.GuildID != "" {
		isMentioned := false
		for _, mention := range m.Mentions {
			if mention.ID == c.botUserID {
				isMentioned = true
				break
			}
		}
		if !isMentioned {
			logger.DebugCF("discord", "Message ignored - bot not mentioned", map[string]any{
				"user_id": m.Author.ID,
			})
			return
		}
	}

	senderID := m.Author.ID
	senderName := m.Author.Username
	if m.Author.Discriminator != "" && m.Author.Discriminator != "0" {
		senderName += "#" + m.Author.Discriminator
	}

	content := m.Content
	content = c.stripBotMention(content)
	mediaPaths := make([]string, 0, len(m.Attachments))
	localFiles := make([]string, 0, len(m.Attachments))

	// Ensure temp files are cleaned up when function returns
	defer func() {
		for _, file := range localFiles {
			if err := os.Remove(file); err != nil {
				logger.DebugCF("discord", "Failed to cleanup temp file", map[string]any{
					"file":  file,
					"error": err.Error(),
				})
			}
		}
	}()

	for _, attachment := range m.Attachments {
		isAudio := utils.IsAudioFile(attachment.Filename, attachment.ContentType)

		if isAudio {
			localPath := c.downloadAttachment(attachment.URL, attachment.Filename)
			if localPath != "" {
				localFiles = append(localFiles, localPath)

				transcribedText := ""
				if c.transcriber != nil && c.transcriber.IsAvailable() {
					ctx, cancel := context.WithTimeout(c.getContext(), transcriptionTimeout)
					result, err := c.transcriber.Transcribe(ctx, localPath)
					cancel() // Release context resources immediately to avoid leaks in for loop

					if err != nil {
						logger.ErrorCF("discord", "Voice transcription failed", map[string]any{
							"error": err.Error(),
						})
						transcribedText = fmt.Sprintf("[audio: %s (transcription failed)]", attachment.Filename)
					} else {
						transcribedText = fmt.Sprintf("[audio transcription: %s]", result.Text)
						logger.DebugCF("discord", "Audio transcribed successfully", map[string]any{
							"text": result.Text,
						})
					}
				} else {
					transcribedText = fmt.Sprintf("[audio: %s]", attachment.Filename)
				}

				content = appendContent(content, transcribedText)
			} else {
				logger.WarnCF("discord", "Failed to download audio attachment", map[string]any{
					"url":      attachment.URL,
					"filename": attachment.Filename,
				})
				mediaPaths = append(mediaPaths, attachment.URL)
				content = appendContent(content, fmt.Sprintf("[attachment: %s]", attachment.URL))
			}
		} else {
			mediaPaths = append(mediaPaths, attachment.URL)
			content = appendContent(content, fmt.Sprintf("[attachment: %s]", attachment.URL))
		}
	}

	if content == "" && len(mediaPaths) == 0 {
		return
	}

	if content == "" {
		content = "[media only]"
	}

	// Start typing after all early returns — guaranteed to have a matching Send()
	c.startTyping(m.ChannelID)

	logger.DebugCF("discord", "Received message", map[string]any{
		"sender_name": senderName,
		"sender_id":   senderID,
		"preview":     utils.Truncate(content, 50),
	})

	peerKind := "channel"
	peerID := m.ChannelID
	if m.GuildID == "" {
		peerKind = "direct"
		peerID = senderID
	}

	metadata := map[string]string{
		"message_id":   m.ID,
		"user_id":      senderID,
		"username":     m.Author.Username,
		"display_name": senderName,
		"guild_id":     m.GuildID,
		"channel_id":   m.ChannelID,
		"is_dm":        fmt.Sprintf("%t", m.GuildID == ""),
		"peer_kind":    peerKind,
		"peer_id":      peerID,
	}

	sessionKey := fmt.Sprintf("discord:%s", m.ChannelID)
	c.HandleMessage(senderID, m.ChannelID, sessionKey, content, mediaPaths, metadata)
}

// startTyping starts a continuous typing indicator loop for the given chatID.
// It stops any existing typing loop for that chatID before starting a new one.
func (c *DiscordChannel) startTyping(chatID string) {
	c.typingMu.Lock()
	// Stop existing loop for this chatID if any
	if stop, ok := c.typingStop[chatID]; ok {
		close(stop)
	}
	stop := make(chan struct{})
	c.typingStop[chatID] = stop
	c.typingMu.Unlock()

	go func() {
		if err := c.session.ChannelTyping(chatID); err != nil {
			logger.DebugCF("discord", "ChannelTyping error", map[string]interface{}{"chatID": chatID, "err": err})
		}
		ticker := time.NewTicker(8 * time.Second)
		defer ticker.Stop()
		timeout := time.After(5 * time.Minute)
		for {
			select {
			case <-stop:
				return
			case <-timeout:
				return
			case <-c.ctx.Done():
				return
			case <-ticker.C:
				if err := c.session.ChannelTyping(chatID); err != nil {
					logger.DebugCF("discord", "ChannelTyping error", map[string]interface{}{"chatID": chatID, "err": err})
				}
			}
		}
	}()
}

// stopTyping stops the typing indicator loop for the given chatID.
func (c *DiscordChannel) stopTyping(chatID string) {
	c.typingMu.Lock()
	defer c.typingMu.Unlock()
	if stop, ok := c.typingStop[chatID]; ok {
		close(stop)
		delete(c.typingStop, chatID)
	}
}

func (c *DiscordChannel) downloadAttachment(url, filename string) string {
	return utils.DownloadFile(url, filename, utils.DownloadOptions{
		LoggerPrefix: "discord",
	})
}

// stripBotMention removes the bot mention from the message content.
// Discord mentions have the format <@USER_ID> or <@!USER_ID> (with nickname).
func (c *DiscordChannel) stripBotMention(text string) string {
	if c.botUserID == "" {
		return text
	}
	// Remove both regular mention <@USER_ID> and nickname mention <@!USER_ID>
	text = strings.ReplaceAll(text, fmt.Sprintf("<@%s>", c.botUserID), "")
	text = strings.ReplaceAll(text, fmt.Sprintf("<@!%s>", c.botUserID), "")
	return strings.TrimSpace(text)
}
