package router

import (
	"sync"

	"github.com/ysqss/notifier/internal/channel"
	"github.com/ysqss/notifier/internal/config"
	"github.com/ysqss/notifier/internal/message"
)

type RouteResult struct {
	Name string
	Type string
}

type Router struct {
	channels   map[string]*config.ChannelConfig
	defaultChs []string
	mu         sync.RWMutex
}

func New(channels []config.ChannelConfig, defaultChannels []string) *Router {
	r := &Router{
		channels:   make(map[string]*config.ChannelConfig),
		defaultChs: defaultChannels,
	}
	for i := range channels {
		r.channels[channels[i].Name] = &channels[i]
	}
	return r
}

func (r *Router) Update(channels []config.ChannelConfig, defaultChannels []string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.channels = make(map[string]*config.ChannelConfig)
	r.defaultChs = defaultChannels
	for i := range channels {
		r.channels[channels[i].Name] = &channels[i]
	}
}

func (r *Router) Route(msg *message.Message) []RouteResult {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var selected []RouteResult
	for name, ch := range r.channels {
		if !ch.Enabled {
			continue
		}

		if msg.Level == message.LevelCritical {
			selected = append(selected, RouteResult{Name: name, Type: ch.Type})
			continue
		}

		if channel.MatchFilter(ch.Filter, msg) {
			selected = append(selected, RouteResult{Name: name, Type: ch.Type})
		}
	}

	if len(selected) == 0 {
		for _, name := range r.defaultChs {
			if ch, ok := r.channels[name]; ok {
				selected = append(selected, RouteResult{Name: name, Type: ch.Type})
			} else {
				selected = append(selected, RouteResult{Name: name, Type: name})
			}
		}
	}
	return selected
}

func (r *Router) GetChannelType(name string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if ch, ok := r.channels[name]; ok {
		return ch.Type
	}
	return ""
}
