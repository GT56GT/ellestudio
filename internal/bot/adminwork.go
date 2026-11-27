package bot

// adminwork.go is the bot's side of the admin panel's actions:
//
//   - jobs: the panel asks for work only the bot can do (it alone holds the
//     Cartesia key), such as deleting a voice; the bot polls a queue table,
//     does the work and records how it went;
//   - names: the bot copies voice and dictionary names from Cartesia into
//     the ownership registry, so the panel can show them;
//   - bans: the bot keeps the ban list in memory, refreshed every
//     banRefresh, and turns banned users away everywhere.

import (
	"context"
	"errors"
	"fmt"
	"html"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"ellestudio/internal/cartesia"
	"ellestudio/internal/telegram"
)

// Admin job kinds.
const (
	JobDeleteVoice = "delete_voice"
	JobDeleteDict  = "delete_dict"
)

// AdminJob is one piece of work the panel queued.
type AdminJob struct {
	ID          int64
	Kind        string
	Target      string // the Cartesia resource ID
	RequestedBy int64
}

// AdminQueue is where admin jobs and resource names live (the database).
type AdminQueue interface {
	// Claim takes the oldest pending job (or one stuck running), if any.
	Claim(ctx context.Context) (AdminJob, bool, error)
	// Finish records how a job went (err nil = done).
	Finish(ctx context.Context, id int64, err error) error
	// SetNames records the display names of resources of one kind.
	SetNames(ctx context.Context, kind string, names map[string]string) error
}

const (
	jobPoll    = 5 * time.Second
	nameSync   = 10 * time.Minute
	banRefresh = 30 * time.Second
)

// SetAdminQueue turns on the admin job worker and name sync.
func (b *Bot) SetAdminQueue(q AdminQueue) { b.adminQueue = q }

// ---------------------------------------------------------------- jobs

func (b *Bot) adminJobs(ctx context.Context) {
	if b.adminQueue == nil {
		return
	}
	t := time.NewTicker(jobPoll)
	defer t.Stop()
	nextSync := time.Now()
	for {
		for ctx.Err() == nil {
			job, ok, err := b.adminQueue.Claim(ctx)
			if err != nil {
				if ctx.Err() == nil {
					b.log.Warn("admin jobs: claim", "err", err)
				}
				break
			}
			if !ok {
				break
			}
			b.runAdminJob(ctx, job)
		}
		if time.Now().After(nextSync) {
			b.syncNames(ctx)
			nextSync = time.Now().Add(nameSync)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (b *Bot) runAdminJob(ctx context.Context, job AdminJob) {
	jctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	var err error
	switch job.Kind {
	case JobDeleteVoice:
		err = b.deleteResource(jctx, kindVoice, job.Target)
	case JobDeleteDict:
		err = b.deleteResource(jctx, kindDict, job.Target)
	default:
		err = fmt.Errorf("unknown job kind %q", job.Kind)
	}
	if err != nil {
		b.log.Warn("admin job failed", "job", job.ID, "kind", job.Kind, "target", job.Target, "err", err)
	} else {
		b.log.Info("admin job done", "job", job.ID, "kind", job.Kind, "target", job.Target, "by", job.RequestedBy)
	}
	fctx, fcancel := context.WithTimeout(context.WithoutCancel(ctx), pgTimeout)
	defer fcancel()
	if ferr := b.adminQueue.Finish(fctx, job.ID, err); ferr != nil {
		b.log.Warn("admin jobs: finish", "job", job.ID, "err", ferr)
	}
}

// deleteResource deletes a voice or dictionary from Cartesia, forgets its
// owner and points anyone who chose it back at the default. A resource
// already gone from Cartesia counts as deleted.
func (b *Bot) deleteResource(ctx context.Context, kind, id string) error {
	var err error
	if kind == kindVoice {
		err = b.ct.DeleteVoice(ctx, id)
	} else {
		err = b.ct.DeletePronunciationDict(ctx, id)
	}
	if ae, ok := cartesia.AsAPIError(err); ok && ae.StatusCode == 404 {
		err = nil
	}
	if err != nil {
		return err
	}
	b.releaseOwner(ctx, kind, id)
	if kind == kindVoice {
		return b.store.ForgetVoice(id)
	}
	return b.store.ForgetDict(id)
}

// syncNames copies the names of the studio's own voices and dictionaries
// into the registry.
func (b *Bot) syncNames(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	voices := map[string]string{}
	for after, pages := "", 0; pages < 50; pages++ {
		page, err := b.ct.ListVoices(ctx, cartesia.ListVoicesParams{Limit: 100, OwnedOnly: true, StartingAfter: after})
		if err != nil {
			b.log.Warn("name sync: voices", "err", err)
			return
		}
		for _, v := range page.Data {
			voices[v.ID] = v.Name
		}
		if !page.HasMore || len(page.Data) == 0 {
			break
		}
		after = page.Data[len(page.Data)-1].ID
	}
	dicts := map[string]string{}
	for after, pages := "", 0; pages < 50; pages++ {
		page, err := b.ct.ListPronunciationDicts(ctx, 100, after)
		if err != nil {
			b.log.Warn("name sync: dictionaries", "err", err)
			return
		}
		for _, d := range page.Data {
			dicts[d.ID] = d.Name
		}
		if !page.HasMore || len(page.Data) == 0 {
			break
		}
		after = page.Data[len(page.Data)-1].ID
	}
	for kind, names := range map[string]map[string]string{kindVoice: voices, kindDict: dicts} {
		if err := b.adminQueue.SetNames(ctx, kind, names); err != nil {
			b.log.Warn("name sync: save", "kind", kind, "err", err)
		}
	}
}

// ---------------------------------------------------------------- bans

// banList is the bot's copy of who is banned.
type banList struct {
	mu   sync.Mutex
	bans map[int64]Ban
}

func (b *Bot) refreshBans() {
	bans, err := b.store.Banned()
	if err != nil {
		b.log.Warn("bans: refresh", "err", err)
		return
	}
	b.bans.mu.Lock()
	b.bans.bans = bans
	b.bans.mu.Unlock()
}

func (b *Bot) banLoop(ctx context.Context) {
	t := time.NewTicker(banRefresh)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			b.refreshBans()
		}
	}
}

// banned returns uid's ban if one is in force. Admins are never banned.
func (b *Bot) banned(uid int64) (Ban, bool) {
	if b.isAdmin(uid) {
		return Ban{}, false
	}
	b.bans.mu.Lock()
	ban, ok := b.bans.bans[uid]
	b.bans.mu.Unlock()
	return ban, ok && ban.Active(time.Now())
}

// updateUser is who sent an update, when it can be banned.
func updateUser(u telegram.Update) (int64, bool) {
	switch {
	case u.Message != nil && u.Message.From != nil && !u.Message.From.IsBot:
		return u.Message.From.ID, true
	case u.CallbackQuery != nil:
		return u.CallbackQuery.From.ID, true
	case u.GuestMessage != nil && u.GuestMessage.From != nil:
		return u.GuestMessage.From.ID, true
	}
	return 0, false
}

func bannedScreen(ban Ban) screen {
	body := "An admin has paused your access to Ellie"
	if ban.Until != nil {
		body += " until <b>" + html.EscapeString(ban.Until.UTC().Format("2 Jan 2006, 15:04")) + " UTC</b>"
	}
	return screen{header(icLock, "Access paused", "") + body + ".", nil}
}

// turnAway answers a banned user: once in the private chat, every time on
// a button or a guest mention (those need an answer), and never in a group
// (so a ban doesn't add noise there).
func (b *Bot) turnAway(ctx context.Context, u telegram.Update, uid int64, ban Ban) {
	switch {
	case u.CallbackQuery != nil:
		_ = b.tg.AnswerCallbackQuery(ctx, u.CallbackQuery.ID, "Your access to Ellie is paused.")
	case u.GuestMessage != nil:
		b.answerGuestText(ctx, u.GuestMessage, bannedScreen(ban))
	case u.Message != nil && !isGroupChat(u.Message.Chat.ID) && !ban.Notified:
		s := bannedScreen(ban)
		b.replyHTML(ctx, u.Message, s.text, nil)
		if _, err := b.store.Update(uid, func(v *Settings) {
			if v.Ban != nil {
				v.Ban.Notified = true
			}
		}); err != nil {
			b.log.Warn("bans: mark notified", "user", uid, "err", err)
		}
		b.bans.mu.Lock()
		if cur, ok := b.bans.bans[uid]; ok {
			cur.Notified = true
			b.bans.bans[uid] = cur
		}
		b.bans.mu.Unlock()
	}
}

// ---------------------------------------------------------------- Postgres

// AdminQueue returns the database's job queue and name registry.
func (d *DB) AdminQueue() AdminQueue { return pgAdminQueue{d.pool} }

type pgAdminQueue struct{ pool *pgxpool.Pool }

// jobStuckAfter is how long a running job may go without finishing before
// another instance takes it over (its runner died mid-way).
const jobStuckAfter = 5 * time.Minute

func (q pgAdminQueue) Claim(ctx context.Context) (AdminJob, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, pgTimeout)
	defer cancel()
	var j AdminJob
	err := q.pool.QueryRow(ctx, `UPDATE ellie_admin_jobs SET status = 'running', updated_at = now()
		WHERE id = (
			SELECT id FROM ellie_admin_jobs
			WHERE status = 'pending' OR (status = 'running' AND updated_at < now() - make_interval(secs => $1))
			ORDER BY id FOR UPDATE SKIP LOCKED LIMIT 1)
		RETURNING id, kind, target, requested_by`, jobStuckAfter.Seconds()).Scan(&j.ID, &j.Kind, &j.Target, &j.RequestedBy)
	if errors.Is(err, pgx.ErrNoRows) {
		return AdminJob{}, false, nil
	}
	if err != nil {
		return AdminJob{}, false, fmt.Errorf("database: claim admin job: %w", err)
	}
	return j, true, nil
}

func (q pgAdminQueue) Finish(ctx context.Context, id int64, jobErr error) error {
	status, msg := "done", ""
	if jobErr != nil {
		status, msg = "failed", jobErr.Error()
		if len(msg) > 500 {
			msg = msg[:500]
		}
	}
	_, err := q.pool.Exec(ctx, `UPDATE ellie_admin_jobs SET status = $2, error = $3, updated_at = now() WHERE id = $1`, id, status, msg)
	if err != nil {
		return fmt.Errorf("database: finish admin job: %w", err)
	}
	return nil
}

func (q pgAdminQueue) SetNames(ctx context.Context, kind string, names map[string]string) error {
	if len(names) == 0 {
		return nil
	}
	ids := make([]string, 0, len(names))
	vals := make([]string, 0, len(names))
	for id, name := range names {
		ids, vals = append(ids, id), append(vals, name)
	}
	_, err := q.pool.Exec(ctx, `UPDATE ellie_ownership o SET name = v.name
		FROM unnest($2::text[], $3::text[]) AS v(id, name)
		WHERE o.kind = $1 AND o.resource_id = v.id AND o.name <> v.name`, kind, ids, vals)
	if err != nil {
		return fmt.Errorf("database: save names: %w", err)
	}
	return nil
}
