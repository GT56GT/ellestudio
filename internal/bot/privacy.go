package bot

// privacy.go decides who sees and manages what. Every Telegram user shares one
// Cartesia account, so Cartesia marks every clone, localized voice and
// dictionary as "owned" by that account and can't tell users apart. The bot
// writes the creator's Telegram ID into the resource's description (stored in
// Cartesia, so it survives redeploys) and uses it like this:
//
//   - Voices are UNLISTED, not locked. A user's library shows Cartesia's
//     public voices plus their own creations, never anyone else's.
//   - A voice ID is the key: whoever has it (pasted, via /voice <id>, or via a
//     share link t.me/<bot>?start=v_<id>) can preview and use that voice.
//   - Only the creator (or an admin) can rename, delete, localize or share it.
//   - Dictionaries are listed and usable only by their creator.
//   - Account voices with no tag (made before tagging, or outside the bot)
//     are listed for admins only, but still open to anyone given their ID.

import (
	"errors"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"ellestudio/internal/cartesia"
)

var ownerTagRe = regexp.MustCompile(`\s*\[ellie-owner:(\d+)\]`)

// ownerTag is appended to the description of anything a user creates.
func ownerTag(uid int64) string { return "[ellie-owner:" + strconv.FormatInt(uid, 10) + "]" }

// withOwner appends the owner tag to a description.
func withOwner(desc string, uid int64) string {
	return strings.TrimSpace(desc + " " + ownerTag(uid))
}

// ownerOf reads the creator from a description.
func ownerOf(desc string) (int64, bool) {
	m := ownerTagRe.FindStringSubmatch(desc)
	if m == nil {
		return 0, false
	}
	id, err := strconv.ParseInt(m[1], 10, 64)
	return id, err == nil
}

// cleanDescription hides the owner tag from people reading a description.
func cleanDescription(desc string) string {
	return strings.TrimSpace(ownerTagRe.ReplaceAllString(desc, ""))
}

// createdBy reports whether uid made this account-owned resource.
func createdBy(uid int64, owned bool, desc string) bool {
	if !owned {
		return false
	}
	owner, ok := ownerOf(desc)
	return ok && owner == uid
}

// isMine reports whether uid created the voice.
func isMine(uid int64, v cartesia.Voice) bool { return createdBy(uid, v.IsOwner, v.Description) }

// canList decides whether a voice appears in uid's library: public voices,
// the user's own creations, and (for admins) everything.
func (b *Bot) canList(uid int64, v cartesia.Voice) bool {
	return !v.IsOwner || b.isAdmin(uid) || isMine(uid, v)
}

// canManage decides whether uid may rename, delete, localize or share a voice.
func (b *Bot) canManage(uid int64, v cartesia.Voice) bool {
	return v.IsOwner && (b.isAdmin(uid) || isMine(uid, v))
}

// canUseDict: dictionaries stay with their creator (admins see all).
func (b *Bot) canUseDict(uid int64, d cartesia.PronunciationDict) bool {
	return !d.IsOwner || b.isAdmin(uid) || createdBy(uid, d.IsOwner, d.Description)
}

var (
	errNotYourVoice = errors.New("Only the person who created this voice can change it.")
	errPrivateDict  = errors.New("That dictionary belongs to someone else.")
)

// ---------------------------------------------------------------- voice IDs & share links

var voiceIDRe = regexp.MustCompile(`(?i)\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b`)

// parseVoiceRef recognises a bare voice ID, a "v_<id>" start payload, or a
// share link (t.me/<bot>?start=v_<id>) and returns the voice ID. The whole
// message must be the reference, so ordinary sentences are still spoken.
func parseVoiceRef(text string) (string, bool) {
	t := strings.TrimSpace(text)
	if t == "" || strings.ContainsAny(t, " \n") {
		return "", false
	}
	if u, err := url.Parse(t); err == nil && u.Host != "" {
		t = u.Query().Get("start")
	}
	t = strings.TrimPrefix(t, "v_")
	if id := voiceIDRe.FindString(t); id != "" && len(id) == len(t) {
		return strings.ToLower(id), true
	}
	return "", false
}

// hideVoiceIDs blanks voice IDs inside a description. An ID is a key to that
// voice, so a description must never hand one to whoever can read it (older
// localized voices named their source by ID).
func hideVoiceIDs(s string) string {
	return strings.TrimSpace(voiceIDRe.ReplaceAllString(s, "another voice"))
}

// shareLink opens the bot straight onto a voice.
func (b *Bot) shareLink(id string) string {
	return "https://t.me/" + orDefault(b.username, "EllieStudioBot") + "?start=v_" + id
}
