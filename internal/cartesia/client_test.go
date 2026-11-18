package cartesia

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newTestClient(t *testing.T, h http.HandlerFunc, opts ...Option) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return New("sk_car_test", append([]Option{WithBaseURL(srv.URL)}, opts...)...)
}

func testTime() time.Time { return time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC) }

func TestTTSBytesSendsSpecShapedRequest(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/tts/bytes" {
			t.Fatalf("got %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk_car_test" {
			t.Errorf("Authorization = %q", got)
		}
		if got := r.Header.Get("Cartesia-Version"); got != APIVersion {
			t.Errorf("Cartesia-Version = %q", got)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["model_id"] != "sonic-3.6" || body["transcript"] != "hello" {
			t.Errorf("body = %v", body)
		}
		if v := body["voice"].(map[string]any); v["id"] != "voice-1" {
			t.Errorf("voice = %v", v)
		}
		of := body["output_format"].(map[string]any)
		if of["container"] != "mp3" || of["sample_rate"].(float64) != 44100 || of["bit_rate"].(float64) != 128000 {
			t.Errorf("output_format = %v", of)
		}
		gc := body["generation_config"].(map[string]any)
		if gc["emotion"] != "calm" || gc["speed"].(float64) != 1.2 {
			t.Errorf("generation_config = %v", gc)
		}
		if _, ok := gc["volume"]; ok {
			t.Errorf("volume should be omitted when unset: %v", gc)
		}
		if _, ok := body["locale"]; ok {
			t.Errorf("locale should be omitted: %v", body)
		}
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write([]byte("ID3fake"))
	})
	audio, err := c.TTSBytes(context.Background(), TTSRequest{
		Transcript:       "hello",
		Voice:            VoiceRef{ID: "voice-1"},
		GenerationConfig: &GenerationConfig{Emotion: "calm", Speed: 1.2},
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(audio) != "ID3fake" {
		t.Fatalf("audio = %q", audio)
	}
}

func TestTTSValidation(t *testing.T) {
	c := New("k")
	cases := map[string]TTSRequest{
		"empty transcript": {Voice: VoiceRef{ID: "v"}},
		"no voice":         {Transcript: "x"},
		"lang and locale":  {Transcript: "x", Voice: VoiceRef{ID: "v"}, Language: "en", Locale: "en-US"},
		"speed too high":   {Transcript: "x", Voice: VoiceRef{ID: "v"}, GenerationConfig: &GenerationConfig{Speed: 1.6}},
		"volume too low":   {Transcript: "x", Voice: VoiceRef{ID: "v"}, GenerationConfig: &GenerationConfig{Volume: 0.4}},
		"bad emotion":      {Transcript: "x", Voice: VoiceRef{ID: "v"}, GenerationConfig: &GenerationConfig{Emotion: "grumpy"}},
	}
	for name, req := range cases {
		if _, err := c.TTSBytes(context.Background(), req); err == nil {
			t.Errorf("%s: expected validation error", name)
		}
	}
}

func TestStructuredAndLegacyErrors(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/voices/legacy" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, "Bad Request: nope")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"error_code":"concurrency_limited","title":"Too many concurrent requests","message":"slow down","request_id":"req-1"}`)
	})
	_, err := c.GetVoice(context.Background(), "x", false)
	ae, ok := AsAPIError(err)
	if !ok || ae.StatusCode != 429 || ae.ErrorCode != ErrConcurrencyLimited || ae.RequestID != "req-1" {
		t.Fatalf("structured error = %#v", err)
	}
	_, err = c.GetVoice(context.Background(), "legacy", false)
	ae, ok = AsAPIError(err)
	if !ok || ae.StatusCode != 400 || !strings.Contains(ae.Message, "nope") {
		t.Fatalf("legacy error = %#v", err)
	}
}

func TestTranscribeMultipart(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/stt" {
			t.Fatalf("path %s", r.URL.Path)
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatal(err)
		}
		if r.FormValue("model") != "ink-whisper" || r.FormValue("language") != "fr" {
			t.Errorf("fields = %v", r.MultipartForm.Value)
		}
		if got := r.MultipartForm.Value["timestamp_granularities[]"]; len(got) != 1 || got[0] != "word" {
			t.Errorf("timestamps = %v", got)
		}
		fh := r.MultipartForm.File["file"][0]
		if fh.Filename != "voice.ogg" {
			t.Errorf("filename = %s", fh.Filename)
		}
		_, _ = io.WriteString(w, `{"type":"transcript","text":"bonjour","language":"fr","duration":1.5,"words":[{"word":"bonjour","start":0,"end":0.6}]}`)
	})
	tr, err := c.Transcribe(context.Background(), STTRequest{
		Audio: strings.NewReader("OggS"), Filename: "voice.ogg", Language: "fr", WordTimestamps: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if tr.Text != "bonjour" || tr.Duration != 1.5 || len(tr.Words) != 1 {
		t.Fatalf("transcription = %+v", tr)
	}
}

func TestListVoicesQuery(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("limit") != "100" || q.Get("q") != "warm" || q.Get("is_owner") != "true" ||
			q.Get("gender") != "feminine" || q["expand[]"][0] != "preview_file_url" {
			t.Errorf("query = %v", q)
		}
		_, _ = io.WriteString(w, `{"data":[{"id":"a","name":"Ava","language":"en","accents":[{"accent":"general-american","locale":"en-US","is_native":true}]}],"has_more":true,"next_page":"a"}`)
	})
	p, err := c.ListVoices(context.Background(), ListVoicesParams{
		Limit: 500, Q: "warm", OwnedOnly: true, Gender: "feminine", ExpandPreview: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Data) != 1 || !p.HasMore || p.NextPage != "a" || !p.Data[0].Accents[0].IsNative {
		t.Fatalf("page = %+v", p)
	}
}

func TestCloneVoiceMultipart(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatal(err)
		}
		if r.FormValue("name") != "Ellie" || r.FormValue("language") != "en" {
			t.Errorf("fields = %v", r.MultipartForm.Value)
		}
		if _, ok := r.MultipartForm.Value["accent"]; ok {
			t.Error("empty accent should not be sent")
		}
		if len(r.MultipartForm.File["clip"]) != 1 {
			t.Error("clip missing")
		}
		_, _ = io.WriteString(w, `{"id":"new-voice","name":"Ellie","language":"en","access":"private"}`)
	})
	v, err := c.CloneVoice(context.Background(), CloneVoiceRequest{
		Clip: strings.NewReader("wav"), Filename: "clip.wav", Name: "Ellie", Language: "en",
	})
	if err != nil || v.ID != "new-voice" {
		t.Fatalf("clone = %+v, %v", v, err)
	}
}

func TestDownloadPreviewRefusesForeignHost(t *testing.T) {
	called := false
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		called = true
		if r.Header.Get("Authorization") == "" {
			t.Error("preview on Cartesia host should carry auth")
		}
		_, _ = w.Write([]byte("mp3"))
	})
	if _, _, err := c.DownloadPreview(context.Background(), "https://evil.example.com/steal"); err == nil {
		t.Fatal("expected refusal for foreign host")
	}
	data, _, err := c.DownloadPreview(context.Background(), c.baseURL.String()+"/voices/a/preview?x=1")
	if err != nil || string(data) != "mp3" || !called {
		t.Fatalf("preview = %q, %v", data, err)
	}
}

func TestAdminEndpointsNeedAdminKey(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer sk_car_admin_x" {
			t.Errorf("Authorization = %q", got)
		}
		_, _ = io.WriteString(w, `{"data":[{"start_ts":"2026-11-01T00:00:00Z","end_ts":"2026-11-08T00:00:00Z","credits":42}]}`)
	}, WithAdminKey("sk_car_admin_x"))
	b, err := c.CreditUsage(context.Background(), testTime(), testTime(), "week")
	if err != nil || len(b) != 1 || b[0].Credits != 42 {
		t.Fatalf("usage = %+v, %v", b, err)
	}
	if _, err := New("k").CreditUsage(context.Background(), testTime(), testTime(), ""); err == nil {
		t.Fatal("expected error without admin key")
	}
}
