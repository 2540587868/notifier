package channel

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ysqss/notifier/internal/message"
	"github.com/ysqss/notifier/internal/retrier"
)

type QmsgChannel struct {
	key     string
	qq      string
	isGroup bool
	client  *http.Client
	retrier *retrier.Retrier
}

func NewQmsgChannel(key, qq string, isGroup bool) *QmsgChannel {
	return &QmsgChannel{
		key:     key,
		qq:      qq,
		isGroup: isGroup,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		retrier: retrier.New(3, 2*time.Second, 30*time.Second),
	}
}

func newQmsgFromConfig(cfg map[string]string) (Channel, error) {
	key := cfg["key"]
	if key == "" {
		return nil, fmt.Errorf("key is required")
	}
	qq := cfg["qq"]
	isGroup := cfg["group"] == "true"
	return NewQmsgChannel(key, qq, isGroup), nil
}

func (q *QmsgChannel) Name() string { return "qmsg" }

func (q *QmsgChannel) Send(ctx context.Context, msg *message.RenderedMessage) error {
	payload, ok := msg.Payload.(string)
	if !ok {
		return fmt.Errorf("invalid payload type for qmsg channel")
	}

	var endpoint string
	if q.isGroup {
		endpoint = fmt.Sprintf("https://qmsg.zendee.cn/group/%s", q.key)
	} else {
		endpoint = fmt.Sprintf("https://qmsg.zendee.cn/send/%s", q.key)
	}

	form := url.Values{}
	form.Set("msg", payload)
	if q.qq != "" {
		form.Set("qq", q.qq)
	}

	return q.retrier.Do(ctx, func() error {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		resp, err := q.client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			respBody, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("qmsg api returned %d: %s", resp.StatusCode, string(respBody))
		}

		return nil
	})
}

func (q *QmsgChannel) Validate(cfg map[string]string) error {
	if cfg["key"] == "" {
		return fmt.Errorf("key is required for qmsg channel")
	}
	return nil
}
