package bot

// chats.go is the group registry: every group the bot is in, whether the
// studio owner blocked it, and how much of the shared daily allowance it has
// used. The row with ID allGroups tracks usage across every group, so one busy
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
)

// allGroups is the registry row that sums usage across every group. No
// Telegram chat has ID 0.
const allGroups int64 = 0

// Chat statuses.
const (
	chatMember = "member" // the bot is in the group
	chatLeft   = "left"   // the bot left or was removed
)

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
}

// usageOn returns the counts for day (zero when they belong to another day).
func (c ChatInfo) usageOn(day string) (chars, secs int) {
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
	errGroupLimit  = errors.New("groups: this group used today's allowance")
	errGroupsLimit = errors.New("groups: every group together used today's allowance")
)

// fits reports which cap, if any, adding chars and secs would pass.
func (l GroupLimits) fits(group, all ChatInfo, day string, chars, secs int) error {
	gc, gs := group.usageOn(day)
	ac, as := all.usageOn(day)
	switch {
	case l.Chars > 0 && chars > 0 && gc+chars > l.Chars, l.Secs > 0 && secs > 0 && gs+secs > l.Secs:
		return errGroupLimit
	case l.AllChars > 0 && chars > 0 && ac+chars > l.AllChars, l.AllSecs > 0 && secs > 0 && as+secs > l.AllSecs:
		return errGroupsLimit
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
	// Spend adds usage to a group and to the all-groups total, or refuses
	// with errGroupLimit / errGroupsLimit and changes nothing.
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
	total int64 // the row that sums every group (allGroups)
}

// OpenFileChats loads the registry from path ("" = memory only).
func OpenFileChats(path string) (Chats, error) {
	f := &fileChats{path: path, rows: map[int64]ChatInfo{}, total: allGroups}
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
