package admin

// store.go is what the panel reads and writes: its own sign-in sessions and
// audit log, and read-only views of the bot's data in the shared database.

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"ellestudio/internal/bot"
)

// Session is one signed-in browser or Telegram app.
type Session struct {
	ID        string    `json:"id"` // short public handle, for revoking
	AdminID   int64     `json:"-"`
	Name      string    `json:"-"` // the admin's Telegram name at sign-in
	CreatedAt time.Time `json:"created_at"`
	LastSeen  time.Time `json:"last_seen"`
	ExpiresAt time.Time `json:"expires_at"`
	UserAgent string    `json:"user_agent"`
	IP        string    `json:"ip"`
	Current   bool      `json:"current"`
}

// sessionID is the public handle of a session: the start of its token hash.
func sessionID(hash []byte) string { return hex.EncodeToString(hash[:8]) }

// AuditEntry is one recorded admin action.
type AuditEntry struct {
	ID      int64           `json:"id"`
	At      time.Time       `json:"at"`
	AdminID int64           `json:"admin_id"`
	Action  string          `json:"action"`
	Target  string          `json:"target"`
	Detail  json.RawMessage `json:"detail"`
	IP      string          `json:"ip"`
}

// Day is one UTC day of usage.
type Day struct {
	Day          string `json:"day"`
	Active       int    `json:"active"`
	New          int    `json:"new"`
	FromGroups   int    `json:"from_groups"`
	FromInvites  int    `json:"from_invites"`
	Readings     int    `json:"readings"`
	Chars        int    `json:"chars"`
	LimitUsers   int    `json:"limit_users"`
	GuestInvites int    `json:"guest_invites"`
}

// Overview is the dashboard.
type Overview struct {
	Bot         *bot.Status `json:"bot"` // nil: the bot has never reported
	BotOnline   bool        `json:"bot_online"`
	StudioChars int         `json:"studio_chars"` // used today, everyone together
	StudioSecs  int         `json:"studio_secs"`
	GroupsChars int         `json:"groups_chars"` // used today in all groups
	Days        []Day       `json:"days"`         // oldest first
	WeekActive  int         `json:"week_active"`
	Users       int         `json:"users"`
	Joined      int         `json:"joined"`
	Groups      int         `json:"groups"`
}

// UserRow is one user in the users list.
type UserRow struct {
	ID          int64     `json:"id"`
	Joined      bool      `json:"joined"`
	JoinedAt    string    `json:"joined_at"`
	Voice       string    `json:"voice"`
	Readings    int       `json:"readings"`
	SpokenChars int       `json:"spoken_chars"`
	Bonus       int       `json:"bonus"`
	Referrals   int       `json:"referrals"`
	TodayChars  int       `json:"today_chars"`
	TodaySecs   int       `json:"today_secs"`
	Banned      bool      `json:"banned"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// UserDetail is one user's page.
type UserDetail struct {
	UserRow
	ReferredBy int64         `json:"referred_by"`
	Ban        *bot.Ban      `json:"ban,omitempty"`
	Language   string        `json:"language"`
	Model      string        `json:"model"`
	Emotion    string        `json:"emotion"`
	Voices     int           `json:"voices"`
	Dicts      int           `json:"dicts"`
	Activity   []ActivityDay `json:"activity"` // last 30 days, oldest first
}

// ActivityDay is one user's readings on one day.
type ActivityDay struct {
	Day      string `json:"day"`
	Readings int    `json:"readings"`
	Chars    int    `json:"chars"`
}

// GroupRow is one group in the groups list.
type GroupRow struct {
	ID         int64           `json:"id"`
	Type       string          `json:"type"`
	Title      string          `json:"title"`
	Status     string          `json:"status"`
	Blocked    bool            `json:"blocked"`
	AddedBy    int64           `json:"added_by"`
	TodayChars int             `json:"today_chars"`
	TodaySecs  int             `json:"today_secs"`
	Settings   json.RawMessage `json:"settings"`
	CreatedAt  time.Time       `json:"created_at"`
}

// UserQuery filters and orders the users list.
type UserQuery struct {
	ID     int64  // 0 = everyone
	Sort   string // "recent" (default), "readings" or "bonus"
	Banned bool   // only users banned now
	Offset int
	Limit  int
}

// Store is everything the panel needs from the database.
type Store interface {
	CreateSession(ctx context.Context, hash []byte, s Session) error
	// LookupSession finds a live session (not expired, not idle past
	// idle) and marks it seen now.
	LookupSession(ctx context.Context, hash []byte, now time.Time, idle time.Duration) (Session, bool, error)
	DeleteSession(ctx context.Context, hash []byte) error
	DeleteSessionByID(ctx context.Context, adminID int64, id string) (bool, error)
	DeleteOtherSessions(ctx context.Context, adminID int64, keep []byte) (int, error)
	Sessions(ctx context.Context, adminID int64, current []byte) ([]Session, error)
	PruneSessions(ctx context.Context, now time.Time) error

	Audit(ctx context.Context, e AuditEntry) error
	AuditLog(ctx context.Context, before int64, limit int) ([]AuditEntry, error)

	// BotStatus is the latest heartbeat (nil if none) and whether it is recent.
	BotStatus(ctx context.Context, now time.Time) (*bot.Status, bool, error)
	Overview(ctx context.Context, now time.Time) (Overview, error)
	Users(ctx context.Context, q UserQuery, today string) ([]UserRow, int, error)
	User(ctx context.Context, id int64, today string) (UserDetail, bool, error)
	Groups(ctx context.Context, offset, limit int, today string) ([]GroupRow, int, error)

	Actions
	Live
}

// botOnlineWithin is how recent a heartbeat must be to call the bot online.
const botOnlineWithin = time.Minute

// ---------------------------------------------------------------- Postgres

// PGStore is the Store on the bot's Postgres database.
type PGStore struct {
	db       *bot.DB
	settings *bot.Store // the bot's own settings store, for user changes
}

// NewPGStore uses db (opened, and migrated, with bot.OpenDatabase) and
// settings (db.SettingsStore), so user changes take the same per-user locks
// as the bot.
func NewPGStore(db *bot.DB, settings *bot.Store) *PGStore { return &PGStore{db, settings} }

func (p *PGStore) CreateSession(ctx context.Context, hash []byte, s Session) error {
	_, err := p.db.Pool().Exec(ctx, `INSERT INTO ellie_admin_sessions
		(token_hash, admin_id, name, created_at, last_seen, expires_at, user_agent, ip) VALUES ($1, $2, $3, $4, $4, $5, $6, $7)`,
		hash, s.AdminID, s.Name, s.CreatedAt, s.ExpiresAt, s.UserAgent, s.IP)
	if err != nil {
		return fmt.Errorf("admin: create session: %w", err)
	}
	return nil
}

func (p *PGStore) LookupSession(ctx context.Context, hash []byte, now time.Time, idle time.Duration) (Session, bool, error) {
	s := Session{ID: sessionID(hash)}
	err := p.db.Pool().QueryRow(ctx, `UPDATE ellie_admin_sessions SET last_seen = $2
		WHERE token_hash = $1 AND expires_at > $2 AND last_seen > $3
		RETURNING admin_id, name, created_at, last_seen, expires_at, user_agent, ip`, hash, now, now.Add(-idle)).
		Scan(&s.AdminID, &s.Name, &s.CreatedAt, &s.LastSeen, &s.ExpiresAt, &s.UserAgent, &s.IP)
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, false, nil
	}
	if err != nil {
		return Session{}, false, fmt.Errorf("admin: look up session: %w", err)
	}
	return s, true, nil
}

func (p *PGStore) DeleteSession(ctx context.Context, hash []byte) error {
	if _, err := p.db.Pool().Exec(ctx, `DELETE FROM ellie_admin_sessions WHERE token_hash = $1`, hash); err != nil {
		return fmt.Errorf("admin: delete session: %w", err)
	}
	return nil
}

func (p *PGStore) DeleteSessionByID(ctx context.Context, adminID int64, id string) (bool, error) {
	tag, err := p.db.Pool().Exec(ctx, `DELETE FROM ellie_admin_sessions
		WHERE admin_id = $1 AND encode(substring(token_hash FROM 1 FOR 8), 'hex') = $2`, adminID, id)
	if err != nil {
		return false, fmt.Errorf("admin: revoke session: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

func (p *PGStore) DeleteOtherSessions(ctx context.Context, adminID int64, keep []byte) (int, error) {
	tag, err := p.db.Pool().Exec(ctx, `DELETE FROM ellie_admin_sessions WHERE admin_id = $1 AND token_hash <> $2`, adminID, keep)
	if err != nil {
		return 0, fmt.Errorf("admin: revoke sessions: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

func (p *PGStore) Sessions(ctx context.Context, adminID int64, current []byte) ([]Session, error) {
	rows, err := p.db.Pool().Query(ctx, `SELECT token_hash, created_at, last_seen, expires_at, user_agent, ip
		FROM ellie_admin_sessions WHERE admin_id = $1 AND expires_at > now() ORDER BY last_seen DESC`, adminID)
	if err != nil {
		return nil, fmt.Errorf("admin: list sessions: %w", err)
	}
	var out []Session
	var hash []byte
	var s Session
	_, err = pgx.ForEachRow(rows, []any{&hash, &s.CreatedAt, &s.LastSeen, &s.ExpiresAt, &s.UserAgent, &s.IP}, func() error {
		s.ID, s.Current = sessionID(hash), string(hash) == string(current)
		out = append(out, s)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("admin: list sessions: %w", err)
	}
	return out, nil
}

func (p *PGStore) PruneSessions(ctx context.Context, now time.Time) error {
	_, err := p.db.Pool().Exec(ctx, `DELETE FROM ellie_admin_sessions WHERE expires_at < $1`, now)
	return err
}

func (p *PGStore) Audit(ctx context.Context, e AuditEntry) error {
	detail := e.Detail
	if len(detail) == 0 {
		detail = json.RawMessage(`{}`)
	}
	_, err := p.db.Pool().Exec(ctx, `INSERT INTO ellie_admin_audit (admin_id, action, target, detail, ip)
		VALUES ($1, $2, $3, $4, $5)`, e.AdminID, e.Action, e.Target, detail, e.IP)
	if err != nil {
		return fmt.Errorf("admin: audit: %w", err)
	}
	return nil
}

func (p *PGStore) AuditLog(ctx context.Context, before int64, limit int) ([]AuditEntry, error) {
	rows, err := p.db.Pool().Query(ctx, `SELECT id, at, admin_id, action, target, detail, ip FROM ellie_admin_audit
		WHERE ($1 = 0 OR id < $1) ORDER BY id DESC LIMIT $2`, before, limit)
	if err != nil {
		return nil, fmt.Errorf("admin: audit log: %w", err)
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (AuditEntry, error) {
		var e AuditEntry
		var detail []byte
		err := r.Scan(&e.ID, &e.At, &e.AdminID, &e.Action, &e.Target, &detail, &e.IP)
		e.Detail = detail
		return e, err
	})
	if err != nil {
		return nil, fmt.Errorf("admin: audit log: %w", err)
	}
	return out, nil
}

func (p *PGStore) BotStatus(ctx context.Context, now time.Time) (*bot.Status, bool, error) {
	st, ok, err := bot.LatestStatus(ctx, p.db.Pool())
	if err != nil || !ok {
		return nil, false, err
	}
	return &st, now.Sub(st.At) < botOnlineWithin, nil
}

func (p *PGStore) Overview(ctx context.Context, now time.Time) (Overview, error) {
	var o Overview
	pool := p.db.Pool()
	today := now.UTC().Format("2006-01-02")

	var err error
	if o.Bot, o.BotOnline, err = p.BotStatus(ctx, now); err != nil {
		return o, err
	}
	if o.StudioChars, o.StudioSecs, err = p.db.Budget().Used(ctx, today); err != nil {
		return o, err
	}
	if err := pool.QueryRow(ctx, `SELECT
			(SELECT count(*) FROM ellie_user_settings),
			(SELECT count(*) FROM ellie_user_settings WHERE (settings->>'joined')::boolean),
			(SELECT count(*) FROM ellie_chats WHERE chat_id <> 0 AND NOT blocked AND status = 'member'),
			(SELECT COALESCE(max(chars) FILTER (WHERE usage_day = $1), 0) FROM ellie_chats WHERE chat_id = 0)`, today).
		Scan(&o.Users, &o.Joined, &o.Groups, &o.GroupsChars); err != nil {
		return o, fmt.Errorf("admin: counts: %w", err)
	}

	m := p.db.Metrics()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -29)
	days, err := m.Days(ctx, start)
	if err != nil {
		return o, err
	}
	byDay := map[string]bot.DayStats{}
	for _, d := range days {
		byDay[d.Day] = d
	}
	for d := start; !d.After(now); d = d.AddDate(0, 0, 1) {
		k := d.Format("2006-01-02")
		s := byDay[k]
		o.Days = append(o.Days, Day{
			Day: k, Active: s.Active, New: s.New, FromGroups: s.FromGroups, FromInvites: s.FromInvites,
			Readings: s.Readings, Chars: s.Chars, LimitUsers: s.LimitUsers, GuestInvites: s.GuestInvites,
		})
	}
	if o.WeekActive, err = m.Active(ctx, now.AddDate(0, 0, -7)); err != nil {
		return o, err
	}
	return o, nil
}

// userOrder maps a sort name to SQL; only these strings ever reach a query.
var userOrder = map[string]string{
	"recent":   "updated_at DESC",
	"readings": "COALESCE((settings->>'readings')::int, 0) DESC, updated_at DESC",
	"bonus":    "COALESCE((settings->>'bonus')::int, 0) DESC, updated_at DESC",
}

func userRow(id int64, v bot.Settings, updated time.Time, today string) UserRow {
	r := UserRow{
		ID: id, Joined: v.Joined, JoinedAt: v.JoinedAt, Voice: v.VoiceName, Readings: v.Readings,
		SpokenChars: v.SpokenChars, Bonus: v.Bonus, Referrals: v.Referrals, UpdatedAt: updated,
		Banned: v.Ban.Active(time.Now()),
	}
	if v.UsageDay == today {
		r.TodayChars, r.TodaySecs = v.UsedChars, v.UsedSecs
	}
	return r
}

func (p *PGStore) Users(ctx context.Context, q UserQuery, today string) ([]UserRow, int, error) {
	order, ok := userOrder[q.Sort]
	if !ok {
		order = userOrder["recent"]
	}
	rows, err := p.db.Pool().Query(ctx, `SELECT user_id, settings, updated_at, count(*) OVER ()
		FROM ellie_user_settings WHERE ($1 = 0 OR user_id = $1)
		  AND (NOT $4 OR (jsonb_typeof(settings->'ban') = 'object'
		       AND (settings->'ban'->'until' IS NULL OR (settings->'ban'->>'until')::timestamptz > now())))
		ORDER BY `+order+` LIMIT $2 OFFSET $3`, q.ID, q.Limit, q.Offset, q.Banned)
	if err != nil {
		return nil, 0, fmt.Errorf("admin: users: %w", err)
	}
	var out []UserRow
	var total int
	var id int64
	var raw []byte
	var updated time.Time
	_, err = pgx.ForEachRow(rows, []any{&id, &raw, &updated, &total}, func() error {
		var v bot.Settings
		if err := json.Unmarshal(raw, &v); err != nil {
			return fmt.Errorf("user %d: %w", id, err)
		}
		out = append(out, userRow(id, v, updated, today))
		return nil
	})
	if err != nil {
		return nil, 0, fmt.Errorf("admin: users: %w", err)
	}
	return out, total, nil
}

func (p *PGStore) User(ctx context.Context, id int64, today string) (UserDetail, bool, error) {
	pool := p.db.Pool()
	var raw []byte
	var updated time.Time
	err := pool.QueryRow(ctx, `SELECT settings, updated_at FROM ellie_user_settings WHERE user_id = $1`, id).Scan(&raw, &updated)
	if errors.Is(err, pgx.ErrNoRows) {
		return UserDetail{}, false, nil
	}
	if err != nil {
		return UserDetail{}, false, fmt.Errorf("admin: user: %w", err)
	}
	var v bot.Settings
	if err := json.Unmarshal(raw, &v); err != nil {
		return UserDetail{}, false, fmt.Errorf("admin: user %d: %w", id, err)
	}
	d := UserDetail{
		UserRow: userRow(id, v, updated, today), ReferredBy: v.ReferredBy, Ban: v.Ban,
		Language: v.Language, Model: v.Model, Emotion: v.Emotion,
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FILTER (WHERE kind = 'voice'), count(*) FILTER (WHERE kind = 'dict')
		FROM ellie_ownership WHERE owner_id = $1`, id).Scan(&d.Voices, &d.Dicts); err != nil {
		return UserDetail{}, false, fmt.Errorf("admin: user ownership: %w", err)
	}
	rows, err := pool.Query(ctx, `SELECT to_char(date_trunc('day', at AT TIME ZONE 'UTC'), 'YYYY-MM-DD') AS day,
			count(*), COALESCE(sum(n), 0)
		FROM ellie_events WHERE user_id = $1 AND kind = 'reading' AND at > now() - interval '30 days'
		GROUP BY day ORDER BY day`, id)
	if err != nil {
		return UserDetail{}, false, fmt.Errorf("admin: user activity: %w", err)
	}
	var a ActivityDay
	if _, err := pgx.ForEachRow(rows, []any{&a.Day, &a.Readings, &a.Chars}, func() error {
		d.Activity = append(d.Activity, a)
		return nil
	}); err != nil {
		return UserDetail{}, false, fmt.Errorf("admin: user activity: %w", err)
	}
	return d, true, nil
}

func (p *PGStore) Groups(ctx context.Context, offset, limit int, today string) ([]GroupRow, int, error) {
	rows, err := p.db.Pool().Query(ctx, `SELECT chat_id, type, title, status, blocked, added_by,
			CASE WHEN usage_day = $1 THEN chars ELSE 0 END, CASE WHEN usage_day = $1 THEN secs ELSE 0 END,
			settings, created_at, count(*) OVER ()
		FROM ellie_chats WHERE chat_id <> 0
		ORDER BY (usage_day = $1) DESC, chars DESC, updated_at DESC LIMIT $2 OFFSET $3`, today, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("admin: groups: %w", err)
	}
	var out []GroupRow
	var total int
	var g GroupRow
	var settings []byte
	_, err = pgx.ForEachRow(rows, []any{&g.ID, &g.Type, &g.Title, &g.Status, &g.Blocked, &g.AddedBy,
		&g.TodayChars, &g.TodaySecs, &settings, &g.CreatedAt, &total}, func() error {
		g.Settings = append(json.RawMessage(nil), settings...)
		out = append(out, g)
		return nil
	})
	if err != nil {
		return nil, 0, fmt.Errorf("admin: groups: %w", err)
	}
	return out, total, nil
}
