package cartesia

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// VoiceAccent is one entry of Voice.accents.
type VoiceAccent struct {
	Accent   string `json:"accent"`
	Locale   string `json:"locale"`
	IsNative bool   `json:"is_native"`
}

// Voice is the Voice schema returned by list/get/update.
type Voice struct {
	ID             string        `json:"id"`
	IsOwner        bool          `json:"is_owner"`
	Status         string        `json:"status"` // active | archived
	Access         string        `json:"access"` // private | public
	Name           string        `json:"name"`
	Tagline        string        `json:"tagline"`
	Description    string        `json:"description"`
	Gender         string        `json:"gender"` // masculine | feminine | gender_neutral
	CreatedAt      time.Time     `json:"created_at"`
	PreviewFileURL string        `json:"preview_file_url"`
	Language       string        `json:"language"`
	Accents        []VoiceAccent `json:"accents"`
	Country        string        `json:"country"`
	IsPro          bool          `json:"is_pro"`
	Visibility     string        `json:"visibility"` // owner | all
}

// VoiceMetadata is returned by clone and localize.
type VoiceMetadata struct {
	ID          string    `json:"id"`
	Access      string    `json:"access"`
	Name        string    `json:"name"`
	Tagline     string    `json:"tagline"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	Language    string    `json:"language"`
	UserID      string    `json:"user_id"`
	Visibility  string    `json:"visibility"`
}

// ListVoicesParams are the query params of GET /voices.
type ListVoicesParams struct {
	Limit           int // 1-100
	StartingAfter   string
	EndingBefore    string
	Q               string
	OwnedOnly       bool
	Gender          string
	Language        string // "en" or "en-GB"
	IncludeArchived bool
	ExpandPreview   bool // expand[]=preview_file_url
}

// VoicePage is the GetVoicesResponse schema.
type VoicePage struct {
	Data     []Voice `json:"data"`
	HasMore  bool    `json:"has_more"`
	NextPage string  `json:"next_page"`
}

// ListVoices calls GET /voices.
func (c *Client) ListVoices(ctx context.Context, p ListVoicesParams) (*VoicePage, error) {
	q := url.Values{}
	if p.Limit > 0 {
		q.Set("limit", strconv.Itoa(min(p.Limit, 100)))
	}
	setIf(q, "starting_after", p.StartingAfter)
	setIf(q, "ending_before", p.EndingBefore)
	setIf(q, "q", p.Q)
	setIf(q, "gender", p.Gender)
	setIf(q, "language", p.Language)
	if p.OwnedOnly {
		q.Set("is_owner", "true")
	}
	if p.IncludeArchived {
		q.Set("include_archived", "true")
	}
	if p.ExpandPreview {
		q.Add("expand[]", "preview_file_url")
	}
	var out VoicePage
	return &out, c.doJSON(ctx, http.MethodGet, "/voices", q, nil, &out, false)
}

// GetVoice calls GET /voices/{id}.
func (c *Client) GetVoice(ctx context.Context, id string, expandPreview bool) (*Voice, error) {
	q := url.Values{}
	if expandPreview {
		q.Add("expand[]", "preview_file_url")
	}
	var out Voice
	return &out, c.doJSON(ctx, http.MethodGet, "/voices/"+url.PathEscape(id), q, nil, &out, false)
}

// UpdateVoiceRequest is the body of PATCH /voices/{id}. Empty fields are left
// unchanged.
type UpdateVoiceRequest struct {
	Name        string `json:"name,omitempty"`
	Tagline     string `json:"tagline,omitempty"`
	Description string `json:"description,omitempty"`
	Gender      string `json:"gender,omitempty"`
	Accent      string `json:"accent,omitempty"`
	Access      string `json:"access,omitempty"`
}

// UpdateVoice calls PATCH /voices/{id}.
func (c *Client) UpdateVoice(ctx context.Context, id string, r UpdateVoiceRequest) (*Voice, error) {
	var out Voice
	return &out, c.doJSON(ctx, http.MethodPatch, "/voices/"+url.PathEscape(id), nil, r, &out, false)
}

// DeleteVoice calls DELETE /voices/{id} (204).
func (c *Client) DeleteVoice(ctx context.Context, id string) error {
	return c.doJSON(ctx, http.MethodDelete, "/voices/"+url.PathEscape(id), nil, nil, nil, false)
}

// MaxCloneClipBytes is the clone endpoint's upload limit.
const MaxCloneClipBytes = 16 << 20

// CloneVoiceRequest is the multipart body of POST /voices/clone.
type CloneVoiceRequest struct {
	Clip        io.Reader
	Filename    string
	Name        string // required
	Tagline     string // at most 32 characters
	Description string
	Language    string // required, e.g. "en"
	Accent      string // optional accent id from GET /accents
	Access      string // private (default) | public
}

// CloneVoice calls POST /voices/clone.
func (c *Client) CloneVoice(ctx context.Context, r CloneVoiceRequest) (*VoiceMetadata, error) {
	if r.Name == "" || r.Language == "" || r.Clip == nil {
		return nil, errors.New("cartesia: clone needs a clip, name, and language")
	}
	f := newForm()
	f.file("clip", r.Filename, r.Clip)
	f.field("name", r.Name)
	f.field("tagline", r.Tagline)
	f.field("description", r.Description)
	f.field("language", r.Language)
	f.field("accent", r.Accent)
	f.field("access", r.Access)
	body, ct, err := f.finish()
	if err != nil {
		return nil, err
	}
	req, err := c.request(ctx, http.MethodPost, "/voices/clone", nil, body, ct, false)
	if err != nil {
		return nil, err
	}
	resp, err := c.send(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out VoiceMetadata
	return &out, decodeJSON(resp.Body, &out)
}

// LocalizeVoiceRequest is the body of POST /voices/localize.
type LocalizeVoiceRequest struct {
	VoiceID     string `json:"voice_id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Tagline     string `json:"tagline,omitempty"`
	Accent      string `json:"accent"` // must be is_localizable in GET /accents
	Access      string `json:"access,omitempty"`
}

// LocalizeVoice calls POST /voices/localize, which creates a new voice.
func (c *Client) LocalizeVoice(ctx context.Context, r LocalizeVoiceRequest) (*VoiceMetadata, error) {
	var out VoiceMetadata
	return &out, c.doJSON(ctx, http.MethodPost, "/voices/localize", nil, r, &out, false)
}

// AddVoiceAccents calls PATCH /voices/{id}/accents (instant clones you own).
func (c *Client) AddVoiceAccents(ctx context.Context, id string, accents []string) (*Voice, error) {
	var out Voice
	body := map[string][]string{"accents": accents}
	return &out, c.doJSON(ctx, http.MethodPatch, "/voices/"+url.PathEscape(id)+"/accents", nil, body, &out, false)
}

// DeleteVoiceAccent calls DELETE /voices/{id}/accents/{accent_id}.
func (c *Client) DeleteVoiceAccent(ctx context.Context, id, accent string) (*Voice, error) {
	var out Voice
	p := "/voices/" + url.PathEscape(id) + "/accents/" + url.PathEscape(accent)
	return &out, c.doJSON(ctx, http.MethodDelete, p, nil, nil, &out, false)
}

// DownloadPreview fetches a voice's preview_file_url. The URL needs the same
// Authorization header as the API, so the key is only attached when the URL
// points at the configured Cartesia host; anything else is refused.
func (c *Client) DownloadPreview(ctx context.Context, rawURL string) ([]byte, string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, "", err
	}
	if u.Scheme != c.baseURL.Scheme || u.Host != c.baseURL.Host {
		return nil, "", errors.New("cartesia: refusing to send credentials to a non-Cartesia preview URL")
	}
	req, err := c.request(ctx, http.MethodGet, u.Path, u.Query(), nil, "", false)
	if err != nil {
		return nil, "", err
	}
	return c.doBytes(req)
}

func setIf(q url.Values, k, v string) {
	if v != "" {
		q.Set(k, v)
	}
}
