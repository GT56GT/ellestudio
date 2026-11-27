// Package admin is Ellie Studio's admin panel: a small web app, run as its
// own service, that shows the bot's health and usage to studio admins. Admins
// sign in with Telegram, so there are no passwords to store or leak.
package admin

// auth.go checks Telegram's two sign-in proofs. Both are signed with a key
// derived from the bot token, which only Telegram and this server know:
//
//   - the Login Widget (a browser), signed with SHA-256(token);
//   - Mini App initData (the panel opened inside Telegram), signed with
//     HMAC-SHA-256("WebAppData", token).
//
// See https://core.telegram.org/widgets/login#checking-authorization and
// https://core.telegram.org/bots/webapps#validating-data-received-via-the-mini-app

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// TelegramUser is who a sign-in proof is for.
type TelegramUser struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name,omitempty"`
	Username  string `json:"username,omitempty"`
}

// Name is how the panel greets the user.
func (u TelegramUser) Name() string {
	if n := strings.TrimSpace(u.FirstName + " " + u.LastName); n != "" {
		return n
	}
	if u.Username != "" {
		return "@" + u.Username
	}
	return strconv.FormatInt(u.ID, 10)
}

var (
	errBadSignature = errors.New("admin: the sign-in proof isn't signed by Telegram")
	errStale        = errors.New("admin: the sign-in proof is too old")
	errMalformed    = errors.New("admin: the sign-in proof is malformed")
)

// checkString is Telegram's data-check-string: every field except hash,
// sorted by key, as key=value lines.
func checkString(fields url.Values) string {
	keys := make([]string, 0, len(fields))
	for k := range fields {
		if k != "hash" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	lines := make([]string, len(keys))
	for i, k := range keys {
		lines[i] = k + "=" + fields.Get(k)
	}
	return strings.Join(lines, "\n")
}

func hmacSHA256(key []byte, msg string) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(msg))
	return m.Sum(nil)
}

// verify checks fields' hash against secret and that auth_date is no older
// than maxAge (nor in the future).
func verify(fields url.Values, secret []byte, now time.Time, maxAge time.Duration) error {
	got, err := hex.DecodeString(fields.Get("hash"))
	if err != nil || len(got) != sha256.Size {
		return errMalformed
	}
	if !hmac.Equal(got, hmacSHA256(secret, checkString(fields))) {
		return errBadSignature
	}
	ts, err := strconv.ParseInt(fields.Get("auth_date"), 10, 64)
	if err != nil {
		return errMalformed
	}
	age := now.Sub(time.Unix(ts, 0))
	if age > maxAge || age < -time.Minute {
		return errStale
	}
	return nil
}

// VerifyLoginWidget checks the fields the Telegram Login Widget sent (as the
// query string of its redirect) and returns who signed in.
func VerifyLoginWidget(botToken string, fields url.Values, now time.Time, maxAge time.Duration) (TelegramUser, error) {
	key := sha256.Sum256([]byte(botToken))
	if err := verify(fields, key[:], now, maxAge); err != nil {
		return TelegramUser{}, err
	}
	id, err := strconv.ParseInt(fields.Get("id"), 10, 64)
	if err != nil || id <= 0 {
		return TelegramUser{}, errMalformed
	}
	return TelegramUser{
		ID: id, FirstName: fields.Get("first_name"), LastName: fields.Get("last_name"), Username: fields.Get("username"),
	}, nil
}

// VerifyWebAppInitData checks a Mini App's initData string and returns who
// opened it.
func VerifyWebAppInitData(botToken, initData string, now time.Time, maxAge time.Duration) (TelegramUser, error) {
	fields, err := url.ParseQuery(initData)
	if err != nil {
		return TelegramUser{}, errMalformed
	}
	if err := verify(fields, hmacSHA256([]byte("WebAppData"), botToken), now, maxAge); err != nil {
		return TelegramUser{}, err
	}
	var u TelegramUser
	if err := json.Unmarshal([]byte(fields.Get("user")), &u); err != nil || u.ID <= 0 {
		return TelegramUser{}, errMalformed
	}
	return u, nil
}
