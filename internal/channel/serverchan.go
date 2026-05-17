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

type ServerChanChannel struct {
	sendkey string
	client  *http.Client
	retrier *retrier.Retrier
}

func NewServerChanChannel(sendkey string) *ServerChanChannel {
	return &ServerChanChannel{
		sendkey: sendkey,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		retrier: retrier.New(3, 2*time.Second, 30*time.Second),
	}
}

func newServerChanFromConfig(cfg map[string]string) (Channel, error) {
	sendkey := cfg["sendkey"]
	if sendkey == "" {
		return nil, fmt.Errorf("sendkey is required")
	}
	return NewServerChanChannel(sendkey), nil
}

func (s *ServerChanChannel) Name() string { return "serverchan" }

func (s *ServerChanChannel) Send(ctx context.Context, msg *message.RenderedMessage) error {
	payload, ok := msg.Payload.(string)
	if !ok {
		return fmt.Errorf("invalid payload type for serverchan channel")
	}

	body := map[string]string{
		"title": msg.Original.Title,
		"desp":  payload,
	}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal serverchan payload: %w", err)
	}

	apiURL := fmt.Sprintf("https://sctapi.ftqq.com/%s.send", s.sendkey)

	return s.retrier.Do(ctx, func() error {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(jsonBody))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := s.client.Do(req)
		if err != nil {
			return err
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			respBody, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("serverchan api returned %d: %s", resp.StatusCode, string(respBody))
		}

		var result struct {
			Code int    `json:"code"`
			Msg  string `json:"message"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return nil
		}
		if result.Code != 0 {
			return fmt.Errorf("serverchan api error: %d %s", result.Code, result.Msg)
		}
		return nil
	})
}

func (s *ServerChanChannel) Validate(cfg map[string]string) error {
	if cfg["sendkey"] == "" {
		return fmt.Errorf("sendkey is required for serverchan channel")
	}
	return nil
}
