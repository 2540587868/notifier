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

type WeChatChannel struct {
	webhookURL string
	client     *http.Client
	retrier    *retrier.Retrier
}

func NewWeChatChannel(webhookURL string) *WeChatChannel {
	return &WeChatChannel{
		webhookURL: webhookURL,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		retrier: retrier.New(3, 2*time.Second, 30*time.Second),
	}
}

func newWeChatFromConfig(cfg map[string]string) (Channel, error) {
	url := cfg["webhook_url"]
	if url == "" {
		return nil, fmt.Errorf("webhook_url is required")
	}
	return NewWeChatChannel(url), nil
}

func (w *WeChatChannel) Name() string { return "wechat" }

func (w *WeChatChannel) Send(ctx context.Context, msg *message.RenderedMessage) error {
	payload, ok := msg.Payload.(string)
	if !ok {
		return fmt.Errorf("invalid payload type for wechat channel")
	}

	body := map[string]any{
		"msgtype": "markdown",
		"markdown": map[string]string{
			"content": payload,
		},
	}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal wechat payload: %w", err)
	}

	return w.retrier.Do(ctx, func() error {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.webhookURL, bytes.NewReader(jsonBody))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := w.client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			respBody, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("wechat api returned %d: %s", resp.StatusCode, string(respBody))
		}

		var result struct {
			ErrCode int    `json:"errcode"`
			ErrMsg  string `json:"errmsg"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return nil
		}
		if result.ErrCode != 0 {
			return fmt.Errorf("wechat api error: %d %s", result.ErrCode, result.ErrMsg)
		}
		return nil
	})
}

func (w *WeChatChannel) Validate(cfg map[string]string) error {
	if cfg["webhook_url"] == "" {
		return fmt.Errorf("webhook_url is required for wechat channel")
	}
	return nil
}
