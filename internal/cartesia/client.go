// Package cartesia is a small, dependency-free client for the Cartesia REST API
// (https://docs.cartesia.ai). It covers the speech, voice, and account
// endpoints used by Ellie Studio. Field names and limits follow the scraped
// reference in docs/CARTESIA_API.md (API version 2026-08-14).
package cartesia

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	// DefaultBaseURL is Cartesia's production API host. HTTPS only.
	DefaultBaseURL = "https://api.cartesia.ai"
	// APIVersion is sent as the Cartesia-Version header on every request.
	APIVersion = "2026-08-14"

	// maxAudioBytes caps how much audio we buffer from a single response.
	maxAudioBytes = 64 << 20
	// maxJSONBytes caps JSON response bodies.
	maxJSONBytes = 8 << 20
)

// Client talks to the Cartesia API. The zero value is not usable; call New.
type Client struct {
	baseURL  *url.URL
	apiKey   string
	adminKey string
	http     *http.Client
}

// Option configures a Client.
type Option func(*Client)

// WithBaseURL points the client at a different host (used by tests).
func WithBaseURL(raw string) Option {
	return func(c *Client) {
		if u, err := url.Parse(strings.TrimRight(raw, "/")); err == nil {
			c.baseURL = u
		}
	}
}

// WithHTTPClient swaps the underlying HTTP client.
func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.http = h } }

// WithAdminKey sets an admin key (sk_car_admin_...) used only by the
// management endpoints (usage, api-keys, organizations). Standard and admin
// keys are not interchangeable.
func WithAdminKey(key string) Option { return func(c *Client) { c.adminKey = key } }

// New returns a client authenticated with a standard API key (sk_car_...).
func New(apiKey string, opts ...Option) *Client {
	u, _ := url.Parse(DefaultBaseURL)
	c := &Client{
		baseURL: u,
		apiKey:  apiKey,
		http:    &http.Client{Timeout: 120 * time.Second},
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// HasAdminKey reports whether admin-only endpoints can be called.
func (c *Client) HasAdminKey() bool { return c.adminKey != "" }

// APIError is Cartesia's structured error (Cartesia-Version >= 2026-03-01).
type APIError struct {
	StatusCode int    `json:"-"`
	ErrorCode  string `json:"error_code"`
	Title      string `json:"title"`
	Message    string `json:"message"`
	RequestID  string `json:"request_id"`
	DocURL     string `json:"doc_url,omitempty"`
}

// Known error codes. More may be added; callers must tolerate unknown values.
const (
	ErrQuotaExceeded        = "quota_exceeded"
	ErrConcurrencyLimited   = "concurrency_limited"
	ErrVoiceModelMismatch   = "voice_model_mismatch"
	ErrVoiceNotFound        = "voice_not_found"
	ErrModelNotFound        = "model_not_found"
	ErrLanguageNotSupported = "language_not_supported"
	ErrFileTooLarge         = "file_too_large"
	ErrUnsupportedAudio     = "unsupported_audio_format"
	ErrPlanUpgradeRequired  = "plan_upgrade_required"
)

func (e *APIError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "cartesia: %d", e.StatusCode)
	if e.ErrorCode != "" {
		fmt.Fprintf(&b, " %s", e.ErrorCode)
	}
	if e.Title != "" {
		fmt.Fprintf(&b, ": %s", e.Title)
	}
	if e.Message != "" && e.Message != e.Title {
		fmt.Fprintf(&b, " - %s", e.Message)
	}
	if e.RequestID != "" {
		fmt.Fprintf(&b, " (request %s)", e.RequestID)
	}
	return b.String()
}

// AsAPIError unwraps err into an *APIError if it is one.
func AsAPIError(err error) (*APIError, bool) {
	var ae *APIError
	ok := errors.As(err, &ae)
	return ae, ok
}

func parseAPIError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	ae := &APIError{StatusCode: resp.StatusCode}
	if err := json.Unmarshal(body, ae); err != nil || (ae.Title == "" && ae.Message == "") {
		// Legacy plain-text "Title: Message" or an unexpected body.
		ae.Title = http.StatusText(resp.StatusCode)
		ae.Message = strings.TrimSpace(string(body))
	}
	return ae
}

// request builds an authenticated request. admin selects the admin key.
func (c *Client) request(ctx context.Context, method, path string, q url.Values, body io.Reader, contentType string, admin bool) (*http.Request, error) {
	u := *c.baseURL
	u.Path = strings.TrimRight(u.Path, "/") + path
	if len(q) > 0 {
		u.RawQuery = q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return nil, err
	}
	key := c.apiKey
	if admin {
		if c.adminKey == "" {
			return nil, errors.New("cartesia: this endpoint needs an admin API key (CARTESIA_ADMIN_KEY)")
		}
		key = c.adminKey
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Cartesia-Version", APIVersion)
	req.Header.Set("User-Agent", "ellie-studio/1.0")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	return req, nil
}

// send executes req and returns the response if it is 2xx.
func (c *Client) send(req *http.Request) (*http.Response, error) {
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cartesia: %s %s: %w", req.Method, req.URL.Path, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		defer resp.Body.Close()
		return nil, parseAPIError(resp)
	}
	return resp, nil
}

// doJSON sends a request with an optional JSON body and decodes a JSON reply
// into out (which may be nil for 204 responses).
func (c *Client) doJSON(ctx context.Context, method, path string, q url.Values, in, out any, admin bool) error {
	var body io.Reader
	ct := ""
	if in != nil {
		buf, err := json.Marshal(in)
		if err != nil {
			return fmt.Errorf("cartesia: encode request: %w", err)
		}
		body, ct = bytes.NewReader(buf), "application/json"
	}
	req, err := c.request(ctx, method, path, q, body, ct, admin)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.send(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if out == nil || resp.StatusCode == http.StatusNoContent {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxJSONBytes)).Decode(out); err != nil {
		return fmt.Errorf("cartesia: decode %s %s: %w", method, path, err)
	}
	return nil
}

// doBytes sends req and returns the (size-capped) response body.
func (c *Client) doBytes(req *http.Request) ([]byte, string, error) {
	resp, err := c.send(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxAudioBytes+1))
	if err != nil {
		return nil, "", fmt.Errorf("cartesia: read body: %w", err)
	}
	if len(data) > maxAudioBytes {
		return nil, "", fmt.Errorf("cartesia: response larger than %d bytes", maxAudioBytes)
	}
	return data, resp.Header.Get("Content-Type"), nil
}

// form accumulates multipart/form-data fields and files.
type form struct {
	buf bytes.Buffer
	w   *multipart.Writer
	err error
}

func newForm() *form {
	f := &form{}
	f.w = multipart.NewWriter(&f.buf)
	return f
}

func (f *form) field(name, value string) {
	if f.err == nil && value != "" {
		f.err = f.w.WriteField(name, value)
	}
}

func (f *form) file(field, filename string, r io.Reader) {
	if f.err != nil {
		return
	}
	if filename == "" {
		filename = "audio"
	}
	part, err := f.w.CreateFormFile(field, filename)
	if err != nil {
		f.err = err
		return
	}
	_, f.err = io.Copy(part, r)
}

func (f *form) finish() (io.Reader, string, error) {
	if f.err == nil {
		f.err = f.w.Close()
	}
	if f.err != nil {
		return nil, "", fmt.Errorf("cartesia: build form: %w", f.err)
	}
	return &f.buf, f.w.FormDataContentType(), nil
}
