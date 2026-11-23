package bot

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"ellestudio/internal/telegram"
)

// guestChat is a group that never added the bot.
const guestChat = int64(-2002)

func guest(uid int64, text string) telegram.Update {
	return telegram.Update{GuestMessage: &telegram.Message{
		MessageID: 30, From: &telegram.User{ID: uid, FirstName: "Sam"}, Text: text, GuestQueryID: "gq1",
		Chat: telegram.Chat{ID: guestChat, Type: "supergroup", Title: "Friends"},
	}}
}

// guestAnswer returns the one answerGuestQuery result, failing unless there
// was exactly one.
func guestAnswer(t *testing.T, fx *fixture) map[string]any {
	t.Helper()
	calls := fx.tg.find("answerGuestQuery")
	if len(calls) != 1 {
		t.Fatalf("want exactly one guest reply, got %d", len(calls))
	}
	if calls[0].JSON["guest_query_id"] != "gq1" {
		t.Errorf("answered query %v", calls[0].JSON["guest_query_id"])
	}
	res, _ := calls[0].JSON["result"].(map[string]any)
	return res
}

func noticeText(res map[string]any) string {
	c, _ := res["input_message_content"].(map[string]any)
	s, _ := c["message_text"].(string)
	return s
}

func TestGuestMentionRepliesWithAVoice(t *testing.T) {
	fx := groupFixture(t, Config{})
	if _, err := fx.bot.store.Update(7, func(s *Settings) { s.VoiceID, s.VoiceName = "v-mine", "My Clone" }); err != nil {
		t.Fatal(err)
	}
	fx.bot.handleUpdate(context.Background(), guest(7, "@EllieStudioBot good morning everyone"))

	if fx.ttsIn == nil || fx.ttsIn["transcript"] != "good morning everyone" {
		t.Fatalf("tts request = %v", fx.ttsIn)
	}
	if v := fx.ttsIn["voice"].(map[string]any)["id"]; v != "v-mine" {
		t.Errorf("spoke with %v, want the caller's own voice", v)
	}
	up := fx.tg.find("sendVoice")
	if len(up) != 1 || up[0].Form["chat_id"] != "1" || up[0].Form["disable_notification"] != "true" {
		t.Fatalf("recording should be uploaded silently to the storage chat (admin 1): %+v", up)
	}
	res := guestAnswer(t, fx)
	if res["type"] != "voice" || res["voice_file_id"] != "vf1" {
		t.Errorf("guest reply = %v, want the uploaded voice", res)
	}
	del := fx.tg.find("deleteMessage")
	if len(del) != 1 || del[0].JSON["chat_id"] != float64(1) {
		t.Errorf("the storage upload should be deleted, got %+v", del)
	}
	n := len("good morning everyone")
	if u := fx.bot.store.UsageOn(7, today()); u.Chars != n {
		t.Errorf("caller charged %d characters, want %d", u.Chars, n)
	}
	info, ok, _ := fx.bot.chats.Get(context.Background(), guestChat)
	if !ok || info.Status != guestStatus || info.Chars != n || info.Title != "Friends" {
		t.Errorf("guest chat = %+v (known %v), want it recorded and charged", info, ok)
	}
}

func TestGuestMentionReadsTheRepliedMessage(t *testing.T) {
	fx := groupFixture(t, Config{})
	u := guest(7, "@EllieStudioBot")
	u.GuestMessage.ReplyToMessage = &telegram.Message{MessageID: 4, Text: "Read this one out"}
	fx.bot.handleUpdate(context.Background(), u)
	if fx.ttsIn == nil || fx.ttsIn["transcript"] != "Read this one out" {
		t.Fatalf("tts request = %v", fx.ttsIn)
	}
}

func TestGuestNoticesAreTheOneReply(t *testing.T) {
	for name, tc := range map[string]struct {
		cfg  Config
		text string
		want string
	}{
		"empty mention": {Config{}, "@EllieStudioBot", "Mention me with something to read"},
		"too long":      {Config{}, "@EllieStudioBot " + strings.Repeat("a", guestMaxChars+1), "lot of text"},
		"not allowed":   {Config{AllowedUsers: map[int64]bool{1: true}}, "@EllieStudioBot hi", "private"},
		"groups off":    {Config{GroupMode: GroupsOff}, "@EllieStudioBot hi", "Not available"},
		"admins only":   {Config{GroupMode: GroupsAdmins}, "@EllieStudioBot hi", "Not available"},
		"user limit":    {Config{DailyChars: 3}, "@EllieStudioBot hello", "allowance is used up"},
		"chat limit":    {Config{GroupLimits: GroupLimits{Chars: 3}}, "@EllieStudioBot hello", "chat&#39;s daily allowance"},
		"total limit":   {Config{GroupLimits: GroupLimits{AllChars: 3}}, "@EllieStudioBot hello", "fully booked"},
	} {
		t.Run(name, func(t *testing.T) {
			fx := groupFixture(t, tc.cfg)
			fx.bot.handleUpdate(context.Background(), guest(7, tc.text))
			res := guestAnswer(t, fx)
			if res["type"] != "article" || !strings.Contains(noticeText(res), tc.want) {
				t.Errorf("reply = %v, want a notice containing %q", res, tc.want)
			}
			raw, _ := json.Marshal(res["reply_markup"])
			if strings.Contains(string(raw), "callback_data") || !strings.Contains(string(raw), "t.me/EllieStudioBot") {
				t.Errorf("guest notices may only link to the private chat: %s", raw)
			}
			if fx.ttsIn != nil || len(fx.tg.find("sendVoice")) != 0 {
				t.Error("a refused mention still spent credits")
			}
			if u := fx.bot.store.UsageOn(7, today()); u.Chars != 0 {
				t.Errorf("refused mention left %d characters charged", u.Chars)
			}
		})
	}
}

func TestGuestFailureRefundsEveryone(t *testing.T) {
	fx := groupFixture(t, Config{})
	fx.ttsFail = true
	fx.bot.handleUpdate(context.Background(), guest(7, "@EllieStudioBot hello"))
	if res := guestAnswer(t, fx); res["type"] != "article" {
		t.Errorf("a failed reading should explain itself, got %v", res)
	}
	if u := fx.bot.store.UsageOn(7, today()); u.Chars != 0 {
		t.Errorf("caller still charged %d", u.Chars)
	}
	if info, _, _ := fx.bot.chats.Get(context.Background(), guestChat); info.Chars != 0 {
		t.Errorf("chat still charged %d", info.Chars)
	}
}

func TestGuestBlockedChatGetsNoReply(t *testing.T) {
	fx := groupFixture(t, Config{})
	if err := fx.bot.chats.SetBlocked(context.Background(), guestChat, true); err != nil {
		t.Fatal(err)
	}
	fx.bot.handleUpdate(context.Background(), guest(7, "@EllieStudioBot hello"))
	if len(fx.tg.find("answerGuestQuery")) != 0 || fx.ttsIn != nil {
		t.Error("a blocked chat was answered")
	}
}

func TestGuestOwnerIsNotChargedAndModeIsKept(t *testing.T) {
	fx := groupFixture(t, Config{GroupMode: GroupsAdmins, GroupLimits: GroupLimits{Chars: 1}})
	fx.bot.handleUpdate(context.Background(), guest(owner, "@EllieStudioBot hello"))
	if res := guestAnswer(t, fx); res["type"] != "voice" {
		t.Fatalf("the studio admin should get a voice, got %v", res)
	}
	if info, _, _ := fx.bot.chats.Get(context.Background(), guestChat); info.Chars != 0 {
		t.Errorf("admin use charged the chat %d", info.Chars)
	}
}

func TestGuestStorageDefaultsToLowestAdmin(t *testing.T) {
	fx := groupFixture(t, Config{AdminUsers: map[int64]bool{40: true, 9: true, 3: false}})
	if fx.bot.cfg.GuestStorageChat != 9 {
		t.Errorf("storage chat = %d, want the lowest admin 9", fx.bot.cfg.GuestStorageChat)
	}
	fx = groupFixture(t, Config{AdminUsers: map[int64]bool{}})
	fx.bot.handleUpdate(context.Background(), guest(7, "@EllieStudioBot hello"))
	if res := guestAnswer(t, fx); !strings.Contains(noticeText(res), "Not set up") {
		t.Errorf("with no storage chat, reply = %v", res)
	}
}

func TestGuestObeysTheGroupsOwnSettings(t *testing.T) {
	ctx := context.Background()
	for name, tc := range map[string]struct {
		gs   GroupSettings
		uid  int64
		text string
		want string // "" means the voice is sent
	}{
		"paused":             {GroupSettings{Paused: true}, 7, "@EllieStudioBot hi", "paused"},
		"no speaking":        {GroupSettings{NoSpeak: true}, 7, "@EllieStudioBot hi", "Speaking is off"},
		"admins only":        {GroupSettings{AdminsOnly: true}, 7, "@EllieStudioBot hi", "Only admins"},
		"admins only, admin": {GroupSettings{AdminsOnly: true}, groupAdmin, "@EllieStudioBot hi", ""},
		"shorter texts":      {GroupSettings{MaxChars: 3}, 7, "@EllieStudioBot hello", "lot of text"},
		"smaller allowance":  {GroupSettings{DailyChars: 3}, 7, "@EllieStudioBot hello", "3 spoken characters"},
	} {
		t.Run(name, func(t *testing.T) {
			fx := groupFixture(t, Config{})
			if err := fx.bot.chats.SetSettings(ctx, guestChat, tc.gs); err != nil {
				t.Fatal(err)
			}
			fx.bot.handleUpdate(ctx, guest(tc.uid, tc.text))
			res := guestAnswer(t, fx)
			if tc.want == "" {
				if res["type"] != "voice" {
					t.Errorf("reply = %v, want a voice", res)
				}
				return
			}
			if !strings.Contains(noticeText(res), tc.want) || fx.ttsIn != nil {
				t.Errorf("reply = %v (tts %v), want a refusal containing %q", res, fx.ttsIn, tc.want)
			}
		})
	}
}

func TestGuestCooldownAndChatJobLimit(t *testing.T) {
	ctx := context.Background()
	fx := groupFixture(t, Config{GroupCooldown: time.Minute})
	fx.bot.handleUpdate(ctx, guest(7, "@EllieStudioBot one"))
	fx.bot.handleUpdate(ctx, guest(7, "@EllieStudioBot two"))
	calls := fx.tg.find("answerGuestQuery")
	if len(calls) != 2 {
		t.Fatalf("want two replies, got %d", len(calls))
	}
	second, _ := calls[1].JSON["result"].(map[string]any)
	if !strings.Contains(noticeText(second), "next request in") {
		t.Errorf("second mention inside the cooldown = %v", second)
	}
	if len(fx.tg.find("sendVoice")) != 1 {
		t.Error("the cooldown did not stop the second reading")
	}

	fx = groupFixture(t, Config{GroupConcurrency: 1})
	fx.bot.groupJobs[guestChat] = 1 // someone else's reading is still running
	fx.bot.handleUpdate(ctx, guest(7, "@EllieStudioBot hi"))
	if res := guestAnswer(t, fx); !strings.Contains(noticeText(res), "already working") {
		t.Errorf("a busy chat should be refused, got %v", res)
	}
	if fx.bot.groupJobs[guestChat] != 1 {
		t.Errorf("job count = %d, want it left at 1", fx.bot.groupJobs[guestChat])
	}
}

func TestGuestUpdatesAreRequested(t *testing.T) {
	fx := groupFixture(t, Config{})
	_, _ = fx.bot.tg.GetUpdates(context.Background(), 0, 0)
	calls := fx.tg.find("getUpdates")
	raw, _ := json.Marshal(calls[len(calls)-1].JSON["allowed_updates"])
	if !strings.Contains(string(raw), "guest_message") {
		t.Errorf("allowed_updates = %s", raw)
	}
}
