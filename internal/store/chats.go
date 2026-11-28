package store

// chats.go is the group registry: every group the bot is in, whether the
// studio owner blocked it, and how much of the shared daily allowance it has
// used. The row with ID AllGroups tracks usage across every group, so one busy
// public group cannot spend the whole Cartesia budget.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AllGroups is the registry row that sums usage across every group. No
// Telegram chat has ID 0.
const AllGroups int64 = 0

// Chat statuses.
const (
	ChatMember = "member" // the bot is in the group
	ChatLeft   = "left"   // the bot left or was removed
)

// GroupSettings are the choices a group's own admins make. The zero value
// is the default: everything on, for everyone, within the studio's limits.
type GroupSettings struct {
	Paused       bool `json:"paused,omitempty"`        // Ellie ignores requests
	AdminsOnly   bool `json:"admins_only,omitempty"`   // only group admins may use it
	NoSpeak      bool `json:"no_speak,omitempty"`      // /say is off
	NoTranscribe bool `json:"no_transcribe,omitempty"` // /transcribe is off
	// DailyChars lowers the group's shared daily allowance (0 = the
	// studio's). It can never raise it.
	DailyChars int `json:"daily_chars,omitempty"`
	// MaxChars lowers the longest text one /say may read (0 = the studio's).
	MaxChars int `json:"max_chars,omitempty"`
}

// ChatInfo is one group in the registry.
type ChatInfo struct {
	ID      int64  `json:"id"`
	Type    string `json:"type"`
	Title   string `json:"title"`
	Status  string `json:"status"`
	Blocked bool   `json:"blocked"`
	AddedBy int64  `json:"added_by"`
	Day     string `json:"day"` // UTC day the usage counts belong to
	Chars   int    `json:"chars"`
	Secs    int    `json:"secs"`

	Settings GroupSettings `json:"settings"`
}

// UsageOn returns the counts for day (zero when they belong to another day).
func (c ChatInfo) UsageOn(day string) (chars, secs int) {
	if c.Day != day {
		return 0, 0
	}
	return c.Chars, c.Secs
}

// GroupLimits are the daily caps for one group and for all groups together.
// Zero means unlimited.
type GroupLimits struct {
	Chars, Secs       int // per group
	AllChars, AllSecs int // every group combined
}

// Why a group spend was refused.
var (
	ErrGroupLimit  = errors.New("groups: this group used today's allowance")
	ErrGroupsLimit = errors.New("groups: every group together used today's allowance")
)

// fits reports which cap, if any, adding chars and secs would pass.
func (l GroupLimits) fits(group, all ChatInfo, day string, chars, secs int) error {
	gc, gs := group.UsageOn(day)
	ac, as := all.UsageOn(day)
	switch {
	case l.Chars > 0 && chars > 0 && gc+chars > l.Chars, l.Secs > 0 && secs > 0 && gs+secs > l.Secs:
		return ErrGroupLimit
	case l.AllChars > 0 && chars > 0 && ac+chars > l.AllChars, l.AllSecs > 0 && secs > 0 && as+secs > l.AllSecs:
		return ErrGroupsLimit
	}
	return nil
}

// add moves c's counts to day and adds the deltas, never going below zero.
func (c *ChatInfo) add(day string, chars, secs int) {
	if c.Day != day {
		c.Day, c.Chars, c.Secs = day, 0, 0
	}
	c.Chars, c.Secs = max(0, c.Chars+chars), max(0, c.Secs+secs)
}

// Chats is the group registry. Implementations must be safe for concurrent
// use; the Postgres one is also safe across bot instances.
type Chats interface {
	// Get returns a group, and false if it has never been seen.
	Get(ctx context.Context, id int64) (ChatInfo, bool, error)
	// Upsert records a group. Empty Type, Title and Status and a zero
	// AddedBy keep what is stored; Blocked and usage are never changed.
	Upsert(ctx context.Context, c ChatInfo) error
	// SetBlocked blocks or unblocks a group, creating it if needed.
	SetBlocked(ctx context.Context, id int64, blocked bool) error
	// SetSettings saves a group's own settings, creating it if needed.
	SetSettings(ctx context.Context, id int64, s GroupSettings) error
	// Spend adds usage to a group and to the all-groups total, or refuses
	// with ErrGroupLimit / ErrGroupsLimit and changes nothing.
	Spend(ctx context.Context, id int64, day string, chars, secs int, lim GroupLimits) error
	// Refund takes usage back from a group and the total.
	Refund(ctx context.Context, id int64, day string, chars, secs int) error
	// Migrate moves a group to its new ID after Telegram upgrades it to a
	// supergroup, keeping its block and today's usage.
	Migrate(ctx context.Context, from, to int64) error
	// Busiest lists up to n groups by usage on day, most characters first.
	Busiest(ctx context.Context, day string, n int) ([]ChatInfo, error)
}

// merge folds a group's old row into its new one.
func merge(dst *ChatInfo, src ChatInfo) {
	dst.Blocked = dst.Blocked || src.Blocked
	dst.Type, dst.Title = orDefault(dst.Type, src.Type), orDefault(dst.Title, src.Title)
	dst.Status = orDefault(dst.Status, src.Status)
	if dst.AddedBy == 0 {
		dst.AddedBy = src.AddedBy
	}
	if dst.Settings == (GroupSettings{}) {
		dst.Settings = src.Settings
	}
	switch {
	case dst.Day == src.Day:
		dst.Chars += src.Chars
		dst.Secs += src.Secs
	case src.Day > dst.Day:
		dst.Day, dst.Chars, dst.Secs = src.Day, src.Chars, src.Secs
	}
}

// ---------------------------------------------------------------- file / memory

// fileChats keeps the registry in memory and, when path is set, in a JSON
// file written atomically on every change. Production uses Postgres.
type fileChats struct {
	mu    sync.Mutex
	path  string
	rows  map[int64]ChatInfo
	total int64 // the row that sums every group (AllGroups)
}

// OpenFileChats loads the registry from path ("" = memory only).
func OpenFileChats(path string) (Chats, error) {
	f := &fileChats{path: path, rows: map[int64]ChatInfo{}, total: AllGroups}
	if path == "" {
		return f, nil
	}
	raw, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return f, nil
	case err != nil:
		return nil, fmt.Errorf("chats: %w", err)
	}
	var rows []ChatInfo
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, fmt.Errorf("chats: parse %s: %w", path, err)
	}
	for _, r := range rows {
		f.rows[r.ID] = r
	}
	return f, nil
}

func (f *fileChats) Get(_ context.Context, id int64) (ChatInfo, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.rows[id]
	return c, ok, nil
}

// change applies fn to a copy of the rows and keeps it only if it saved.
func (f *fileChats) change(fn func(rows map[int64]ChatInfo) error) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	next := make(map[int64]ChatInfo, len(f.rows)+1)
	for k, v := range f.rows {
		next[k] = v
	}
	if err := fn(next); err != nil {
		return err
	}
	if err := f.save(next); err != nil {
		return err
	}
	f.rows = next
	return nil
}

func (f *fileChats) Upsert(_ context.Context, c ChatInfo) error {
	return f.change(func(rows map[int64]ChatInfo) error {
		cur, ok := rows[c.ID]
		if !ok {
			cur = ChatInfo{ID: c.ID}
		}
		cur.Type, cur.Title, cur.Status = orDefault(c.Type, cur.Type), orDefault(c.Title, cur.Title), orDefault(c.Status, cur.Status)
		if c.AddedBy != 0 {
			cur.AddedBy = c.AddedBy
		}
		rows[c.ID] = cur
		return nil
	})
}

func (f *fileChats) SetBlocked(_ context.Context, id int64, blocked bool) error {
	return f.change(func(rows map[int64]ChatInfo) error {
		cur := rows[id]
		cur.ID, cur.Blocked = id, blocked
		rows[id] = cur
		return nil
	})
}

func (f *fileChats) SetSettings(_ context.Context, id int64, gs GroupSettings) error {
	return f.change(func(rows map[int64]ChatInfo) error {
		cur := rows[id]
		cur.ID, cur.Settings = id, gs
		rows[id] = cur
		return nil
	})
}

func (f *fileChats) Spend(_ context.Context, id int64, day string, chars, secs int, lim GroupLimits) error {
	return f.change(func(rows map[int64]ChatInfo) error {
		g, all := rows[id], rows[f.total]
		g.ID, all.ID, all.Type = id, f.total, "total"
		if err := lim.fits(g, all, day, chars, secs); err != nil {
			return err
		}
		g.add(day, chars, secs)
		all.add(day, chars, secs)
		rows[id], rows[f.total] = g, all
		return nil
	})
}

func (f *fileChats) Refund(_ context.Context, id int64, day string, chars, secs int) error {
	return f.change(func(rows map[int64]ChatInfo) error {
		for _, k := range []int64{id, f.total} {
			if c, ok := rows[k]; ok && c.Day == day {
				c.add(day, -chars, -secs)
				rows[k] = c
			}
		}
		return nil
	})
}

func (f *fileChats) Migrate(_ context.Context, from, to int64) error {
	return f.change(func(rows map[int64]ChatInfo) error {
		old, ok := rows[from]
		if !ok || from == to {
			return nil
		}
		dst, ok := rows[to]
		if !ok {
			dst = ChatInfo{ID: to}
		}
		merge(&dst, old)
		dst.Type = "supergroup"
		rows[to] = dst
		delete(rows, from)
		return nil
	})
}

func (f *fileChats) Busiest(_ context.Context, day string, n int) ([]ChatInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []ChatInfo
	for _, c := range f.rows {
		if c.ID != f.total && c.Day == day && (c.Chars > 0 || c.Secs > 0) {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Chars != out[j].Chars {
			return out[i].Chars > out[j].Chars
		}
		return out[i].ID < out[j].ID
	})
	if len(out) > n {
		out = out[:n]
	}
	return out, nil
}

func (f *fileChats) save(rows map[int64]ChatInfo) error {
	if f.path == "" {
		return nil
	}
	list := make([]ChatInfo, 0, len(rows))
	for _, r := range rows {
		list = append(list, r)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].ID < list[j].ID })
	raw, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(f.path, raw)
}

// ---------------------------------------------------------------- Postgres

// Chats returns the group registry. Like Owners it reads the database on
// every call, so instances running side by side during a deploy agree.
func (d *DB) Chats() Chats { return pgChats{d.Pool(), AllGroups} }

type pgChats struct {
	pool  *pgxpool.Pool
	total int64 // the row that sums every group (AllGroups; tests use their own)
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
	ctx, cancel := context.WithTimeout(ctx, PgTimeout)
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
	ctx, cancel := context.WithTimeout(ctx, PgTimeout)
	defer cancel()
	if _, err := p.pool.Exec(ctx, `INSERT INTO ellie_chats (chat_id, settings) VALUES ($1, $2::jsonb)
		ON CONFLICT (chat_id) DO UPDATE SET settings = EXCLUDED.settings, updated_at = now()`, id, string(raw)); err != nil {
		return fmt.Errorf("database: save group settings: %w", err)
	}
	return nil
}

func (p pgChats) Upsert(ctx context.Context, c ChatInfo) error {
	ctx, cancel := context.WithTimeout(ctx, PgTimeout)
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
	ctx, cancel := context.WithTimeout(ctx, PgTimeout)
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
	ctx, cancel := context.WithTimeout(ctx, PgTimeout)
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
	ctx, cancel := context.WithTimeout(ctx, PgTimeout)
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
	ctx, cancel := context.WithTimeout(ctx, PgTimeout)
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
	ctx, cancel := context.WithTimeout(ctx, PgTimeout)
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

// orDefault returns s, or d when s is empty.
func orDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}
