package template

import (
	"bytes"
	"fmt"
	"sync"
	texttemplate "text/template"

	"github.com/ysqss/notifier/internal/message"
)

type Engine struct {
	templates map[string]*texttemplate.Template
	mu        sync.RWMutex
}

func NewEngine() *Engine {
	e := &Engine{
		templates: make(map[string]*texttemplate.Template),
	}
	e.loadBuiltin()
	return e
}

func (e *Engine) Render(msg *message.Message, channel string) (*message.RenderedMessage, error) {
	e.mu.RLock()
	tmpl, ok := e.templates[channel]
	e.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("template not found for channel: %s", channel)
	}

	data := map[string]any{
		"Title":   msg.Title,
		"Content": msg.Content,
		"Level":   string(msg.Level),
		"Tags":    msg.Tags,
		"Time":    msg.Time.Format("2006-01-02 15:04:05"),
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("render template: %w", err)
	}

	return &message.RenderedMessage{
		Original: msg,
		Channel:  channel,
		Payload:  buf.String(),
	}, nil
}

func (e *Engine) Register(channel string, body string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	tmpl, err := texttemplate.New(channel).Parse(body)
	if err != nil {
		return fmt.Errorf("parse template for %s: %w", channel, err)
	}
	e.templates[channel] = tmpl
	return nil
}

func (e *Engine) loadBuiltin() {
	builtins := map[string]string{
		"wechat": `## {{.Title}}
{{.Content}}
---
> 级别: {{.Level}} | 来源: {{index .Tags "source"}} | {{.Time}}`,

		"dingtalk": `## {{.Title}}
{{.Content}}
---
> 级别: {{.Level}} | 来源: {{index .Tags "source"}} | {{.Time}}`,

		"email": `<html>
<body>
<h2>{{.Title}}</h2>
<div>{{.Content}}</div>
<hr>
<p style="color:#999">级别: {{.Level}} | 来源: {{index .Tags "source"}} | {{.Time}}</p>
</body>
</html>`,

		"telegram": `*{{.Title}}*
{{.Content}}
_级别: {{.Level}} | 来源: {{index .Tags "source"}}_`,

		"webhook": `{"title":"{{.Title}}","content":"{{.Content}}","level":"{{.Level}}","time":"{{.Time}}"}`,

		"qmsg": `【{{.Level}}】{{.Title}}
{{.Content}}
---
级别: {{.Level}} | 来源: {{index .Tags "source"}} | {{.Time}}`,

		"serverchan": `## {{.Title}}

{{.Content}}

---
> 级别: {{.Level}} | 来源: {{index .Tags "source"}} | {{.Time}}`,

		"pushplus": `<h2>{{.Title}}</h2>
<div>{{.Content}}</div>
<hr>
<p style="color:#999">级别: {{.Level}} | 来源: {{index .Tags "source"}} | {{.Time}}</p>`,

		"napcat": `【{{.Level}}】{{.Title}}
{{.Content}}
---
级别: {{.Level}} | 来源: {{index .Tags "source"}} | {{.Time}}`,
	}

	for ch, body := range builtins {
		tmpl, err := texttemplate.New(ch).Parse(body)
		if err != nil {
			continue
		}
		e.templates[ch] = tmpl
	}
}
