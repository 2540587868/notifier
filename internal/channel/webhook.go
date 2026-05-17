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

type WebhookChannel struct {
	url     string
	headers map[string]string
	client  *http.Client
	retrier *retrier.Retrier
}

func NewWebhookChannel(url string, headers map[string]string) *WebhookChannel {
	return &WebhookChannel{
		url:     url,
		headers: headers,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		retrier: retrier.New(3, 2*time.Second, 30*time.Second),
	}
}

func newWebhookFromConfig(cfg map[string]string) (Channel, error) {
	url := cfg["url"]
	if url == "" {
		return nil, fmt.Errorf("url is required")
	}
	headers := make(map[string]string)
	if h := cfg["headers"]; h != "" {
		json.Unmarshal([]byte(h), &headers)
	}
	return NewWebhookChannel(url, headers), nil
}

func (w *WebhookChannel) Name() string { return "webhook" }

func (w *WebhookChannel) Send(ctx context.Context, msg *message.RenderedMessage) error {
	var jsonBody []byte
	var err error

	switch p := msg.Payload.(type) {
	case string:
		body := map[string]any{
			"title":   msg.Original.Title,
			"content": p,
			"level":   string(msg.Original.Level),
			"tags":    msg.Original.Tags,
			"channel": msg.Channel,
			"time":    msg.Original.Time.Format(time.RFC3339),
		}
		jsonBody, err = json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal webhook payload: %w", err)
		}
	case []byte:
		jsonBody = p
	default:
		return fmt.Errorf("invalid payload type for webhook channel")
	}

	return w.retrier.Do(ctx, func() error {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.url, bytes.NewReader(jsonBody))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		for k, v := range w.headers {
			req.Header.Set(k, v)
		}

		resp, err := w.client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			respBody, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("webhook returned %d: %s", resp.StatusCode, string(respBody))
		}
		return nil
	})
}

func (w *WebhookChannel) Validate(cfg map[string]string) error {
	if cfg["url"] == "" {
		return fmt.Errorf("url is required for webhook channel")
	}
	return nil
}
