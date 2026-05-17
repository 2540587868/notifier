package channel

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/ysqss/notifier/internal/message"
	"github.com/ysqss/notifier/internal/retrier"
)

type NapCatChannel struct {
	apiURL      string
	accessToken string
	userID      int64
	groupID     int64
	msgType     string
	client      *http.Client
	retrier     *retrier.Retrier
}

func NewNapCatChannel(apiURL, accessToken string, userID, groupID int64, msgType string) *NapCatChannel {
	return &NapCatChannel{
		apiURL:      apiURL,
		accessToken: accessToken,
		userID:      userID,
		groupID:     groupID,
		msgType:     msgType,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		retrier: retrier.New(3, 2*time.Second, 30*time.Second),
	}
}

func newNapCatFromConfig(cfg map[string]string) (Channel, error) {
	apiURL := cfg["api_url"]
	if apiURL == "" {
		return nil, fmt.Errorf("api_url is required")
	}

	var userID int64
	if v := cfg["user_id"]; v != "" {
		var err error
		userID, err = strconv.ParseInt(v, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid user_id: %w", err)
		}
	}

	var groupID int64
	if v := cfg["group_id"]; v != "" {
		var err error
		groupID, err = strconv.ParseInt(v, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid group_id: %w", err)
		}
	}

	msgType := cfg["message_type"]
	if msgType == "" {
		if groupID > 0 {
			msgType = "group"
		} else {
			msgType = "private"
		}
	}

	return NewNapCatChannel(apiURL, cfg["access_token"], userID, groupID, msgType), nil
}

func (n *NapCatChannel) Name() string { return "napcat" }

type onebotMessage struct {
	Type string `json:"type"`
	Data struct {
		Text string `json:"text"`
	} `json:"data"`
}

func (n *NapCatChannel) Send(ctx context.Context, msg *message.RenderedMessage) error {
	payload, ok := msg.Payload.(string)
	if !ok {
		return fmt.Errorf("invalid payload type for napcat channel")
	}

	segments := []onebotMessage{
		{
			Type: "text",
			Data: struct {
				Text string `json:"text"`
			}{Text: payload},
		},
	}

	body := map[string]any{
		"message_type": n.msgType,
		"message":      segments,
	}

	switch n.msgType {
	case "private":
		body["user_id"] = n.userID
	case "group":
		body["group_id"] = n.groupID
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal napcat payload: %w", err)
	}

	endpoint := n.apiURL + "/send_msg"

	return n.retrier.Do(ctx, func() error {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(jsonBody))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		if n.accessToken != "" {
			req.Header.Set("Authorization", "Bearer "+n.accessToken)
		}

		resp, err := n.client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		respBody, _ := io.ReadAll(resp.Body)

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("napcat api returned %d: %s", resp.StatusCode, string(respBody))
		}

		var result struct {
			Status  string `json:"status"`
			RetCode int    `json:"retcode"`
			Msg     string `json:"msg"`
			Wording string `json:"wording"`
		}
		if err := json.Unmarshal(respBody, &result); err != nil {
			return nil
		}
		if result.RetCode != 0 {
			errMsg := result.Msg
			if result.Wording != "" {
				errMsg = result.Wording
			}
			return fmt.Errorf("napcat error %d: %s", result.RetCode, errMsg)
		}
		return nil
	})
}

func (n *NapCatChannel) Validate(cfg map[string]string) error {
	if cfg["api_url"] == "" {
		return fmt.Errorf("api_url is required for napcat channel")
	}
	return nil
}
