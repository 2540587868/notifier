package message

import "time"

type Level string

const (
	LevelCritical Level = "critical"
	LevelError    Level = "error"
	LevelWarning  Level = "warning"
	LevelInfo     Level = "info"
)

func (l Level) Priority() int {
	switch l {
	case LevelCritical:
		return 0
	case LevelError:
		return 1
	case LevelWarning:
		return 2
	default:
		return 3
	}
}

func (l Level) Valid() bool {
	switch l {
	case LevelCritical, LevelError, LevelWarning, LevelInfo:
		return true
	default:
		return false
	}
}

type Message struct {
	ID      string            `json:"id"`
	Title   string            `json:"title"`
	Content string            `json:"content"`
	Level   Level             `json:"level"`
	Tags    map[string]string `json:"tags"`
	Time    time.Time         `json:"time"`
}

type RenderedMessage struct {
	Original *Message
	Channel  string
	Payload  any
}

type NotifyRequest struct {
	Title   string            `json:"title"`
	Content string            `json:"content"`
	Level   Level             `json:"level"`
	Tags    map[string]string `json:"tags"`
}

func (r *NotifyRequest) Validate() map[string]string {
	errs := make(map[string]string)
	if r.Title == "" {
		errs["title"] = "title is required"
	}
	if r.Content == "" {
		errs["content"] = "content is required"
	}
	if !r.Level.Valid() {
		errs["level"] = "level must be one of: critical, error, warning, info"
	}
	return errs
}
