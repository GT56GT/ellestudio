package cartesia

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

// DefaultSTTModel is the batch speech-to-text model (ink-whisper family).
const DefaultSTTModel = "ink-whisper"

// STTFormats are the container formats POST /stt accepts.
var STTFormats = []string{"flac", "m4a", "mp3", "mp4", "mpeg", "mpga", "oga", "ogg", "wav", "webm"}

// STTRequest describes a batch transcription (POST /stt, multipart).
type STTRequest struct {
	Audio    io.Reader
	Filename string // used by the server to sniff the container
	Model    string // defaults to ink-whisper
	// Language of the audio. If empty the API assumes "en" and may translate
	// other languages into English, so set it whenever it is known.
	Language       string
	WordTimestamps bool
	// Encoding and SampleRate are only needed for headerless raw PCM.
	Encoding   string
	SampleRate int
}

// Word is one word-level timestamp.
type Word struct {
	Word  string  `json:"word"`
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

// Transcription is the TranscriptionResponse schema.
type Transcription struct {
	Type      string  `json:"type"`
	RequestID string  `json:"request_id"`
	Text      string  `json:"text"`
	Language  string  `json:"language"`
	Duration  float64 `json:"duration"`
	Words     []Word  `json:"words"`
}

// Transcribe calls POST /stt.
func (c *Client) Transcribe(ctx context.Context, r STTRequest) (*Transcription, error) {
	if r.Model == "" {
		r.Model = DefaultSTTModel
	}
	f := newForm()
	f.file("file", r.Filename, r.Audio)
	f.field("model", r.Model)
	f.field("language", r.Language)
	if r.WordTimestamps {
		f.field("timestamp_granularities[]", "word")
	}
	body, ct, err := f.finish()
	if err != nil {
		return nil, err
	}
	q := url.Values{}
	if r.Encoding != "" {
		q.Set("encoding", r.Encoding)
	}
	if r.SampleRate > 0 {
		q.Set("sample_rate", strconv.Itoa(r.SampleRate))
	}
	req, err := c.request(ctx, http.MethodPost, "/stt", q, body, ct, false)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.send(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out Transcription
	if err := decodeJSON(resp.Body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
