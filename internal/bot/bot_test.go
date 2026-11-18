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
	bot   *Bot
	tg    *fakeTelegram
	ttsIn map[string]any
	sttIn map[string]string
}

func newFixture(t *testing.T, cfg Config) *fixture {
	t.Helper()
	fx := &fixture{tg: &fakeTelegram{}}
	tgSrv := httptest.NewServer(fx.tg.handler(t))
	t.Cleanup(tgSrv.Close)

	ctSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/tts/bytes":
			_ = json.NewDecoder(r.Body).Decode(&fx.ttsIn)
			w.Header().Set("Content-Type", "audio/mpeg")
			_, _ = io.WriteString(w, "ID3-mp3")
		case "/stt":
			_ = r.ParseMultipartForm(1 << 20)
			fx.sttIn = map[string]string{"language": r.FormValue("language"), "filename": r.MultipartForm.File["file"][0].Filename}
			_, _ = io.WriteString(w, `{"type":"transcript","text":"hello from a voice note","duration":2.5,"language":"en"}`)
		case "/voices/v-new":
			_, _ = io.WriteString(w, `{"id":"v-new","name":"Nova","language":"en"}`)
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
	var replies []string
	for _, c := range fx.tg.find("sendMessage") {
		replies = append(replies, c.JSON["text"].(string))
	}
	last := replies[len(replies)-1]
	if !strings.Contains(last, "hello from a voice note") || !strings.Contains(last, "2.5s") {
		t.Errorf("transcript reply = %q", last)
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

func TestAdminOnlyCommands(t *testing.T) {
	fx := newFixture(t, Config{AdminUsers: map[int64]bool{1: true}})
	fx.bot.handleUpdate(context.Background(), msg(2, "/deletevoice v-new"))
	m := fx.tg.find("sendMessage")
	if len(m) != 1 || !strings.Contains(m[0].JSON["text"].(string), "Only studio admins") {
		t.Fatalf("non-admin delete should be refused, got %+v", m)
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
