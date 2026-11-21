package bot

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"ellestudio/internal/telegram"
)

const (
	testGroup = int64(-1001)
	owner     = int64(1) // the studio admin in group tests
)

// groupFixture is a fixture whose bot knows its username, with user 7 an
// ordinary member and user 1 the studio admin.
func groupFixture(t *testing.T, cfg Config) *fixture {
	t.Helper()
	if cfg.AdminUsers == nil {
		cfg.AdminUsers = map[int64]bool{owner: true}
	}
	fx := newFixture(t, cfg)
	fx.bot.username = "EllieStudioBot"
	fx.bot.tg.SetGroupPacing(time.Millisecond, 100, time.Minute) // keep tests fast
	return fx
}

func gmsg(uid, chat int64, text string) telegram.Update {
	return telegram.Update{Message: &telegram.Message{
		MessageID: 20, From: &telegram.User{ID: uid, FirstName: "Sam"}, Text: text,
		Chat: telegram.Chat{ID: chat, Type: "supergroup", Title: "Test group"},
	}}
}

func replyTo(u telegram.Update, to *telegram.Message) telegram.Update {
	u.Message.ReplyToMessage = to
	return u
}

// callbackButtons reports whether any recorded call carried a callback button.
func (f *fakeTelegram) callbackButtons() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.calls {
		raw, _ := json.Marshal(c.JSON["reply_markup"])
		if strings.Contains(string(raw), "callback_data") || strings.Contains(c.Form["reply_markup"], "callback_data") {
			return true
		}
	}
	return false
}

func (f *fakeTelegram) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func TestGroupIgnoresMessagesNotForIt(t *testing.T) {
	fx := groupFixture(t, Config{})
	ctx := context.Background()
	for _, text := range []string{
		"just chatting",
		"/say@OtherBot hello",
		"@EllieStudioBotFan hello", // a different, longer username
	} {
		fx.bot.handleUpdate(ctx, gmsg(7, testGroup, text))
	}
	// A plain reply to one of the bot's messages is chat, not a request.
	fx.bot.handleUpdate(ctx, replyTo(gmsg(7, testGroup, "lol"), &telegram.Message{MessageID: 3, Text: "a voice"}))
	if n := fx.tg.count(); n != 0 || fx.ttsIn != nil {
		t.Fatalf("bot reacted to messages not addressed to it: %d calls, tts=%v", n, fx.ttsIn)
	}
}

func TestCommandsForOtherBotsAreIgnoredInPrivate(t *testing.T) {
	fx := groupFixture(t, Config{})
	fx.bot.handleUpdate(context.Background(), msg(7, "/help@OtherBot"))
	if n := fx.tg.count(); n != 0 {
		t.Fatalf("answered a command meant for another bot (%d calls)", n)
	}
	fx.bot.handleUpdate(context.Background(), msg(7, "/help@EllieStudioBot"))
	if fx.tg.count() == 0 {
		t.Fatal("ignored a command addressed to this bot")
	}
}

func TestGroupSaySpeaksInTheSendersVoice(t *testing.T) {
	fx := groupFixture(t, Config{})
	if _, err := fx.bot.store.Update(7, func(s *Settings) { s.VoiceID, s.VoiceName = "v-mine", "My Clone" }); err != nil {
		t.Fatal(err)
	}
	fx.bot.handleUpdate(context.Background(), gmsg(7, testGroup, "/say@EllieStudioBot Hello everyone"))
	if fx.ttsIn == nil || fx.ttsIn["transcript"] != "Hello everyone" {
		t.Fatalf("tts request = %v", fx.ttsIn)
	}
	if v := fx.ttsIn["voice"].(map[string]any)["id"]; v != "v-mine" {
		t.Errorf("spoke with voice %v, want the sender's own v-mine", v)
	}
	sent := fx.tg.find("sendVoice")
	if len(sent) != 1 || sent[0].Form["chat_id"] != "-1001" {
		t.Fatalf("voice not sent to the group: %+v", sent)
	}
	if fx.tg.callbackButtons() {
		t.Error("a group message carried callback buttons anyone could press")
	}
	if !strings.Contains(sent[0].Form["reply_markup"], "t.me/EllieStudioBot") {
		t.Errorf("speech should link to the private chat, markup = %s", sent[0].Form["reply_markup"])
	}
}

func TestGroupSayReadsTheRepliedMessage(t *testing.T) {
	fx := groupFixture(t, Config{})
	fx.bot.handleUpdate(context.Background(),
		replyTo(gmsg(7, testGroup, "/say"), &telegram.Message{MessageID: 4, Text: "Read this one out"}))
	if fx.ttsIn == nil || fx.ttsIn["transcript"] != "Read this one out" {
		t.Fatalf("tts request = %v", fx.ttsIn)
	}
}

func TestGroupMentionSpeaks(t *testing.T) {
	fx := groupFixture(t, Config{})
	fx.bot.handleUpdate(context.Background(), gmsg(7, testGroup, "hey @ellieStudioBot good morning"))
	if fx.ttsIn == nil || fx.ttsIn["transcript"] != "hey good morning" {
		t.Fatalf("tts request = %v", fx.ttsIn)
	}
}

func TestGroupPrivateFeaturesLinkToThePrivateChat(t *testing.T) {
	fx := groupFixture(t, Config{})
	ctx := context.Background()
	for cmd, link := range map[string]string{
		"/voices": "start=voices", "/clone": "start=clone", "/rename x": "start=studio", "/deletevoice": "start=studio",
	} {
		before := len(fx.tg.find("sendMessage"))
		fx.bot.handleUpdate(ctx, gmsg(7, testGroup, cmd))
		sent := fx.tg.find("sendMessage")
		if len(sent) != before+1 {
			t.Fatalf("%s: want one reply, got %d", cmd, len(sent)-before)
		}
		raw, _ := json.Marshal(sent[len(sent)-1].JSON["reply_markup"])
		if !strings.Contains(string(raw), "t.me/EllieStudioBot?start=") || !strings.Contains(string(raw), link) {
			t.Errorf("%s: markup = %s, want a %s link", cmd, raw, link)
		}
	}
	if fx.tg.callbackButtons() || fx.cloned || len(fx.deleted) > 0 {
		t.Error("a private feature ran in the group")
	}
	if fx.bot.getFlow(7) != nil {
		t.Error("a guided step started from a group")
	}
}

func TestGroupButtonsNeverActOnPrivateSettings(t *testing.T) {
	fx := groupFixture(t, Config{})
	q := press(7, "reset")
	q.CallbackQuery.Message.Chat = telegram.Chat{ID: testGroup, Type: "supergroup"}
	if _, err := fx.bot.store.Update(7, func(s *Settings) { s.Emotion = "calm" }); err != nil {
		t.Fatal(err)
	}
	fx.bot.handleUpdate(context.Background(), q)
	if fx.bot.store.Get(7).Emotion != "calm" {
		t.Error("a button in a group changed someone's settings")
	}
	acks := fx.tg.find("answerCallbackQuery")
	if len(acks) != 1 || !strings.Contains(acks[0].JSON["url"].(string), "t.me/EllieStudioBot") {
		t.Errorf("press should open the private chat, got %+v", acks)
	}
	if len(fx.tg.find("editMessageText")) != 0 {
		t.Error("a group message was redrawn with private settings")
	}
}

func TestGroupMessagesDontFeedPrivateSteps(t *testing.T) {
	fx := groupFixture(t, Config{})
	ctx := context.Background()
	fx.bot.handleUpdate(ctx, press(7, "do:search")) // waiting for a search term in private
	fx.bot.handleUpdate(ctx, gmsg(7, testGroup, "@EllieStudioBot warm narrator"))
	if fx.ttsIn == nil || fx.ttsIn["transcript"] != "warm narrator" {
		t.Fatalf("group mention should be spoken, tts = %v", fx.ttsIn)
	}
	if f := fx.bot.getFlow(7); f == nil || f.kind != flowSearch {
		t.Error("the private search step was consumed by a group message")
	}
}

func TestGroupCooldown(t *testing.T) {
	fx := groupFixture(t, Config{GroupCooldown: time.Minute})
	ctx := context.Background()
	fx.bot.handleUpdate(ctx, gmsg(7, testGroup, "/say one"))
	fx.ttsIn = nil
	fx.bot.handleUpdate(ctx, gmsg(7, testGroup, "/say two"))
	fx.bot.handleUpdate(ctx, gmsg(7, testGroup, "/say three"))
	if fx.ttsIn != nil {
		t.Fatalf("spoke during the cooldown: %v", fx.ttsIn)
	}
	reminders := 0
	for _, c := range fx.tg.find("sendMessage") {
		if strings.Contains(c.JSON["text"].(string), "next request") {
			reminders++
		}
	}
	if reminders != 1 {
		t.Errorf("want exactly one cooldown reminder, got %d", reminders)
	}
	// Someone else in the group isn't held up, and neither is the admin.
	fx.bot.handleUpdate(ctx, gmsg(8, testGroup, "/say four"))
	if fx.ttsIn == nil {
		t.Error("another member was blocked by someone else's cooldown")
	}
	fx.ttsIn = nil
	fx.bot.handleUpdate(ctx, gmsg(owner, testGroup, "/say five"))
	fx.bot.handleUpdate(ctx, gmsg(owner, testGroup, "/say six"))
	if fx.ttsIn == nil || fx.ttsIn["transcript"] != "six" {
		t.Error("the admin should not have a cooldown")
	}
}

func TestGroupConcurrencyLimit(t *testing.T) {
	fx := groupFixture(t, Config{GroupConcurrency: 1})
	ctx := context.Background()
	done, ok := fx.bot.groupTurn(ctx, gmsg(7, testGroup, "/say hi").Message)
	if !ok {
		t.Fatal("first job refused")
	}
	if _, ok := fx.bot.groupTurn(ctx, gmsg(8, testGroup, "/say hi").Message); ok {
		t.Error("a second job ran while the group was at its limit")
	}
	if _, ok := fx.bot.groupTurn(ctx, gmsg(8, -2002, "/say hi").Message); !ok {
		t.Error("another group was held up")
	}
	done()
	if _, ok := fx.bot.groupTurn(ctx, gmsg(8, testGroup, "/say hi").Message); !ok {
		t.Error("the slot was not given back")
	}
}

func TestGroupSharedAllowance(t *testing.T) {
	fx := groupFixture(t, Config{DailyChars: 1000, GroupLimits: GroupLimits{Chars: 10}})
	ctx := context.Background()
	fx.bot.handleUpdate(ctx, gmsg(7, testGroup, "/say twelve chars"))
	if fx.ttsIn != nil {
		t.Fatal("spoke past the group's allowance")
	}
	if !strings.Contains(fx.tg.lastText(), "daily allowance is used up") {
		t.Errorf("refusal = %q", fx.tg.lastText())
	}
	if u := fx.bot.store.UsageOn(7, today()); u.Chars != 0 {
		t.Errorf("the member was charged for a refused request: %+v", u)
	}
	fx.bot.handleUpdate(ctx, gmsg(7, testGroup, "/say short"))
	if fx.ttsIn == nil {
		t.Fatal("a request inside the allowance was refused")
	}
	if c, _, _ := fx.bot.chats.Get(ctx, testGroup); c.Chars != 5 {
		t.Errorf("group usage = %d, want 5", c.Chars)
	}
	if u := fx.bot.store.UsageOn(7, today()); u.Chars != 5 {
		t.Errorf("member usage = %d, want 5 (their own allowance still counts)", u.Chars)
	}
	// Private chats don't touch the group's allowance.
	fx.bot.handleUpdate(ctx, msg(7, "a private message that is long"))
	if c, _, _ := fx.bot.chats.Get(ctx, testGroup); c.Chars != 5 {
		t.Errorf("private speech counted against the group: %d", c.Chars)
	}
}

func TestAllGroupsShareOneCap(t *testing.T) {
	fx := groupFixture(t, Config{GroupLimits: GroupLimits{AllChars: 8}})
	ctx := context.Background()
	fx.bot.handleUpdate(ctx, gmsg(7, testGroup, "/say hello"))
	fx.ttsIn = nil
	fx.bot.handleUpdate(ctx, gmsg(8, -2002, "/say world"))
	if fx.ttsIn != nil {
		t.Fatal("groups together spoke past the total cap")
	}
	if !strings.Contains(fx.tg.lastText(), "fully booked") {
		t.Errorf("refusal = %q", fx.tg.lastText())
	}
}

func TestGroupAllowanceIsRefundedOnFailure(t *testing.T) {
	fx := groupFixture(t, Config{GroupLimits: GroupLimits{Chars: 100}})
	fx.ttsFail = true
	fx.bot.handleUpdate(context.Background(), gmsg(7, testGroup, "/say this will fail"))
	if c, _, _ := fx.bot.chats.Get(context.Background(), testGroup); c.Chars != 0 {
		t.Errorf("failed speech still counted: %d chars", c.Chars)
	}
	if all, _, _ := fx.bot.chats.Get(context.Background(), allGroups); all.Chars != 0 {
		t.Errorf("failed speech still counted in the total: %d chars", all.Chars)
	}
}

func TestGroupTranscribesARepliedVoiceNote(t *testing.T) {
	fx := groupFixture(t, Config{GroupLimits: GroupLimits{Secs: 600}})
	note := &telegram.Message{MessageID: 4, Voice: &telegram.Voice{FileID: "f1", Duration: 3, FileSize: 16}}
	fx.bot.handleUpdate(context.Background(), replyTo(gmsg(7, testGroup, "/transcribe"), note))
	if fx.sttIn == nil {
		t.Fatal("voice note was not transcribed")
	}
	if !strings.Contains(fx.tg.lastText(), "hello from a voice note") {
		t.Errorf("transcript not shown: %q", fx.tg.lastText())
	}
	// 3s reserved, Cartesia reported 2.5s, which rounds up to 3.
	if c, _, _ := fx.bot.chats.Get(context.Background(), testGroup); c.Secs != 3 {
		t.Errorf("group transcription = %ds, want 3", c.Secs)
	}
	if fx.tg.callbackButtons() {
		t.Error("the transcript carried callback buttons")
	}
}

func TestForumTopicRepliesStayInTheTopic(t *testing.T) {
	fx := groupFixture(t, Config{})
	u := gmsg(7, testGroup, "/say in the topic")
	u.Message.IsTopicMessage, u.Message.MessageThreadID = true, 42
	fx.bot.handleUpdate(context.Background(), u)
	sent := fx.tg.find("sendVoice")
	if len(sent) != 1 || sent[0].Form["message_thread_id"] != "42" {
		t.Fatalf("speech left the topic: %+v", sent)
	}
	if acts := fx.tg.find("sendChatAction"); len(acts) != 1 || acts[0].JSON["message_thread_id"] != float64(42) {
		t.Errorf("chat action not shown in the topic: %+v", acts)
	}
}

func joined(by int64, chat telegram.Chat) telegram.Update {
	return telegram.Update{MyChatMember: &telegram.ChatMemberUpdated{
		Chat: chat, From: telegram.User{ID: by},
		OldChatMember: telegram.ChatMember{Status: "left"},
		NewChatMember: telegram.ChatMember{Status: "member"},
	}}
}

func TestJoiningAGroupWelcomesAndRegistersIt(t *testing.T) {
	fx := groupFixture(t, Config{})
	ctx := context.Background()
	fx.bot.handleUpdate(ctx, joined(7, telegram.Chat{ID: testGroup, Type: "supergroup", Title: "Book club"}))
	if !strings.Contains(fx.tg.lastText(), "Hi everyone") {
		t.Errorf("no welcome: %q", fx.tg.lastText())
	}
	c, ok, _ := fx.bot.chats.Get(ctx, testGroup)
	if !ok || c.Status != chatMember || c.AddedBy != 7 || c.Title != "Book club" {
		t.Fatalf("group not registered: %+v", c)
	}
	// Being removed is recorded too.
	fx.bot.handleUpdate(ctx, telegram.Update{MyChatMember: &telegram.ChatMemberUpdated{
		Chat:          telegram.Chat{ID: testGroup, Type: "supergroup"},
		OldChatMember: telegram.ChatMember{Status: "member"},
		NewChatMember: telegram.ChatMember{Status: "kicked"},
	}})
	if c, _, _ := fx.bot.chats.Get(ctx, testGroup); c.Status != chatLeft {
		t.Errorf("removal not recorded: %+v", c)
	}
}

func TestGroupModeAdminsOnlyLetsAdminsAddTheBot(t *testing.T) {
	fx := groupFixture(t, Config{GroupMode: GroupsAdmins})
	ctx := context.Background()
	fx.bot.handleUpdate(ctx, joined(7, telegram.Chat{ID: testGroup, Type: "group"}))
	if len(fx.tg.find("leaveChat")) != 1 {
		t.Fatal("stayed in a group a non-admin added")
	}
	fx.bot.handleUpdate(ctx, joined(owner, telegram.Chat{ID: -2002, Type: "group"}))
	if len(fx.tg.find("leaveChat")) != 1 {
		t.Fatal("left a group the admin added")
	}
	fx.bot.handleUpdate(ctx, gmsg(7, -2002, "/say allowed here"))
	if fx.ttsIn == nil {
		t.Error("members of an admin-added group can't use the bot")
	}
	// A group the bot joined before the registry, with no admin on record.
	fx.bot.handleUpdate(ctx, gmsg(7, -3003, "/say who added me?"))
	if len(fx.tg.find("leaveChat")) != 2 {
		t.Error("stayed in a group no admin vouched for")
	}
}

func TestGroupModeOff(t *testing.T) {
	fx := groupFixture(t, Config{GroupMode: GroupsOff})
	fx.bot.handleUpdate(context.Background(), gmsg(7, testGroup, "/say hi"))
	if fx.ttsIn != nil || len(fx.tg.find("leaveChat")) != 1 {
		t.Fatal("worked in a group with groups turned off")
	}
}

func TestBlockedGroupsAreLeft(t *testing.T) {
	fx := groupFixture(t, Config{})
	ctx := context.Background()
	fx.bot.handleUpdate(ctx, msg(owner, "/blockgroup -1001"))
	if len(fx.tg.find("leaveChat")) != 1 {
		t.Fatal("blocking did not leave the group")
	}
	fx.bot.handleUpdate(ctx, gmsg(7, testGroup, "/say hi"))
	fx.bot.handleUpdate(ctx, joined(7, telegram.Chat{ID: testGroup, Type: "supergroup"}))
	if fx.ttsIn != nil || len(fx.tg.find("leaveChat")) != 3 {
		t.Errorf("a blocked group got service: tts=%v leaves=%d", fx.ttsIn, len(fx.tg.find("leaveChat")))
	}
	fx.bot.handleUpdate(ctx, msg(owner, "/unblockgroup -1001"))
	fx.bot.handleUpdate(ctx, joined(7, telegram.Chat{ID: testGroup, Type: "supergroup"}))
	fx.bot.handleUpdate(ctx, gmsg(7, testGroup, "/say back again"))
	if fx.ttsIn == nil {
		t.Error("an unblocked group still can't use the bot")
	}
}

func TestGroupAdminCommandsNeedTheOwner(t *testing.T) {
	fx := groupFixture(t, Config{})
	ctx := context.Background()
	fx.bot.handleUpdate(ctx, msg(7, "/blockgroup -1001"))
	fx.bot.handleUpdate(ctx, gmsg(7, testGroup, "/blockgroup"))
	if len(fx.tg.find("leaveChat")) != 0 {
		t.Fatal("an ordinary member blocked a group")
	}
	fx.bot.handleUpdate(ctx, msg(7, "/groups"))
	if strings.Contains(fx.tg.lastText(), "Groups today") {
		t.Error("an ordinary member saw the groups list")
	}
	fx.bot.handleUpdate(ctx, gmsg(7, testGroup, "/say hello"))
	fx.bot.handleUpdate(ctx, msg(owner, "/groups"))
	if text := fx.tg.lastText(); !strings.Contains(text, "Test group") || !strings.Contains(text, "-1001") {
		t.Errorf("groups list = %q", text)
	}
}

func TestGroupUpgradeKeepsItsRecord(t *testing.T) {
	fx := groupFixture(t, Config{})
	ctx := context.Background()
	fx.bot.handleUpdate(ctx, gmsg(7, testGroup, "/say hello"))
	up := gmsg(7, testGroup, "")
	up.Message.MigrateToChatID = -100777
	fx.bot.handleUpdate(ctx, up)
	if _, ok, _ := fx.bot.chats.Get(ctx, testGroup); ok {
		t.Error("the old group ID is still registered")
	}
	c, ok, _ := fx.bot.chats.Get(ctx, -100777)
	if !ok || c.Chars != 5 || c.Status != chatMember || c.Type != "supergroup" {
		t.Errorf("record not carried to the supergroup: %+v", c)
	}
}

func TestDeepLinkOpensTheScreen(t *testing.T) {
	fx := groupFixture(t, Config{})
	fx.bot.handleUpdate(context.Background(), msg(7, "/start style"))
	if want := fx.bot.styleScreen(7).text; fx.tg.lastText() != want {
		t.Errorf("deep link opened %q, want the Style screen", fx.tg.lastText())
	}
}

func TestMentionCommand(t *testing.T) {
	note := &telegram.Message{Voice: &telegram.Voice{FileID: "f"}}
	for _, tc := range []struct {
		rest, reply, cmd, args string
		audio                  bool
	}{
		{"say hi there", "", "say", "hi there", false},
		{"transcribe", "", "transcribe", "", false},
		{"good morning", "", "say", "good morning", false},
		{"", "quoted", "say", "", false},
		{"", "", "transcribe", "", true},
		{"", "", "help", "", false},
	} {
		m := &telegram.Message{}
		if tc.reply != "" {
			m.ReplyToMessage = &telegram.Message{Text: tc.reply}
		}
		if tc.audio {
			m.ReplyToMessage = note
		}
		if cmd, args := mentionCommand(tc.rest, m); cmd != tc.cmd || args != tc.args {
			t.Errorf("mentionCommand(%q) = %q %q, want %q %q", tc.rest, cmd, args, tc.cmd, tc.args)
		}
	}
}

func TestGroupExemptionsNeedANamedAdmin(t *testing.T) {
	// With no ADMIN_USER_IDS every user counts as an admin in the private
	// chat, but nobody may skip a group's shared limits or add the bot
	// under GroupsAdmins.
	fx := groupFixture(t, Config{AdminUsers: map[int64]bool{}, GroupLimits: GroupLimits{Chars: 4}, GroupMode: GroupsAdmins})
	ctx := context.Background()
	fx.bot.handleUpdate(ctx, joined(7, telegram.Chat{ID: testGroup, Type: "group"}))
	if len(fx.tg.find("leaveChat")) != 1 {
		t.Fatal("anyone could add the bot because no admins were named")
	}

	fx = groupFixture(t, Config{AdminUsers: map[int64]bool{}, GroupLimits: GroupLimits{Chars: 4}})
	fx.bot.handleUpdate(ctx, gmsg(7, testGroup, "/say too long"))
	if fx.ttsIn != nil {
		t.Error("a member skipped the group's allowance because no admins were named")
	}
}

func TestFloodingGroupDropsNewWork(t *testing.T) {
	fx := groupFixture(t, Config{})
	fx.bot.tg.SetGroupPacing(time.Hour, 1, time.Hour) // one send, then a long backlog
	ctx := context.Background()
	fx.bot.handleUpdate(ctx, gmsg(7, testGroup, "/say first"))
	if fx.ttsIn == nil {
		t.Fatal("the first request was refused")
	}
	fx.ttsIn = nil
	before := fx.tg.count()
	fx.bot.handleUpdate(ctx, gmsg(8, testGroup, "/say second"))
	if fx.ttsIn != nil || fx.tg.count() != before {
		t.Error("started work, or replied, in a group whose sends are backed up")
	}
	// Other groups are unaffected.
	fx.bot.handleUpdate(ctx, gmsg(8, -2002, "/say elsewhere"))
	if fx.ttsIn == nil {
		t.Error("another group was held up")
	}
}
