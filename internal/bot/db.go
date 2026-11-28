package bot

// db.go holds the Postgres-backed settings store, ownership registry, chat
// registry and metrics. The connection pool and migrations live in
// internal/store.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ---------------------------------------------------------------- settings

// NewSettingsStore opens the settings store. The database is the source of
// truth: reads query it, and each change is a transaction that locks only
// the users it touches. ctx is only used to check the connection.
func NewSettingsStore(ctx context.Context, d *DB, defaults Settings) (*Store, error) {
	ctx, cancel := context.WithTimeout(ctx, pgTimeout)
	defer cancel()
	if err := d.Pool().Ping(ctx); err != nil {
		return nil, fmt.Errorf("database: settings: %w", err)
	}
	p := pgSettings{d.Pool()}
	return &Store{backend: p, shared: p, defaults: defaults, data: map[int64]Settings{}}, nil
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
	ctx, cancel := context.WithTimeout(context.Background(), pgTimeout)
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
	ctx, cancel := context.WithTimeout(context.Background(), pgTimeout)
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
	ctx, cancel := context.WithTimeout(context.Background(), pgTimeout)
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
	ctx, cancel := context.WithTimeout(context.Background(), pgTimeout)
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

// ---------------------------------------------------------------- ownership

// Owners returns the ownership registry. It reads the database on every call
// (no cache), so instances running side by side during a deploy always agree.
func NewOwners(d *DB) Owners { return pgOwners{d.Pool()} }

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
func NewChats(d *DB) Chats { return pgChats{d.Pool(), allGroups} }

// Metrics returns the Postgres-backed usage events.
func NewMetrics(d *DB) Metrics { return pgMetrics{d.Pool()} }

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
