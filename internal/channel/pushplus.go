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

type PushPlusChannel struct {
	token    string
	template string
	client   *http.Client
	retrier  *retrier.Retrier
}

func NewPushPlusChannel(token, template string) *PushPlusChannel {
	return &PushPlusChannel{
		token:    token,
		template: template,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		retrier: retrier.New(3, 2*time.Second, 30*time.Second),
	}
}

func newPushPlusFromConfig(cfg map[string]string) (Channel, error) {
	token := cfg["token"]
	if token == "" {
		return nil, fmt.Errorf("token is required")
	}
	template := cfg["template"]
	if template == "" {
		template = "html"
	}
	return NewPushPlusChannel(token, template), nil
}

func (p *PushPlusChannel) Name() string { return "pushplus" }

func (p *PushPlusChannel) Send(ctx context.Context, msg *message.RenderedMessage) error {
	payload, ok := msg.Payload.(string)
	if !ok {
		return fmt.Errorf("invalid payload type for pushplus channel")
	}

	body := map[string]any{
		"token":    p.token,
		"title":    msg.Original.Title,
		"content":  payload,
		"template": p.template,
	}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal pushplus payload: %w", err)
	}

	return p.retrier.Do(ctx, func() error {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://www.pushplus.plus/send", bytes.NewReader(jsonBody))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := p.client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			respBody, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("pushplus api returned %d: %s", resp.StatusCode, string(respBody))
		}

		var result struct {
			Code int    `json:"code"`
			Msg  string `json:"msg"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return nil
		}
		if result.Code != 200 {
			return fmt.Errorf("pushplus api error: %d %s", result.Code, result.Msg)
		}
		return nil
	})
}

func (p *PushPlusChannel) Validate(cfg map[string]string) error {
	if cfg["token"] == "" {
		return fmt.Errorf("token is required for pushplus channel")
	}
	return nil
}
