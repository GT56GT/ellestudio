package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// pgStore keeps each user's settings as one JSONB row, so new settings fields
// need no migration. The Store still serves reads from memory; every change is
// written through to the row of the user who changed.
type pgStore struct{ pool *pgxpool.Pool }

const pgSchema = `
CREATE TABLE IF NOT EXISTS ellie_user_settings (
	user_id    BIGINT PRIMARY KEY,
	settings   JSONB NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
)`

// pgTimeout bounds every database write so a slow database can't stall a chat.
const pgTimeout = 5 * time.Second

// OpenPostgresStore connects to databaseURL, creates the table if needed and
// loads every user's settings.
func OpenPostgresStore(ctx context.Context, databaseURL string, defaults Settings) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		// pgx errors can echo the URL; never let the password reach the logs.
		return nil, fmt.Errorf("database: DATABASE_URL is not a valid Postgres URL")
	}
	cfg.MaxConns = 4
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("database: %w", err)
	}
	if _, err := pool.Exec(ctx, pgSchema); err != nil {
		pool.Close()
		return nil, fmt.Errorf("database: create table: %w", err)
	}
	data, err := pgLoad(ctx, pool)
	if err != nil {
		pool.Close()
		return nil, err
	}
	return &Store{backend: pgStore{pool: pool}, defaults: defaults, data: data}, nil
}

func pgLoad(ctx context.Context, pool *pgxpool.Pool) (map[int64]Settings, error) {
	rows, err := pool.Query(ctx, `SELECT user_id, settings FROM ellie_user_settings`)
	if err != nil {
		return nil, fmt.Errorf("database: load: %w", err)
	}
	data := map[int64]Settings{}
	var uid int64
	var raw []byte
	_, err = pgx.ForEachRow(rows, []any{&uid, &raw}, func() error {
		var v Settings
		if err := json.Unmarshal(raw, &v); err != nil {
			return fmt.Errorf("user %d: %w", uid, err)
		}
		data[uid] = v
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("database: load: %w", err)
	}
	return data, nil
}

// save upserts the changed users in one round trip.
func (p pgStore) save(all map[int64]Settings, changed []int64) error {
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
		return fmt.Errorf("database: save: %w", err)
	}
	return nil
}

func (p pgStore) close() error {
	p.pool.Close()
	return nil
}
