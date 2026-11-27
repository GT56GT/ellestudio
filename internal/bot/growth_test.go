package bot

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------- spoken text

func TestSpeakableDropsWhatShouldNotBeRead(t *testing.T) {
	for in, want := range map[string]string{
		"hello @WhereAirEnds see https://example.com/x?y=1 ok": "hello see ok",
		"mail me at sam@example.com":                           "mail me at sam@example.com",
		"good morning ☀️🌸 everyone 👨‍👩‍👧 👍🏽 🇮🇳":                "good morning everyone",
		"नमस्ते, कैसे हो? 🙏":                                   "नमस्ते, कैसे हो?",
		"line one\n\n  line two  ":                             "line one\nline two",
		"t.me/EllieStudioBot www.site.in 😂":                    "",
		"Price: ₹500, 50% off!":                                "Price: ₹500, 50% off!",
	} {
		if got := speakable(in); got != want {
			t.Errorf("speakable(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLooksHinglish(t *testing.T) {
	for text, want := range map[string]bool{
		"Abhi to bhut time xam me":          true,
		"kya kar rahe ho yaar":              true,
		"Acha aap hi ho kya":                true,
		"I want to go to the store with me": false,
		"hai":                               false, // one word isn't enough
		"अभी तो बहुत time है":               false, // Hindi script is real Hindi
		"": false,
	} {
		if got := looksHinglish(text); got != want {
			t.Errorf("looksHinglish(%q) = %v", text, got)
		}
	}
}

func TestSpeakChargesOnlyWhatIsSpoken(t *testing.T) {
	fx := newFixture(t, Config{DailyChars: 100, AdminUsers: map[int64]bool{owner: true}})
	ctx := context.Background()
	fx.bot.handleUpdate(ctx, msg(7, "👋 hi @friend https://x.y"))
	if fx.ttsIn == nil || fx.ttsIn["transcript"] != "hi" {
		t.Fatalf("tts request = %v", fx.ttsIn)
	}
	if u := fx.bot.store.UsageOn(7, today()); u.Chars != 2 {
		t.Errorf("charged %d characters, want 2", u.Chars)
	}

	fx.ttsIn = nil
	fx.bot.handleUpdate(ctx, msg(7, "😂😂 https://example.com"))
	if fx.ttsIn != nil || !strings.Contains(fx.tg.lastText(), "Nothing to read") {
		t.Errorf("emoji and links alone should not be read: tts %v, reply %q", fx.ttsIn, fx.tg.lastText())
	}
}

// ---------------------------------------------------------------- bonus characters

func TestBonusIsSpentAfterTheDailyLimitAndRefunded(t *testing.T) {
	fx := newFixture(t, Config{})
	s, day := fx.bot.store, today()
	if _, err := s.Update(7, func(v *Settings) { v.Bonus = 5 }); err != nil {
		t.Fatal(err)
	}
	ok, u, _ := s.Spend(7, day, 8, 0, 10, 0)
	if !ok || u.Chars != 8 || u.Bonus != 5 {
		t.Fatalf("within the limit: ok %v, usage %+v", ok, u)
	}
	ok, u, _ = s.Spend(7, day, 4, 0, 10, 0) // 2 from the allowance, 2 from bonus
	if !ok || u.Chars != 12 || u.Bonus != 3 {
		t.Fatalf("past the limit: ok %v, usage %+v", ok, u)
	}
	if ok, _, _ = s.Spend(7, day, 4, 0, 10, 0); ok {
		t.Fatal("spent 4 with only 3 bonus characters left")
	}
	s.Refund(7, day, 4, 0, 10)
	if u := s.UsageOn(7, day); u.Chars != 8 || u.Bonus != 5 {
		t.Errorf("refund should return the bonus part: %+v", u)
	}
	if u := s.UsageOn(7, "2999-01-01"); u.Chars != 0 || u.Bonus != 5 {
		t.Errorf("a new day resets usage but keeps bonus: %+v", u)
	}
}

func TestResetKeepsBookkeeping(t *testing.T) {
	fx := newFixture(t, Config{})
	s := fx.bot.store
	if _, err := s.Update(7, func(v *Settings) {
		v.VoiceID, v.Emotion, v.Bonus, v.Referrals, v.Readings, v.Joined = "v-mine", "calm", 9, 2, 4, true
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.Reset(7); err != nil {
		t.Fatal(err)
	}
	v := s.Get(7)
	if v.VoiceID != "v-default" || v.Emotion != "" {
		t.Errorf("reset kept preferences: voice %q emotion %q", v.VoiceID, v.Emotion)
	}
	if v.Bonus != 9 || v.Referrals != 2 || v.Readings != 4 || !v.Joined {
		t.Errorf("reset lost bookkeeping: %+v", v)
	}
}

// ---------------------------------------------------------------- referrals

func referralFixture(t *testing.T, dailyCap int) *fixture {
	t.Helper()
	// A named admin, so that user 7 is an ordinary user with limits.
	fx := newFixture(t, Config{ReferralBonus: 1000, ReferralDailyCap: dailyCap, DailyChars: 2000,
		AdminUsers: map[int64]bool{owner: true}})
	fx.bot.username = "EllieStudioBot"
	if err := fx.bot.store.Join(7, today()); err != nil {
		t.Fatal(err)
	}
	return fx
}

func TestReferralRewardsBothAndThanksTheInviter(t *testing.T) {
	fx := referralFixture(t, 5)
	ctx := context.Background()
	u := msg(8, "/start r_7")
	u.Message.From.FirstName = "Riya"
	fx.bot.handleUpdate(ctx, u)

	if got := fx.bot.store.Get(8); got.Bonus != 1000 || got.ReferredBy != 7 {
		t.Errorf("new user = bonus %d, referred by %d", got.Bonus, got.ReferredBy)
	}
	if got := fx.bot.store.Get(7); got.Bonus != 1000 || got.Referrals != 1 {
		t.Errorf("inviter = bonus %d, referrals %d", got.Bonus, got.Referrals)
	}
	var thanked, welcomed bool
	for _, c := range fx.tg.find("sendMessage") {
		text, _ := c.JSON["text"].(string)
		if c.JSON["chat_id"] == float64(7) && strings.Contains(text, "Riya joined Ellie from your invite") {
			thanked = true
		}
		if c.JSON["chat_id"] == float64(8) && strings.Contains(text, "+1,000 bonus characters") {
			welcomed = true
		}
	}
	if !thanked || !welcomed {
		t.Errorf("thanked inviter %v, told new user %v", thanked, welcomed)
	}
	if days, _ := fx.bot.metrics.Days(ctx, time.Time{}); len(days) != 1 || days[0].FromInvites != 1 {
		t.Errorf("the referral start wasn't counted: %+v", days)
	}

	// Coming back through the link again changes nothing.
	fx.bot.handleUpdate(ctx, msg(8, "/start r_7"))
	if got := fx.bot.store.Get(7); got.Bonus != 1000 || got.Referrals != 1 {
		t.Errorf("a returning user was counted again: %+v", got)
	}
}

func TestReferralAbuseIsRefused(t *testing.T) {
	ctx := context.Background()

	fx := referralFixture(t, 1)
	fx.bot.handleUpdate(ctx, msg(8, "/start r_7"))
	fx.bot.handleUpdate(ctx, msg(9, "/start r_7"))
	if got := fx.bot.store.Get(7); got.Bonus != 1000 || got.Referrals != 2 {
		t.Errorf("daily cap: inviter = bonus %d, referrals %d; want 1000 and 2", got.Bonus, got.Referrals)
	}
	if got := fx.bot.store.Get(9); got.Bonus != 1000 {
		t.Errorf("the friend still gets their bonus past the inviter's cap, got %d", got.Bonus)
	}

	fx = referralFixture(t, 5)
	fx.bot.handleUpdate(ctx, msg(8, "/start r_99")) // inviter never joined
	fx.bot.handleUpdate(ctx, msg(9, "/start r_9"))  // inviting yourself
	fx.bot.handleUpdate(ctx, msg(10, "/start r_x")) // garbage
	for _, uid := range []int64{8, 9, 10} {
		if got := fx.bot.store.Get(uid); got.Bonus != 0 {
			t.Errorf("user %d got %d bonus from an invalid invite", uid, got.Bonus)
		}
	}

	fx = referralFixture(t, 5)
	fx.bot.handleUpdate(ctx, msg(7, "hello")) // 7 is already a user
	fx.bot.handleUpdate(ctx, msg(7, "/start r_7"))
	if got := fx.bot.store.Get(7); got.Bonus != 0 {
		t.Errorf("an existing user claimed a referral: %+v", got)
	}

	off := newFixture(t, Config{})
	off.bot.username = "EllieStudioBot"
	_ = off.bot.store.Join(7, today())
	off.bot.handleUpdate(ctx, msg(8, "/start r_7"))
	if got := off.bot.store.Get(7); got.Bonus != 0 || got.Referrals != 0 {
		t.Errorf("referrals are off without a bonus: %+v", got)
	}
}

func TestStartSource(t *testing.T) {
	for payload, want := range map[string]string{
		"": srcDirect, "g": srcGuest, "r_7": srcReferral, "v_abc": srcShared, "studio": srcGroup, "voices": srcGroup,
	} {
		if got := startSource(payload); got != want {
			t.Errorf("startSource(%q) = %q, want %q", payload, got, want)
		}
	}
}

func TestFirstVisitTeachesTheBasics(t *testing.T) {
	fx := newFixture(t, Config{DailyChars: 2000, DailyAudioSecs: 600})
	fx.bot.username = "EllieStudioBot"
	fx.bot.handleUpdate(context.Background(), msg(8, "/start"))
	first := fx.tg.find("sendMessage")[0].JSON["text"].(string)
	for _, want := range []string{"Hi there, I'm Ellie", "2,000 characters", "Try it now", "@EllieStudioBot your text"} {
		if !strings.Contains(first, want) {
			t.Errorf("first visit lacks %q:\n%s", want, first)
		}
	}
}

// ---------------------------------------------------------------- profile and nudges

func TestProfileShowsAllowanceBonusAndInvite(t *testing.T) {
	fx := referralFixture(t, 5)
	if _, err := fx.bot.store.Update(7, func(v *Settings) { v.Bonus, v.Readings, v.SpokenChars = 500, 3, 120 }); err != nil {
		t.Fatal(err)
	}
	fx.bot.handleUpdate(context.Background(), msg(7, "/profile"))
	sent := fx.tg.find("sendMessage")
	last := sent[len(sent)-1]
	text := last.JSON["text"].(string)
	for _, want := range []string{"Your profile", "Member since", "2,000</b> of 2,000 characters left", "Bonus characters: <b>500", "3 · 120 characters"} {
		if !strings.Contains(text, want) {
			t.Errorf("profile lacks %q:\n%s", want, text)
		}
	}
	raw, _ := json.Marshal(last.JSON["reply_markup"])
	if !strings.Contains(string(raw), "t.me/share/url") || !strings.Contains(string(raw), "start%3Dr_7") {
		t.Errorf("profile should offer a share button with the invite link: %s", raw)
	}
}

func TestPrivateReadingsShowWhatIsLeftButGroupsDont(t *testing.T) {
	fx := groupFixture(t, Config{DailyChars: 100})
	ctx := context.Background()
	fx.bot.handleUpdate(ctx, msg(7, "hello there"))
	fx.bot.handleUpdate(ctx, gmsg(7, testGroup, "/say hello group"))
	sent := fx.tg.find("sendVoice")
	if len(sent) != 2 {
		t.Fatalf("want 2 voices, got %d", len(sent))
	}
	if !strings.Contains(sent[0].Form["caption"], "89 of 100 characters left today") {
		t.Errorf("private caption = %q", sent[0].Form["caption"])
	}
	if strings.Contains(sent[1].Form["caption"], "left today") {
		t.Errorf("a group caption showed someone's allowance: %q", sent[1].Form["caption"])
	}
	if got := fx.bot.store.Get(7); got.Readings != 2 || got.SpokenChars != 22 {
		t.Errorf("lifetime totals = %d readings, %d characters", got.Readings, got.SpokenChars)
	}
}

func TestLanguageTipIsASuggestionOnceADay(t *testing.T) {
	fx := groupFixture(t, Config{})
	ctx := context.Background()
	tips := func() int {
		n := 0
		for _, c := range fx.tg.find("sendMessage") {
			if s, _ := c.JSON["text"].(string); strings.Contains(s, "language and style saved in your settings") {
				n++
			}
		}
		return n
	}
	fx.bot.handleUpdate(ctx, msg(7, "Hello, how are you today?"))
	if tips() != 0 {
		t.Fatal("English got a Hinglish tip")
	}
	fx.bot.handleUpdate(ctx, gmsg(7, testGroup, "/say kya kar rahe ho yaar"))
	if tips() != 0 {
		t.Fatal("tips must not be posted in groups")
	}
	fx.bot.handleUpdate(ctx, msg(7, "Abhi to bhut time hai"))
	fx.bot.handleUpdate(ctx, msg(7, "kya kar rahe ho yaar"))
	if tips() != 1 {
		t.Errorf("want one tip a day, got %d", tips())
	}
	if lang := fx.bot.store.Get(7).Language; lang != "" {
		t.Errorf("a tip must not change settings, language = %q", lang)
	}
}

func TestLimitScreenOffersInvites(t *testing.T) {
	fx := referralFixture(t, 5)
	fx.bot.cfg.DailyChars = 3
	fx.bot.handleUpdate(context.Background(), msg(7, "hello"))
	text := fx.tg.lastText()
	if !strings.Contains(text, "Daily limit reached") || !strings.Contains(text, "Invite a friend") {
		t.Errorf("limit screen = %q", text)
	}
	if days, _ := fx.bot.metrics.Days(context.Background(), time.Time{}); len(days) != 1 || days[0].LimitUsers != 1 {
		t.Errorf("the limit hit wasn't counted: %+v", days)
	}
}

func TestGuestVoiceInvitesListeners(t *testing.T) {
	fx := guestFixture(t, Config{})
	fx.bot.handleUpdate(context.Background(), guest(7, "@EllieStudioBot hello 👋"))
	res := guestAnswer(t, fx)
	raw, _ := json.Marshal(res["reply_markup"])
	if res["type"] != "voice" || !strings.Contains(string(raw), "Make your own") || !strings.Contains(string(raw), "start=g") {
		t.Errorf("guest voice = %v, markup %s", res, raw)
	}
	if fx.ttsIn["transcript"] != "hello" {
		t.Errorf("guest readings should skip emoji too, read %v", fx.ttsIn["transcript"])
	}
}

// ---------------------------------------------------------------- stats

func TestStatsCountWhatMatters(t *testing.T) {
	fx := groupFixture(t, Config{DailyChars: 50})
	ctx := context.Background()
	fx.bot.handleUpdate(ctx, msg(8, "/start g")) // a new user from guest mode
	fx.bot.handleUpdate(ctx, msg(8, "hello"))
	fx.bot.handleUpdate(ctx, msg(9, "hi"))
	fx.bot.handleUpdate(ctx, msg(9, "hi there"))
	fx.bot.handleUpdate(ctx, msg(9, strings.Repeat("a", 60))) // over the limit

	days, err := fx.bot.metrics.Days(ctx, time.Now().Add(-time.Hour))
	if err != nil || len(days) != 1 {
		t.Fatalf("days = %+v, %v", days, err)
	}
	d := days[0]
	if d.Active != 2 || d.New != 1 || d.FromGroups != 1 || d.Readings != 3 || d.Chars != 15 || d.LimitUsers != 1 {
		t.Errorf("today = %+v", d)
	}

	fx.bot.handleUpdate(ctx, msg(7, "/stats"))
	if text := fx.tg.lastText(); !strings.Contains(text, "Unknown command") {
		t.Errorf("non-admins must not see stats: %q", text)
	}
	fx.bot.handleUpdate(ctx, msg(owner, "/stats"))
	text := fx.tg.lastText()
	for _, want := range []string{"Active users: <b>2 this week · 2 today", "Readings: <b>3 · 1.5 per active user", "1 from groups", "Hit the daily limit: <b>1"} {
		if !strings.Contains(text, want) {
			t.Errorf("stats lack %q:\n%s", want, text)
		}
	}
}

func TestMemMetricsPrune(t *testing.T) {
	m := newMemMetrics()
	ctx := context.Background()
	old, now := time.Now().Add(-100*24*time.Hour), time.Now()
	_ = m.Record(ctx, Event{At: old, User: 1, Kind: evReading})
	_ = m.Record(ctx, Event{At: now, User: 2, Kind: evReading})
	if err := m.Prune(ctx, now.Add(-metricsRetention)); err != nil {
		t.Fatal(err)
	}
	if n, _ := m.Active(ctx, time.Time{}); n != 1 {
		t.Errorf("after pruning, %d users remain, want 1", n)
	}
}

func TestGroupSaySkipsUsernames(t *testing.T) {
	fx := groupFixture(t, Config{})
	fx.bot.handleUpdate(context.Background(), gmsg(7, testGroup, "/say hi @friend look"))
	if fx.ttsIn == nil || fx.ttsIn["transcript"] != "hi look" {
		t.Fatalf("tts request = %v", fx.ttsIn)
	}
}
