package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store/sqlstore"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
)

// Store wraps the whatsmeow device container plus the bridge's own tables
// (session registry and a message cache used for replies and media download).
type Store struct {
	DB        *sql.DB
	Container *sqlstore.Container
}

type SessionRecord struct {
	ID            string
	DeviceJID     string
	WebhookURL    string
	WebhookSecret string
	ImportHistory bool
}

type StoredMessage struct {
	SessionID string
	ID        string
	Chat      string
	Sender    string
	FromMe    bool
	Message   *waE2E.Message
	CreatedAt time.Time
}

var ErrNotFound = errors.New("not found")

const bridgeSchema = `
CREATE TABLE IF NOT EXISTS bridge_sessions (
	id              TEXT PRIMARY KEY,
	device_jid      TEXT,
	webhook_url     TEXT NOT NULL,
	webhook_secret  TEXT NOT NULL,
	import_history  BOOLEAN NOT NULL DEFAULT TRUE,
	created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS bridge_messages (
	session_id  TEXT NOT NULL REFERENCES bridge_sessions(id) ON DELETE CASCADE,
	id          TEXT NOT NULL,
	chat        TEXT NOT NULL,
	sender      TEXT NOT NULL,
	from_me     BOOLEAN NOT NULL DEFAULT FALSE,
	raw         BYTEA NOT NULL,
	created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
	PRIMARY KEY (session_id, id)
);
CREATE INDEX IF NOT EXISTS bridge_messages_created_at_idx ON bridge_messages (created_at);
`

func OpenStore(ctx context.Context, dsn string, log waLog.Logger) (*Store, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}
	container := sqlstore.NewWithDB(db, "postgres", log.Sub("store"))
	if err := container.Upgrade(ctx); err != nil {
		return nil, fmt.Errorf("upgrade whatsmeow schema: %w", err)
	}
	if _, err := db.ExecContext(ctx, bridgeSchema); err != nil {
		return nil, fmt.Errorf("create bridge schema: %w", err)
	}
	return &Store{DB: db, Container: container}, nil
}

func (s *Store) Close() { _ = s.DB.Close() }

func (s *Store) UpsertSession(ctx context.Context, rec SessionRecord) error {
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO bridge_sessions (id, webhook_url, webhook_secret, import_history)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (id) DO UPDATE SET webhook_url = EXCLUDED.webhook_url,
			webhook_secret = EXCLUDED.webhook_secret, import_history = EXCLUDED.import_history, updated_at = now()`,
		rec.ID, rec.WebhookURL, rec.WebhookSecret, rec.ImportHistory)
	return err
}

func (s *Store) SetSessionDevice(ctx context.Context, id, deviceJID string) error {
	_, err := s.DB.ExecContext(ctx,
		`UPDATE bridge_sessions SET device_jid = NULLIF($2, ''), updated_at = now() WHERE id = $1`, id, deviceJID)
	return err
}

func (s *Store) DeleteSession(ctx context.Context, id string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM bridge_sessions WHERE id = $1`, id)
	return err
}

func (s *Store) ListSessions(ctx context.Context) ([]SessionRecord, error) {
	rows, err := s.DB.QueryContext(ctx,
		`SELECT id, COALESCE(device_jid, ''), webhook_url, webhook_secret, import_history FROM bridge_sessions`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SessionRecord
	for rows.Next() {
		var r SessionRecord
		if err := rows.Scan(&r.ID, &r.DeviceJID, &r.WebhookURL, &r.WebhookSecret, &r.ImportHistory); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) SaveMessage(ctx context.Context, m StoredMessage) error {
	raw, err := proto.Marshal(m.Message)
	if err != nil {
		return err
	}
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now()
	}
	_, err = s.DB.ExecContext(ctx, `
		INSERT INTO bridge_messages (session_id, id, chat, sender, from_me, raw, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (session_id, id) DO UPDATE SET raw = EXCLUDED.raw`,
		m.SessionID, m.ID, m.Chat, m.Sender, m.FromMe, raw, m.CreatedAt)
	return err
}

func (s *Store) GetMessage(ctx context.Context, sessionID, id string) (*StoredMessage, error) {
	var (
		m   StoredMessage
		raw []byte
	)
	err := s.DB.QueryRowContext(ctx, `
		SELECT session_id, id, chat, sender, from_me, raw, created_at
		FROM bridge_messages WHERE session_id = $1 AND id = $2`, sessionID, id).
		Scan(&m.SessionID, &m.ID, &m.Chat, &m.Sender, &m.FromMe, &raw, &m.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, err
	}
	m.Message = &waE2E.Message{}
	if err := proto.Unmarshal(raw, m.Message); err != nil {
		return nil, err
	}
	return &m, nil
}

// PruneMessages drops cached messages older than the retention window.
func (s *Store) PruneMessages(ctx context.Context, olderThan time.Duration) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM bridge_messages WHERE created_at < $1`, time.Now().Add(-olderThan))
	return err
}
