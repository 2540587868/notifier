package main

import (
	"context"
	"database/sql"
	"flag"
	"log/slog"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "modernc.org/sqlite"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/ysqss/notifier/internal/api"
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

var (
	configPath = flag.String("config", "config.yaml", "path to config file")
)

func main() {
	flag.Parse()

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	cfgMgr, err := config.Load(*configPath)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	cfg := cfgMgr.Get()

	if err := os.MkdirAll("data", 0755); err != nil {
		slog.Error("failed to create data directory", "error", err)
		os.Exit(1)
	}

	db, err := sql.Open("sqlite", cfg.Database.Path)
	if err != nil {
		slog.Error("failed to open database", "error", err)
		os.Exit(1)
	}
	defer func() { _ = db.Close() }()

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	st, err := store.New(db)
	if err != nil {
		slog.Error("failed to init store", "error", err)
		os.Exit(1)
	}

	registry, err := channel.BuildFromConfig(cfg.Channels)
	if err != nil {
		slog.Warn("some channels failed to initialize", "error", err)
	}
	slog.Info("channels initialized", "channels", registry.List())

	tmpl := template.NewEngine()

	rt := router.New(cfg.Channels, cfg.DefaultChannels)

	rl := ratelimit.New(cfg.RateLimit)

	sil := silence.NewChecker(cfg.Silence.Windows)

	dd := dedup.New(5*time.Minute, 10000)

	q := queue.New(4096)

	q.Start(5, func(ctx context.Context, task *queue.DispatchTask) {
		ch, ok := registry.Get(task.ChannelName)
		if !ok {
			slog.Error("channel not found", "channel", task.ChannelName)
			return
		}

		recordID := task.Message.Original.ID + ":" + task.ChannelName
		if err := ch.Send(ctx, task.Message); err != nil {
			slog.Error("failed to send notification",
				"channel", task.ChannelName,
				"attempt", task.Attempt,
				"error", err,
			)
			if err := st.UpdateNotificationStatus(recordID, "failed", err.Error()); err != nil {
				slog.Error("failed to update notification status", "error", err)
			}
		} else {
			slog.Info("notification sent",
				"channel", task.ChannelName,
				"level", string(task.Message.Original.Level),
				"title", task.Message.Original.Title,
			)
			if err := st.UpdateNotificationStatus(recordID, "sent", ""); err != nil {
				slog.Error("failed to update notification status", "error", err)
			}
		}
	})

	server := api.NewServer(st, cfgMgr, registry, tmpl, rt, rl, sil, dd, q)
	handler := api.ApplyMiddleware(server.Handler(), cfgMgr, st)

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.Handle("/", handler)

	httpServer := &http.Server{
		Addr:         cfg.Server.Listen,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		slog.Info("notifier server starting", "addr", cfg.Server.Listen)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	if cfg.Server.DebugPort != "" {
		go func() {
			pprofMux := http.NewServeMux()
			pprofMux.Handle("/debug/pprof/", http.DefaultServeMux)
			pprofServer := &http.Server{
				Addr:    cfg.Server.DebugPort,
				Handler: pprofMux,
			}
			slog.Info("pprof server starting", "addr", cfg.Server.DebugPort)
			if err := pprofServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				slog.Error("pprof server error", "error", err)
			}
		}()
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	for sig := range sigCh {
		switch sig {
		case syscall.SIGHUP:
			slog.Info("received SIGHUP, reloading config")
			if err := cfgMgr.Reload(); err != nil {
				slog.Error("failed to reload config", "error", err)
			} else {
				slog.Info("config reloaded successfully")
				newCfg := cfgMgr.Get()
				rt.Update(newCfg.Channels, newCfg.DefaultChannels)

				newRegistry, err := channel.BuildFromConfig(newCfg.Channels)
				if err != nil {
					slog.Warn("some channels failed to reinitialize", "error", err)
				}
				slog.Info("channels reinitialized", "channels", newRegistry.List())

				newChannels := make(map[string]channel.Channel)
				for _, name := range newRegistry.List() {
					if ch, ok := newRegistry.Get(name); ok {
						newChannels[name] = ch
					}
				}
				registry.ReplaceAll(newChannels)
			}
		case syscall.SIGINT, syscall.SIGTERM:
			slog.Info("shutting down", "signal", sig.String())

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			httpServer.SetKeepAlivesEnabled(false)

			slog.Info("draining message queue...")
			q.Shutdown()

			if err := httpServer.Shutdown(ctx); err != nil {
				slog.Error("http server shutdown error", "error", err)
			}

			slog.Info("notifier stopped")
			return
		}
	}
}

var _ = message.LevelCritical
