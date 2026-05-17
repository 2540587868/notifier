package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ysqss/notifier/internal/message"
)

type Store struct {
	db *sql.DB
}

func New(db *sql.DB) (*Store, error) {
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
		PRAGMA journal_mode=WAL;
		PRAGMA synchronous=NORMAL;
		PRAGMA busy_timeout=5000;

		CREATE TABLE IF NOT EXISTS channels (
			id        INTEGER PRIMARY KEY AUTOINCREMENT,
			name      TEXT NOT NULL UNIQUE,
			type      TEXT NOT NULL,
			config    TEXT NOT NULL DEFAULT '{}',
			filter    TEXT DEFAULT '',
			is_enabled INTEGER DEFAULT 1,
			created_at TEXT NOT NULL DEFAULT (datetime('now')),
			updated_at TEXT NOT NULL DEFAULT (datetime('now'))
		);

		CREATE TABLE IF NOT EXISTS notifications (
			id         TEXT PRIMARY KEY,
			title      TEXT NOT NULL,
			content    TEXT NOT NULL,
			level      TEXT NOT NULL,
			tags       TEXT DEFAULT '{}',
			channel    TEXT NOT NULL,
			status     TEXT NOT NULL,
			error      TEXT DEFAULT '',
			created_at TEXT NOT NULL DEFAULT (datetime('now')),
			sent_at    TEXT
		);

		CREATE INDEX IF NOT EXISTS idx_notifications_time
			ON notifications(created_at DESC);
		CREATE INDEX IF NOT EXISTS idx_notifications_status
			ON notifications(status);
		CREATE INDEX IF NOT EXISTS idx_notifications_level
			ON notifications(level);

		CREATE TABLE IF NOT EXISTS api_tokens (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			name         TEXT NOT NULL,
			token        TEXT NOT NULL UNIQUE,
			token_prefix TEXT NOT NULL,
			is_enabled   INTEGER DEFAULT 1,
			created_at   TEXT NOT NULL DEFAULT (datetime('now'))
		);

		CREATE INDEX IF NOT EXISTS idx_tokens ON api_tokens(token);
	`)
	return err
}

type ChannelRecord struct {
	ID        int64
	Name      string
	Type      string
	Config    map[string]string
	Filter    string
	IsEnabled bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (s *Store) InsertChannel(ch *ChannelRecord) error {
	configJSON, err := json.Marshal(ch.Config)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	_, err = s.db.Exec(
		`INSERT INTO channels (name, type, config, filter, is_enabled)
		 VALUES (?, ?, ?, ?, ?)`,
		ch.Name, ch.Type, string(configJSON), ch.Filter, boolToInt(ch.IsEnabled),
	)
	return err
}

func (s *Store) GetChannelByName(name string) (*ChannelRecord, error) {
	row := s.db.QueryRow(
		`SELECT id, name, type, config, filter, is_enabled, created_at, updated_at
		 FROM channels WHERE name = ?`, name,
	)
	return s.scanChannel(row)
}

func (s *Store) ListChannels() ([]*ChannelRecord, error) {
	rows, err := s.db.Query(
		`SELECT id, name, type, config, filter, is_enabled, created_at, updated_at
		 FROM channels ORDER BY id`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var channels []*ChannelRecord
	for rows.Next() {
		ch, err := s.scanChannelFromRows(rows)
		if err != nil {
			return nil, err
		}
		channels = append(channels, ch)
	}
	return channels, rows.Err()
}

func (s *Store) UpdateChannel(id int64, ch *ChannelRecord) error {
	configJSON, err := json.Marshal(ch.Config)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	_, err = s.db.Exec(
		`UPDATE channels SET name = ?, type = ?, config = ?, filter = ?, is_enabled = ?, updated_at = ?
		 WHERE id = ?`,
		ch.Name, ch.Type, string(configJSON), ch.Filter, boolToInt(ch.IsEnabled),
		time.Now().Format(time.RFC3339), id,
	)
	return err
}

func (s *Store) DeleteChannel(id int64) error {
	_, err := s.db.Exec(`DELETE FROM channels WHERE id = ?`, id)
	return err
}

type NotificationRecord struct {
	ID        string
	Title     string
	Content   string
	Level     message.Level
	Tags      map[string]string
	Channel   string
	Status    string
	Error     string
	CreatedAt time.Time
	SentAt    *time.Time
}

func (s *Store) InsertNotification(n *NotificationRecord) error {
	tagsJSON, err := json.Marshal(n.Tags)
	if err != nil {
		return fmt.Errorf("marshal tags: %w", err)
	}
	var sentAt any
	if n.SentAt != nil {
		sentAt = n.SentAt.Format(time.RFC3339)
	}
	_, err = s.db.Exec(
		`INSERT INTO notifications (id, title, content, level, tags, channel, status, error, created_at, sent_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		n.ID, n.Title, n.Content, string(n.Level), string(tagsJSON),
		n.Channel, n.Status, n.Error,
		n.CreatedAt.Format(time.RFC3339), sentAt,
	)
	return err
}

func (s *Store) UpdateNotificationStatus(id, status, errMsg string) error {
	var sentAt any
	if status == "sent" || status == "failed" {
		sentAt = time.Now().Format(time.RFC3339)
	}
	_, err := s.db.Exec(
		`UPDATE notifications SET status = ?, error = ?, sent_at = COALESCE(?, sent_at) WHERE id = ?`,
		status, errMsg, sentAt, id,
	)
	return err
}

func (s *Store) ListNotifications(page, pageSize int, level, status string) ([]*NotificationRecord, int64, error) {
	var where string
	var args []any
	var conditions []string

	if level != "" {
		conditions = append(conditions, "level = ?")
		args = append(args, level)
	}
	if status != "" {
		conditions = append(conditions, "status = ?")
		args = append(args, status)
	}
	if len(conditions) > 0 {
		where = " WHERE " + joinStrings(conditions, " AND ")
	}

	var total int64
	countSQL := "SELECT COUNT(*) FROM notifications" + where
	err := s.db.QueryRow(countSQL, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	querySQL := "SELECT id, title, content, level, tags, channel, status, error, created_at, sent_at FROM notifications" +
		where + " ORDER BY created_at DESC LIMIT ? OFFSET ?"
	queryArgs := append(args, pageSize, offset)

	rows, err := s.db.Query(querySQL, queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var notifications []*NotificationRecord
	for rows.Next() {
		n, err := s.scanNotificationFromRows(rows)
		if err != nil {
			return nil, 0, err
		}
		notifications = append(notifications, n)
	}
	return notifications, total, rows.Err()
}

type TokenRecord struct {
	ID          int64
	Name        string
	Token       string
	TokenPrefix string
	IsEnabled   bool
	CreatedAt   time.Time
}

func (s *Store) InsertToken(t *TokenRecord) error {
	_, err := s.db.Exec(
		`INSERT INTO api_tokens (name, token, token_prefix, is_enabled)
		 VALUES (?, ?, ?, ?)`,
		t.Name, t.Token, t.TokenPrefix, boolToInt(t.IsEnabled),
	)
	return err
}

func (s *Store) GetTokenByHash(hash string) (*TokenRecord, error) {
	row := s.db.QueryRow(
		`SELECT id, name, token, token_prefix, is_enabled, created_at
		 FROM api_tokens WHERE token = ? AND is_enabled = 1`, hash,
	)
	return s.scanToken(row)
}

func (s *Store) ListTokens() ([]*TokenRecord, error) {
	rows, err := s.db.Query(
		`SELECT id, name, token, token_prefix, is_enabled, created_at
		 FROM api_tokens ORDER BY id`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tokens []*TokenRecord
	for rows.Next() {
		t, err := s.scanTokenFromRows(rows)
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, t)
	}
	return tokens, rows.Err()
}

func (s *Store) DeleteToken(id int64) error {
	_, err := s.db.Exec(`DELETE FROM api_tokens WHERE id = ?`, id)
	return err
}

func (s *Store) GetStats() (map[string]any, error) {
	var total, sent, failed, rateLimited int64
	s.db.QueryRow(`SELECT COUNT(*) FROM notifications`).Scan(&total)
	s.db.QueryRow(`SELECT COUNT(*) FROM notifications WHERE status = 'sent'`).Scan(&sent)
	s.db.QueryRow(`SELECT COUNT(*) FROM notifications WHERE status = 'failed'`).Scan(&failed)
	s.db.QueryRow(`SELECT COUNT(*) FROM notifications WHERE status = 'rate_limited'`).Scan(&rateLimited)

	return map[string]any{
		"total":         total,
		"sent":          sent,
		"failed":        failed,
		"rate_limited":  rateLimited,
		"success_rate":  successRate(total, sent),
	}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) scanChannel(row *sql.Row) (*ChannelRecord, error) {
	var ch ChannelRecord
	var configJSON, createdAt, updatedAt string
	var isEnabled int
	err := row.Scan(&ch.ID, &ch.Name, &ch.Type, &configJSON, &ch.Filter,
		&isEnabled, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(configJSON), &ch.Config); err != nil {
		return nil, fmt.Errorf("unmarshal channel config: %w", err)
	}
	ch.IsEnabled = isEnabled == 1
	ch.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	ch.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return &ch, nil
}

func (s *Store) scanChannelFromRows(rows *sql.Rows) (*ChannelRecord, error) {
	var ch ChannelRecord
	var configJSON, createdAt, updatedAt string
	var isEnabled int
	err := rows.Scan(&ch.ID, &ch.Name, &ch.Type, &configJSON, &ch.Filter,
		&isEnabled, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(configJSON), &ch.Config); err != nil {
		return nil, fmt.Errorf("unmarshal channel config: %w", err)
	}
	ch.IsEnabled = isEnabled == 1
	ch.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	ch.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return &ch, nil
}

func (s *Store) scanNotificationFromRows(rows *sql.Rows) (*NotificationRecord, error) {
	var n NotificationRecord
	var tagsJSON, createdAt string
	var sentAt sql.NullString
	err := rows.Scan(&n.ID, &n.Title, &n.Content, &n.Level, &tagsJSON,
		&n.Channel, &n.Status, &n.Error, &createdAt, &sentAt)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(tagsJSON), &n.Tags); err != nil {
		return nil, fmt.Errorf("unmarshal notification tags: %w", err)
	}
	n.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	if sentAt.Valid {
		t, err := time.Parse(time.RFC3339, sentAt.String)
		if err == nil {
			n.SentAt = &t
		}
	}
	return &n, nil
}

func (s *Store) scanToken(row *sql.Row) (*TokenRecord, error) {
	var t TokenRecord
	var isEnabled int
	var createdAt string
	err := row.Scan(&t.ID, &t.Name, &t.Token, &t.TokenPrefix, &isEnabled, &createdAt)
	if err != nil {
		return nil, err
	}
	t.IsEnabled = isEnabled == 1
	t.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return &t, nil
}

func (s *Store) scanTokenFromRows(rows *sql.Rows) (*TokenRecord, error) {
	var t TokenRecord
	var isEnabled int
	var createdAt string
	err := rows.Scan(&t.ID, &t.Name, &t.Token, &t.TokenPrefix, &isEnabled, &createdAt)
	if err != nil {
		return nil, err
	}
	t.IsEnabled = isEnabled == 1
	t.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return &t, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func joinStrings(ss []string, sep string) string {
	if len(ss) == 0 {
		return ""
	}
	result := ss[0]
	for _, s := range ss[1:] {
		result += sep + s
	}
	return result
}

func successRate(total, sent int64) float64 {
	if total == 0 {
		return 100.0
	}
	return float64(sent) / float64(total) * 100.0
}
