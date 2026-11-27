package bot

// db.go is the Postgres backend: one connection pool shared by the settings
// store and the ownership registry, with versioned schema migrations.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
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

// ---------------------------------------------------------------- groups

// Chats returns the group registry. Like Owners it reads the database on
// every call, so instances running side by side during a deploy agree.
func (d *DB) Chats() Chats { return pgChats{d.pool, allGroups} }

// Metrics returns the Postgres-backed usage events.
func (d *DB) Metrics() Metrics { return pgMetrics{d.pool} }

type pgMetrics struct{ pool *pgxpool.Pool }

func (p pgMetrics) Record(ctx context.Context, e Event) error {
	ctx, cancel := context.WithTimeout(ctx, pgTimeout)
	defer cancel()
	_, err := p.pool.Exec(ctx, `INSERT INTO ellie_events (at, user_id, kind, source, n) VALUES ($1, $2, $3, $4, $5)`,
		e.At, e.User, e.Kind, e.Source, e.N)
	if err != nil {
		return fmt.Errorf("database: record event: %w", err)
	}
	return nil
}

func (p pgMetrics) Days(ctx context.Context, since time.Time) ([]DayStats, error) {
	ctx, cancel := context.WithTimeout(ctx, pgTimeout)
	defer cancel()
	rows, err := p.pool.Query(ctx, `SELECT to_char(at AT TIME ZONE 'UTC', 'YYYY-MM-DD') AS day,
			count(DISTINCT user_id) FILTER (WHERE kind IN ('start', 'reading', 'transcribe')),
			count(*) FILTER (WHERE kind = 'start'),
			count(*) FILTER (WHERE kind = 'start' AND source IN ('group', 'guest')),
			count(*) FILTER (WHERE kind = 'start' AND source = 'referral'),
			count(*) FILTER (WHERE kind = 'reading'),
			COALESCE(sum(n) FILTER (WHERE kind = 'reading'), 0),
			count(DISTINCT user_id) FILTER (WHERE kind = 'limit'),
			count(*) FILTER (WHERE kind = 'guest_invite')
		FROM ellie_events WHERE at >= $1 GROUP BY day ORDER BY day DESC`, since)
	if err != nil {
		return nil, fmt.Errorf("database: summarise events: %w", err)
	}
	defer rows.Close()
	var out []DayStats
	for rows.Next() {
		var d DayStats
		if err := rows.Scan(&d.Day, &d.Active, &d.New, &d.FromGroups, &d.FromInvites,
			&d.Readings, &d.Chars, &d.LimitUsers, &d.GuestInvites); err != nil {
			return nil, fmt.Errorf("database: summarise events: %w", err)
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("database: summarise events: %w", err)
	}
	return out, nil
}

func (p pgMetrics) Active(ctx context.Context, since time.Time) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, pgTimeout)
	defer cancel()
	var n int
	err := p.pool.QueryRow(ctx, `SELECT count(DISTINCT user_id) FROM ellie_events
		WHERE at >= $1 AND kind IN ('start', 'reading', 'transcribe')`, since).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("database: count active users: %w", err)
	}
	return n, nil
}

func (p pgMetrics) Prune(ctx context.Context, before time.Time) error {
	_, err := p.pool.Exec(ctx, `DELETE FROM ellie_events WHERE at < $1`, before)
	if err != nil {
		return fmt.Errorf("database: prune events: %w", err)
	}
	return nil
}

type pgChats struct {
	pool  *pgxpool.Pool
	total int64 // the row that sums every group (allGroups; tests use their own)
}

const chatCols = `chat_id, type, title, status, blocked, added_by, usage_day, chars, secs, settings`

// chatRow scans one ellie_chats row; settings arrive as raw JSON.
type chatRow struct {
	c   ChatInfo
	raw []byte
}

func (r *chatRow) dest() []any {
	c := &r.c
	return []any{&c.ID, &c.Type, &c.Title, &c.Status, &c.Blocked, &c.AddedBy, &c.Day, &c.Chars, &c.Secs, &r.raw}
}

// info decodes the row. Unknown settings fields are ignored, so an older
// build can read what a newer one saved.
func (r *chatRow) info() (ChatInfo, error) {
	c := r.c
	c.Settings = GroupSettings{}
	if len(r.raw) > 0 {
		if err := json.Unmarshal(r.raw, &c.Settings); err != nil {
			return ChatInfo{}, fmt.Errorf("chat %d settings: %w", c.ID, err)
		}
	}
	return c, nil
}

func (p pgChats) Get(ctx context.Context, id int64) (ChatInfo, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, pgTimeout)
	defer cancel()
	var r chatRow
	err := p.pool.QueryRow(ctx, `SELECT `+chatCols+` FROM ellie_chats WHERE chat_id = $1`, id).Scan(r.dest()...)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return ChatInfo{}, false, nil
	case err != nil:
		return ChatInfo{}, false, fmt.Errorf("database: get chat: %w", err)
	}
	c, err := r.info()
	if err != nil {
		return ChatInfo{}, false, fmt.Errorf("database: get chat: %w", err)
	}
	return c, true, nil
}

func (p pgChats) SetSettings(ctx context.Context, id int64, gs GroupSettings) error {
	raw, err := json.Marshal(gs)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, pgTimeout)
	defer cancel()
	if _, err := p.pool.Exec(ctx, `INSERT INTO ellie_chats (chat_id, settings) VALUES ($1, $2::jsonb)
		ON CONFLICT (chat_id) DO UPDATE SET settings = EXCLUDED.settings, updated_at = now()`, id, string(raw)); err != nil {
		return fmt.Errorf("database: save group settings: %w", err)
	}
	return nil
}

func (p pgChats) Upsert(ctx context.Context, c ChatInfo) error {
	ctx, cancel := context.WithTimeout(ctx, pgTimeout)
	defer cancel()
	_, err := p.pool.Exec(ctx, `INSERT INTO ellie_chats (chat_id, type, title, status, added_by) VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (chat_id) DO UPDATE SET
			type       = COALESCE(NULLIF(EXCLUDED.type, ''), ellie_chats.type),
			title      = COALESCE(NULLIF(EXCLUDED.title, ''), ellie_chats.title),
			status     = COALESCE(NULLIF(EXCLUDED.status, ''), ellie_chats.status),
			added_by   = CASE WHEN EXCLUDED.added_by <> 0 THEN EXCLUDED.added_by ELSE ellie_chats.added_by END,
			updated_at = now()`, c.ID, c.Type, c.Title, c.Status, c.AddedBy)
	if err != nil {
		return fmt.Errorf("database: save chat: %w", err)
	}
	return nil
}

func (p pgChats) SetBlocked(ctx context.Context, id int64, blocked bool) error {
	ctx, cancel := context.WithTimeout(ctx, pgTimeout)
	defer cancel()
	_, err := p.pool.Exec(ctx, `INSERT INTO ellie_chats (chat_id, blocked) VALUES ($1, $2)
		ON CONFLICT (chat_id) DO UPDATE SET blocked = EXCLUDED.blocked, updated_at = now()`, id, blocked)
	if err != nil {
		return fmt.Errorf("database: block chat: %w", err)
	}
	return nil
}

// lockChats creates any missing rows and locks them in ID order, so two
// transactions touching the same groups can never deadlock.
func lockChats(ctx context.Context, tx pgx.Tx, ids ...int64) (map[int64]ChatInfo, error) {
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		if _, err := tx.Exec(ctx, `INSERT INTO ellie_chats (chat_id) VALUES ($1) ON CONFLICT DO NOTHING`, id); err != nil {
			return nil, err
		}
	}
	rows, err := tx.Query(ctx, `SELECT `+chatCols+` FROM ellie_chats WHERE chat_id = ANY($1) ORDER BY chat_id FOR UPDATE`, ids)
	if err != nil {
		return nil, err
	}
	out := map[int64]ChatInfo{}
	var r chatRow
	_, err = pgx.ForEachRow(rows, r.dest(), func() error {
		c, err := r.info()
		out[c.ID] = c
		return err
	})
	return out, err
}

func saveUsage(ctx context.Context, tx pgx.Tx, c ChatInfo) error {
	_, err := tx.Exec(ctx, `UPDATE ellie_chats SET usage_day = $2, chars = $3, secs = $4, updated_at = now() WHERE chat_id = $1`,
		c.ID, c.Day, c.Chars, c.Secs)
	return err
}

func (p pgChats) Spend(ctx context.Context, id int64, day string, chars, secs int, lim GroupLimits) error {
	ctx, cancel := context.WithTimeout(ctx, pgTimeout)
	defer cancel()
	var refused error
	err := pgx.BeginFunc(ctx, p.pool, func(tx pgx.Tx) error {
		rows, err := lockChats(ctx, tx, id, p.total)
		if err != nil {
			return err
		}
		g, all := rows[id], rows[p.total]
		if refused = lim.fits(g, all, day, chars, secs); refused != nil {
			return nil
		}
		g.add(day, chars, secs)
		all.add(day, chars, secs)
		if err := saveUsage(ctx, tx, g); err != nil {
			return err
		}
		return saveUsage(ctx, tx, all)
	})
	if err != nil {
		return fmt.Errorf("database: spend group allowance: %w", err)
	}
	return refused
}

func (p pgChats) Refund(ctx context.Context, id int64, day string, chars, secs int) error {
	ctx, cancel := context.WithTimeout(ctx, pgTimeout)
	defer cancel()
	_, err := p.pool.Exec(ctx, `UPDATE ellie_chats SET chars = GREATEST(0, chars - $3), secs = GREATEST(0, secs - $4), updated_at = now()
		WHERE chat_id = ANY($1) AND usage_day = $2`, []int64{id, p.total}, day, chars, secs)
	if err != nil {
		return fmt.Errorf("database: refund group allowance: %w", err)
	}
	return nil
}

func (p pgChats) Migrate(ctx context.Context, from, to int64) error {
	if from == to {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, pgTimeout)
	defer cancel()
	err := pgx.BeginFunc(ctx, p.pool, func(tx pgx.Tx) error {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM ellie_chats WHERE chat_id = $1)`, from).Scan(&exists); err != nil || !exists {
			return err
		}
		rows, err := lockChats(ctx, tx, from, to)
		if err != nil {
			return err
		}
		dst := rows[to]
		merge(&dst, rows[from])
		dst.Type = "supergroup"
		raw, err := json.Marshal(dst.Settings)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE ellie_chats SET type = $2, title = $3, status = $4, blocked = $5, added_by = $6,
			usage_day = $7, chars = $8, secs = $9, settings = $10::jsonb, updated_at = now() WHERE chat_id = $1`,
			dst.ID, dst.Type, dst.Title, dst.Status, dst.Blocked, dst.AddedBy, dst.Day, dst.Chars, dst.Secs, string(raw)); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `DELETE FROM ellie_chats WHERE chat_id = $1`, from)
		return err
	})
	if err != nil {
		return fmt.Errorf("database: migrate chat: %w", err)
	}
	return nil
}

func (p pgChats) Busiest(ctx context.Context, day string, n int) ([]ChatInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, pgTimeout)
	defer cancel()
	rows, err := p.pool.Query(ctx, `SELECT `+chatCols+` FROM ellie_chats
		WHERE chat_id <> $3 AND usage_day = $1 AND (chars > 0 OR secs > 0)
		ORDER BY chars DESC, chat_id LIMIT $2`, day, n, p.total)
	if err != nil {
		return nil, fmt.Errorf("database: busiest chats: %w", err)
	}
	var out []ChatInfo
	var r chatRow
	if _, err := pgx.ForEachRow(rows, r.dest(), func() error {
		c, err := r.info()
		out = append(out, c)
		return err
	}); err != nil {
		return nil, fmt.Errorf("database: busiest chats: %w", err)
	}
	return out, nil
}
