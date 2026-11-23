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
	"sync"
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

	// Group sends are paced to stay under Telegram's per-group flood limit
	// (about 20 messages a minute): a burst of groupBurst, then one message
	// every groupGap. A send that would wait longer than maxWait fails with
	// ErrFlood instead of queueing, so a flood in one group can't tie up the
	// caller's workers.
	groupGap   time.Duration
	groupBurst float64
	maxWait    time.Duration
	paceMu     sync.Mutex
	buckets    map[int64]*bucket
}

// ErrFlood means a group already has as many sends queued as it may.
var ErrFlood = errors.New("telegram: too many messages queued for this group")

type bucket struct {
	tokens float64
	at     time.Time
}

// maxRetryAfter is the longest flood wait the client sits out by itself;
// longer waits are returned to the caller as errors.
const maxRetryAfter = 30 * time.Second

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
		http:       &http.Client{},
		groupGap:   3 * time.Second,
		groupBurst: 5,
		maxWait:    10 * time.Second,
		buckets:    map[int64]*bucket{},
	}
}

// SetGroupPacing changes how sends to groups are spaced and how long one
// may wait (tests use tiny values).
func (c *Client) SetGroupPacing(gap time.Duration, burst int, maxWait time.Duration) {
	c.paceMu.Lock()
	defer c.paceMu.Unlock()
	c.groupGap, c.groupBurst, c.maxWait = gap, float64(max(burst, 1)), maxWait
}

// refill tops a group's bucket up for the time since it was last used.
// paceMu must be held.
func (c *Client) refill(chatID int64, now time.Time) *bucket {
	bk := c.buckets[chatID]
	if bk == nil {
		bk = &bucket{tokens: c.groupBurst, at: now}
		c.buckets[chatID] = bk
	}
	bk.tokens = min(c.groupBurst, bk.tokens+float64(now.Sub(bk.at))/float64(c.groupGap))
	bk.at = now
	return bk
}

// Backlog is how long a new send to chatID would wait (0 for private chats).
func (c *Client) Backlog(chatID int64) time.Duration {
	if chatID >= 0 {
		return 0
	}
	c.paceMu.Lock()
	defer c.paceMu.Unlock()
	if bk := c.refill(chatID, time.Now()); bk.tokens < 1 {
		return time.Duration((1 - bk.tokens) * float64(c.groupGap))
	}
	return 0
}

// pace waits for a send slot in a group or channel (negative chat IDs), or
// fails with ErrFlood if the wait would pass maxWait. Private chats are not
// paced.
func (c *Client) pace(ctx context.Context, chatID int64) error {
	if chatID >= 0 {
		return nil
	}
	c.paceMu.Lock()
	bk := c.refill(chatID, time.Now())
	var wait time.Duration
	if left := bk.tokens - 1; left < 0 {
		wait = time.Duration(-left * float64(c.groupGap))
	}
	if wait > c.maxWait {
		c.paceMu.Unlock()
		return ErrFlood
	}
	bk.tokens--
	if len(c.buckets) > 10000 { // forget idle groups
		for id, o := range c.buckets {
			if o.tokens >= c.groupBurst {
				delete(c.buckets, id)
			}
		}
	}
	c.paceMu.Unlock()
	return sleepCtx(ctx, wait)
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
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
	ID      int64  `json:"id"`
	Type    string `json:"type"` // "private", "group", "supergroup" or "channel"
	Title   string `json:"title"`
	IsForum bool   `json:"is_forum"`
}

// IsGroup reports whether the chat is a group or supergroup.
func (c Chat) IsGroup() bool { return c.Type == "group" || c.Type == "supergroup" }

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

	MessageThreadID int64 `json:"message_thread_id"`
	IsTopicMessage  bool  `json:"is_topic_message"`
	// EphemeralMessageID is set on ephemeral messages: commands only the bot
	// can see, and the bot's private replies inside a group.
	EphemeralMessageID int64 `json:"ephemeral_message_id"`
	ReceiverUser       *User `json:"receiver_user"`
	SenderChat         *Chat `json:"sender_chat"`
	MigrateToChatID    int64 `json:"migrate_to_chat_id"`
	MigrateFromChatID  int64 `json:"migrate_from_chat_id"`
	// GuestQueryID is set when someone mentions the bot in a chat it is not
	// a member of (guest mode). Answer it once with AnswerGuestQuery.
	GuestQueryID string `json:"guest_query_id"`
}

// Thread is the forum topic the message belongs to (0 outside topics).
func (m *Message) Thread() int64 {
	if m == nil || !m.IsTopicMessage {
		return 0
	}
	return m.MessageThreadID
}

// ChatMember is a user's membership in a chat.
type ChatMember struct {
	Status string `json:"status"` // creator, administrator, member, restricted, left, kicked
	User   User   `json:"user"`
	// IsMember is set for "restricted" members who are still in the chat.
	IsMember bool `json:"is_member"`
}

// Present reports whether the member is in the chat.
func (m ChatMember) Present() bool {
	switch m.Status {
	case "creator", "administrator", "member":
		return true
	case "restricted":
		return m.IsMember
	}
	return false
}

// ChatMemberUpdated reports a change to the bot's own membership.
type ChatMemberUpdated struct {
	Chat          Chat       `json:"chat"`
	From          User       `json:"from"`
	OldChatMember ChatMember `json:"old_chat_member"`
	NewChatMember ChatMember `json:"new_chat_member"`
}

type CallbackQuery struct {
	ID      string   `json:"id"`
	From    User     `json:"from"`
	Message *Message `json:"message"`
	Data    string   `json:"data"`
}

type Update struct {
	UpdateID      int64              `json:"update_id"`
	Message       *Message           `json:"message"`
	CallbackQuery *CallbackQuery     `json:"callback_query"`
	MyChatMember  *ChatMemberUpdated `json:"my_chat_member"`
	// GuestMessage is a mention of the bot in a chat it is not a member of.
	GuestMessage *Message `json:"guest_message"`
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
	// IsEphemeral makes the command invisible to everyone but the bot, and
	// lets the bot answer it privately inside a group.
	IsEphemeral bool `json:"is_ephemeral,omitempty"`
}

// Ephemeral makes a message visible only to one member of a group. Telegram
// accepts it within 15 seconds of the member's action: a button press
// (CallbackQueryID) or an ephemeral command (the ReplyToEphemeral field of
// the send).
type Ephemeral struct {
	ReceiverUserID  int64  // who sees it; leave 0 when answering a button press
	CallbackQueryID string // the button press being answered
	ReplaceMessage  bool   // show it in place of the pressed message
}

func (e *Ephemeral) param() map[string]any {
	m := map[string]any{}
	if e.ReceiverUserID != 0 {
		m["receiver_user_id"] = e.ReceiverUserID
	}
	if e.CallbackQueryID != "" {
		m["callback_query_id"] = e.CallbackQueryID
	}
	if e.ReplaceMessage {
		m["replace_callback_query_message"] = true
	}
	return m
}

// replyParams builds reply_parameters for a send, or nil for none.
func replyParams(replyTo, replyToEphemeral int64) map[string]any {
	switch {
	case replyToEphemeral != 0:
		return map[string]any{"ephemeral_message_id": replyToEphemeral}
	case replyTo != 0:
		return map[string]any{"message_id": replyTo, "allow_sending_without_reply": true}
	}
	return nil
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

// post sends body, sitting out short flood waits (HTTP 429 with a
// retry_after of up to maxRetryAfter) and trying again, at most twice.
func (c *Client) post(ctx context.Context, method string, body []byte, contentType string, out any) error {
	for attempt := 0; ; attempt++ {
		err := c.postOnce(ctx, method, bytes.NewReader(body), contentType, out)
		var te *Error
		if attempt >= 2 || !errors.As(err, &te) || te.Code != http.StatusTooManyRequests ||
			te.RetryAfter <= 0 || time.Duration(te.RetryAfter)*time.Second > maxRetryAfter {
			return err
		}
		if serr := sleepCtx(ctx, time.Duration(te.RetryAfter)*time.Second); serr != nil {
			return err
		}
	}
}

func (c *Client) postOnce(ctx context.Context, method string, body io.Reader, contentType string, out any) error {
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
	return c.post(ctx, method, buf, "application/json", out)
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
		"allowed_updates": []string{"message", "callback_query", "my_chat_member", "guest_message"},
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
	ThreadID    int64 // forum topic, send only
	ReplyMarkup any   // *InlineKeyboardMarkup or *ReplyKeyboardMarkup (edits: inline only)
	NoPreview   bool
	// Ephemeral sends the message to one member of a group only, and
	// ReplyToEphemeral answers their ephemeral command (send only).
	Ephemeral        *Ephemeral
	ReplyToEphemeral int64
}

func (p SendMessageParams) payload() map[string]any {
	m := map[string]any{"chat_id": p.ChatID, "text": p.Text}
	if p.MessageID != 0 {
		m["message_id"] = p.MessageID
	} else if p.ThreadID != 0 {
		m["message_thread_id"] = p.ThreadID
	}
	if p.ParseMode != "" {
		m["parse_mode"] = p.ParseMode
	}
	if rp := replyParams(p.ReplyTo, p.ReplyToEphemeral); rp != nil {
		m["reply_parameters"] = rp
	}
	if p.Ephemeral != nil {
		m["ephemeral_message_parameters"] = p.Ephemeral.param()
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
	if p.Ephemeral == nil { // private replies don't add to the group's timeline
		if err := c.pace(ctx, p.ChatID); err != nil {
			return nil, err
		}
	}
	var m Message
	return &m, c.call(ctx, "sendMessage", p.payload(), &m)
}

// EphemeralEdit identifies an ephemeral message to edit.
type EphemeralEdit struct {
	ChatID      int64
	UserID      int64 // who received it
	EphemeralID int64
	Text        string
	ParseMode   string
	ReplyMarkup *InlineKeyboardMarkup
	NoPreview   bool
}

// EditEphemeralMessageText replaces the text and keyboard of an ephemeral
// message. Telegram doesn't guarantee the member sees the edit.
func (c *Client) EditEphemeralMessageText(ctx context.Context, e EphemeralEdit) error {
	p := map[string]any{
		"chat_id": e.ChatID, "receiver_user_id": e.UserID, "ephemeral_message_id": e.EphemeralID, "text": e.Text,
	}
	if e.ParseMode != "" {
		p["parse_mode"] = e.ParseMode
	}
	if e.ReplyMarkup != nil {
		p["reply_markup"] = e.ReplyMarkup
	}
	if e.NoPreview {
		p["link_preview_options"] = map[string]any{"is_disabled": true}
	}
	return c.call(ctx, "editEphemeralMessageText", p, nil)
}

// DeleteEphemeralMessage removes an ephemeral message.
func (c *Client) DeleteEphemeralMessage(ctx context.Context, chatID, userID, ephemeralID int64) error {
	return c.call(ctx, "deleteEphemeralMessage", map[string]any{
		"chat_id": chatID, "receiver_user_id": userID, "ephemeral_message_id": ephemeralID,
	}, nil)
}

// GetChatMember returns a user's membership in a chat.
func (c *Client) GetChatMember(ctx context.Context, chatID, userID int64) (*ChatMember, error) {
	var m ChatMember
	return &m, c.call(ctx, "getChatMember", map[string]any{"chat_id": chatID, "user_id": userID}, &m)
}

// IsAdmin reports whether the member runs the chat.
func (m ChatMember) IsAdmin() bool { return m.Status == "creator" || m.Status == "administrator" }

// EditMessageText replaces the text (and keyboard) of an existing message.
func (c *Client) EditMessageText(ctx context.Context, p SendMessageParams) error {
	return c.call(ctx, "editMessageText", p.payload(), nil)
}

// AnswerCallbackQuery acknowledges a button press, optionally with a toast.
func (c *Client) AnswerCallbackQuery(ctx context.Context, id, text string) error {
	return c.call(ctx, "answerCallbackQuery", map[string]any{"callback_query_id": id, "text": text}, nil)
}

// AnswerCallbackQueryURL acknowledges a button press by opening url, which
// may be a t.me/<bot>?start=<payload> link into the private chat.
func (c *Client) AnswerCallbackQueryURL(ctx context.Context, id, url string) error {
	return c.call(ctx, "answerCallbackQuery", map[string]any{"callback_query_id": id, "url": url}, nil)
}

// SendChatAction shows "recording voice…" etc. for about 5 seconds.
func (c *Client) SendChatAction(ctx context.Context, chatID, threadID int64, action string) error {
	p := map[string]any{"chat_id": chatID, "action": action}
	if threadID != 0 {
		p["message_thread_id"] = threadID
	}
	return c.call(ctx, "sendChatAction", p, nil)
}

// Command scopes for SetMyCommands.
const (
	ScopeDefault      = "default"
	ScopePrivateChats = "all_private_chats"
	ScopeGroupChats   = "all_group_chats"
	ScopeGroupAdmins  = "all_chat_administrators"
)

// SetMyCommands sets the command menu clients show in the given scope.
func (c *Client) SetMyCommands(ctx context.Context, cmds []BotCommand, scope string) error {
	p := map[string]any{"commands": cmds}
	if scope != "" && scope != ScopeDefault {
		p["scope"] = map[string]any{"type": scope}
	}
	return c.call(ctx, "setMyCommands", p, nil)
}

// SetMyCommandsForChat sets the command menu shown in one chat (for a
// private chat, the chat ID is the user's ID).
func (c *Client) SetMyCommandsForChat(ctx context.Context, cmds []BotCommand, chatID int64) error {
	return c.call(ctx, "setMyCommands", map[string]any{
		"commands": cmds, "scope": map[string]any{"type": "chat", "chat_id": chatID},
	}, nil)
}

// LeaveChat removes the bot from a group.
func (c *Client) LeaveChat(ctx context.Context, chatID int64) error {
	return c.call(ctx, "leaveChat", map[string]any{"chat_id": chatID}, nil)
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
	ThreadID int64 // forum topic
	// Ephemeral and ReplyToEphemeral work as in SendMessageParams.
	Ephemeral        *Ephemeral
	ReplyToEphemeral int64
	Caption          string
	Filename         string
	Data             []byte
	Title            string // sendAudio only
	// ReplyMarkup is an optional keyboard attached under the media.
	ReplyMarkup any
	// Silent sends without a notification sound.
	Silent bool
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
	if p.Ephemeral == nil {
		if err := c.pace(ctx, p.ChatID); err != nil {
			return nil, err
		}
	}
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("chat_id", strconv.FormatInt(p.ChatID, 10))
	if p.ThreadID != 0 {
		_ = w.WriteField("message_thread_id", strconv.FormatInt(p.ThreadID, 10))
	}
	if p.Caption != "" {
		_ = w.WriteField("caption", p.Caption)
	}
	if p.Title != "" {
		_ = w.WriteField("title", p.Title)
	}
	if p.Silent {
		_ = w.WriteField("disable_notification", "true")
	}
	if rp := replyParams(p.ReplyTo, p.ReplyToEphemeral); rp != nil {
		raw, _ := json.Marshal(rp)
		_ = w.WriteField("reply_parameters", string(raw))
	}
	if p.Ephemeral != nil {
		raw, _ := json.Marshal(p.Ephemeral.param())
		_ = w.WriteField("ephemeral_message_parameters", string(raw))
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
	return &m, c.post(ctx, method, buf.Bytes(), w.FormDataContentType(), &m)
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

// ---------------------------------------------------------------- guest mode

// InlineQueryResultCachedVoice is a voice note already on Telegram's servers.
type InlineQueryResultCachedVoice struct {
	Type        string                `json:"type"` // "voice"
	ID          string                `json:"id"`
	VoiceFileID string                `json:"voice_file_id"`
	Title       string                `json:"title"`
	Caption     string                `json:"caption,omitempty"`
	ReplyMarkup *InlineKeyboardMarkup `json:"reply_markup,omitempty"`
}

// InputTextMessageContent is the text an article result posts.
type InputTextMessageContent struct {
	MessageText string `json:"message_text"`
	ParseMode   string `json:"parse_mode,omitempty"`
}

// InlineQueryResultArticle posts a text message.
type InlineQueryResultArticle struct {
	Type                string                  `json:"type"` // "article"
	ID                  string                  `json:"id"`
	Title               string                  `json:"title"`
	InputMessageContent InputTextMessageContent `json:"input_message_content"`
	ReplyMarkup         *InlineKeyboardMarkup   `json:"reply_markup,omitempty"`
}

// AnswerGuestQuery posts the bot's one reply to a guest mention and returns
// the sent message's inline_message_id.
func (c *Client) AnswerGuestQuery(ctx context.Context, queryID string, result any) (string, error) {
	var sent struct {
		InlineMessageID string `json:"inline_message_id"`
	}
	err := c.call(ctx, "answerGuestQuery", map[string]any{"guest_query_id": queryID, "result": result}, &sent)
	return sent.InlineMessageID, err
}
