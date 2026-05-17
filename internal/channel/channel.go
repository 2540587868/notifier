package channel

import (
	"context"
	"fmt"
	"sync"

	"github.com/ysqss/notifier/internal/config"
	"github.com/ysqss/notifier/internal/message"
)

type Channel interface {
	Name() string
	Send(ctx context.Context, msg *message.RenderedMessage) error
	Validate(cfg map[string]string) error
}

type Registry struct {
	mu       sync.RWMutex
	channels map[string]Channel
}

func NewRegistry() *Registry {
	return &Registry{
		channels: make(map[string]Channel),
	}
}

func (r *Registry) Register(name string, ch Channel) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.channels[name] = ch
}

func (r *Registry) Get(name string) (Channel, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ch, ok := r.channels[name]
	return ch, ok
}

func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.channels))
	for name := range r.channels {
		names = append(names, name)
	}
	return names
}

func (r *Registry) ReplaceAll(channels map[string]Channel) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.channels = channels
}

func BuildFromConfig(channels []config.ChannelConfig) (*Registry, error) {
	reg := NewRegistry()
	var errs []string

	for _, ch := range channels {
		if !ch.Enabled {
			continue
		}
		instance, err := createChannel(ch.Type, ch.Config)
		if err != nil {
			errs = append(errs, fmt.Sprintf("channel %q: %v", ch.Name, err))
			continue
		}
		reg.Register(ch.Name, instance)
	}

	if len(errs) > 0 {
		return reg, fmt.Errorf("some channels failed to build: %s", joinErrors(errs))
	}
	return reg, nil
}

func createChannel(chType string, cfg map[string]string) (Channel, error) {
	switch chType {
	case "wechat":
		return newWeChatFromConfig(cfg)
	case "dingtalk":
		return newDingTalkFromConfig(cfg)
	case "email":
		return newEmailFromConfig(cfg)
	case "telegram":
		return newTelegramFromConfig(cfg)
	case "webhook":
		return newWebhookFromConfig(cfg)
	case "qmsg":
		return newQmsgFromConfig(cfg)
	case "napcat":
		return newNapCatFromConfig(cfg)
	case "serverchan":
		return newServerChanFromConfig(cfg)
	case "pushplus":
		return newPushPlusFromConfig(cfg)
	default:
		return nil, fmt.Errorf("unknown channel type: %s", chType)
	}
}

func joinErrors(errs []string) string {
	result := errs[0]
	for _, e := range errs[1:] {
		result += "; " + e
	}
	return result
}

func MatchFilter(filter *config.RouteFilter, msg *message.Message) bool {
	if filter == nil {
		return true
	}

	if len(filter.Levels) > 0 {
		found := false
		for _, l := range filter.Levels {
			if string(msg.Level) == l {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	if len(filter.Sources) > 0 {
		source, ok := msg.Tags["source"]
		if !ok {
			return false
		}
		found := false
		for _, s := range filter.Sources {
			if source == s {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	if len(filter.Tags) > 0 {
		for k, v := range filter.Tags {
			if msg.Tags[k] != v {
				return false
			}
		}
	}

	return true
}
