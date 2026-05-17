package channel

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/ysqss/notifier/internal/message"
	"github.com/ysqss/notifier/internal/retrier"
)

type TelegramChannel struct {
	botToken string
	chatID   string
	client   *http.Client
	retrier  *retrier.Retrier
}

func NewTelegramChannel(botToken, chatID string) *TelegramChannel {
	return &TelegramChannel{
		botToken: botToken,
		chatID:   chatID,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		retrier: retrier.New(3, 2*time.Second, 30*time.Second),
	}
}

func newTelegramFromConfig(cfg map[string]string) (Channel, error) {
	botToken := cfg["bot_token"]
	chatID := cfg["chat_id"]
	if botToken == "" || chatID == "" {
		return nil, fmt.Errorf("bot_token and chat_id are required")
	}
	return NewTelegramChannel(botToken, chatID), nil
}

func (t *TelegramChannel) Name() string { return "telegram" }

func (t *TelegramChannel) Send(ctx context.Context, msg *message.RenderedMessage) error {
	payload, ok := msg.Payload.(string)
	if !ok {
		return fmt.Errorf("invalid payload type for telegram channel")
	}

	body := map[string]any{
		"chat_id":    t.chatID,
		"text":       payload,
		"parse_mode": "Markdown",
	}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal telegram payload: %w", err)
	}

	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", t.botToken)

	return t.retrier.Do(ctx, func() error {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(jsonBody))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := t.client.Do(req)
		if err != nil {
			return err
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			respBody, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("telegram api returned %d: %s", resp.StatusCode, string(respBody))
		}
		return nil
	})
}

func (t *TelegramChannel) Validate(cfg map[string]string) error {
	if cfg["bot_token"] == "" {
		return fmt.Errorf("bot_token is required for telegram channel")
	}
	if cfg["chat_id"] == "" {
		return fmt.Errorf("chat_id is required for telegram channel")
	}
	return nil
}
