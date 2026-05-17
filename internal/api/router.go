package api

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ysqss/notifier/internal/channel"
	"github.com/ysqss/notifier/internal/config"
	"github.com/ysqss/notifier/internal/dedup"
	"github.com/ysqss/notifier/internal/message"
	"github.com/ysqss/notifier/internal/queue"
	"github.com/ysqss/notifier/internal/ratelimit"
	"github.com/ysqss/notifier/internal/router"
	"github.com/ysqss/notifier/internal/silence"
	"github.com/ysqss/notifier/internal/store"
	"github.com/ysqss/notifier/internal/template"
)

type Server struct {
	store     *store.Store
	cfg       *config.Manager
	mux       *http.ServeMux
	registry  *channel.Registry
	tmpl      *template.Engine
	router    *router.Router
	rateLimit *ratelimit.RateLimiter
	silence   *silence.Checker
	dedup     *dedup.Deduplicator
	queue     *queue.Queue
}

func NewServer(
	st *store.Store,
	cfg *config.Manager,
	reg *channel.Registry,
	tmpl *template.Engine,
	rt *router.Router,
	rl *ratelimit.RateLimiter,
	sil *silence.Checker,
	dd *dedup.Deduplicator,
	q *queue.Queue,
) *Server {
	s := &Server{
		store:     st,
		cfg:       cfg,
		mux:       http.NewServeMux(),
		registry:  reg,
		tmpl:      tmpl,
		router:    rt,
		rateLimit: rl,
		silence:   sil,
		dedup:     dd,
		queue:     q,
	}
	s.registerRoutes()
	return s
}

func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) registerRoutes() {
	s.mux.HandleFunc("/api/v1/notify", s.handleNotify)
	s.mux.HandleFunc("/api/v1/channels", s.handleChannels)
	s.mux.HandleFunc("/api/v1/channels/", s.handleChannelByID)
	s.mux.HandleFunc("/api/v1/notifications", s.handleNotifications)
	s.mux.HandleFunc("/api/v1/tokens", s.handleTokens)
	s.mux.HandleFunc("/api/v1/tokens/", s.handleTokenByID)
	s.mux.HandleFunc("/api/v1/stats", s.handleStats)
	s.mux.HandleFunc("/health", s.handleHealth)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (s *Server) handleNotify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	var req message.NotifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}

	if errs := req.Validate(); len(errs) > 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": errs})
		return
	}

	msg := &message.Message{
		ID:      generateID(),
		Title:   req.Title,
		Content: req.Content,
		Level:   req.Level,
		Tags:    req.Tags,
		Time:    time.Now(),
	}
	if msg.Tags == nil {
		msg.Tags = make(map[string]string)
	}

	if s.rateLimit.Reject(msg) {
		s.recordNotification(msg, "", "rate_limited", "")
		writeJSON(w, http.StatusTooManyRequests, map[string]any{
			"error":  "rate limit exceeded",
			"level":  string(msg.Level),
			"source": msg.Tags["source"],
		})
		return
	}

	if s.silence.IsSilent(msg) {
		s.recordNotification(msg, "", "silenced", "")
		writeJSON(w, http.StatusAccepted, map[string]any{
			"id":      msg.ID,
			"status":  "silenced",
			"message": "notification silenced due to quiet hours",
		})
		return
	}

	if s.dedup.IsDuplicate(msg) {
		writeJSON(w, http.StatusAccepted, map[string]any{
			"id":      msg.ID,
			"status":  "duplicate",
			"message": "duplicate notification within dedup window",
		})
		return
	}

	targetChannels := s.router.Route(msg)
	if len(targetChannels) == 0 {
		writeJSON(w, http.StatusAccepted, map[string]any{
			"id":      msg.ID,
			"status":  "no_channel",
			"message": "no channel matched for this notification",
		})
		return
	}

	var channelNames []string
	for _, route := range targetChannels {
		channelNames = append(channelNames, route.Name)

		rendered, err := s.tmpl.Render(msg, route.Type)
		if err != nil {
			slog.Error("failed to render template", "channel", route.Name, "type", route.Type, "error", err)
			s.recordNotification(msg, route.Name, "failed", err.Error())
			continue
		}

		task := &queue.DispatchTask{
			Message:     rendered,
			ChannelName: route.Name,
			Attempt:     0,
			MaxRetries:  s.cfg.Get().Retry.MaxAttempts,
		}

		if err := s.queue.Enqueue(task); err != nil {
			slog.Error("failed to enqueue task", "channel", route.Name, "error", err)
			s.recordNotification(msg, route.Name, "dropped", err.Error())
			continue
		}

		s.recordNotification(msg, route.Name, "queued", "")
	}

	writeJSON(w, http.StatusAccepted, map[string]any{
		"id":       msg.ID,
		"status":   "accepted",
		"channels": channelNames,
		"message":  "notification queued for delivery",
	})
}

func (s *Server) recordNotification(msg *message.Message, ch, status, errMsg string) {
	recordID := msg.ID
	if ch != "" {
		recordID = msg.ID + ":" + ch
	}
	n := &store.NotificationRecord{
		ID:        recordID,
		Title:     msg.Title,
		Content:   msg.Content,
		Level:     msg.Level,
		Tags:      msg.Tags,
		Channel:   ch,
		Status:    status,
		Error:     errMsg,
		CreatedAt: time.Now(),
	}
	if err := s.store.InsertNotification(n); err != nil {
		slog.Error("failed to record notification", "error", err)
	}
}

func (s *Server) handleChannels(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		channels, err := s.store.ListChannels()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		if channels == nil {
			channels = []*store.ChannelRecord{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"channels": channels})

	case http.MethodPost:
		var req struct {
			Name    string            `json:"name"`
			Type    string            `json:"type"`
			Config  map[string]string `json:"config"`
			Filter  string            `json:"filter"`
			Enabled bool              `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
			return
		}
		if req.Name == "" || req.Type == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "name and type are required"})
			return
		}

		ch, ok := s.registry.Get(req.Type)
		if !ok {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": fmt.Sprintf("unknown channel type: %s", req.Type)})
			return
		}
		if err := ch.Validate(req.Config); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}

		rec := &store.ChannelRecord{
			Name:      req.Name,
			Type:      req.Type,
			Config:    req.Config,
			Filter:    req.Filter,
			IsEnabled: req.Enabled,
		}
		if err := s.store.InsertChannel(rec); err != nil {
			if strings.Contains(err.Error(), "UNIQUE constraint") {
				writeJSON(w, http.StatusConflict, map[string]any{"error": "channel name already exists"})
			} else {
				writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			}
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"message": "channel created", "name": req.Name})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
	}
}

func (s *Server) handleChannelByID(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/api/v1/channels/")
	if strings.HasSuffix(idStr, "/test") {
		s.handleChannelTest(w, r, strings.TrimSuffix(idStr, "/test"))
		return
	}

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid channel id"})
		return
	}

	switch r.Method {
	case http.MethodDelete:
		if err := s.store.DeleteChannel(id); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"message": "channel deleted"})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
	}
}

func (s *Server) handleChannelTest(w http.ResponseWriter, r *http.Request, idStr string) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid channel id"})
		return
	}

	channels, err := s.store.ListChannels()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	var target *store.ChannelRecord
	for _, ch := range channels {
		if ch.ID == id {
			target = ch
			break
		}
	}
	if target == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "channel not found"})
		return
	}

	chImpl, ok := s.registry.Get(target.Type)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": fmt.Sprintf("channel type %s not registered", target.Type)})
		return
	}

	testMsg := &message.Message{
		ID:      generateID(),
		Title:   "Test Notification",
		Content: "This is a test notification from Notifier.",
		Level:   message.LevelInfo,
		Tags:    map[string]string{"source": "notifier", "type": "test"},
		Time:    time.Now(),
	}

	rendered, err := s.tmpl.Render(testMsg, target.Type)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": fmt.Sprintf("render failed: %v", err)})
		return
	}

	if err := chImpl.Send(r.Context(), rendered); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": fmt.Sprintf("send failed: %v", err)})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"message": "test notification sent", "channel": target.Name})
}

func (s *Server) handleNotifications(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	level := r.URL.Query().Get("level")
	status := r.URL.Query().Get("status")

	notifications, total, err := s.store.ListNotifications(page, pageSize, level, status)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if notifications == nil {
		notifications = []*store.NotificationRecord{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"notifications": notifications,
		"total":         total,
		"page":          page,
		"page_size":     pageSize,
	})
}

func (s *Server) handleTokens(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		tokens, err := s.store.ListTokens()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		if tokens == nil {
			tokens = []*store.TokenRecord{}
		}

		var masked []map[string]any
		for _, t := range tokens {
			masked = append(masked, map[string]any{
				"id":           t.ID,
				"name":         t.Name,
				"token_prefix": t.TokenPrefix,
				"is_enabled":   t.IsEnabled,
				"created_at":   t.CreatedAt.Format(time.RFC3339),
			})
		}
		if masked == nil {
			masked = []map[string]any{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"tokens": masked})

	case http.MethodPost:
		var req struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
			return
		}
		if req.Name == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "name is required"})
			return
		}

		token := generateToken()
		prefix := token[:8]
		hash := hashToken(token)

		t := &store.TokenRecord{
			Name:        req.Name,
			Token:       hash,
			TokenPrefix: prefix,
			IsEnabled:   true,
		}
		if err := s.store.InsertToken(t); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}

		writeJSON(w, http.StatusCreated, map[string]any{
			"name":         req.Name,
			"token":        token,
			"token_prefix": prefix,
		})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
	}
}

func (s *Server) handleTokenByID(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/api/v1/tokens/")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid token id"})
		return
	}

	if r.Method == http.MethodDelete {
		if err := s.store.DeleteToken(id); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"message": "token revoked"})
		return
	}

	writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	stats, err := s.store.GetStats()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	stats["queue_dropped"] = s.queue.Dropped()
	stats["queue_enqueued"] = s.queue.Enqueued()
	writeJSON(w, http.StatusOK, stats)
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		slog.Error("failed to write JSON response", "error", err)
	}
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func authMiddleware(next http.Handler, cfg *config.Manager, st *store.Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}

		token := r.Header.Get("Authorization")
		token = strings.TrimPrefix(token, "Bearer ")

		if token == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
			return
		}

		if r.URL.Path == "/api/v1/notify" {
			hash := hashToken(token)
			t, err := st.GetTokenByHash(hash)
			if err != nil || t == nil {
				writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid or unknown api token"})
				return
			}
		} else {
			c := cfg.Get()
			if token != c.Server.AdminToken {
				writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		slog.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"remote", r.RemoteAddr,
		)
		next.ServeHTTP(w, r)
		slog.Debug("request completed",
			"method", r.Method,
			"path", r.URL.Path,
			"duration", time.Since(start),
		)
	})
}

func ApplyMiddleware(handler http.Handler, cfg *config.Manager, st *store.Store) http.Handler {
	h := handler
	h = corsMiddleware(h)
	h = loggingMiddleware(h)
	h = authMiddleware(h, cfg, st)
	return h
}

func generateID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("crypto/rand.Read failed: %v", err))
	}
	return hex.EncodeToString(b)
}

func generateToken() string {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("crypto/rand.Read failed: %v", err))
	}
	return "nt_" + base64.RawURLEncoding.EncodeToString(b)
}

func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}
