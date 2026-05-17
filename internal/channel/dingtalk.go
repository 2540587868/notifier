package channel

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/ysqss/notifier/internal/message"
	"github.com/ysqss/notifier/internal/retrier"
)

type DingTalkChannel struct {
	webhookURL string
	secret     string
	client     *http.Client
	retrier    *retrier.Retrier
}

func NewDingTalkChannel(webhookURL, secret string) *DingTalkChannel {
	return &DingTalkChannel{
		webhookURL: webhookURL,
		secret:     secret,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		retrier: retrier.New(3, 2*time.Second, 30*time.Second),
	}
}

func newDingTalkFromConfig(cfg map[string]string) (Channel, error) {
	webhookURL := cfg["webhook_url"]
	if webhookURL == "" {
		return nil, fmt.Errorf("webhook_url is required")
	}
	return NewDingTalkChannel(webhookURL, cfg["secret"]), nil
}

func (d *DingTalkChannel) Name() string { return "dingtalk" }

func (d *DingTalkChannel) sign(timestamp int64) string {
	signStr := fmt.Sprintf("%d\n%s", timestamp, d.secret)
	mac := hmac.New(sha256.New, []byte(d.secret))
	mac.Write([]byte(signStr))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func (d *DingTalkChannel) buildURL() string {
	if d.secret == "" {
		return d.webhookURL
	}
	timestamp := time.Now().UnixMilli()
	sign := d.sign(timestamp)
	u, err := url.Parse(d.webhookURL)
	if err != nil {
		return d.webhookURL
	}
	q := u.Query()
	q.Set("timestamp", fmt.Sprintf("%d", timestamp))
	q.Set("sign", sign)
	u.RawQuery = q.Encode()
	return u.String()
}

func (d *DingTalkChannel) Send(ctx context.Context, msg *message.RenderedMessage) error {
	payload, ok := msg.Payload.(string)
	if !ok {
		return fmt.Errorf("invalid payload type for dingtalk channel")
	}

	body := map[string]any{
		"msgtype": "markdown",
		"markdown": map[string]string{
			"title": msg.Original.Title,
			"text":  payload,
		},
	}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal dingtalk payload: %w", err)
	}

	targetURL := d.buildURL()

	return d.retrier.Do(ctx, func() error {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(jsonBody))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := d.client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			respBody, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("dingtalk api returned %d: %s", resp.StatusCode, string(respBody))
		}

		var result struct {
			ErrCode int    `json:"errcode"`
			ErrMsg  string `json:"errmsg"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return nil
		}
		if result.ErrCode != 0 {
			return fmt.Errorf("dingtalk api error: %d %s", result.ErrCode, result.ErrMsg)
		}
		return nil
	})
}

func (d *DingTalkChannel) Validate(cfg map[string]string) error {
	if cfg["webhook_url"] == "" {
		return fmt.Errorf("webhook_url is required for dingtalk channel")
	}
	return nil
}
