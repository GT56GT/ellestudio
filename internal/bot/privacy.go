package bot

// privacy.go decides who sees and manages what. Every Telegram user shares one
// Cartesia account, so Cartesia marks every clone, localized voice and
// dictionary as "owned" by that account and can't tell users apart. The Owners
// registry (owners.go) records the Telegram user who created each one, and:
//
//   - Voices are UNLISTED, not locked. A user's library shows Cartesia's
//     public voices plus their own creations, never anyone else's.
//   - A voice ID is the key: whoever has it (pasted, via /voice <id>, or via a
//     share link t.me/<bot>?start=v_<id>) can preview and use that voice.
//   - Only the creator (or an admin) can rename, delete, localize or share it.
//   - Dictionaries are listed and usable only by their creator.
//   - Account voices with no owner (made outside the bot) are listed for
//     admins only, but still open to anyone given their ID.
//
// Older versions recorded the owner as a "[ellie-owner:<id>]" tag at the end
// of the description instead. Those tags are imported into the registry at
// startup, and a tagged resource the registry hasn't seen yet is adopted the
// first time it's looked at. New resources are never tagged.

import (
	"context"
	"errors"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"ellestudio/internal/cartesia"
)

var (
	// ownerTagRe matches a legacy owner tag anywhere, for hiding it.
	ownerTagRe = regexp.MustCompile(`\s*\[ellie-owner:(\d+)\]`)
	// trailingOwnerRe matches only the tag older versions appended at the very
	// end. A tag anywhere else (for example copied in from a voice name) is
	// never trusted.
	trailingOwnerRe = regexp.MustCompile(`\[ellie-owner:(\d+)\]\s*$`)
	// tagLikeRe catches look-alike tags in user text, however they are spaced.
	tagLikeRe = regexp.MustCompile(`(?i)\[\s*ellie[\s_-]*owner\s*:[^\]]*\]?`)
)

// stripTags removes anything that looks like an owner tag from user text, so
// names can never smuggle one into a legacy import.
func stripTags(s string) string {
	return strings.TrimSpace(tagLikeRe.ReplaceAllString(s, ""))
}

// legacyOwner reads the creator from an older version's description tag.
func legacyOwner(desc string) (int64, bool) {
	m := trailingOwnerRe.FindStringSubmatch(desc)
	if m == nil {
		return 0, false
	}
	id, err := strconv.ParseInt(m[1], 10, 64)
	return id, err == nil && id > 0
}

// cleanDescription hides legacy owner tags from people reading a description.
func cleanDescription(desc string) string {
	return strings.TrimSpace(ownerTagRe.ReplaceAllString(desc, ""))
}

// resource is the part of a voice or dictionary that ownership depends on.
type resource struct {
	id    string
	owned bool // owned by the shared Cartesia account
	desc  string
}

func voiceRes(v cartesia.Voice) resource { return resource{v.ID, v.IsOwner, v.Description} }
func dictRes(d cartesia.PronunciationDict) resource {
	return resource{d.ID, d.IsOwner, d.Description}
}

// storageError hides database details from users; the real error is logged.
type storageError struct{ err error }

func (e storageError) Error() string {
	return "I couldn't check that just now. Please try again in a moment."
}
func (e storageError) Unwrap() error { return e.err }

func (b *Bot) storageErr(err error) error {
	b.log.Error("ownership registry", "err", err)
	return storageError{err}
}

// ownersOf returns the creator of each account-owned resource that has one.
// Public resources and unowned account resources are absent from the map.
func (b *Bot) ownersOf(ctx context.Context, kind string, rs []resource) (map[string]int64, error) {
	var ids []string
	for _, r := range rs {
		if r.owned && r.id != "" {
			ids = append(ids, r.id)
		}
	}
	if len(ids) == 0 {
		return map[string]int64{}, nil
	}
	got, err := b.owners.Lookup(ctx, kind, ids)
	if err != nil {
		return nil, b.storageErr(err)
	}
	for _, r := range rs {
		if !r.owned || r.id == "" {
			continue
		}
		if _, ok := got[r.id]; ok {
			continue
		}
		uid, ok := legacyOwner(r.desc)
		if !ok {
			continue
		}
		adopted, err := b.owners.Adopt(ctx, kind, r.id, uid)
		if err != nil {
			return nil, b.storageErr(err)
		}
		if adopted {
			got[r.id] = uid
			continue
		}
		// Someone recorded an owner between our lookup and adopt; theirs wins.
		again, err := b.owners.Lookup(ctx, kind, []string{r.id})
		if err != nil {
			return nil, b.storageErr(err)
		}
		if o, ok := again[r.id]; ok {
			got[r.id] = o
		}
	}
	return got, nil
}

// ownerOf returns the creator of one resource.
func (b *Bot) ownerOf(ctx context.Context, kind string, r resource) (int64, bool, error) {
	m, err := b.ownersOf(ctx, kind, []resource{r})
	if err != nil {
		return 0, false, err
	}
	o, ok := m[r.id]
	return o, ok, nil
}

// visible decides whether a resource appears in uid's lists: public ones,
// the user's own creations, and (for admins) everything.
func (b *Bot) visible(uid int64, r resource, owners map[string]int64) bool {
	if !r.owned || b.isAdmin(uid) {
		return true
	}
	o, ok := owners[r.id]
	return ok && o == uid
}

// isMineIn reports whether uid created r, given its looked-up owners.
func isMineIn(uid int64, r resource, owners map[string]int64) bool {
	o, ok := owners[r.id]
	return r.owned && ok && o == uid
}

// canManage decides whether uid may rename, delete, localize or share a voice.
// It fails closed: if ownership can't be checked, the answer is an error.
func (b *Bot) canManage(ctx context.Context, uid int64, v cartesia.Voice) (bool, error) {
	if !v.IsOwner {
		return false, nil
	}
	if b.isAdmin(uid) {
		return true, nil
	}
	o, ok, err := b.ownerOf(ctx, kindVoice, voiceRes(v))
	if err != nil {
		return false, err
	}
	return ok && o == uid, nil
}

// mustManage returns errNotYourVoice unless uid may manage v.
func (b *Bot) mustManage(ctx context.Context, uid int64, v cartesia.Voice) error {
	ok, err := b.canManage(ctx, uid, v)
	switch {
	case err != nil:
		return err
	case !ok:
		return errNotYourVoice
	}
	return nil
}

// canUseDict: dictionaries stay with their creator (admins see all).
func (b *Bot) canUseDict(ctx context.Context, uid int64, d cartesia.PronunciationDict) (bool, error) {
	if !d.IsOwner || b.isAdmin(uid) {
		return true, nil
	}
	o, ok, err := b.ownerOf(ctx, kindDict, dictRes(d))
	if err != nil {
		return false, err
	}
	return ok && o == uid, nil
}

// claimOwner records uid as the creator of a resource the bot just made. It runs
// even if ctx was cancelled (for example during shutdown), because a created
// resource without an owner could never be managed.
func (b *Bot) claimOwner(ctx context.Context, kind, id string, uid int64) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), pgTimeout)
	defer cancel()
	if err := b.owners.Claim(ctx, kind, id, uid); err != nil {
		return b.storageErr(err)
	}
	return nil
}

// releaseOwner forgets a deleted resource. A failure only leaves a stale row,
// which listing prunes later, so it is logged rather than shown.
func (b *Bot) releaseOwner(ctx context.Context, kind, id string) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), pgTimeout)
	defer cancel()
	if err := b.owners.Release(ctx, kind, id); err != nil {
		b.log.Warn("release ownership", "kind", kind, "id", id, "err", err)
	}
}

// importLegacyOwners copies "[ellie-owner:<id>]" tags written by older
// versions into the registry. It is idempotent (existing owners always win)
// and bounded in time; anything it misses is adopted on first lookup instead,
// so a failure here never blocks the bot.
func (b *Bot) importLegacyOwners(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	voices, dicts := 0, 0
	for after := ""; ; {
		page, err := b.ct.ListVoices(ctx, cartesia.ListVoicesParams{Limit: 100, OwnedOnly: true, StartingAfter: after})
		if err != nil {
			b.log.Warn("legacy owner import: list voices", "err", err)
			break
		}
		for _, v := range page.Data {
			if uid, ok := legacyOwner(v.Description); ok && v.IsOwner {
				if added, err := b.owners.Adopt(ctx, kindVoice, v.ID, uid); err != nil {
					b.log.Warn("legacy owner import: voice", "id", v.ID, "err", err)
				} else if added {
					voices++
				}
			}
		}
		if !page.HasMore || len(page.Data) == 0 {
			break
		}
		after = orDefault(page.NextPage, page.Data[len(page.Data)-1].ID)
	}
	for after := ""; ; {
		page, err := b.ct.ListPronunciationDicts(ctx, 100, after)
		if err != nil {
			b.log.Warn("legacy owner import: list dictionaries", "err", err)
			break
		}
		for _, d := range page.Data {
			if uid, ok := legacyOwner(d.Description); ok && d.IsOwner {
				if added, err := b.owners.Adopt(ctx, kindDict, d.ID, uid); err != nil {
					b.log.Warn("legacy owner import: dictionary", "id", d.ID, "err", err)
				} else if added {
					dicts++
				}
			}
		}
		if !page.HasMore || len(page.Data) == 0 {
			break
		}
		after = orDefault(page.NextPage, page.Data[len(page.Data)-1].ID)
	}
	b.log.Info("legacy owner import done", "voices", voices, "dictionaries", dicts)
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
