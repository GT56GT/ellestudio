package store

// owners.go records who created each voice and dictionary.
//
// Every Telegram user reaches Cartesia through the same API key, so Cartesia
// marks everything the bot creates as "owned by the account" and cannot tell
// users apart. The Owners registry is the single source of truth for that: it
// maps a Cartesia resource ID to the Telegram user who created it.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Kinds of resource a user can own.
const (
	KindVoice = "voice"
	KindDict  = "dict"
)

// ErrAlreadyOwned means a claim was made on a resource someone else owns.
var ErrAlreadyOwned = errors.New("ownership: resource belongs to another user")

// Owned is one resource in an owner's list.
type Owned struct {
	ID      string
	Created time.Time
}

// Cursor marks a position in an owner's list (newest first). The zero Cursor
// is the start of the list.
type Cursor struct {
	Created time.Time
	ID      string
}

func (c Cursor) IsZero() bool { return c.ID == "" }

// CursorOf is the position just after o.
func CursorOf(o Owned) Cursor { return Cursor{o.Created, o.ID} }

// String encodes the cursor for a browse session.
func (c Cursor) String() string {
	if c.IsZero() {
		return ""
	}
	return strconv.FormatInt(c.Created.UnixMicro(), 10) + "|" + c.ID
}

// ParseCursor reverses Cursor.String. Anything malformed is the start.
func ParseCursor(s string) Cursor {
	us, id, ok := strings.Cut(s, "|")
	n, err := strconv.ParseInt(us, 10, 64)
	if !ok || err != nil || id == "" {
		return Cursor{}
	}
	return Cursor{time.UnixMicro(n).UTC(), id}
}

// before reports whether a sorts before b in newest-first order.
func before(a, b Cursor) bool {
	if !a.Created.Equal(b.Created) {
		return a.Created.After(b.Created)
	}
	return a.ID > b.ID
}

// Owners is the ownership registry. Implementations must be safe for
// concurrent use; the Postgres one is also safe across bot instances.
type Owners interface {
	// Lookup returns the owner of each id that has one.
	Lookup(ctx context.Context, kind string, ids []string) (map[string]int64, error)
	// List returns up to limit resources owned by owner, newest first,
	// starting after the cursor.
	List(ctx context.Context, kind string, owner int64, after Cursor, limit int) ([]Owned, error)
	// Claim records owner as the creator of id. Claiming again for the same
	// owner is a no-op; claiming someone else's resource fails with
	// ErrAlreadyOwned.
	Claim(ctx context.Context, kind, id string, owner int64) error
	// Adopt records owner only if id has no owner yet, and reports whether it
	// did. It is used to import ownership from older versions of the bot.
	Adopt(ctx context.Context, kind, id string, owner int64) (bool, error)
	// Release forgets id, after the resource was deleted.
	Release(ctx context.Context, kind, id string) error
}

// ---------------------------------------------------------------- file / memory

type ownerKey struct{ kind, id string }

type ownerRow struct {
	Kind    string    `json:"kind"`
	ID      string    `json:"id"`
	Owner   int64     `json:"owner"`
	Created time.Time `json:"created"`
}

// fileOwners keeps the registry in memory and, when path is set, in a JSON
// file written atomically on every change. It is for local runs and tests;
// production uses Postgres.
type fileOwners struct {
	mu   sync.Mutex
	path string
	rows map[ownerKey]ownerRow
	now  func() time.Time
}

// OpenFileOwners loads the registry from path ("" = memory only).
func OpenFileOwners(path string) (Owners, error) {
	f := &fileOwners{path: path, rows: map[ownerKey]ownerRow{}, now: time.Now}
	if path == "" {
		return f, nil
	}
	raw, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return f, nil
	case err != nil:
		return nil, fmt.Errorf("owners: %w", err)
	}
	var rows []ownerRow
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, fmt.Errorf("owners: parse %s: %w", path, err)
	}
	for _, r := range rows {
		f.rows[ownerKey{r.Kind, r.ID}] = r
	}
	return f, nil
}

func (f *fileOwners) Lookup(_ context.Context, kind string, ids []string) (map[string]int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[string]int64{}
	for _, id := range ids {
		if r, ok := f.rows[ownerKey{kind, id}]; ok {
			out[id] = r.Owner
		}
	}
	return out, nil
}

func (f *fileOwners) List(_ context.Context, kind string, owner int64, after Cursor, limit int) ([]Owned, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var all []Owned
	for _, r := range f.rows {
		if r.Kind == kind && r.Owner == owner {
			o := Owned{r.ID, r.Created}
			if after.IsZero() || before(after, CursorOf(o)) {
				all = append(all, o)
			}
		}
	}
	sort.Slice(all, func(i, j int) bool { return before(CursorOf(all[i]), CursorOf(all[j])) })
	if len(all) > limit {
		all = all[:limit]
	}
	return all, nil
}

func (f *fileOwners) Claim(_ context.Context, kind, id string, owner int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := ownerKey{kind, id}
	if r, ok := f.rows[k]; ok {
		if r.Owner != owner {
			return ErrAlreadyOwned
		}
		return nil
	}
	f.rows[k] = ownerRow{kind, id, owner, f.now().UTC().Truncate(time.Microsecond)}
	if err := f.saveLocked(); err != nil {
		delete(f.rows, k)
		return err
	}
	return nil
}

func (f *fileOwners) Adopt(_ context.Context, kind, id string, owner int64) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := ownerKey{kind, id}
	if _, ok := f.rows[k]; ok {
		return false, nil
	}
	f.rows[k] = ownerRow{kind, id, owner, f.now().UTC().Truncate(time.Microsecond)}
	if err := f.saveLocked(); err != nil {
		delete(f.rows, k)
		return false, err
	}
	return true, nil
}

func (f *fileOwners) Release(_ context.Context, kind, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := ownerKey{kind, id}
	r, ok := f.rows[k]
	if !ok {
		return nil
	}
	delete(f.rows, k)
	if err := f.saveLocked(); err != nil {
		f.rows[k] = r
		return err
	}
	return nil
}

func (f *fileOwners) saveLocked() error {
	if f.path == "" {
		return nil
	}
	rows := make([]ownerRow, 0, len(f.rows))
	for _, r := range f.rows {
		rows = append(rows, r)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Kind != rows[j].Kind {
			return rows[i].Kind < rows[j].Kind
		}
		return rows[i].ID < rows[j].ID
	})
	raw, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(f.path, raw)
}

// ---------------------------------------------------------------- Postgres

// Owners returns the ownership registry. It reads the database on every call
// (no cache), so instances running side by side during a deploy always agree.
func (d *DB) Owners() Owners { return pgOwners{d.Pool()} }

type pgOwners struct{ pool *pgxpool.Pool }

func (p pgOwners) Lookup(ctx context.Context, kind string, ids []string) (map[string]int64, error) {
	out := map[string]int64{}
	if len(ids) == 0 {
		return out, nil
	}
	ctx, cancel := context.WithTimeout(ctx, PgTimeout)
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
	ctx, cancel := context.WithTimeout(ctx, PgTimeout)
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
	ctx, cancel := context.WithTimeout(ctx, PgTimeout)
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
		return ErrAlreadyOwned
	}
	return nil
}

func (p pgOwners) Adopt(ctx context.Context, kind, id string, owner int64) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, PgTimeout)
	defer cancel()
	tag, err := p.pool.Exec(ctx, `INSERT INTO ellie_ownership (kind, resource_id, owner_id) VALUES ($1, $2, $3)
		ON CONFLICT (kind, resource_id) DO NOTHING`, kind, id, owner)
	if err != nil {
		return false, fmt.Errorf("database: adopt: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

func (p pgOwners) Release(ctx context.Context, kind, id string) error {
	ctx, cancel := context.WithTimeout(ctx, PgTimeout)
	defer cancel()
	if _, err := p.pool.Exec(ctx, `DELETE FROM ellie_ownership WHERE kind = $1 AND resource_id = $2`, kind, id); err != nil {
		return fmt.Errorf("database: release: %w", err)
	}
	return nil
}
