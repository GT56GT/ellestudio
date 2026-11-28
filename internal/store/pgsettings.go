package store

// pgsettings.go is the Postgres backend of the settings store.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SettingsStore opens the settings store. The database is the source of
// truth: reads query it, and each change is a transaction that locks only
// the users it touches. ctx is only used to check the connection.
func (d *DB) SettingsStore(ctx context.Context, defaults Settings) (*SettingsStore, error) {
	ctx, cancel := context.WithTimeout(ctx, PgTimeout)
	defer cancel()
	if err := d.Pool().Ping(ctx); err != nil {
		return nil, fmt.Errorf("database: settings: %w", err)
	}
	p := pgSettings{d.Pool()}
	return &SettingsStore{backend: p, shared: p, defaults: defaults, data: map[int64]Settings{}}, nil
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
	ctx, cancel := context.WithTimeout(context.Background(), PgTimeout)
	defer cancel()
	if err := p.pool.SendBatch(ctx, batch).Close(); err != nil {
		return fmt.Errorf("database: save settings: %w", err)
	}
	return nil
}

// The pool belongs to DB, which closes it.
func (pgSettings) close() error { return nil }

// userLockSQL takes a transaction-scoped lock on one user. It is an advisory
// lock (rather than a row lock) so users without a row yet are covered too:
// two referrals for the same newcomer can't both apply.
const userLockSQL = `SELECT pg_advisory_xact_lock(hashtextextended('ellie-user:' || $1::bigint, 0))`

// mutate locks uids (in id order, so two changes can never deadlock), reads
// their rows, runs fn and writes back what it changed, all in one
// transaction of three round trips.
func (p pgSettings) mutate(uids []int64, fn func(*userTx)) (map[int64]Settings, error) {
	ids := slices.Clone(uids)
	slices.Sort(ids)
	ids = slices.Compact(ids)
	ctx, cancel := context.WithTimeout(context.Background(), PgTimeout)
	defer cancel()
	var saved map[int64]Settings
	err := pgx.BeginFunc(ctx, p.pool, func(tx pgx.Tx) error {
		read := &pgx.Batch{}
		for _, id := range ids {
			read.Queue(userLockSQL, id)
		}
		read.Queue(`SELECT user_id, settings FROM ellie_user_settings WHERE user_id = ANY($1) FOR UPDATE`, ids)
		br := tx.SendBatch(ctx, read)
		for range ids {
			if _, err := br.Exec(); err != nil {
				br.Close()
				return fmt.Errorf("lock user: %w", err)
			}
		}
		t := newUserTx()
		rows, err := br.Query()
		if err == nil {
			var uid int64
			var raw []byte
			_, err = pgx.ForEachRow(rows, []any{&uid, &raw}, func() error {
				var v Settings
				if err := json.Unmarshal(raw, &v); err != nil {
					return fmt.Errorf("user %d: %w", uid, err)
				}
				t.cur[uid], t.known[uid] = v, true
				return nil
			})
		}
		if cerr := br.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return fmt.Errorf("read users: %w", err)
		}

		fn(t)
		if len(t.dirty) == 0 {
			return nil
		}
		saved = make(map[int64]Settings, len(t.dirty))
		write := &pgx.Batch{}
		for uid := range t.dirty {
			raw, err := json.Marshal(t.cur[uid])
			if err != nil {
				return err
			}
			saved[uid] = t.cur[uid]
			write.Queue(`INSERT INTO ellie_user_settings (user_id, settings, updated_at)
				VALUES ($1, $2, now())
				ON CONFLICT (user_id) DO UPDATE SET settings = EXCLUDED.settings, updated_at = now()`, uid, raw)
		}
		return tx.SendBatch(ctx, write).Close()
	})
	if err != nil {
		return nil, fmt.Errorf("database: save settings: %w", err)
	}
	return saved, nil
}

func (p pgSettings) load(uid int64) (Settings, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), PgTimeout)
	defer cancel()
	var raw []byte
	err := p.pool.QueryRow(ctx, `SELECT settings FROM ellie_user_settings WHERE user_id = $1`, uid).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return Settings{}, false, nil
	}
	if err != nil {
		return Settings{}, false, fmt.Errorf("database: load settings: %w", err)
	}
	var v Settings
	if err := json.Unmarshal(raw, &v); err != nil {
		return Settings{}, false, fmt.Errorf("database: user %d: %w", uid, err)
	}
	return v, true, nil
}

// forget is one UPDATE; it waits for any change holding those rows. The keys
// come from the fixed refs in settings.go and are passed as parameters.
func (p pgSettings) forget(idKey, nameKey, id string) ([]int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), PgTimeout)
	defer cancel()
	rows, err := p.pool.Query(ctx, `UPDATE ellie_user_settings
		SET settings = settings || jsonb_build_object($2::text, '', $3::text, ''), updated_at = now()
		WHERE settings->>$2::text = $1 RETURNING user_id`, id, idKey, nameKey)
	if err != nil {
		return nil, fmt.Errorf("database: forget %s: %w", idKey, err)
	}
	uids, err := pgx.CollectRows(rows, pgx.RowTo[int64])
	if err != nil {
		return nil, fmt.Errorf("database: forget %s: %w", idKey, err)
	}
	return uids, nil
}

func (p pgSettings) banned() (map[int64]Ban, error) {
	ctx, cancel := context.WithTimeout(context.Background(), PgTimeout)
	defer cancel()
	rows, err := p.pool.Query(ctx, `SELECT user_id, settings->'ban' FROM ellie_user_settings
		WHERE jsonb_typeof(settings->'ban') = 'object'`)
	if err != nil {
		return nil, fmt.Errorf("database: bans: %w", err)
	}
	out := map[int64]Ban{}
	var uid int64
	var raw []byte
	if _, err := pgx.ForEachRow(rows, []any{&uid, &raw}, func() error {
		var b Ban
		if err := json.Unmarshal(raw, &b); err != nil {
			return fmt.Errorf("user %d: %w", uid, err)
		}
		out[uid] = b
		return nil
	}); err != nil {
		return nil, fmt.Errorf("database: bans: %w", err)
	}
	return out, nil
}
