package cartesia

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

func decodeJSON(r io.Reader, out any) error {
	if err := json.NewDecoder(io.LimitReader(r, maxJSONBytes)).Decode(out); err != nil {
		return fmt.Errorf("cartesia: decode response: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------- status

// APIInfo is the response of GET / (no auth needed).
type APIInfo struct {
	OK      bool   `json:"ok"`
	Version string `json:"version"`
}

// Status calls GET /.
func (c *Client) Status(ctx context.Context) (*APIInfo, error) {
	var out APIInfo
	return &out, c.doJSON(ctx, http.MethodGet, "/", nil, nil, &out, false)
}

// ---------------------------------------------------------------- models

// Model is a ModelCatalogEntry.
type Model struct {
	ID                 string   `json:"id"`
	Capability         string   `json:"capability"`          // tts | stt
	Modes              []string `json:"modes"`               // stt only: batch | stream
	SupportedLanguages []string `json:"supported_languages"` // tts only
}

// ListModels calls GET /models. capability is "tts", "stt", or "" for both.
func (c *Client) ListModels(ctx context.Context, capability string) ([]Model, error) {
	q := url.Values{}
	setIf(q, "capability", capability)
	var out struct {
		Data []Model `json:"data"`
	}
	return out.Data, c.doJSON(ctx, http.MethodGet, "/models", q, nil, &out, false)
}

// ---------------------------------------------------------------- accents

// Accent is an entry of GET /accents.
type Accent struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Language        string `json:"language"`
	Locale          string `json:"locale"`
	IsLocaleDefault bool   `json:"is_locale_default"`
	IsLocalizable   bool   `json:"is_localizable"`
}

// AccentFilter narrows GET /accents. Every set filter must match.
type AccentFilter struct {
	LocalizableOnly bool
	Language        string
	Locale          string
}

// ListAccents calls GET /accents.
func (c *Client) ListAccents(ctx context.Context, f AccentFilter) ([]Accent, error) {
	q := url.Values{}
	if f.LocalizableOnly {
		q.Set("is_localizable", "true")
	}
	setIf(q, "language", f.Language)
	setIf(q, "locale", f.Locale)
	var out struct {
		Accents []Accent `json:"accents"`
	}
	return out.Accents, c.doJSON(ctx, http.MethodGet, "/accents", q, nil, &out, false)
}

// ---------------------------------------------------------------- watermark

// Watermark detection limits from POST /v1/watermark/detect.
const (
	MaxWatermarkBytes = 32 << 20
	MinWatermarkSecs  = 5
	MaxWatermarkSecs  = 300
)

// DetectWatermark calls POST /v1/watermark/detect (paid plans only).
func (c *Client) DetectWatermark(ctx context.Context, audio io.Reader, filename string) (bool, error) {
	f := newForm()
	f.file("file", filename, audio)
	body, ct, err := f.finish()
	if err != nil {
		return false, err
	}
	req, err := c.request(ctx, http.MethodPost, "/v1/watermark/detect", nil, body, ct, false)
	if err != nil {
		return false, err
	}
	resp, err := c.send(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	var out struct {
		Detected bool `json:"detected"`
	}
	return out.Detected, decodeJSON(resp.Body, &out)
}

// ---------------------------------------------------------------- pronunciation dictionaries

// PronunciationItem maps text to a custom pronunciation.
type PronunciationItem struct {
	Text          string `json:"text"`
	Pronunciation string `json:"pronunciation"`
	CaseSensitive bool   `json:"case_sensitive,omitempty"`
}

// PronunciationDict is the PronunciationDict schema.
type PronunciationDict struct {
	ID          string              `json:"id"`
	Name        string              `json:"name"`
	Description string              `json:"description"`
	IsOwner     bool                `json:"is_owner"`
	Access      string              `json:"access"`
	Pinned      bool                `json:"pinned"`
	Items       []PronunciationItem `json:"items"`
	CreatedAt   string              `json:"created_at"`
	Visibility  string              `json:"visibility"`
}

// PronunciationDictPage is PaginatedPronunciationDicts.
type PronunciationDictPage struct {
	Data     []PronunciationDict `json:"data"`
	HasMore  bool                `json:"has_more"`
	NextPage string              `json:"next_page"`
}

// ListPronunciationDicts calls GET /pronunciation-dicts/.
func (c *Client) ListPronunciationDicts(ctx context.Context, limit int, startingAfter string) (*PronunciationDictPage, error) {
	q := url.Values{}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(min(limit, 100)))
	}
	setIf(q, "starting_after", startingAfter)
	var out PronunciationDictPage
	return &out, c.doJSON(ctx, http.MethodGet, "/pronunciation-dicts/", q, nil, &out, false)
}

// GetPronunciationDict calls GET /pronunciation-dicts/{id}.
func (c *Client) GetPronunciationDict(ctx context.Context, id string) (*PronunciationDict, error) {
	var out PronunciationDict
	return &out, c.doJSON(ctx, http.MethodGet, "/pronunciation-dicts/"+url.PathEscape(id), nil, nil, &out, false)
}

// PronunciationDictRequest is the create/update body.
type PronunciationDictRequest struct {
	Name        string              `json:"name,omitempty"`
	Description string              `json:"description,omitempty"`
	Items       []PronunciationItem `json:"items,omitempty"`
	Access      string              `json:"access,omitempty"`
}

// CreatePronunciationDict calls POST /pronunciation-dicts/.
func (c *Client) CreatePronunciationDict(ctx context.Context, r PronunciationDictRequest) (*PronunciationDict, error) {
	var out PronunciationDict
	return &out, c.doJSON(ctx, http.MethodPost, "/pronunciation-dicts/", nil, r, &out, false)
}

// UpdatePronunciationDict calls PATCH /pronunciation-dicts/{id}.
func (c *Client) UpdatePronunciationDict(ctx context.Context, id string, r PronunciationDictRequest) (*PronunciationDict, error) {
	var out PronunciationDict
	return &out, c.doJSON(ctx, http.MethodPatch, "/pronunciation-dicts/"+url.PathEscape(id), nil, r, &out, false)
}

// DeletePronunciationDict calls DELETE /pronunciation-dicts/{id}.
func (c *Client) DeletePronunciationDict(ctx context.Context, id string) error {
	return c.doJSON(ctx, http.MethodDelete, "/pronunciation-dicts/"+url.PathEscape(id), nil, nil, nil, false)
}

// ---------------------------------------------------------------- usage (admin key)

// CreditBucket is a UsageCreditsBucket.
type CreditBucket struct {
	StartTS time.Time `json:"start_ts"`
	EndTS   time.Time `json:"end_ts"`
	Credits int64     `json:"credits"`
}

// CreditUsage calls GET /usage/credits without group_by, returning flat
// buckets. interval is day | week | month, or "" for a single bucket.
// Requires an admin key.
func (c *Client) CreditUsage(ctx context.Context, start, end time.Time, interval string) ([]CreditBucket, error) {
	q := url.Values{}
	if !start.IsZero() {
		q.Set("start_ts", start.UTC().Format(time.RFC3339))
	}
	if !end.IsZero() {
		q.Set("end_ts", end.UTC().Format(time.RFC3339))
	}
	setIf(q, "interval", interval)
	var out struct {
		Data []CreditBucket `json:"data"`
	}
	return out.Data, c.doJSON(ctx, http.MethodGet, "/usage/credits", q, nil, &out, true)
}
