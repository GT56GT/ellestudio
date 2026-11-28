package store

// db.go is the Postgres backend: one connection pool shared by every store,
// with versioned schema migrations.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DB is a connection to the bot's Postgres database.
type DB struct{ pool *pgxpool.Pool }

// PgTimeout bounds every query so a slow database can't stall a chat.
const PgTimeout = 5 * time.Second

// migrations are applied in order, each exactly once, inside one transaction
// per startup. Never edit or reorder a released migration; append a new one.
var migrations = []string{
	// 1: per-user settings, one JSONB document per Telegram user.
	`CREATE TABLE IF NOT EXISTS ellie_user_settings (
		user_id    BIGINT PRIMARY KEY,
		settings   JSONB NOT NULL,
		updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`,
	// 2: who created each Cartesia voice and dictionary.
	`CREATE TABLE ellie_ownership (
		kind        TEXT        NOT NULL CHECK (kind IN ('voice', 'dict')),
		resource_id TEXT        NOT NULL CHECK (resource_id <> ''),
		owner_id    BIGINT      NOT NULL,
		created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
		PRIMARY KEY (kind, resource_id)
	);
	CREATE INDEX ellie_ownership_by_owner
		ON ellie_ownership (owner_id, kind, created_at DESC, resource_id DESC)`,
	// 3: groups the bot is in, and their shared daily usage. Row 0 is the
	// total across every group.
	`CREATE TABLE ellie_chats (
		chat_id    BIGINT      PRIMARY KEY,
		type       TEXT        NOT NULL DEFAULT '',
		title      TEXT        NOT NULL DEFAULT '',
		status     TEXT        NOT NULL DEFAULT '',
		blocked    BOOLEAN     NOT NULL DEFAULT false,
		added_by   BIGINT      NOT NULL DEFAULT 0,
		usage_day  TEXT        NOT NULL DEFAULT '',
		chars      INT         NOT NULL DEFAULT 0 CHECK (chars >= 0),
		secs       INT         NOT NULL DEFAULT 0 CHECK (secs >= 0),
		created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
	);
	CREATE INDEX ellie_chats_by_usage ON ellie_chats (usage_day, chars DESC) WHERE chat_id <> 0`,
	// 4: each group's own settings, chosen by its admins ('{}' = defaults).
	`ALTER TABLE ellie_chats ADD COLUMN settings JSONB NOT NULL DEFAULT '{}'`,
	// 5: usage events for the admin Stats screen (see metrics.go), pruned
	// after 90 days.
	`CREATE TABLE ellie_events (
		id      BIGSERIAL   PRIMARY KEY,
		at      TIMESTAMPTZ NOT NULL DEFAULT now(),
		user_id BIGINT      NOT NULL,
		kind    TEXT        NOT NULL,
		source  TEXT        NOT NULL DEFAULT '',
		n       INT         NOT NULL DEFAULT 0
	);
	CREATE INDEX ellie_events_by_time ON ellie_events (at)`,
	// 6: what the whole studio used each UTC day (see budget.go).
	`CREATE TABLE ellie_studio_usage (
		day        TEXT        PRIMARY KEY,
		chars      INT         NOT NULL DEFAULT 0 CHECK (chars >= 0),
		secs       INT         NOT NULL DEFAULT 0 CHECK (secs >= 0),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`,
	// 7: the admin panel: bot heartbeats (status.go), panel sign-in
	// sessions (stored as token hashes) and the audit log of admin actions.
	`CREATE TABLE ellie_status (
		instance   TEXT        PRIMARY KEY,
		started_at TIMESTAMPTZ NOT NULL,
		at         TIMESTAMPTZ NOT NULL,
		info       JSONB       NOT NULL
	);
	CREATE TABLE ellie_admin_sessions (
		token_hash BYTEA       PRIMARY KEY,
		admin_id   BIGINT      NOT NULL,
		name       TEXT        NOT NULL DEFAULT '',
		created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
		last_seen  TIMESTAMPTZ NOT NULL DEFAULT now(),
		expires_at TIMESTAMPTZ NOT NULL,
		user_agent TEXT        NOT NULL DEFAULT '',
		ip         TEXT        NOT NULL DEFAULT ''
	);
	CREATE INDEX ellie_admin_sessions_by_admin ON ellie_admin_sessions (admin_id);
	CREATE TABLE ellie_admin_audit (
		id       BIGSERIAL   PRIMARY KEY,
		at       TIMESTAMPTZ NOT NULL DEFAULT now(),
		admin_id BIGINT      NOT NULL,
		action   TEXT        NOT NULL,
		target   TEXT        NOT NULL DEFAULT '',
		detail   JSONB       NOT NULL DEFAULT '{}',
		ip       TEXT        NOT NULL DEFAULT ''
	);
	CREATE INDEX ellie_admin_audit_by_time ON ellie_admin_audit (at DESC)`,
	// 8: voice and dictionary names (filled in by the bot from Cartesia),
	// and work the admin panel asks the bot to do (it holds the Cartesia
	// key). At most one open job per resource.
	`ALTER TABLE ellie_ownership ADD COLUMN name TEXT NOT NULL DEFAULT '';
	CREATE TABLE ellie_admin_jobs (
		id           BIGSERIAL   PRIMARY KEY,
		kind         TEXT        NOT NULL CHECK (kind IN ('delete_voice', 'delete_dict')),
		target       TEXT        NOT NULL CHECK (target <> ''),
		requested_by BIGINT      NOT NULL,
		status       TEXT        NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'running', 'done', 'failed')),
		error        TEXT        NOT NULL DEFAULT '',
		created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
		updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
	);
	CREATE UNIQUE INDEX ellie_admin_jobs_one_open ON ellie_admin_jobs (kind, target) WHERE status IN ('pending', 'running');
	CREATE INDEX ellie_admin_jobs_open ON ellie_admin_jobs (id) WHERE status IN ('pending', 'running')`,
	// 9: limits admins changed in the panel (limits.go), and announcements
	// (broadcast.go). At most one broadcast is queued or sending at a time.
	`CREATE TABLE ellie_config (
		key        TEXT        PRIMARY KEY,
		value      JSONB       NOT NULL,
		updated_by BIGINT      NOT NULL DEFAULT 0,
		updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
	);
	CREATE TABLE ellie_broadcasts (
		id          BIGSERIAL   PRIMARY KEY,
		text        TEXT        NOT NULL CHECK (text <> ''),
		audience    TEXT        NOT NULL CHECK (audience IN ('everyone', 'active_30d')),
		created_by  BIGINT      NOT NULL,
		status      TEXT        NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'sending', 'done', 'cancelled')),
		total       INT         NOT NULL DEFAULT 0,
		sent        INT         NOT NULL DEFAULT 0,
		failed      INT         NOT NULL DEFAULT 0,
		cursor      BIGINT      NOT NULL DEFAULT 0,
		created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
		started_at  TIMESTAMPTZ,
		finished_at TIMESTAMPTZ,
		updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
	);
	CREATE UNIQUE INDEX ellie_broadcasts_one_open ON ellie_broadcasts ((true)) WHERE status IN ('queued', 'sending')`,
}

// migrationLock is the advisory lock key ("ellie") that serialises
// migrations when two instances start at once, as they do during a deploy.
const migrationLock = 0x656c6c6965

// OpenDatabase connects to databaseURL and brings the schema up to date.
func OpenDatabase(ctx context.Context, databaseURL string) (*DB, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		// pgx parse errors can echo the URL; never let the password reach logs.
		return nil, errors.New("database: DATABASE_URL is not a valid Postgres URL")
	}
	cfg.MaxConns = 16 // each change holds one for a few milliseconds
	cfg.MaxConnIdleTime = 5 * time.Minute
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("database: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("database: %w", err)
	}
	if err := migrate(ctx, pool); err != nil {
		pool.Close()
		return nil, err
	}
	return &DB{pool: pool}, nil
}

// Close releases every connection.
func (d *DB) Close() { d.pool.Close() }

func migrate(ctx context.Context, pool *pgxpool.Pool) error {
	return pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, int64(migrationLock)); err != nil {
			return fmt.Errorf("database: lock: %w", err)
		}
		if _, err := tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS ellie_schema_migrations (
			version    INT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`); err != nil {
			return fmt.Errorf("database: migrations table: %w", err)
		}
		var current int
		if err := tx.QueryRow(ctx, `SELECT COALESCE(max(version), 0) FROM ellie_schema_migrations`).Scan(&current); err != nil {
			return fmt.Errorf("database: read version: %w", err)
		}
		if current > len(migrations) {
			return fmt.Errorf("database: schema version %d is newer than this build (%d); deploy a newer bot", current, len(migrations))
		}
		for v := current + 1; v <= len(migrations); v++ {
			if _, err := tx.Exec(ctx, migrations[v-1]); err != nil {
				return fmt.Errorf("database: migration %d: %w", v, err)
			}
			if _, err := tx.Exec(ctx, `INSERT INTO ellie_schema_migrations (version) VALUES ($1)`, v); err != nil {
				return fmt.Errorf("database: record migration %d: %w", v, err)
			}
		}
		return nil
	})
}

// Pool exposes the connection pool to the admin panel, which runs its own
// read queries against the same database.
func (d *DB) Pool() *pgxpool.Pool { return d.pool }
