package bot

import (
	"context"
	"fmt"
	"html"
	"net/url"
	"strconv"
	"strings"
	"time"

	"ellestudio/internal/telegram"
)

// growth.go is everything that helps people discover Ellie and come back:
// where new users come from, referrals, the profile, and gentle nudges
// (characters left, language tips). It never spends credits.

// ---------------------------------------------------------------- where users come from

// startSource maps a /start payload to where a new user came from.
func startSource(payload string) string {
	switch {
	case payload == "g":
		return srcGuest
	case strings.HasPrefix(payload, "r_"):
		return srcReferral
	case strings.HasPrefix(payload, "v_"):
		return srcShared
	case deepLinks[payload]:
		return srcGroup
	}
	return srcDirect
}

// ---------------------------------------------------------------- referrals

// parseReferral reads the inviter from a "r_<user id>" start payload.
func parseReferral(payload string) (int64, bool) {
	rest, ok := strings.CutPrefix(payload, "r_")
	if !ok {
		return 0, false
	}
	id, err := strconv.ParseInt(rest, 10, 64)
	return id, err == nil && id > 0
}

// referrals reports whether invites earn bonus characters.
func (b *Bot) referrals() bool { return b.cfg.ReferralBonus > 0 && b.username != "" }

func (b *Bot) inviteLink(uid int64) string { return b.startLink("r_" + strconv.FormatInt(uid, 10)) }

// shareURL opens Telegram's "share to a chat" sheet with the invite.
func (b *Bot) shareURL(uid int64) string {
	text := "I turn my texts into voice notes with Ellie 🎙 Join with my link and we both get " +
		fmtInt(b.cfg.ReferralBonus) + " free bonus characters."
	return "https://t.me/share/url?url=" + url.QueryEscape(b.inviteLink(uid)) + "&text=" + url.QueryEscape(text)
}

// applyReferral credits a brand-new user who arrived through a friend's
// invite, and thanks the friend. It returns the new user's bonus (0 if the
// invite didn't apply). It must run before the new user is first saved.
func (b *Bot) applyReferral(ctx context.Context, m *telegram.Message, payload string) int {
	inviter, ok := parseReferral(payload)
	if !ok || !b.referrals() || !b.allowed(inviter) {
		return 0
	}
	applied, rewarded, err := b.store.Referral(inviter, m.From.ID, today(), b.cfg.ReferralBonus, b.cfg.ReferralDailyCap)
	if err != nil {
		b.log.Warn("save referral", "err", err)
	}
	if !applied {
		return 0
	}
	b.log.Info("referral", "inviter", inviter, "user", m.From.ID, "rewarded", rewarded)
	name := html.EscapeString(orDefault(m.From.FirstName, "A friend"))
	body := "You got <b>+" + fmtInt(b.cfg.ReferralBonus) + " bonus characters</b>. " +
		"They're used after your daily allowance and never expire."
	if !rewarded {
		body = "You've reached today's limit of invite rewards. Friends who join tomorrow earn you bonus characters again."
	}
	s := screen{header("🎉", name+" joined Ellie from your invite!", "") + body,
		(&keys{}).row(btn("👤 My profile", "nav:profile")).build()}
	b.sendScreen(ctx, inviter, 0, s)
	return b.cfg.ReferralBonus
}

// ---------------------------------------------------------------- first visit

// firstVisitText greets a new user: what Ellie does, what's free, and one
// thing to try right now.
func (b *Bot) firstVisitText(name string, bonus int) string {
	var sb strings.Builder
	sb.WriteString("👋 <b>Hi " + name + ", I'm Ellie!</b>\n" +
		"I turn your text into natural voice notes, in 100+ voices or even your own.\n\n")
	if free := b.freeAllowance(); free != "" {
		sb.WriteString("🎁 <b>Free every day:</b> " + free + "\n")
	}
	if bonus > 0 {
		sb.WriteString("🎉 Your friend's invite added <b>+" + fmtInt(bonus) + " bonus characters</b>.\n")
	}
	sb.WriteString("\n<b>Try it now:</b> send me any message, for example\n" +
		"<code>Hi! This is my first voice note from Ellie.</code>\n\n" +
		"🎤 Send a voice note and I'll write it down\n")
	if b.cfg.GroupMode != GroupsOff && b.username != "" {
		sb.WriteString("👥 In any group, type <code>@" + b.username + " your text</code> and I'll read it there\n")
	}
	return sb.String()
}

// freeAllowance lists the daily allowance, or "" when nothing is limited.
func (b *Bot) freeAllowance() string {
	var parts []string
	if b.cfg.DailyChars > 0 {
		parts = append(parts, fmtInt(b.cfg.DailyChars)+" characters")
	}
	if b.cfg.DailyAudioSecs > 0 {
		parts = append(parts, fmtMinutes(b.cfg.DailyAudioSecs)+" of transcription")
	}
	if b.cfg.DailyCreations > 0 {
		parts = append(parts, strconv.Itoa(b.cfg.DailyCreations)+" new voices")
	}
	return strings.Join(parts, " · ")
}

// guestInviteScreen is the reply a stranger gets when they mention Ellie in
// another chat: why Ellie is worth a tap, and how to start.
func (b *Bot) guestInviteScreen() screen {
	body := "I turn text into voice notes in 100+ voices, or even your own cloned voice."
	if free := b.freeAllowance(); free != "" {
		body += "\n🎁 Free every day: " + free + "."
	}
	body += "\n\nTap <b>Start Ellie</b> below, then mention me again and I'll read it here."
	return noticeScreen(icStudio, "Make Ellie read for you", body)
}

// ---------------------------------------------------------------- profile

// bar draws how much of total is left as ten blocks.
func bar(left, total int) string {
	if total <= 0 {
		return ""
	}
	n := min(10, max(0, (left*10+total-1)/total))
	return strings.Repeat("▰", n) + strings.Repeat("▱", 10-n)
}

func fmtDay(day string) string {
	t, err := time.Parse("2006-01-02", day)
	if err != nil {
		return day
	}
	return t.Format("Jan 2, 2006")
}

func (b *Bot) profileScreen(uid int64) screen {
	s := b.store.Get(uid)
	var sb strings.Builder
	sub := ""
	if s.JoinedAt != "" {
		sub = "Member since " + fmtDay(s.JoinedAt)
	}
	sb.WriteString(header("👤", "Your profile", sub))
	sb.WriteString(field(icVoices, "Voice", orDefault(s.VoiceName, "not chosen yet")))
	sb.WriteString(field(icStyle, "Style", styleSummary(s)))

	sb.WriteString("\n<b>Today</b>\n")
	if !b.limited(uid) {
		sb.WriteString(field("🎟", "Allowance", "unlimited"))
	} else {
		u := b.store.UsageOn(uid, today())
		if c := b.cfg.DailyChars; c > 0 {
			left := max(0, c-u.Chars)
			sb.WriteString("🎟 Speech: " + bar(left, c) + " <b>" + fmtInt(left) + "</b> of " + fmtInt(c) + " characters left\n")
		}
		if a := b.cfg.DailyAudioSecs; a > 0 {
			sb.WriteString(field(icListen, "Transcription left", fmtMinutes(max(0, a-u.Secs))+" of "+fmtMinutes(a)))
		}
		if n := b.cfg.DailyCreations; n > 0 {
			sb.WriteString(field(icCreate, "New voices left", strconv.Itoa(max(0, n-u.Makes))+" of "+strconv.Itoa(n)))
		}
		sb.WriteString("🔄 Refreshes in <b>" + fmtWait(untilReset()) + "</b> (midnight UTC)\n")
	}
	if s.Bonus > 0 {
		sb.WriteString(field("🎁", "Bonus characters", fmtInt(s.Bonus)))
		sb.WriteString("<i>Used after your daily allowance. They never expire.</i>\n")
	}

	sb.WriteString("\n<b>All time</b>\n")
	sb.WriteString(field("🔊", "Readings", fmtInt(s.Readings)+" · "+fmtInt(s.SpokenChars)+" characters"))
	if b.referrals() {
		sb.WriteString(field("🤝", "Friends invited", fmtInt(s.Referrals)))
	}

	k := &keys{}
	if b.referrals() {
		sb.WriteString("\n🤝 <b>Invite friends:</b> you both get <b>+" + fmtInt(b.cfg.ReferralBonus) + "</b> bonus characters.")
		k.row(telegram.InlineKeyboardButton{Text: "📨 Invite friends", URL: b.shareURL(uid)}, btn("🔗 My invite link", "nav:invite"))
	}
	k.row(btn(icVoices+" Voices", "nav:voices"), btn(icStyle+" Style", "nav:style")).nav("", "")
	return screen{sb.String(), k.build()}
}

func (b *Bot) inviteScreen(uid int64) screen {
	if !b.referrals() {
		return screen{header("🤝", "Invite friends", "") + "Invites are turned off right now.", (&keys{}).nav("", "").build()}
	}
	s := b.store.Get(uid)
	text := header("🤝", "Invite friends", "You both get bonus characters") +
		"When a friend starts Ellie with your link, you each get <b>+" + fmtInt(b.cfg.ReferralBonus) + " bonus characters</b>. " +
		"Bonus characters are used after your daily allowance and never expire.\n\n" +
		"Your link:\n<code>" + html.EscapeString(b.inviteLink(uid)) + "</code>\n\n" +
		field("🤝", "Friends invited so far", fmtInt(s.Referrals))
	if c := b.cfg.ReferralDailyCap; c > 0 {
		text += "<i>You can earn up to " + strconv.Itoa(c) + " invite rewards a day.</i>"
	}
	k := (&keys{}).
		row(telegram.InlineKeyboardButton{Text: "📨 Send to a friend", URL: b.shareURL(uid)}).
		row(btn("👤 My profile", "nav:profile")).nav("", "")
	return screen{text, k.build()}
}

// leftLine is the plain-text "characters left" line under a private reading,
// or "" for people without a limit.
func (b *Bot) leftLine(uid int64) string {
	if !b.limited(uid) || b.cfg.DailyChars <= 0 {
		return ""
	}
	u := b.store.UsageOn(uid, today())
	left := max(0, b.cfg.DailyChars-u.Chars)
	line := "🎟 " + fmtInt(left) + " of " + fmtInt(b.cfg.DailyChars) + " characters left today"
	if left == 0 {
		line = "🎟 Daily allowance used"
	}
	if u.Bonus > 0 {
		line += " · 🎁 " + fmtInt(u.Bonus) + " bonus"
	}
	if left*5 < b.cfg.DailyChars && b.referrals() {
		line += " · /invite for more"
	}
	return line
}

// ---------------------------------------------------------------- language tips

// languageTip suggests (once a day, in the private chat only) how to make
// Hindi typed in English letters sound right. It changes nothing: Ellie
// always reads with the language and style saved in the user's settings.
func (b *Bot) languageTip(ctx context.Context, m *telegram.Message, s Settings, text string) {
	if isGroupChat(m.Chat.ID) || !looksHinglish(text) || !b.store.MarkTip(m.From.ID, today()) {
		return
	}
	lang := "automatic (I guess from your text)"
	if s.Language != "" {
		lang = langLabel(s.Language)
	}
	body := "I always read with the language and style saved in your settings. Right now the language is <b>" +
		html.EscapeString(lang) + "</b>.\n\n" +
		"Your message looks like Hindi typed in English letters, which can sound a little off. For the clearest voice:\n" +
		"• type it in Hindi script, like <i>नमस्ते, कैसे हो?</i>\n" +
		"• or keep English letters and set the language to <b>English</b>"
	k := (&keys{}).row(btn(icLang+" Language", "nav:lang"), btn(icStyle+" Style", "nav:style")).build()
	b.replyHTML(ctx, m, header("💡", "Tip", "")+body, k)
}

// ---------------------------------------------------------------- admin stats

func (b *Bot) statsScreen(ctx context.Context) screen {
	nav := (&keys{}).row(btn("🔄 Refresh", "nav:stats")).nav("Admin", "nav:admin").build()
	now := time.Now().UTC()
	since := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -6)
	days, err := b.metrics.Days(ctx, since)
	if err != nil {
		b.log.Error("stats", "err", err)
		return screen{header("📊", "Stats", "") + "Stats are unavailable right now. Try again in a moment.", nav}
	}
	weekly, err := b.metrics.Active(ctx, since)
	if err != nil {
		b.log.Error("stats", "err", err)
	}
	var tot DayStats
	for _, d := range days {
		tot.New += d.New
		tot.FromGroups += d.FromGroups
		tot.FromInvites += d.FromInvites
		tot.Readings += d.Readings
		tot.Chars += d.Chars
		tot.LimitUsers += d.LimitUsers
		tot.GuestInvites += d.GuestInvites
	}
	todayActive := 0
	if len(days) > 0 && days[0].Day == now.Format("2006-01-02") {
		todayActive = days[0].Active
	}

	var sb strings.Builder
	sb.WriteString(header("📊", "Stats", "Last 7 days, UTC"))
	sb.WriteString(field("👥", "Active users", fmtInt(weekly)+" this week · "+fmtInt(todayActive)+" today"))
	perUser := "–"
	if weekly > 0 {
		perUser = strconv.FormatFloat(float64(tot.Readings)/float64(weekly), 'f', 1, 64)
	}
	sb.WriteString(field("🔊", "Readings", fmtInt(tot.Readings)+" · "+perUser+" per active user · "+fmtInt(tot.Chars)+" characters"))
	sb.WriteString(field("🆕", "New users", fmt.Sprintf("%s (%s from groups · %s invited by friends)",
		fmtInt(tot.New), fmtInt(tot.FromGroups), fmtInt(tot.FromInvites))))
	sb.WriteString(field("📣", "Group invites shown", fmtInt(tot.GuestInvites)))
	sb.WriteString(field("🎟", "Hit the daily limit", fmtInt(tot.LimitUsers)+" user-days"))
	if len(days) > 0 {
		sb.WriteString("\n<b>By day</b>\n")
		for _, d := range days {
			t, _ := time.Parse("2006-01-02", d.Day)
			fmt.Fprintf(&sb, "%s · %d active · %d readings · %d new · %d at limit\n",
				t.Format("Jan 2"), d.Active, d.Readings, d.New, d.LimitUsers)
		}
	} else {
		sb.WriteString("\n<i>Nothing recorded yet.</i>")
	}
	return screen{sb.String(), nav}
}
