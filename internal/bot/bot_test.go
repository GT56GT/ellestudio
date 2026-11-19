package bot

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"ellestudio/internal/cartesia"
	"ellestudio/internal/telegram"
)

// fakeTelegram records Bot API calls and serves one downloadable file.
type fakeTelegram struct {
	mu    sync.Mutex
	calls []tgCall
}

type tgCall struct {
	Method string
	JSON   map[string]any
	Form   map[string]string
	File   []byte
}

func (f *fakeTelegram) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/file/botTOKEN/") {
			_, _ = io.WriteString(w, "OggS-voice-bytes")
			return
		}
		method := strings.TrimPrefix(r.URL.Path, "/botTOKEN/")
		c := tgCall{Method: method, Form: map[string]string{}}
		if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Error(err)
				return
			}
			for k, v := range r.MultipartForm.Value {
				c.Form[k] = v[0]
			}
			for _, fhs := range r.MultipartForm.File {
				fh, _ := fhs[0].Open()
				c.File, _ = io.ReadAll(fh)
				c.Form["_filename"] = fhs[0].Filename
			}
		} else {
			_ = json.NewDecoder(r.Body).Decode(&c.JSON)
		}
		f.mu.Lock()
		f.calls = append(f.calls, c)
		f.mu.Unlock()

		result := `{"message_id":99,"chat":{"id":1}}`
		switch method {
		case "getFile":
			result = `{"file_id":"f1","file_size":16,"file_path":"voice/file_1.oga"}`
		case "sendChatAction", "answerCallbackQuery", "editMessageText", "setMyCommands":
			result = `true`
		}
		_, _ = io.WriteString(w, `{"ok":true,"result":`+result+`}`)
	}
}

func (f *fakeTelegram) find(method string) []tgCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []tgCall
	for _, c := range f.calls {
		if c.Method == method {
			out = append(out, c)
		}
	}
	return out
}

type fixture struct {
	bot       *Bot
	tg        *fakeTelegram
	ttsIn     map[string]any
	sttIn     map[string]string
	cloned    bool
	cloneDesc string
	ttsFail   bool
}

func press(uid int64, data string) telegram.Update {
	return telegram.Update{CallbackQuery: &telegram.CallbackQuery{
		ID: "cb", From: telegram.User{ID: uid}, Data: data,
		Message: &telegram.Message{MessageID: 5, Chat: telegram.Chat{ID: uid}, Text: "screen"},
	}}
}

// lastText returns the text of the most recent sendMessage or editMessageText.
func (f *fakeTelegram) lastText() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := len(f.calls) - 1; i >= 0; i-- {
		c := f.calls[i]
		if c.Method == "sendMessage" || c.Method == "editMessageText" {
			s, _ := c.JSON["text"].(string)
			return s
		}
	}
	return ""
}

func newFixture(t *testing.T, cfg Config) *fixture {
	t.Helper()
	fx := &fixture{tg: &fakeTelegram{}}
	tgSrv := httptest.NewServer(fx.tg.handler(t))
	t.Cleanup(tgSrv.Close)

	ctSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/tts/bytes":
			if fx.ttsFail {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = io.WriteString(w, `{"error_code":null,"title":"Unexpected error","message":"boom","request_id":"r1"}`)
				return
			}
			_ = json.NewDecoder(r.Body).Decode(&fx.ttsIn)
			w.Header().Set("Content-Type", "audio/mpeg")
			_, _ = io.WriteString(w, "ID3-mp3")
		case "/stt":
			_ = r.ParseMultipartForm(1 << 20)
			fx.sttIn = map[string]string{"language": r.FormValue("language"), "filename": r.MultipartForm.File["file"][0].Filename}
			_, _ = io.WriteString(w, `{"type":"transcript","text":"hello from a voice note","duration":2.5,"language":"en"}`)
		case "/voices/v-new":
			_, _ = io.WriteString(w, `{"id":"v-new","name":"Nova","language":"en"}`)
		case "/voices":
			_, _ = io.WriteString(w, `{"data":[`+
				`{"id":"v-new","name":"Nova","language":"en","gender":"feminine"},`+
				`{"id":"v-secret","name":"Someone Else","is_owner":true,"description":"Cloned in Ellie Studio [ellie-owner:99]"},`+
				`{"id":"v-mine","name":"My Clone","is_owner":true,"description":"Cloned in Ellie Studio [ellie-owner:7]"},`+
				`{"id":"v-legacy","name":"Old Clone","is_owner":true,"description":"made elsewhere"}`+
				`],"has_more":false}`)
		case "/voices/v-secret":
			_, _ = io.WriteString(w, `{"id":"v-secret","name":"Someone Else","is_owner":true,"description":"Cloned in Ellie Studio [ellie-owner:99]","preview_file_url":"`+"http://"+r.Host+`/p.mp3"}`)
		case "/voices/v-mine":
			_, _ = io.WriteString(w, `{"id":"v-mine","name":"My Clone","is_owner":true,"description":"Cloned in Ellie Studio [ellie-owner:7]"}`)
		case "/pronunciation-dicts/d-secret":
			_, _ = io.WriteString(w, `{"id":"d-secret","name":"Their words","is_owner":true,"description":"Created in Ellie Studio [ellie-owner:99]","items":[]}`)
		case "/voices/clone":
			_ = r.ParseMultipartForm(1 << 20)
			fx.cloneDesc = r.FormValue("description")
			fx.cloned = true
			_, _ = io.WriteString(w, `{"id":"v-clone","name":"My Voice","language":"en"}`)
		case "/models":
			_, _ = io.WriteString(w, `{"data":[{"id":"sonic-3.6","capability":"tts","supported_languages":["en","fr"]},{"id":"ink-whisper","capability":"stt"}]}`)
		case "/pronunciation-dicts/":
			_, _ = io.WriteString(w, `{"data":[],"has_more":false}`)
		case "/":
			_, _ = io.WriteString(w, `{"ok":true,"version":"2026-08-14"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(ctSrv.Close)

	store, err := OpenStore(filepath.Join(t.TempDir(), "settings.json"), Settings{
		VoiceID: "v-default", VoiceName: "Ellie", Model: cartesia.DefaultTTSModel,
	})
	if err != nil {
		t.Fatal(err)
	}
	fx.bot = New(
		telegram.New("TOKEN", tgSrv.URL),
		cartesia.New("sk_car_test", cartesia.WithBaseURL(ctSrv.URL)),
		store, cfg, slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	return fx
}

func msg(uid int64, text string) telegram.Update {
	return telegram.Update{Message: &telegram.Message{
		MessageID: 10, From: &telegram.User{ID: uid}, Chat: telegram.Chat{ID: uid}, Text: text,
	}}
}

func TestTextBecomesVoiceNote(t *testing.T) {
	fx := newFixture(t, Config{})
	ctx := context.Background()
	fx.bot.handleUpdate(ctx, msg(7, "/speed 1.2"))
	fx.bot.handleUpdate(ctx, msg(7, "/emotion calm"))
	fx.bot.handleUpdate(ctx, msg(7, "Hello from Ellie Studio"))

	sent := fx.tg.find("sendVoice")
	if len(sent) != 1 {
		t.Fatalf("sendVoice calls = %d (all: %+v)", len(sent), fx.tg.calls)
	}
	if string(sent[0].File) != "ID3-mp3" || sent[0].Form["_filename"] != "ellie-studio.mp3" {
		t.Errorf("voice upload = %q %q", sent[0].File, sent[0].Form["_filename"])
	}
	if !strings.Contains(sent[0].Form["caption"], "Ellie") || !strings.Contains(sent[0].Form["caption"], "calm") {
		t.Errorf("caption = %q", sent[0].Form["caption"])
	}
	if fx.ttsIn["transcript"] != "Hello from Ellie Studio" || fx.ttsIn["model_id"] != "sonic-3.6" {
		t.Errorf("tts body = %v", fx.ttsIn)
	}
	if v := fx.ttsIn["voice"].(map[string]any); v["id"] != "v-default" {
		t.Errorf("voice = %v", v)
	}
	gc := fx.ttsIn["generation_config"].(map[string]any)
	if gc["speed"].(float64) != 1.2 || gc["emotion"] != "calm" {
		t.Errorf("generation_config = %v", gc)
	}
	if !strings.Contains(sent[0].Form["reply_markup"], `"do:again"`) {
		t.Errorf("voice note should carry the Again/Emotion/Voice/Style buttons: %q", sent[0].Form["reply_markup"])
	}
}

func TestAgainRepeatsLastText(t *testing.T) {
	fx := newFixture(t, Config{})
	ctx := context.Background()
	fx.bot.handleUpdate(ctx, msg(7, "First line"))
	fx.ttsIn = nil
	fx.bot.handleUpdate(ctx, press(7, "do:again"))
	if fx.ttsIn["transcript"] != "First line" {
		t.Fatalf("Again should resend the last text, got %v", fx.ttsIn)
	}
}

func TestEveryScreenHasAWayHome(t *testing.T) {
	fx := newFixture(t, Config{})
	ctx := context.Background()
	names := []string{"style", "emotion", "emotions", "speed", "volume", "lang", "model", "output",
		"voices", "studio", "dicts", "create", "localize", "help", "status", "usage"}
	for _, n := range names {
		s := fx.bot.navScreen(ctx, 7, n)
		if s.kb == nil || len(s.kb.InlineKeyboard) == 0 {
			t.Errorf("%s: no buttons", n)
			continue
		}
		last := s.kb.InlineKeyboard[len(s.kb.InlineKeyboard)-1]
		if last[len(last)-1].CallbackData != "nav:home" {
			t.Errorf("%s: last row should end with Home, got %+v", n, last)
		}
		for _, row := range s.kb.InlineKeyboard {
			for _, bt := range row {
				if len(bt.CallbackData) > 64 || bt.CallbackData == "" {
					t.Errorf("%s: bad callback_data %q", n, bt.CallbackData)
				}
			}
		}
	}
	for _, topic := range []string{"", "speak", "voices", "listen", "create", "commands"} {
		kb := helpScreen(topic).kb.InlineKeyboard
		if last := kb[len(kb)-1]; last[len(last)-1].CallbackData != "nav:home" {
			t.Errorf("help %q: no Home", topic)
		}
	}
}

func TestPickerRedrawsWithCheckmark(t *testing.T) {
	fx := newFixture(t, Config{})
	fx.bot.handleUpdate(context.Background(), press(7, "s:1.2"))
	edits := fx.tg.find("editMessageText")
	if len(edits) != 1 {
		t.Fatalf("expected the speed picker to redraw in place, got %d edits", len(edits))
	}
	if !strings.Contains(edits[0].JSON["text"].(string), "1.2×") {
		t.Errorf("picker text = %q", edits[0].JSON["text"])
	}
	kb, _ := json.Marshal(edits[0].JSON["reply_markup"])
	if !strings.Contains(string(kb), "✅ 1.2×") || !strings.Contains(string(kb), "nav:home") {
		t.Errorf("picker keyboard should mark 1.2× and keep Home: %s", kb)
	}
	if fx.bot.store.Get(7).Speed != 1.2 {
		t.Error("speed not saved")
	}
}

func TestStartShowsMenuKeyboardAndHome(t *testing.T) {
	fx := newFixture(t, Config{})
	fx.bot.handleUpdate(context.Background(), msg(7, "/start"))
	sent := fx.tg.find("sendMessage")
	if len(sent) != 2 {
		t.Fatalf("expected welcome + home card, got %d messages", len(sent))
	}
	kb, _ := json.Marshal(sent[0].JSON["reply_markup"])
	if !strings.Contains(string(kb), `"keyboard"`) || !strings.Contains(string(kb), menuVoices) {
		t.Errorf("welcome should pin the menu keyboard: %s", kb)
	}
	if !strings.Contains(sent[1].JSON["text"].(string), "Ellie Studio") {
		t.Errorf("second message should be the home card: %q", sent[1].JSON["text"])
	}
	if !fx.bot.store.Known(7) {
		t.Error("user should be remembered after onboarding")
	}
}

func TestMenuButtonIsNotSpoken(t *testing.T) {
	fx := newFixture(t, Config{})
	fx.bot.handleUpdate(context.Background(), msg(7, menuVoices))
	if fx.ttsIn != nil {
		t.Fatal("menu label must not be sent to TTS")
	}
	if !strings.Contains(fx.tg.lastText(), "Nova") {
		t.Errorf("voices screen should list the library, got %q", fx.tg.lastText())
	}
}

func TestGuidedCloneFlow(t *testing.T) {
	fx := newFixture(t, Config{})
	ctx := context.Background()
	fx.bot.handleUpdate(ctx, press(7, "do:clone"))
	if !strings.Contains(fx.tg.lastText(), "Step 1 of 2") {
		t.Fatalf("clone should start at step 1, got %q", fx.tg.lastText())
	}
	fx.bot.handleUpdate(ctx, msg(7, "not audio"))
	if fx.ttsIn != nil || !strings.Contains(fx.tg.lastText(), "recording") {
		t.Fatalf("text during step 1 should be redirected, not spoken: %q", fx.tg.lastText())
	}
	u := msg(7, "")
	u.Message.Voice = &telegram.Voice{FileID: "f1", Duration: 8, FileSize: 16}
	fx.bot.handleUpdate(ctx, u)
	if !strings.Contains(fx.tg.lastText(), "Step 2 of 2") {
		t.Fatalf("clip should advance to step 2, got %q", fx.tg.lastText())
	}
	fx.bot.handleUpdate(ctx, msg(7, "My Voice"))
	if !fx.cloned {
		t.Fatal("clone endpoint was not called")
	}
	if s := fx.bot.store.Get(7); s.VoiceID != "v-clone" {
		t.Errorf("cloned voice should become current, got %+v", s)
	}
	if !strings.Contains(fx.tg.lastText(), "is ready") {
		t.Errorf("expected a ready card, got %q", fx.tg.lastText())
	}
}

func TestCancelEndsFlow(t *testing.T) {
	fx := newFixture(t, Config{})
	ctx := context.Background()
	fx.bot.handleUpdate(ctx, press(7, "do:search"))
	fx.bot.handleUpdate(ctx, msg(7, "/cancel"))
	if fx.bot.getFlow(7) != nil {
		t.Fatal("flow should be cleared")
	}
	fx.bot.handleUpdate(ctx, msg(7, "hello"))
	if fx.ttsIn == nil {
		t.Fatal("after cancel, text should be spoken again")
	}
}

func TestVoiceNoteIsTranscribed(t *testing.T) {
	fx := newFixture(t, Config{})
	ctx := context.Background()
	fx.bot.handleUpdate(ctx, msg(7, "/lang en-gb"))
	u := msg(7, "")
	u.Message.Voice = &telegram.Voice{FileID: "f1", Duration: 3, FileSize: 16}
	fx.bot.handleUpdate(ctx, u)

	if fx.sttIn["language"] != "en" || fx.sttIn["filename"] != "voice.ogg" {
		t.Errorf("stt input = %v", fx.sttIn)
	}
	edits := fx.tg.find("editMessageText")
	if len(edits) == 0 {
		t.Fatal("the ⏳ progress message should turn into the transcript")
	}
	last := edits[len(edits)-1].JSON["text"].(string)
	if !strings.Contains(last, "hello from a voice note") || !strings.Contains(last, "2.5s") {
		t.Errorf("transcript = %q", last)
	}
	fx.bot.handleUpdate(ctx, press(7, "do:readback"))
	if fx.ttsIn["transcript"] != "hello from a voice note" {
		t.Errorf("Read aloud should speak the transcript, got %v", fx.ttsIn)
	}
}

func TestPrivateStudioRejectsStrangers(t *testing.T) {
	fx := newFixture(t, Config{AllowedUsers: map[int64]bool{1: true}})
	fx.bot.handleUpdate(context.Background(), msg(666, "speak for me"))
	if len(fx.tg.find("sendVoice")) != 0 || fx.ttsIn != nil {
		t.Fatal("stranger should not reach Cartesia")
	}
	m := fx.tg.find("sendMessage")
	if len(m) != 1 || !strings.Contains(m[0].JSON["text"].(string), "private") {
		t.Fatalf("expected a private-studio notice, got %+v", m)
	}
}

func TestVoiceButtonSelectsVoice(t *testing.T) {
	fx := newFixture(t, Config{})
	fx.bot.handleUpdate(context.Background(), telegram.Update{CallbackQuery: &telegram.CallbackQuery{
		ID: "cb", From: telegram.User{ID: 7}, Data: "v:v-new",
		Message: &telegram.Message{MessageID: 5, Chat: telegram.Chat{ID: 7}},
	}})
	if s := fx.bot.store.Get(7); s.VoiceID != "v-new" || s.VoiceName != "Nova" {
		t.Fatalf("settings = %+v", s)
	}
}

func TestTooLongTextIsRefused(t *testing.T) {
	fx := newFixture(t, Config{MaxTTSChars: 10})
	fx.bot.handleUpdate(context.Background(), msg(7, strings.Repeat("a", 11)))
	if fx.ttsIn != nil {
		t.Fatal("over-limit text must not be sent to Cartesia")
	}
}

func TestParseCommand(t *testing.T) {
	cases := []struct{ in, cmd, args string }{
		{"/say hi there", "say", "hi there"},
		{"/Voices@EllieStudioBot warm", "voices", "warm"},
		{"/newdict\nBrand\nA = b", "newdict", "Brand\nA = b"},
		{"hello", "", ""},
	}
	for _, c := range cases {
		cmd, args := parseCommand(c.in)
		if cmd != c.cmd || args != c.args {
			t.Errorf("parseCommand(%q) = %q, %q", c.in, cmd, args)
		}
	}
}

func TestParseDict(t *testing.T) {
	name, items, err := parseDict("Brand words\nCartesia = car-TEE-zhuh\nEllie => EH-lee\n\nGo: gopher")
	if err != nil || name != "Brand words" || len(items) != 3 || items[1].Pronunciation != "EH-lee" {
		t.Fatalf("parseDict = %q %+v %v", name, items, err)
	}
	if _, _, err := parseDict("Only a name"); err == nil {
		t.Error("expected error with no entries")
	}
	if _, _, err := parseDict("Name\nbroken line"); err == nil {
		t.Error("expected error for a malformed line")
	}
}

func TestSplitTextKeepsUTF8(t *testing.T) {
	s := strings.Repeat("héllo wörld ", 500)
	parts := splitText(s, 100)
	for _, p := range parts {
		if len(p) > 100 || !strings.HasPrefix(p, "h") && !strings.HasPrefix(p, "w") {
			t.Fatalf("bad chunk %q", p)
		}
	}
	if strings.TrimSpace(strings.Join(parts, " ")) != strings.TrimSpace(s) {
		t.Fatal("chunks lost content")
	}
}

func TestNormalizeLocale(t *testing.T) {
	for in, want := range map[string]string{"en-gb": "en-GB", "FR": "fr", "es-mx": "es-MX", "zh-Hant": "zh-Hant"} {
		if got := normalizeLocale(in); got != want {
			t.Errorf("normalizeLocale(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestStorePersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.json")
	s, _ := OpenStore(path, Settings{Model: "sonic-3.6"})
	if _, err := s.Update(42, func(v *Settings) { v.Emotion = "happy"; v.VoiceID = "abc" }); err != nil {
		t.Fatal(err)
	}
	s2, err := OpenStore(path, Settings{Model: "sonic-3.6"})
	if err != nil {
		t.Fatal(err)
	}
	got := s2.Get(42)
	if got.Emotion != "happy" || got.VoiceID != "abc" || got.Model != "sonic-3.6" || got.Speed != 1 {
		t.Fatalf("reloaded = %+v", got)
	}
	if err := s2.ForgetVoice("abc"); err != nil || s2.Get(42).VoiceID != "" {
		t.Fatalf("ForgetVoice failed: %+v %v", s2.Get(42), err)
	}
}

func TestDailyCharLimit(t *testing.T) {
	fx := newFixture(t, Config{AdminUsers: map[int64]bool{1: true}, DailyChars: 10})
	ctx := context.Background()

	fx.bot.handleUpdate(ctx, msg(7, "hello"))
	if fx.ttsIn == nil {
		t.Fatal("5 of 10 characters should be spoken")
	}
	fx.ttsIn = nil
	fx.bot.handleUpdate(ctx, msg(7, "hello world"))
	if fx.ttsIn != nil {
		t.Fatal("11 more characters must be refused")
	}
	if got := fx.tg.lastText(); !strings.Contains(got, "Daily limit reached") || !strings.Contains(got, "5</b> are left") {
		t.Errorf("limit screen = %q", got)
	}

	if err := fx.bot.store.Reset(7); err != nil {
		t.Fatal(err)
	}
	fx.bot.handleUpdate(ctx, msg(7, "sixsix"))
	if fx.ttsIn != nil {
		t.Fatal("resetting settings must not refill the allowance")
	}

	fx.bot.handleUpdate(ctx, msg(1, strings.Repeat("a", 50)))
	if fx.ttsIn == nil {
		t.Fatal("admins are exempt from the allowance")
	}
	if !strings.Contains(fx.bot.studioScreen(7).text, "5 of 10 characters") {
		t.Errorf("studio should show what's left: %q", fx.bot.studioScreen(7).text)
	}
}

func TestFailedSpeechIsRefunded(t *testing.T) {
	fx := newFixture(t, Config{AdminUsers: map[int64]bool{1: true}, DailyChars: 10})
	fx.ttsFail = true
	fx.bot.handleUpdate(context.Background(), msg(7, "hello"))
	if u := fx.bot.store.UsageOn(7, today()); u.Chars != 0 {
		t.Fatalf("failed speech should be refunded, used = %d", u.Chars)
	}
	if !strings.Contains(fx.tg.lastText(), "Something went wrong") {
		t.Errorf("error screen = %q", fx.tg.lastText())
	}
}

func TestDailyTranscriptionLimit(t *testing.T) {
	fx := newFixture(t, Config{AdminUsers: map[int64]bool{1: true}, DailyAudioSecs: 5})
	ctx := context.Background()
	voice := func(secs int) telegram.Update {
		u := msg(7, "")
		u.Message.Voice = &telegram.Voice{FileID: "f1", Duration: secs, FileSize: 16}
		return u
	}
	fx.bot.handleUpdate(ctx, voice(2)) // fake STT reports 2.5s, so 3s are recorded
	if fx.sttIn == nil {
		t.Fatal("first recording should be transcribed")
	}
	if u := fx.bot.store.UsageOn(7, today()); u.Secs != 3 {
		t.Errorf("usage should settle to the real duration (3s), got %d", u.Secs)
	}
	fx.sttIn = nil
	fx.bot.handleUpdate(ctx, voice(3))
	if fx.sttIn != nil {
		t.Fatal("3s more would pass the 5s allowance")
	}
	if !strings.Contains(fx.tg.lastText(), "Daily limit reached") {
		t.Errorf("limit screen = %q", fx.tg.lastText())
	}
}

func TestOneRequestAtATime(t *testing.T) {
	fx := newFixture(t, Config{})
	if !fx.bot.begin(7) {
		t.Fatal("first job should start")
	}
	fx.bot.handleUpdate(context.Background(), msg(7, "hello"))
	if fx.ttsIn != nil || !strings.Contains(fx.tg.lastText(), "still working") {
		t.Fatalf("a second job while busy should wait: %q", fx.tg.lastText())
	}
	fx.bot.end(7)
	fx.bot.handleUpdate(context.Background(), msg(7, "hello"))
	if fx.ttsIn == nil {
		t.Fatal("after the first job ends, the next should run")
	}
}

func TestAudioEstimateDoesNotTrustClaimedDuration(t *testing.T) {
	// A 2 MB file claiming 1 second (or sent as a document with no duration)
	// is reserved by size: 2 MB at 16 kbps is about 1,048 seconds.
	if got := audioEstimate(audioRef{Seconds: 1, Size: 2 << 20}); got < 1000 {
		t.Fatalf("estimate = %ds, want the size-based floor", got)
	}
	if got := audioEstimate(audioRef{Seconds: 30, Size: 1000}); got != 30 {
		t.Fatalf("estimate = %ds, want the longer claimed duration", got)
	}

	fx := newFixture(t, Config{AdminUsers: map[int64]bool{1: true}, DailyAudioSecs: 600})
	u := msg(7, "")
	u.Message.Document = &telegram.Document{FileID: "f1", FileName: "talk.mp3", MimeType: "audio/mpeg", FileSize: 2 << 20}
	fx.bot.handleUpdate(context.Background(), u)
	if fx.sttIn != nil {
		t.Fatal("a ~17 minute file must not slip past a 10 minute allowance")
	}
}

func TestOverEstimateIsRefunded(t *testing.T) {
	fx := newFixture(t, Config{AdminUsers: map[int64]bool{1: true}, DailyAudioSecs: 600})
	u := msg(7, "")
	u.Message.Voice = &telegram.Voice{FileID: "f1", Duration: 2, FileSize: 40000} // estimated 20s
	fx.bot.handleUpdate(context.Background(), u)
	if got := fx.bot.store.UsageOn(7, today()).Secs; got != 3 {
		t.Fatalf("usage should settle to the real 2.5s (3s), got %ds", got)
	}
}

func TestOwnerTag(t *testing.T) {
	d := withOwner("Cloned in Ellie Studio", 42)
	if id, ok := ownerOf(d); !ok || id != 42 {
		t.Fatalf("ownerOf(%q) = %d, %v", d, id, ok)
	}
	if got := cleanDescription(d); got != "Cloned in Ellie Studio" {
		t.Errorf("cleanDescription = %q", got)
	}
	if _, ok := ownerOf("no tag here"); ok {
		t.Error("untagged description has no owner")
	}
}

func TestDictionariesArePrivate(t *testing.T) {
	fx := newFixture(t, Config{AdminUsers: map[int64]bool{1: true}})
	fx.bot.handleUpdate(context.Background(), press(7, "d:d-secret"))
	if fx.bot.store.Get(7).DictID == "d-secret" {
		t.Fatal("user 7 must not be able to use someone else's dictionary")
	}
}

func TestClonesAreTaggedWithTheirCreator(t *testing.T) {
	fx := newFixture(t, Config{AdminUsers: map[int64]bool{1: true}})
	u := msg(1, "/clone Studio Voice")
	u.Message.Voice = &telegram.Voice{FileID: "f1", Duration: 8, FileSize: 16}
	fx.bot.handleUpdate(context.Background(), u)
	if owner, ok := ownerOf(fx.cloneDesc); !ok || owner != 1 {
		t.Fatalf("clone description %q should carry the creator's tag", fx.cloneDesc)
	}
}

func TestOnlyTheCreatorManagesAVoice(t *testing.T) {
	fx := newFixture(t, Config{AdminUsers: map[int64]bool{1: true}})
	ctx := context.Background()
	if s := fx.bot.deleteConfirmScreen(ctx, 7, "v-secret"); !strings.Contains(s.text, "only delete voices you created") {
		t.Errorf("deleting someone else's voice must be refused: %q", s.text)
	}
	if s := fx.bot.deleteConfirmScreen(ctx, 7, "v-mine"); !strings.Contains(s.text, "Delete My Clone?") {
		t.Errorf("the creator should be asked to confirm: %q", s.text)
	}
	if s := fx.bot.shareScreen(ctx, 7, "v-secret"); !strings.Contains(s.text, "Only the person who created") {
		t.Errorf("sharing someone else's voice must be refused: %q", s.text)
	}
	card := fx.bot.voiceCard(ctx, 7, "v-secret")
	kb, _ := json.Marshal(card.kb)
	if strings.Contains(string(kb), "sh:") || strings.Contains(string(kb), "dv:") {
		t.Errorf("a shared voice's card must not offer Share/Delete: %s", kb)
	}
}

func TestVoicesAreUnlistedButOpenByID(t *testing.T) {
	fx := newFixture(t, Config{AdminUsers: map[int64]bool{1: true}})
	ctx := context.Background()

	// The library shows public voices and your own, never anyone else's.
	list := fx.bot.voicesScreen(ctx, 7, fx.bot.resetSession(7, "", nil))
	for _, want := range []string{"Nova", "My Clone"} {
		if !strings.Contains(list.text, want) {
			t.Errorf("user 7 should see %q", want)
		}
	}
	for _, hidden := range []string{"Someone Else", "Old Clone"} {
		if strings.Contains(list.text, hidden) {
			t.Errorf("%q must not be listed for user 7", hidden)
		}
	}
	owned := true
	mine := fx.bot.voicesScreen(ctx, 7, fx.bot.resetSession(7, "", &owned))
	if !strings.Contains(mine.text, "My Clone") || strings.Contains(mine.text, "Nova") {
		t.Errorf("🗂 Mine should list only user 7's creations: %q", mine.text)
	}

	// But the ID is the key: pasting it opens the voice instead of reading it aloud.
	fx.bot.handleUpdate(ctx, msg(7, "v-secret-is-not-a-uuid"))
	fx.ttsIn = nil
	id := "a0e99841-438c-4a64-b679-ae501e7d6091"
	fx.bot.handleUpdate(ctx, msg(7, id))
	if fx.ttsIn != nil {
		t.Fatal("a pasted voice ID must not be spoken")
	}
	fx.bot.handleUpdate(ctx, msg(7, "https://t.me/EllieStudioBot?start=v_"+strings.ToUpper(id)))
	if fx.ttsIn != nil {
		t.Fatal("a pasted share link must not be spoken")
	}

	// Choosing an unlisted voice by its ID works.
	fx.bot.handleUpdate(ctx, press(7, "v:v-secret"))
	if fx.bot.store.Get(7).VoiceID != "v-secret" {
		t.Fatal("anyone with the ID should be able to use the voice")
	}
	card := fx.bot.voiceCard(ctx, 7, "v-secret")
	if !strings.Contains(card.text, "shared with you") {
		t.Errorf("card should explain it was shared: %q", card.text)
	}

	admin := fx.bot.voicesScreen(ctx, 1, fx.bot.resetSession(1, "", nil))
	if !strings.Contains(admin.text, "Someone Else") || !strings.Contains(admin.text, "Old Clone") {
		t.Error("admins see every voice")
	}
}

func TestParseVoiceRef(t *testing.T) {
	id := "a0e99841-438c-4a64-b679-ae501e7d6091"
	for _, in := range []string{id, strings.ToUpper(id), "v_" + id, "https://t.me/EllieStudioBot?start=v_" + id, "  " + id + "\n"} {
		if got, ok := parseVoiceRef(in); !ok || got != id {
			t.Errorf("parseVoiceRef(%q) = %q, %v", in, got, ok)
		}
	}
	for _, in := range []string{"hello there", "my voice is " + id, "https://example.com", id + "x", ""} {
		if _, ok := parseVoiceRef(in); ok {
			t.Errorf("parseVoiceRef(%q) should not match", in)
		}
	}
}

func TestShareLinkOpensTheVoice(t *testing.T) {
	fx := newFixture(t, Config{AdminUsers: map[int64]bool{1: true}})
	share := fx.bot.shareScreen(context.Background(), 7, "v-mine")
	if !strings.Contains(share.text, "?start=v_v-mine") || !strings.Contains(share.text, "<code>v-mine</code>") {
		t.Errorf("share screen should give the link and the ID: %q", share.text)
	}
	kb, _ := json.Marshal(share.kb)
	if !strings.Contains(string(kb), "t.me/share/url") {
		t.Errorf("share screen should offer a Send to a friend button: %s", kb)
	}
	id := "a0e99841-438c-4a64-b679-ae501e7d6091"
	fx.bot.handleUpdate(context.Background(), msg(8, "/start v_"+id))
	sent := fx.tg.find("sendMessage")
	if len(sent) != 2 || !strings.Contains(sent[0].JSON["text"].(string), "shared a voice") {
		t.Fatalf("a share link should greet and open the voice, got %d messages", len(sent))
	}
}

func TestAnyoneCanCloneWithinTheDailyLimit(t *testing.T) {
	fx := newFixture(t, Config{AdminUsers: map[int64]bool{1: true}, DailyCreations: 1})
	ctx := context.Background()
	clone := func(name string) {
		u := msg(7, "/clone "+name)
		u.Message.Voice = &telegram.Voice{FileID: "f1", Duration: 8, FileSize: 16}
		fx.bot.handleUpdate(ctx, u)
	}
	clone("First")
	if !fx.cloned {
		t.Fatal("a regular user should be able to clone")
	}
	if owner, ok := ownerOf(fx.cloneDesc); !ok || owner != 7 {
		t.Errorf("the clone should be tagged with user 7: %q", fx.cloneDesc)
	}
	fx.cloned = false
	clone("Second")
	if fx.cloned || !strings.Contains(fx.tg.lastText(), "Daily voice limit reached") {
		t.Fatalf("the second clone should hit the daily limit: %q", fx.tg.lastText())
	}
}

func TestDictionariesHaveADailyLimit(t *testing.T) {
	fx := newFixture(t, Config{AdminUsers: map[int64]bool{1: true}, DailyDicts: 1})
	created := 0
	fx.bot.ct = cartesiaWithDicts(t, &created)
	ctx := context.Background()
	fx.bot.handleUpdate(ctx, msg(7, "/newdict First\nA = ay"))
	fx.bot.handleUpdate(ctx, msg(7, "/newdict Second\nB = bee"))
	if created != 1 || !strings.Contains(fx.tg.lastText(), "Daily dictionary limit reached") {
		t.Fatalf("created %d dictionaries, last reply %q", created, fx.tg.lastText())
	}
}

func TestDescriptionsNeverRevealVoiceIDs(t *testing.T) {
	id := "a0e99841-438c-4a64-b679-ae501e7d6091"
	v := &cartesia.Voice{ID: "x", Name: "Loc", IsOwner: true,
		Description: "Localized in Ellie Studio from " + id + " [ellie-owner:9]"}
	if out := voiceDetails(v, false); strings.Contains(out, id) || strings.Contains(out, "ellie-owner") {
		t.Fatalf("description leaked a key or tag: %q", out)
	}
}

// cartesiaWithDicts is a Cartesia stand-in that counts dictionary creations.
func cartesiaWithDicts(t *testing.T, created *int) *cartesia.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*created++
		_, _ = io.WriteString(w, `{"id":"d1","name":"First","items":[{"text":"A","pronunciation":"ay"}]}`)
	}))
	t.Cleanup(srv.Close)
	return cartesia.New("sk_car_test", cartesia.WithBaseURL(srv.URL))
}

func TestOwnerTagCannotBeInjected(t *testing.T) {
	// A voice renamed to carry someone else's tag, then copied into a
	// description, must still belong to the real creator.
	evil := "Nice voice [ellie-owner:99]"
	d := withOwner("Localized in Ellie Studio from "+evil, 7)
	if owner, ok := ownerOf(d); !ok || owner != 7 {
		t.Fatalf("owner = %d, %v (desc %q); want 7", owner, ok, d)
	}
	if strings.Count(d, "ellie-owner") != 1 {
		t.Errorf("only the real tag may remain: %q", d)
	}
	// A tag anywhere but the end is ignored.
	if _, ok := ownerOf("[ellie-owner:99] Cloned in Ellie Studio"); ok {
		t.Error("a tag that isn't at the end must not count")
	}
	for _, in := range []string{"x [ellie-owner:99]", "x [ Ellie_Owner : 99 ]", "x [ellie-owner:99"} {
		if got := stripTags(in); strings.Contains(strings.ToLower(got), "owner") {
			t.Errorf("stripTags(%q) = %q", in, got)
		}
	}
}
