package silence

import (
	"time"

	"github.com/ysqss/notifier/internal/config"
	"github.com/ysqss/notifier/internal/message"
)

type Checker struct {
	windows []config.SilenceWindow
}

func NewChecker(windows []config.SilenceWindow) *Checker {
	return &Checker{windows: windows}
}

func (c *Checker) IsSilent(msg *message.Message) bool {
	if len(c.windows) == 0 {
		return false
	}

	if msg.Level == message.LevelCritical {
		return false
	}

	now := time.Now()

	for _, w := range c.windows {
		loc, err := time.LoadLocation(w.Timezone)
		if err != nil {
			loc = time.UTC
		}
		localNow := now.In(loc)

		start, err := parseTime(w.Start)
		if err != nil {
			continue
		}
		end, err := parseTime(w.End)
		if err != nil {
			continue
		}

		currentMinutes := localNow.Hour()*60 + localNow.Minute()
		if currentMinutes >= start && currentMinutes < end {
			for _, l := range w.Levels {
				if string(msg.Level) == l {
					return true
				}
			}
		}
	}

	return false
}

func parseTime(s string) (int, error) {
	t, err := time.Parse("15:04", s)
	if err != nil {
		return 0, err
	}
	return t.Hour()*60 + t.Minute(), nil
}
