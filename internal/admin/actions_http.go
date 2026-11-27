package admin

// actions_http.go is the API for admin changes. Every change needs a reason,
// is checked server-side, and is written to the audit log with what it was
// before and after.

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

	"ellestudio/internal/bot"
)

// Messenger is how the panel reaches Telegram: to tell a user about a
// change and to leave a blocked group. It is the bot's own account.
type Messenger interface {
	Notify(ctx context.Context, chatID int64, html string) error
	Leave(ctx context.Context, chatID int64) error
}

// Limits on what an admin can enter.
const (
	maxBonus      = 100_000
	maxGroupChars = 1_000_000
	maxReadChars  = 10_000
)

// actionIn is the body of every change.
type actionIn struct {
	Reason   string             `json:"reason"`
	Amount   int                `json:"amount"`
	Days     int                `json:"days"`
	Notify   bool               `json:"notify"`
	Settings *bot.GroupSettings `json:"settings"`
}

// readAction decodes the body and checks the reason, which every change needs.
func readAction(w http.ResponseWriter, r *http.Request) (actionIn, bool) {
	var in actionIn
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "That request wasn't understood.")
		return in, false
	}
	in.Reason = strings.TrimSpace(in.Reason)
	if n := utf8.RuneCountInString(in.Reason); n < 3 || n > 300 {
		writeError(w, http.StatusBadRequest, "Give a reason of 3 to 300 characters. It goes in the audit log.")
		return in, false
	}
	return in, true
}

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "That isn't a valid ID.")
		return 0, false
	}
	return id, true
}

// notify messages a user when asked to; failure is reported, not fatal.
func (s *Server) notify(ctx context.Context, uid int64, text string) (bool, string) {
	if s.cfg.Messenger == nil {
		return false, "The panel can't send Telegram messages."
	}
	if err := s.cfg.Messenger.Notify(ctx, uid, text); err != nil {
		s.log.Warn("notify user", "user", uid, "err", err)
		return false, "Couldn't message them. They may have blocked Ellie."
	}
	return true, ""
}

func (s *Server) failAction(w http.ResponseWriter, what string, err error) {
	if errors.Is(err, ErrNotFound) {
		writeError(w, http.StatusNotFound, "That doesn't exist (any more).")
		return
	}
	s.fail(w, what, err)
}

// ---------------------------------------------------------------- users

func (s *Server) giveBonus(w http.ResponseWriter, r *http.Request, a authCtx) {
	uid, ok := pathID(w, r)
	if !ok {
		return
	}
	in, ok := readAction(w, r)
	if !ok {
		return
	}
	if in.Amount < 1 || in.Amount > maxBonus {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("Give between 1 and %d bonus characters.", maxBonus))
		return
	}
	before, after, err := s.store.GiveBonus(r.Context(), uid, in.Amount)
	if err != nil {
		s.failAction(w, "give bonus", err)
		return
	}
	out := map[string]any{"before": before, "after": after}
	if in.Notify {
		out["notified"], out["notify_error"] = s.notify(r.Context(), uid, fmt.Sprintf(
			"🎁 <b>You got %d bonus characters</b> from the Ellie team.\nThey're used once your daily allowance runs out, and they don't expire.", in.Amount))
	}
	s.audit(r, a.session.AdminID, "bonus_given", userTarget(uid), map[string]any{
		"amount": in.Amount, "before": before, "after": after, "reason": in.Reason, "notified": out["notified"],
	})
	writeJSON(w, out)
}

func (s *Server) resetToday(w http.ResponseWriter, r *http.Request, a authCtx) {
	uid, ok := pathID(w, r)
	if !ok {
		return
	}
	in, ok := readAction(w, r)
	if !ok {
		return
	}
	was, err := s.store.ResetToday(r.Context(), uid, s.today())
	if err != nil {
		s.failAction(w, "reset allowance", err)
		return
	}
	s.audit(r, a.session.AdminID, "allowance_reset", userTarget(uid), map[string]any{"before": was, "reason": in.Reason})
	writeJSON(w, map[string]any{"before": was})
}

// banDays are the ban lengths the panel offers (0 = until lifted).
var banDays = map[int]bool{0: true, 1: true, 7: true, 30: true}

func (s *Server) banUser(w http.ResponseWriter, r *http.Request, a authCtx) {
	uid, ok := pathID(w, r)
	if !ok {
		return
	}
	in, ok := readAction(w, r)
	if !ok {
		return
	}
	if s.cfg.Admins[uid] {
		writeError(w, http.StatusBadRequest, "Admins can't be banned. Remove them from ADMIN_USER_IDS first.")
		return
	}
	if !banDays[in.Days] {
		writeError(w, http.StatusBadRequest, "Ban for 1, 7 or 30 days, or until you lift it.")
		return
	}
	now := s.now().UTC()
	ban := &bot.Ban{Reason: in.Reason, By: a.session.AdminID, At: now}
	if in.Days > 0 {
		until := now.Add(time.Duration(in.Days) * 24 * time.Hour)
		ban.Until = &until
	}
	was, err := s.store.SetBan(r.Context(), uid, ban)
	if err != nil {
		s.failAction(w, "ban", err)
		return
	}
	s.audit(r, a.session.AdminID, "user_banned", userTarget(uid), map[string]any{
		"until": ban.Until, "days": in.Days, "reason": in.Reason, "before": was,
	})
	writeJSON(w, map[string]any{"ban": ban})
}

func (s *Server) unbanUser(w http.ResponseWriter, r *http.Request, a authCtx) {
	uid, ok := pathID(w, r)
	if !ok {
		return
	}
	in, ok := readAction(w, r)
	if !ok {
		return
	}
	was, err := s.store.SetBan(r.Context(), uid, nil)
	if err != nil {
		s.failAction(w, "unban", err)
		return
	}
	out := map[string]any{"was_banned": was.Active(s.now())}
	if in.Notify {
		out["notified"], out["notify_error"] = s.notify(r.Context(), uid, "✅ <b>Your access to Ellie is back.</b> Send me some text and I'll read it aloud.")
	}
	s.audit(r, a.session.AdminID, "user_unbanned", userTarget(uid), map[string]any{
		"before": was, "reason": in.Reason, "notified": out["notified"],
	})
	writeJSON(w, out)
}

func userTarget(uid int64) string { return "user:" + strconv.FormatInt(uid, 10) }

// ---------------------------------------------------------------- groups

func groupTarget(id int64) string { return "group:" + strconv.FormatInt(id, 10) }

func (s *Server) blockGroup(w http.ResponseWriter, r *http.Request, a authCtx) {
	s.setBlocked(w, r, a, true)
}

func (s *Server) unblockGroup(w http.ResponseWriter, r *http.Request, a authCtx) {
	s.setBlocked(w, r, a, false)
}

func (s *Server) setBlocked(w http.ResponseWriter, r *http.Request, a authCtx, blocked bool) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	in, ok := readAction(w, r)
	if !ok {
		return
	}
	g, err := s.store.Group(r.Context(), id)
	if err != nil {
		s.failAction(w, "group", err)
		return
	}
	if err := s.store.SetGroupBlocked(r.Context(), id, blocked); err != nil {
		s.failAction(w, "block group", err)
		return
	}
	left := false
	if blocked && g.Status == "member" && s.cfg.Messenger != nil {
		// Leave now, as /blockgroup does. Ellie may already be gone; that's fine.
		if err := s.cfg.Messenger.Leave(r.Context(), id); err != nil {
			s.log.Warn("leave blocked group", "chat", id, "err", err)
		} else {
			left = true
		}
		if err := s.store.MarkGroupLeft(r.Context(), id); err != nil {
			s.log.Warn("mark group left", "chat", id, "err", err)
		}
	}
	action := "group_unblocked"
	if blocked {
		action = "group_blocked"
	}
	s.audit(r, a.session.AdminID, action, groupTarget(id), map[string]any{
		"title": g.Title, "was_blocked": g.Blocked, "left": left, "reason": in.Reason,
	})
	writeJSON(w, map[string]any{"blocked": blocked, "left": left})
}

func (s *Server) groupSettings(w http.ResponseWriter, r *http.Request, a authCtx) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	in, ok := readAction(w, r)
	if !ok {
		return
	}
	if in.Settings == nil {
		writeError(w, http.StatusBadRequest, "Send the group's settings.")
		return
	}
	gs := *in.Settings
	if gs.DailyChars < 0 || gs.DailyChars > maxGroupChars || gs.MaxChars < 0 || gs.MaxChars > maxReadChars {
		writeError(w, http.StatusBadRequest, "Limits must be 0 (the studio's) or positive, and not absurdly large.")
		return
	}
	g, err := s.store.Group(r.Context(), id)
	if err != nil {
		s.failAction(w, "group", err)
		return
	}
	if err := s.store.SetGroupSettings(r.Context(), id, gs); err != nil {
		s.failAction(w, "group settings", err)
		return
	}
	s.audit(r, a.session.AdminID, "group_settings", groupTarget(id), map[string]any{
		"title": g.Title, "before": g.Settings, "after": gs, "reason": in.Reason,
	})
	writeJSON(w, map[string]any{"settings": gs})
}

// ---------------------------------------------------------------- voices

func (s *Server) voices(w http.ResponseWriter, r *http.Request, _ authCtx) {
	q := VoiceQuery{Kind: r.URL.Query().Get("kind"), Limit: pageSize}
	if q.Kind != "" && q.Kind != "voice" && q.Kind != "dict" {
		writeError(w, http.StatusBadRequest, "Kind must be voice or dict.")
		return
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("owner")); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "Filter by a Telegram user ID (numbers only).")
			return
		}
		q.OwnerID = id
	}
	var number int
	q.Offset, number = page(r, pageSize)
	rows, total, err := s.store.Voices(r.Context(), q)
	if err != nil {
		s.fail(w, "voices", err)
		return
	}
	jobs, err := s.store.Jobs(r.Context(), 10)
	if err != nil {
		s.fail(w, "jobs", err)
		return
	}
	writeJSON(w, map[string]any{"items": nonNil(rows), "total": total, "page": number, "page_size": pageSize, "jobs": nonNil(jobs)})
}

func (s *Server) deleteVoice(w http.ResponseWriter, r *http.Request, a authCtx) {
	kind, id := r.PathValue("kind"), r.PathValue("id")
	if (kind != "voice" && kind != "dict") || id == "" || len(id) > 200 {
		writeError(w, http.StatusBadRequest, "That isn't a voice or dictionary.")
		return
	}
	in, ok := readAction(w, r)
	if !ok {
		return
	}
	job, row, created, err := s.store.QueueDelete(r.Context(), kind, id, a.session.AdminID)
	if err != nil {
		s.failAction(w, "queue delete", err)
		return
	}
	if created {
		s.audit(r, a.session.AdminID, "delete_requested", kind+":"+id, map[string]any{
			"name": row.Name, "owner": row.OwnerID, "job": job, "reason": in.Reason,
		})
	}
	_, online, _ := s.store.BotStatus(r.Context(), s.now().UTC())
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]any{"job": job, "already_requested": !created, "bot_online": online})
}
