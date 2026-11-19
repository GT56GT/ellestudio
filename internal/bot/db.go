package bot

// db.go is the Postgres backend: one connection pool shared by the settings
// store and the ownership registry, with versioned schema migrations.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DB is a connection to the bot's Postgres database.
type DB struct{ pool *pgxpool.Pool }

// pgTimeout bounds every query so a slow database can't stall a chat.
const pgTimeout = 5 * time.Second

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
	cfg.MaxConns = 8
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

// ---------------------------------------------------------------- settings

// SettingsStore loads every user's settings. Reads are served from memory;
// each change is written through to the changed user's row.
func (d *DB) SettingsStore(ctx context.Context, defaults Settings) (*Store, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	rows, err := d.pool.Query(ctx, `SELECT user_id, settings FROM ellie_user_settings`)
	if err != nil {
		return nil, fmt.Errorf("database: load settings: %w", err)
	}
	data := map[int64]Settings{}
	var uid int64
	var raw []byte
	if _, err := pgx.ForEachRow(rows, []any{&uid, &raw}, func() error {
		var v Settings
		if err := json.Unmarshal(raw, &v); err != nil {
			return fmt.Errorf("user %d: %w", uid, err)
		}
		data[uid] = v
		return nil
	}); err != nil {
		return nil, fmt.Errorf("database: load settings: %w", err)
	}
	return &Store{backend: pgSettings{d.pool}, defaults: defaults, data: data}, nil
}

type pgSettings struct{ pool *pgxpool.Pool }

// save upserts the changed users in one round trip.
func (p pgSettings) save(all map[int64]Settings, changed []int64) error {
	if len(changed) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for _, uid := range changed {
		raw, err := json.Marshal(all[uid])
		if err != nil {
			return err
		}
		batch.Queue(`INSERT INTO ellie_user_settings (user_id, settings, updated_at)
			VALUES ($1, $2, now())
			ON CONFLICT (user_id) DO UPDATE SET settings = EXCLUDED.settings, updated_at = now()`, uid, raw)
	}
	ctx, cancel := context.WithTimeout(context.Background(), pgTimeout)
	defer cancel()
	if err := p.pool.SendBatch(ctx, batch).Close(); err != nil {
		return fmt.Errorf("database: save settings: %w", err)
	}
	return nil
}

// The pool belongs to DB, which closes it.
func (pgSettings) close() error { return nil }

// ---------------------------------------------------------------- ownership

// Owners returns the ownership registry. It reads the database on every call
// (no cache), so instances running side by side during a deploy always agree.
func (d *DB) Owners() Owners { return pgOwners{d.pool} }

type pgOwners struct{ pool *pgxpool.Pool }

func (p pgOwners) Lookup(ctx context.Context, kind string, ids []string) (map[string]int64, error) {
	out := map[string]int64{}
	if len(ids) == 0 {
		return out, nil
	}
	ctx, cancel := context.WithTimeout(ctx, pgTimeout)
	defer cancel()
	rows, err := p.pool.Query(ctx,
		`SELECT resource_id, owner_id FROM ellie_ownership WHERE kind = $1 AND resource_id = ANY($2)`, kind, ids)
	if err != nil {
		return nil, fmt.Errorf("database: lookup owners: %w", err)
	}
	var id string
	var owner int64
	if _, err := pgx.ForEachRow(rows, []any{&id, &owner}, func() error { out[id] = owner; return nil }); err != nil {
		return nil, fmt.Errorf("database: lookup owners: %w", err)
	}
	return out, nil
}

func (p pgOwners) List(ctx context.Context, kind string, owner int64, after Cursor, limit int) ([]Owned, error) {
	ctx, cancel := context.WithTimeout(ctx, pgTimeout)
	defer cancel()
	var rows pgx.Rows
	var err error
	if after.IsZero() {
		rows, err = p.pool.Query(ctx, `SELECT resource_id, created_at FROM ellie_ownership
			WHERE owner_id = $1 AND kind = $2
			ORDER BY created_at DESC, resource_id DESC LIMIT $3`, owner, kind, limit)
	} else {
		rows, err = p.pool.Query(ctx, `SELECT resource_id, created_at FROM ellie_ownership
			WHERE owner_id = $1 AND kind = $2 AND (created_at, resource_id) < ($3, $4)
			ORDER BY created_at DESC, resource_id DESC LIMIT $5`, owner, kind, after.Created, after.ID, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("database: list owned: %w", err)
	}
	var out []Owned
	var o Owned
	if _, err := pgx.ForEachRow(rows, []any{&o.ID, &o.Created}, func() error {
		o.Created = o.Created.UTC()
		out = append(out, o)
		return nil
	}); err != nil {
		return nil, fmt.Errorf("database: list owned: %w", err)
	}
	return out, nil
}

func (p pgOwners) Claim(ctx context.Context, kind, id string, owner int64) error {
	ctx, cancel := context.WithTimeout(ctx, pgTimeout)
	defer cancel()
	// The no-op update makes RETURNING report the existing owner on conflict,
	// so one statement both claims and detects someone else's claim.
	var got int64
	err := p.pool.QueryRow(ctx, `INSERT INTO ellie_ownership (kind, resource_id, owner_id) VALUES ($1, $2, $3)
		ON CONFLICT (kind, resource_id) DO UPDATE SET owner_id = ellie_ownership.owner_id
		RETURNING owner_id`, kind, id, owner).Scan(&got)
	if err != nil {
		return fmt.Errorf("database: claim: %w", err)
	}
	if got != owner {
		return errAlreadyOwned
	}
	return nil
}

func (p pgOwners) Adopt(ctx context.Context, kind, id string, owner int64) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, pgTimeout)
	defer cancel()
	tag, err := p.pool.Exec(ctx, `INSERT INTO ellie_ownership (kind, resource_id, owner_id) VALUES ($1, $2, $3)
		ON CONFLICT (kind, resource_id) DO NOTHING`, kind, id, owner)
	if err != nil {
		return false, fmt.Errorf("database: adopt: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

func (p pgOwners) Release(ctx context.Context, kind, id string) error {
	ctx, cancel := context.WithTimeout(ctx, pgTimeout)
	defer cancel()
	if _, err := p.pool.Exec(ctx, `DELETE FROM ellie_ownership WHERE kind = $1 AND resource_id = $2`, kind, id); err != nil {
		return fmt.Errorf("database: release: %w", err)
	}
	return nil
}
