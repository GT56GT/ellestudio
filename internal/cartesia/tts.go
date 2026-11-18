package cartesia

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
)

// TTS models accepted by POST /tts/bytes (TTSModelID enum).
var TTSModels = []string{"sonic-3.6", "sonic-3.5", "sonic-3", "sonic-latest"}

// DefaultTTSModel is the spec's default model_id.
const DefaultTTSModel = "sonic-3.6"

// Emotions accepted by generation_config.emotion (Emotion enum, 58 values).
// The docs call out neutral, calm, angry, content, sad, and scared as the
// primary set.
var Emotions = []string{
	"neutral", "happy", "excited", "enthusiastic", "elated", "euphoric", "triumphant",
	"amazed", "surprised", "flirtatious", "curious", "content", "peaceful", "serene",
	"calm", "grateful", "affectionate", "trust", "sympathetic", "anticipation",
	"mysterious", "angry", "mad", "outraged", "frustrated", "agitated", "threatened",
	"disgusted", "contempt", "envious", "sarcastic", "ironic", "sad", "dejected",
	"melancholic", "disappointed", "hurt", "guilty", "bored", "tired", "rejected",
	"nostalgic", "wistful", "apologetic", "hesitant", "insecure", "confused",
	"resigned", "anxious", "panicked", "alarmed", "scared", "proud", "confident",
	"distant", "skeptical", "contemplative", "determined",
}

// IsEmotion reports whether e is a valid emotion value.
func IsEmotion(e string) bool {
	for _, v := range Emotions {
		if v == e {
			return true
		}
	}
	return false
}

// Ranges for generation_config (inclusive), from the GenerationConfig schema.
const (
	MinSpeed, MaxSpeed   = 0.6, 1.5
	MinVolume, MaxVolume = 0.5, 2.0
)

// Valid sample rates and MP3 bit rates from the OutputFormat schemas.
var (
	SampleRates = []int{8000, 16000, 22050, 24000, 44100, 48000}
	MP3BitRates = []int{32000, 64000, 96000, 128000, 192000}
)

// OutputFormat is the TTS output_format union (wav | mp3 | raw).
type OutputFormat struct {
	Container  string `json:"container"`
	Encoding   string `json:"encoding,omitempty"` // wav/raw: pcm_f32le|pcm_s16le|pcm_mulaw|pcm_alaw
	SampleRate int    `json:"sample_rate"`
	BitRate    int    `json:"bit_rate,omitempty"` // mp3 only
}

// MP3 returns an mp3 output format.
func MP3(sampleRate, bitRate int) OutputFormat {
	return OutputFormat{Container: "mp3", SampleRate: sampleRate, BitRate: bitRate}
}

// WAV returns a 16-bit PCM wav output format.
func WAV(sampleRate int) OutputFormat {
	return OutputFormat{Container: "wav", Encoding: "pcm_s16le", SampleRate: sampleRate}
}

// GenerationConfig tunes volume, speed, and emotion (sonic-3 and newer).
type GenerationConfig struct {
	Volume  float64 `json:"volume,omitempty"`
	Speed   float64 `json:"speed,omitempty"`
	Emotion string  `json:"emotion,omitempty"`
}

// VoiceRef is the object form of TTSRequestVoiceSpecifier.
type VoiceRef struct {
	ID string `json:"id"`
}

// TTSRequest is the body of POST /tts/bytes.
type TTSRequest struct {
	ModelID             string            `json:"model_id"`
	Transcript          string            `json:"transcript"`
	Voice               VoiceRef          `json:"voice"`
	Language            string            `json:"language,omitempty"` // set language or locale, never both
	Locale              string            `json:"locale,omitempty"`
	Accent              string            `json:"accent,omitempty"`
	Normalization       string            `json:"normalization,omitempty"` // auto | off | <lang>
	OutputFormat        OutputFormat      `json:"output_format"`
	PronunciationDictID string            `json:"pronunciation_dict_id,omitempty"`
	GenerationConfig    *GenerationConfig `json:"generation_config,omitempty"`
}

func (r TTSRequest) validate() error {
	switch {
	case r.Transcript == "":
		return fmt.Errorf("cartesia: transcript is empty")
	case r.Voice.ID == "":
		return fmt.Errorf("cartesia: voice id is empty")
	case r.Language != "" && r.Locale != "":
		return fmt.Errorf("cartesia: set language or locale, not both")
	}
	if g := r.GenerationConfig; g != nil {
		if g.Speed != 0 && (g.Speed < MinSpeed || g.Speed > MaxSpeed) {
			return fmt.Errorf("cartesia: speed %.2f outside [%.1f, %.1f]", g.Speed, MinSpeed, MaxSpeed)
		}
		if g.Volume != 0 && (g.Volume < MinVolume || g.Volume > MaxVolume) {
			return fmt.Errorf("cartesia: volume %.2f outside [%.1f, %.1f]", g.Volume, MinVolume, MaxVolume)
		}
		if g.Emotion != "" && !IsEmotion(g.Emotion) {
			return fmt.Errorf("cartesia: unknown emotion %q", g.Emotion)
		}
	}
	return nil
}

// TTSBytes calls POST /tts/bytes and returns the complete audio file.
func (c *Client) TTSBytes(ctx context.Context, r TTSRequest) ([]byte, error) {
	if r.ModelID == "" {
		r.ModelID = DefaultTTSModel
	}
	if r.OutputFormat.Container == "" {
		r.OutputFormat = MP3(44100, 128000)
	}
	if err := r.validate(); err != nil {
		return nil, err
	}
	buf, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}
	req, err := c.request(ctx, http.MethodPost, "/tts/bytes", nil, bytes.NewReader(buf), "application/json", false)
	if err != nil {
		return nil, err
	}
	audio, _, err := c.doBytes(req)
	return audio, err
}

// InfillRequest is the multipart body of POST /infill/bytes. It generates
// audio that bridges left_audio -> transcript -> right_audio.
type InfillRequest struct {
	LeftAudio, RightAudio []byte
	ModelID               string // sonic-3 | sonic-3-2026-01-12
	Language              string
	Transcript            string
	VoiceID               string
	OutputFormat          OutputFormat
}

// Infill calls POST /infill/bytes.
func (c *Client) Infill(ctx context.Context, r InfillRequest) ([]byte, error) {
	if r.ModelID == "" {
		r.ModelID = "sonic-3"
	}
	if r.OutputFormat.Container == "" {
		r.OutputFormat = MP3(44100, 128000)
	}
	f := newForm()
	if len(r.LeftAudio) > 0 {
		f.file("left_audio", "left.wav", bytes.NewReader(r.LeftAudio))
	}
	if len(r.RightAudio) > 0 {
		f.file("right_audio", "right.wav", bytes.NewReader(r.RightAudio))
	}
	f.field("model_id", r.ModelID)
	f.field("language", r.Language)
	f.field("transcript", r.Transcript)
	f.field("voice_id", r.VoiceID)
	f.field("output_format[container]", r.OutputFormat.Container)
	f.field("output_format[sample_rate]", strconv.Itoa(r.OutputFormat.SampleRate))
	f.field("output_format[encoding]", r.OutputFormat.Encoding)
	if r.OutputFormat.BitRate > 0 {
		f.field("output_format[bit_rate]", strconv.Itoa(r.OutputFormat.BitRate))
	}
	body, ct, err := f.finish()
	if err != nil {
		return nil, err
	}
	req, err := c.request(ctx, http.MethodPost, "/infill/bytes", nil, body, ct, false)
	if err != nil {
		return nil, err
	}
	audio, _, err := c.doBytes(req)
	return audio, err
}
