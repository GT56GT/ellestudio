// Package telegram is a minimal, dependency-free Telegram Bot API client with
// just the methods Ellie Studio needs (long polling, messages, keyboards,
// voice/audio upload, and file download).
package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// DefaultAPIURL is the public Bot API host.
const DefaultAPIURL = "https://api.telegram.org"

// MaxDownloadBytes is the Bot API's getFile limit (20 MB).
const MaxDownloadBytes = 20 << 20

// MaxMessageLen is the longest text a single message can carry.
const MaxMessageLen = 4096

// Client calls the Bot API for one bot token.
type Client struct {
	token string
	base  string
	http  *http.Client
}

// New creates a client. apiURL may be "" for the public API.
func New(token, apiURL string) *Client {
	if apiURL == "" {
		apiURL = DefaultAPIURL
	}
	return &Client{
		token: token,
		base:  strings.TrimRight(apiURL, "/"),
		// Long polls hold the connection for up to the poll timeout, so the
		// per-request deadline comes from ctx rather than a global timeout.
		http: &http.Client{},
	}
}

// ---------------------------------------------------------------- types

type User struct {
	ID        int64  `json:"id"`
	IsBot     bool   `json:"is_bot"`
	FirstName string `json:"first_name"`
	Username  string `json:"username"`
}

type Chat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

type Voice struct {
	FileID   string `json:"file_id"`
	Duration int    `json:"duration"`
	MimeType string `json:"mime_type"`
	FileSize int64  `json:"file_size"`
}

type Audio struct {
	FileID   string `json:"file_id"`
	Duration int    `json:"duration"`
	FileName string `json:"file_name"`
	MimeType string `json:"mime_type"`
	FileSize int64  `json:"file_size"`
}

type Document struct {
	FileID   string `json:"file_id"`
	FileName string `json:"file_name"`
	MimeType string `json:"mime_type"`
	FileSize int64  `json:"file_size"`
}

type Message struct {
	MessageID      int64     `json:"message_id"`
	From           *User     `json:"from"`
	Chat           Chat      `json:"chat"`
	Text           string    `json:"text"`
	Caption        string    `json:"caption"`
	Voice          *Voice    `json:"voice"`
	Audio          *Audio    `json:"audio"`
	VideoNote      *Voice    `json:"video_note"`
	Document       *Document `json:"document"`
	ReplyToMessage *Message  `json:"reply_to_message"`
}

type CallbackQuery struct {
	ID      string   `json:"id"`
	From    User     `json:"from"`
	Message *Message `json:"message"`
	Data    string   `json:"data"`
}

type Update struct {
	UpdateID      int64          `json:"update_id"`
	Message       *Message       `json:"message"`
	CallbackQuery *CallbackQuery `json:"callback_query"`
}

type InlineKeyboardButton struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data,omitempty"`
	URL          string `json:"url,omitempty"`
}

type InlineKeyboardMarkup struct {
	InlineKeyboard [][]InlineKeyboardButton `json:"inline_keyboard"`
}

// KeyboardButton is one button of a reply keyboard; tapping it sends Text.
type KeyboardButton struct {
	Text string `json:"text"`
}

// ReplyKeyboardMarkup is the menu pinned under the message input.
type ReplyKeyboardMarkup struct {
	Keyboard              [][]KeyboardButton `json:"keyboard"`
	ResizeKeyboard        bool               `json:"resize_keyboard,omitempty"`
	IsPersistent          bool               `json:"is_persistent,omitempty"`
	InputFieldPlaceholder string             `json:"input_field_placeholder,omitempty"`
}

// hasMarkup reports whether v is a real keyboard (not nil or a typed nil).
func hasMarkup(v any) bool {
	switch k := v.(type) {
	case nil:
		return false
	case *InlineKeyboardMarkup:
		return k != nil
	case *ReplyKeyboardMarkup:
		return k != nil
	}
	return true
}

type File struct {
	FileID   string `json:"file_id"`
	FileSize int64  `json:"file_size"`
	FilePath string `json:"file_path"`
}

type BotCommand struct {
	Command     string `json:"command"`
	Description string `json:"description"`
}

// Error is a Bot API failure ({"ok":false,...}).
type Error struct {
	Code        int
	Description string
	RetryAfter  int
}

func (e *Error) Error() string {
	return fmt.Sprintf("telegram: %d %s", e.Code, e.Description)
}

// ---------------------------------------------------------------- transport

type envelope struct {
	OK          bool            `json:"ok"`
	Result      json.RawMessage `json:"result"`
	ErrorCode   int             `json:"error_code"`
	Description string          `json:"description"`
	Parameters  *struct {
		RetryAfter int `json:"retry_after"`
	} `json:"parameters"`
}

func (c *Client) methodURL(method string) string {
	return c.base + "/bot" + c.token + "/" + method
}

// scrub removes the bot token from transport errors, whose messages embed
// the request URL.
func (c *Client) scrub(err error) error {
	if err == nil || c.token == "" {
		return err
	}
	return errors.New(strings.ReplaceAll(err.Error(), c.token, "<token>"))
}

func (c *Client) post(ctx context.Context, method string, body io.Reader, contentType string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.methodURL(method), body)
	if err != nil {
		return c.scrub(err)
	}
	req.Header.Set("Content-Type", contentType)
	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("telegram %s: %w", method, c.scrub(err))
	}
	defer resp.Body.Close()
	var env envelope
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&env); err != nil {
		return fmt.Errorf("telegram %s: decode: %w (HTTP %d)", method, err, resp.StatusCode)
	}
	if !env.OK {
		e := &Error{Code: env.ErrorCode, Description: env.Description}
		if env.Parameters != nil {
			e.RetryAfter = env.Parameters.RetryAfter
		}
		return e
	}
	if out != nil {
		return json.Unmarshal(env.Result, out)
	}
	return nil
}

func (c *Client) call(ctx context.Context, method string, params, out any) error {
	buf, err := json.Marshal(params)
	if err != nil {
		return err
	}
	return c.post(ctx, method, bytes.NewReader(buf), "application/json", out)
}

// ---------------------------------------------------------------- methods

// GetMe returns the bot's own user.
func (c *Client) GetMe(ctx context.Context) (*User, error) {
	var u User
	return &u, c.call(ctx, "getMe", struct{}{}, &u)
}

// GetUpdates long-polls for updates after offset.
func (c *Client) GetUpdates(ctx context.Context, offset int64, timeout time.Duration) ([]Update, error) {
	params := map[string]any{
		"offset":          offset,
		"timeout":         int(timeout.Seconds()),
		"allowed_updates": []string{"message", "callback_query"},
	}
	var ups []Update
	return ups, c.call(ctx, "getUpdates", params, &ups)
}

// SendMessageParams configures sendMessage / editMessageText.
type SendMessageParams struct {
	ChatID      int64
	MessageID   int64 // edit only
	Text        string
	ParseMode   string
	ReplyTo     int64
	ReplyMarkup any // *InlineKeyboardMarkup or *ReplyKeyboardMarkup (edits: inline only)
	NoPreview   bool
}

func (p SendMessageParams) payload() map[string]any {
	m := map[string]any{"chat_id": p.ChatID, "text": p.Text}
	if p.MessageID != 0 {
		m["message_id"] = p.MessageID
	}
	if p.ParseMode != "" {
		m["parse_mode"] = p.ParseMode
	}
	if p.ReplyTo != 0 {
		m["reply_parameters"] = map[string]any{"message_id": p.ReplyTo, "allow_sending_without_reply": true}
	}
	if hasMarkup(p.ReplyMarkup) {
		m["reply_markup"] = p.ReplyMarkup
	}
	if p.NoPreview {
		m["link_preview_options"] = map[string]any{"is_disabled": true}
	}
	return m
}

// SendMessage sends a text message.
func (c *Client) SendMessage(ctx context.Context, p SendMessageParams) (*Message, error) {
	var m Message
	return &m, c.call(ctx, "sendMessage", p.payload(), &m)
}

// EditMessageText replaces the text (and keyboard) of an existing message.
func (c *Client) EditMessageText(ctx context.Context, p SendMessageParams) error {
	return c.call(ctx, "editMessageText", p.payload(), nil)
}

// AnswerCallbackQuery acknowledges a button press, optionally with a toast.
func (c *Client) AnswerCallbackQuery(ctx context.Context, id, text string) error {
	return c.call(ctx, "answerCallbackQuery", map[string]any{"callback_query_id": id, "text": text}, nil)
}

// SendChatAction shows "recording voice…" etc. for about 5 seconds.
func (c *Client) SendChatAction(ctx context.Context, chatID int64, action string) error {
	return c.call(ctx, "sendChatAction", map[string]any{"chat_id": chatID, "action": action}, nil)
}

// SetMyCommands sets the command menu shown in clients.
func (c *Client) SetMyCommands(ctx context.Context, cmds []BotCommand) error {
	return c.call(ctx, "setMyCommands", map[string]any{"commands": cmds}, nil)
}

// SetMyDescription sets the text shown in an empty chat before /start.
func (c *Client) SetMyDescription(ctx context.Context, text string) error {
	return c.call(ctx, "setMyDescription", map[string]any{"description": text}, nil)
}

// SetMyShortDescription sets the bot's profile blurb.
func (c *Client) SetMyShortDescription(ctx context.Context, text string) error {
	return c.call(ctx, "setMyShortDescription", map[string]any{"short_description": text}, nil)
}

// DeleteMessage removes a message the bot sent.
func (c *Client) DeleteMessage(ctx context.Context, chatID, messageID int64) error {
	return c.call(ctx, "deleteMessage", map[string]any{"chat_id": chatID, "message_id": messageID}, nil)
}

// MediaParams is shared by sendVoice / sendAudio / sendDocument.
type MediaParams struct {
	ChatID   int64
	ReplyTo  int64
	Caption  string
	Filename string
	Data     []byte
	Title    string // sendAudio only
	// ReplyMarkup is an optional keyboard attached under the media.
	ReplyMarkup any
}

// SendVoice uploads a voice note (OGG/Opus, MP3, or M4A).
func (c *Client) SendVoice(ctx context.Context, p MediaParams) (*Message, error) {
	return c.sendMedia(ctx, "sendVoice", "voice", p)
}

// SendAudio uploads a music-style audio file.
func (c *Client) SendAudio(ctx context.Context, p MediaParams) (*Message, error) {
	return c.sendMedia(ctx, "sendAudio", "audio", p)
}

// SendDocument uploads a generic file.
func (c *Client) SendDocument(ctx context.Context, p MediaParams) (*Message, error) {
	return c.sendMedia(ctx, "sendDocument", "document", p)
}

func (c *Client) sendMedia(ctx context.Context, method, field string, p MediaParams) (*Message, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("chat_id", strconv.FormatInt(p.ChatID, 10))
	if p.Caption != "" {
		_ = w.WriteField("caption", p.Caption)
	}
	if p.Title != "" {
		_ = w.WriteField("title", p.Title)
	}
	if p.ReplyTo != 0 {
		rp, _ := json.Marshal(map[string]any{"message_id": p.ReplyTo, "allow_sending_without_reply": true})
		_ = w.WriteField("reply_parameters", string(rp))
	}
	if hasMarkup(p.ReplyMarkup) {
		rm, err := json.Marshal(p.ReplyMarkup)
		if err != nil {
			return nil, err
		}
		_ = w.WriteField("reply_markup", string(rm))
	}
	part, err := w.CreateFormFile(field, p.Filename)
	if err != nil {
		return nil, err
	}
	if _, err := part.Write(p.Data); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	var m Message
	return &m, c.post(ctx, method, &buf, w.FormDataContentType(), &m)
}

// GetFile resolves a file_id to a downloadable path.
func (c *Client) GetFile(ctx context.Context, fileID string) (*File, error) {
	var f File
	return &f, c.call(ctx, "getFile", map[string]any{"file_id": fileID}, &f)
}

// Download fetches a file by file_id, refusing anything over maxBytes. It
// returns the bytes and Telegram's file_path (useful for the extension).
func (c *Client) Download(ctx context.Context, fileID string, maxBytes int64) ([]byte, string, error) {
	f, err := c.GetFile(ctx, fileID)
	if err != nil {
		return nil, "", err
	}
	if f.FileSize > maxBytes {
		return nil, "", fmt.Errorf("telegram: file is %d bytes, limit is %d", f.FileSize, maxBytes)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/file/bot"+c.token+"/"+f.FilePath, nil)
	if err != nil {
		return nil, "", c.scrub(err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("telegram: file download: %w", c.scrub(err))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("telegram: file download HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, "", err
	}
	if int64(len(data)) > maxBytes {
		return nil, "", fmt.Errorf("telegram: file exceeds %d bytes", maxBytes)
	}
	return data, f.FilePath, nil
}
