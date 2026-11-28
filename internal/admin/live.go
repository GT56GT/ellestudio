package admin

// live.go is the panel's control room: limits that take effect without a
// redeploy, the emergency pause, and announcements to everyone. The bot
// picks up limits and the pause within 30 seconds and sends broadcasts
// itself, at a gentle pace.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"ellestudio/internal/store"
)

// ErrBusy means another broadcast is already queued or sending.
var ErrBusy = errors.New("admin: a broadcast is already going out")

// BroadcastRow is one announcement.
type BroadcastRow struct {
	ID         int64      `json:"id"`
	Text       string     `json:"text"`
	Audience   string     `json:"audience"`
	CreatedBy  int64      `json:"created_by"`
	Status     string     `json:"status"`
	Total      int        `json:"total"`
	Sent       int        `json:"sent"`
	Failed     int        `json:"failed"`
	CreatedAt  time.Time  `json:"created_at"`
	StartedAt  *time.Time `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`
}

// Live is the control-room side of the Store.
type Live interface {
	Overrides(ctx context.Context) (store.LimitOverrides, error)
	SaveOverrides(ctx context.Context, o store.LimitOverrides, by int64) (before store.LimitOverrides, err error)
	Broadcasts(ctx context.Context, limit int) ([]BroadcastRow, error)
	AudienceSize(ctx context.Context, audience string) (int, error)
	CreateBroadcast(ctx context.Context, text, audience string, by int64) (int64, error)
	CancelBroadcast(ctx context.Context, id int64) (bool, error)
}

// ---------------------------------------------------------------- Postgres

func (p *PGStore) Overrides(ctx context.Context) (store.LimitOverrides, error) {
	return store.LoadOverrides(ctx, p.db.Pool())
}

func (p *PGStore) SaveOverrides(ctx context.Context, o store.LimitOverrides, by int64) (store.LimitOverrides, error) {
	return store.SaveOverrides(ctx, p.db.Pool(), o, by)
}

func (p *PGStore) Broadcasts(ctx context.Context, limit int) ([]BroadcastRow, error) {
	rows, err := p.db.Pool().Query(ctx, `SELECT id, text, audience, created_by, status, total, sent, failed,
		created_at, started_at, finished_at FROM ellie_broadcasts ORDER BY id DESC LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("admin: broadcasts: %w", err)
	}
	out, err := pgx.CollectRows(rows, pgx.RowToStructByPos[BroadcastRow])
	if err != nil {
		return nil, fmt.Errorf("admin: broadcasts: %w", err)
	}
	return out, nil
}

func (p *PGStore) AudienceSize(ctx context.Context, audience string) (int, error) {
	return store.CountRecipients(ctx, p.db.Pool(), audience)
}

func (p *PGStore) CreateBroadcast(ctx context.Context, text, audience string, by int64) (int64, error) {
	var id int64
	err := p.db.Pool().QueryRow(ctx, `INSERT INTO ellie_broadcasts (text, audience, created_by) VALUES ($1, $2, $3) RETURNING id`,
		text, audience, by).Scan(&id)
	var pe *pgconn.PgError
	if errors.As(err, &pe) && pe.Code == "23505" { // the one-open-broadcast index
		return 0, ErrBusy
	}
	if err != nil {
		return 0, fmt.Errorf("admin: create broadcast: %w", err)
	}
	return id, nil
}

func (p *PGStore) CancelBroadcast(ctx context.Context, id int64) (bool, error) {
	tag, err := p.db.Pool().Exec(ctx, `UPDATE ellie_broadcasts SET status = 'cancelled', finished_at = now(), updated_at = now()
		WHERE id = $1 AND status IN ('queued', 'sending')`, id)
	if err != nil {
		return false, fmt.Errorf("admin: cancel broadcast: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// ---------------------------------------------------------------- API

func (s *Server) settings(w http.ResponseWriter, r *http.Request, _ authCtx) {
	o, err := s.store.Overrides(r.Context())
	if err != nil {
		s.fail(w, "settings", err)
		return
	}
	st, online, err := s.store.BotStatus(r.Context(), s.now().UTC())
	if err != nil {
		s.fail(w, "settings", err)
		return
	}
	out := map[string]any{"overrides": o, "bot_online": online, "defaults": nil, "limits": nil}
	if st != nil {
		out["defaults"], out["limits"] = st.Defaults, st.Limits
	}
	writeJSON(w, out)
}

// limitsIn is the body of a limits change.
type limitsIn struct {
	Reason    string               `json:"reason"`
	Overrides store.LimitOverrides `json:"overrides"`
}

func (s *Server) saveSettings(w http.ResponseWriter, r *http.Request, a authCtx) {
	var in limitsIn
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "That request wasn't understood.")
		return
	}
	if !validReason(w, &in.Reason) {
		return
	}
	if err := in.Overrides.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, "Check the numbers: "+err.Error()+".")
		return
	}
	cur, err := s.store.Overrides(r.Context())
	if err != nil {
		s.fail(w, "settings", err)
		return
	}
	in.Overrides.Paused = cur.Paused // the pause has its own switch
	before, err := s.store.SaveOverrides(r.Context(), in.Overrides, a.session.AdminID)
	if err != nil {
		s.fail(w, "save settings", err)
		return
	}
	s.audit(r, a.session.AdminID, "limits_changed", "limits", map[string]any{"before": before, "after": in.Overrides, "reason": in.Reason})
	writeJSON(w, map[string]any{"overrides": in.Overrides})
}

func (s *Server) setPause(w http.ResponseWriter, r *http.Request, a authCtx) {
	var in struct {
		Reason string `json:"reason"`
		Paused bool   `json:"paused"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "That request wasn't understood.")
		return
	}
	if !validReason(w, &in.Reason) {
		return
	}
	o, err := s.store.Overrides(r.Context())
	if err != nil {
		s.fail(w, "pause", err)
		return
	}
	was := o.Paused
	o.Paused = in.Paused
	if _, err := s.store.SaveOverrides(r.Context(), o, a.session.AdminID); err != nil {
		s.fail(w, "pause", err)
		return
	}
	action := "voice_resumed"
	if in.Paused {
		action = "voice_paused"
	}
	if was != in.Paused {
		s.audit(r, a.session.AdminID, action, "voice", map[string]any{"reason": in.Reason})
	}
	writeJSON(w, map[string]any{"paused": in.Paused})
}

// validReason trims the reason and checks its length, answering if not.
func validReason(w http.ResponseWriter, reason *string) bool {
	*reason = strings.TrimSpace(*reason)
	if n := utf8.RuneCountInString(*reason); n < 3 || n > 300 {
		writeError(w, http.StatusBadRequest, "Give a reason of 3 to 300 characters. It goes in the audit log.")
		return false
	}
	return true
}

// maxBroadcast keeps an announcement inside one Telegram message.
const maxBroadcast = 3500

func validBroadcast(w http.ResponseWriter, text, audience string) bool {
	if strings.TrimSpace(text) == "" || utf8.RuneCountInString(text) > maxBroadcast {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("Write a message of 1 to %d characters.", maxBroadcast))
		return false
	}
	if audience != store.AudienceEveryone && audience != store.AudienceActive {
		writeError(w, http.StatusBadRequest, "Choose who gets it.")
		return false
	}
	return true
}

func (s *Server) broadcasts(w http.ResponseWriter, r *http.Request, _ authCtx) {
	list, err := s.store.Broadcasts(r.Context(), 20)
	if err != nil {
		s.fail(w, "broadcasts", err)
		return
	}
	sizes := map[string]int{}
	for _, a := range []string{store.AudienceEveryone, store.AudienceActive} {
		if sizes[a], err = s.store.AudienceSize(r.Context(), a); err != nil {
			s.fail(w, "audience", err)
			return
		}
	}
	writeJSON(w, map[string]any{"broadcasts": nonNil(list), "audience": sizes})
}

func (s *Server) testBroadcast(w http.ResponseWriter, r *http.Request, a authCtx) {
	var in struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "That request wasn't understood.")
		return
	}
	if !validBroadcast(w, in.Text, store.AudienceEveryone) {
		return
	}
	ok, msg := s.notify(r.Context(), a.session.AdminID, store.BroadcastHTML(in.Text)+"\n\n<i>Test: only you got this.</i>")
	if !ok {
		writeError(w, http.StatusBadGateway, orDefault(msg, "Couldn't send the test.")+" Start a chat with Ellie first.")
		return
	}
	writeJSON(w, map[string]any{"sent": true})
}

func (s *Server) createBroadcast(w http.ResponseWriter, r *http.Request, a authCtx) {
	var in struct {
		Reason   string `json:"reason"`
		Text     string `json:"text"`
		Audience string `json:"audience"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "That request wasn't understood.")
		return
	}
	if !validReason(w, &in.Reason) || !validBroadcast(w, in.Text, in.Audience) {
		return
	}
	size, err := s.store.AudienceSize(r.Context(), in.Audience)
	if err != nil {
		s.fail(w, "audience", err)
		return
	}
	id, err := s.store.CreateBroadcast(r.Context(), in.Text, in.Audience, a.session.AdminID)
	if errors.Is(err, ErrBusy) {
		writeError(w, http.StatusConflict, "Another announcement is still going out. Wait for it, or cancel it first.")
		return
	}
	if err != nil {
		s.fail(w, "create broadcast", err)
		return
	}
	s.audit(r, a.session.AdminID, "broadcast_queued", "broadcast:"+strconv.FormatInt(id, 10), map[string]any{
		"audience": in.Audience, "recipients": size, "text": truncate(in.Text, 300), "reason": in.Reason,
	})
	writeJSON(w, map[string]any{"id": id, "recipients": size})
}

func (s *Server) cancelBroadcast(w http.ResponseWriter, r *http.Request, a authCtx) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	in, ok := readAction(w, r)
	if !ok {
		return
	}
	done, err := s.store.CancelBroadcast(r.Context(), id)
	if err != nil {
		s.fail(w, "cancel broadcast", err)
		return
	}
	if !done {
		writeError(w, http.StatusConflict, "That announcement has already finished.")
		return
	}
	s.audit(r, a.session.AdminID, "broadcast_cancelled", "broadcast:"+strconv.FormatInt(id, 10), map[string]any{"reason": in.Reason})
	writeJSON(w, map[string]any{"cancelled": true})
}
