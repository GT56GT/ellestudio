package bot

// privacy.go keeps each person's creations to themselves. Every Telegram user
// shares one Cartesia account, so Cartesia marks every clone, localized voice
// and dictionary as "owned" by that account and can't tell users apart. The
// bot therefore writes the creator's Telegram ID into the resource's
// description (stored in Cartesia, so it survives redeploys) and checks it
// wherever a voice or dictionary can be listed, previewed, opened or chosen.
//
// Rules:
//   - public library voices (not owned by the account) are open to everyone;
//   - an owned voice or dictionary is visible only to the user in its tag;
//   - owned resources with no tag (made before tagging, or outside the bot)
//     are visible to admins only;
//   - admins see everything.

import (
	"errors"
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

// canSee decides whether uid may see or use an account-owned resource.
func (b *Bot) canSee(uid int64, owned bool, desc string) bool {
	if !owned || b.isAdmin(uid) {
		return true
	}
	owner, ok := ownerOf(desc)
	return ok && owner == uid
}

func (b *Bot) canUseVoice(uid int64, v cartesia.Voice) bool {
	return b.canSee(uid, v.IsOwner, v.Description)
}

func (b *Bot) canUseDict(uid int64, d cartesia.PronunciationDict) bool {
	return b.canSee(uid, d.IsOwner, d.Description)
}

var (
	errPrivateVoice = errors.New("That voice is private to the person who created it. Pick another one from the library.")
	errPrivateDict  = errors.New("That dictionary is private to the person who created it.")
)
