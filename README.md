# Ellie Studio

[![CI](https://github.com/GT56GT/ellestudio/actions/workflows/ci.yml/badge.svg)](https://github.com/GT56GT/ellestudio/actions/workflows/ci.yml)

A Telegram voice studio powered by [Cartesia](https://cartesia.ai). Send text and get it back as speech. Send a voice note and get a transcript. Browse, clone, and localize voices, all from chat.

Written in Go. The Cartesia and Telegram clients use only the standard library; the one dependency is [pgx](https://github.com/jackc/pgx), for the optional Postgres store.

## Quick start

```bash
cp .env.example .env
```

Fill in `TELEGRAM_BOT_TOKEN` (from @BotFather) and `CARTESIA_API_KEY` (from play.cartesia.ai/keys). Set `ALLOWED_USER_IDS` too, so strangers can't spend your credits. Then:

```bash
go run ./cmd/ellie
```

Don't know your Telegram ID? Start the bot with `ALLOWED_USER_IDS` set to anything (for example `1`), message it, and it replies with your ID.

## How it feels

- **A menu that never goes away:** 🎙 Speak · 🎧 Voices · 🎛 Style · 📝 Transcribe · 🏠 Home · ❓ Help sit under the message box.
- **A Home card** shows your current voice, style and output, with buttons to every area.
- **No dead ends:** every screen ends with `◀ Back` / `🏠 Home`. Pickers mark your choice with ✅ and redraw in place.
- **Guided steps** for search, cloning (clip → name) and pronunciation dictionaries. Each shows "Step 1 of 2" and a ✖ Cancel button.
- **A next step after every result:** voice notes come with 🔁 Again · 🎭 Emotion · 🎧 Voice · 🎛 Style · 🏠 Home. Transcripts offer 🔊 Read aloud. Previews offer ✅ Use this voice.
- **Progress you can see:** "⏳ Listening…" turns into the transcript, and "⏳ Cloning…" turns into the new voice card.
- **Errors point to the fix,** for example "voice not found" comes with a 🎧 Pick a voice button.
- Slash commands still work for power users. Help → Commands lists them all.

## Where data is kept

| What | Where |
|---|---|
| Voices and dictionaries | Cartesia |
| Who created each voice and dictionary | The bot's database (`ellie_ownership`) |
| Each user's choices (voice, style, output) and today's usage | The bot's database (`ellie_user_settings`) |
| Groups the bot is in, blocked groups, and each group's usage today | The bot's database (`ellie_chats`) |
| What the whole studio used each day | The bot's database (`ellie_studio_usage`) |

The bot's database is one of:

- **Postgres** (production): set `DATABASE_URL`. On Render, use the database's **Internal** connection string. Tables are created and upgraded on startup by versioned migrations (`ellie_schema_migrations`), which are safe when two instances start at once during a deploy. The database is the source of truth for user settings: each change is one short transaction that locks only that user (an advisory lock, so it covers users with no row yet), so users never wait on each other and two instances running side by side can't overwrite each other's changes. If the database is briefly unreachable, the bot carries on from a memory copy and logs the error.
- **Files** (local runs): without `DATABASE_URL`, the bot writes `DATA_DIR/settings.json`, `DATA_DIR/owners.json` and `DATA_DIR/chats.json`. Hosts without a persistent disk, such as Render, wipe these on every deploy.

### Ownership

Every user reaches Cartesia through the same API key, so Cartesia can't tell users apart. The bot records the creator's Telegram user ID against each Cartesia voice and dictionary ID:

- The owner is recorded as soon as Cartesia creates the resource. If it can't be recorded, the new voice or dictionary is deleted again and the user's daily allowance is refunded, so nothing is ever left without an owner.
- Every check reads the database directly (no cache), so instances running side by side agree.
- If the database can't be reached, the bot refuses to rename, delete or share rather than guessing, and shows the user a "try again" message. The real error goes to the logs only.
- 🗂 Mine and the dictionary list come straight from the registry, newest first, however many voices other people have made. Voices deleted outside the bot are pruned automatically.
- Earlier versions stored the owner as an `[ellie-owner:<id>]` tag at the end of the Cartesia description. Those tags are imported at startup, and a tagged voice the import missed is adopted the first time it's looked at. New resources are never tagged, so rolling back to a pre-registry version would show new voices to admins only.

## Opening it to everyone

Leave `ALLOWED_USER_IDS` empty and keep your own ID in `ADMIN_USER_IDS`. Then:

- Each user gets a **daily allowance**: `DAILY_CHAR_LIMIT` characters of speech (default 2,000) and `DAILY_TRANSCRIBE_MINUTES` minutes of transcription and watermark checks (default 10). It resets at midnight UTC. Users see what's left in ⚙️ Studio, and a friendly screen tells them when it refreshes. Failed requests are refunded. Resetting settings doesn't refill it. Admins are unlimited.
- Each user runs **one request at a time**, so nobody can tie up the shared Cartesia slots.
- **A cap for everyone together** (optional, off by default): `STUDIO_DAILY_CHAR_LIMIT` characters and `STUDIO_DAILY_TRANSCRIBE_MINUTES` minutes a day across private chats, groups and guest replies. When it's reached, people see "Ellie is resting for today" (and aren't charged), and admins get one Telegram alert that day. Set it from your Cartesia plan's credits so a sudden rush can't run them out. Admins are exempt. If the database is unreachable, the cap is still enforced from a local count seeded from the database each day; if today's total was never learned, requests are refused rather than let through.

### Busy times

- **A fair waiting line.** Cartesia work (readings, transcriptions, clones, watermark checks) waits in one first-come, first-served line for the `CARTESIA_CONCURRENCY` slots. After two seconds in line, people see "You're #3 in line", and the note disappears when their turn starts. Menus, buttons and `/start` never wait behind it. At most `JOB_QUEUE_LIMIT` jobs wait (default 50, `0` = no cap); past that, new requests get "Ellie is very busy, try again in a minute" and aren't charged. Audio is downloaded only when its turn comes, so a long line holds no files in memory.
- **Automatic pause when Cartesia fails.** After 5 Cartesia failures in a row from at least 3 different people (server errors or timeouts, not bad requests), voice work pauses for a minute, then one request tries again. Needing several people means one person sending audio that Cartesia chokes on can't pause the studio for everyone. If Cartesia says the credits ran out, it pauses for 10 minutes. While paused, people see a short "try again in a few minutes" message and aren't charged, and admins get a Telegram alert (at most one per kind every 30 minutes).
- **Telegram's limits.** Besides the per-group pacing, every outgoing message is spaced to 25 a second overall, under Telegram's limit of about 30. Polling, button answers and typing indicators aren't delayed.
- **Flat memory.** Browsing state, remembered text (for Again and Read aloud) and unfinished steps are dropped once idle, so memory doesn't grow with the number of people who have ever used the bot.
- **Everyone can clone** (and localize their own voices), up to `DAILY_VOICE_CREATIONS` new voices a day (default 3). Anyone can create pronunciation dictionaries.
- **Voices are unlisted, and the ID is the key.** Your library shows Cartesia's public voices plus voices *you* made, never anyone else's. To share one, open it and tap 🔗 **Share**. You get a link (`t.me/EllieStudioBot?start=v_<id>`) and the voice ID. Whoever has either can use the voice: tapping the link, pasting it, or pasting the ID opens the voice right away.
- **Only the creator** (or an admin) can rename, delete, localize or share a voice. Ownership is recorded in the bot's database (see [Ownership](#ownership)).
- Usage stays admin-only.

## Groups

Add the bot to any group or supergroup. Keep **privacy mode on** in @BotFather (the default) and don't make the bot an admin: then Telegram only sends it commands, @mentions and replies to its messages, and it answers only when someone talks to it.

| In the group | What happens |
|---|---|
| `/say Good morning!` | Spoken in the **sender's own** voice and style |
| Reply `/say` to a message | That message is read aloud |
| Reply `/transcribe` to a voice note | The transcript is posted |
| `@EllieStudioBot good morning` | Same as `/say` (a bare mention replying to a message reads or transcribes it) |
| `/voices`, `/style`, `/help` from the command menu | Answered **privately**: only the sender sees the command and the reply |
| **🎛 My voice & style** under any group message | Opens the presser's own settings, privately |
| `/clone`, `/rename`, dictionaries, … | A **Continue in private chat** button that opens that screen in the private chat |
| `/groupsettings` (group admins) | The group's own settings, privately |

- **Private views inside the group** use Telegram's ephemeral messages (Bot API 10.2+), which only one member and the bot can see. `/voices`, `/style`, `/help` and `/groupsettings` are marked ephemeral in the group command menu, and the **🎛 My voice & style** button answers whoever presses it with their own settings. Inside a private view a member can browse and preview voices and change their voice and style; anything bigger (cloning, renaming, deleting, sharing, dictionaries) opens the private chat. Telegram only allows these replies within 15 seconds of the member's command or press.
- **Private stays private.** Public group messages carry only buttons that act for the person who presses them, plus a link into the private chat, so nobody can press someone else's menu. Buttons on older group messages just open the private chat. Guided steps (clone, rename, search) only run in the private chat, so group messages can't be mistaken for their answers. Commands addressed to other bots (`/say@OtherBot`) are ignored.
- **No clutter, no oversharing:** short notices (hints, cooldowns, "limit reached") delete themselves after `GROUP_NOTICE_SECONDS` (default 45). A member's own usage numbers are never posted in the group; **🎟 My allowance** shows them privately.
- **Group admins** (the group's creator and admins, checked with Telegram on every change) get `/groupsettings`: pause Ellie, keep it to admins, turn `/say` or `/transcribe` off, and lower the group's daily allowance (2,500 / 1,000 / 500 characters) or the longest message (500 / 200 characters). They can only ever lower the studio's limits, never raise them. Settings are stored with the group and survive a supergroup upgrade.
- **Everyone keeps their own allowance.** Each member's speech counts against their own daily allowance, as in the private chat.
- **Each group shares an allowance too:** `GROUP_DAILY_CHAR_LIMIT` characters (default 5,000) and `GROUP_DAILY_TRANSCRIBE_MINUTES` minutes (default 30) a day. All groups together share `GROUPS_TOTAL_DAILY_CHAR_LIMIT` (default 50,000) and `GROUPS_TOTAL_DAILY_TRANSCRIBE_MINUTES` (default 120), so one busy public group can't spend the whole budget. The checks are atomic in Postgres, so two instances during a deploy can't overspend. Failed requests are refunded. Only the people in `ADMIN_USER_IDS` are exempt; with no admins named, nobody is.
- **No spamming:** each member waits `GROUP_COOLDOWN_SECONDS` (default 10) between requests, with one reminder per cooldown, and at most `GROUP_CONCURRENCY` jobs (default 2) run per group at once.
- **Telegram's flood limits are respected:** sends to a group are paced to about 20 a minute (a burst of 5, then one every 3 seconds), and short "retry after" waits from Telegram are sat out and retried automatically. If a group floods the bot, sends that would wait more than 10 seconds are dropped, and new requests there are ignored until it catches up, so one busy group never slows down other chats.
- **Forum topics:** replies stay in the topic they were asked in.
- **Who can add the bot:** `GROUP_MODE=open` (anyone, the default), `admins` (only `ADMIN_USER_IDS`, which must then be set; the bot leaves groups anyone else adds) or `off` (the bot leaves every group).
- **Your controls (admins, in the private chat):** `/admin` gathers them in one screen (and appears only in admins' own command menus). `/groups` shows today's busiest groups, their own settings, and the totals, `/blockgroup <id>` makes the bot leave a group for good, `/unblockgroup <id>` lets it back. `/blockgroup` sent inside a group blocks that group.
- When the bot joins it posts a short how-to. When a group is upgraded to a supergroup (its ID changes), its record, block and today's usage move with it.

### Guest mode (no need to add the bot)

With Telegram's guest mode (Bot API 10.0), anyone can type `@EllieStudioBot good morning everyone` in a group or private chat the bot was never added to, and Ellie posts the reading there in that person's own voice. Mentioning the bot in a reply to a message reads that message out.

- **Turn it on** in @BotFather's Mini App (open @BotFather → *Open*, pick the bot → *Bot Settings* → **Guest Mode**). The code needs nothing else.
- **Only your users, by default:** `GUEST_ACCESS=members` (the default) serves people who have used Ellie's private chat at least once (any message or button press, `/start` included). Anyone else gets a "Start Ellie first" reply with a link to the private chat. Using `/say` in a group doesn't count. `GUEST_ACCESS=everyone` serves anyone. `ALLOWED_USER_IDS`, when set, still applies, and admins are always served.
- **What Ellie sees:** only the message that mentions it and the message it replies to. Not the chat's history or members. Each mention tells Ellie who sent it, so it reads in that person's own voice and style.
- **One reply per mention:** the voice, or a short notice (limit reached, text too long, pick a voice first) with a link to the private chat. Texts are capped at 500 characters (or `MAX_TTS_CHARS` if lower) so the reply arrives in time.
- **Same limits as groups:** the caller's own daily allowance, plus the chat's shared `GROUP_DAILY_CHAR_LIMIT` and the all-groups total. `GROUP_MODE=off` turns guest replies off too, and `admins` limits them to `ADMIN_USER_IDS`. `/blockgroup <id>` silences a chat; guest chats appear in `/groups`.
- **How the voice gets there:** a guest reply can only point at a file already on Telegram, so each reading is uploaded silently to a storage chat, used, and deleted again. That chat is `GUEST_STORAGE_CHAT_ID`, or by default the lowest ID in `ADMIN_USER_IDS` (your own chat with the bot; you may glimpse the upload for a moment). A private channel with the bot as admin keeps it out of sight.

## Profiles, invites and stats

### Not wasting anyone's allowance

- **Only words are read and charged.** Links, @usernames and emoji are dropped before reading (otherwise Ellie would spell them out), and only the characters actually spoken count. A message that is only links or emoji gets a short "Nothing to read" instead of a reading.
- **Characters left, after every private reading:** the voice's caption ends with "🎟 1,450 of 2,000 characters left today" (and bonus characters, if any). Group captions never show anyone's allowance.
- **Language tips, never changes:** when a message looks like Hindi typed in English letters ("abhi to bhut time hai"), Ellie suggests, at most once a day and only in the private chat, writing it in Hindi script or setting the language to English. It always reads with the language and style saved in the user's settings.

### Profile and referrals

- **`/profile`** (and 👤 Profile on Home): member since, voice and style, today's allowance with a bar, bonus characters, lifetime readings, and friends invited.
- **`/invite`** gives a personal link (`t.me/<bot>?start=r_<id>`) and a "Send to a friend" button. When someone new starts Ellie through it, both get `REFERRAL_BONUS_CHARS` bonus characters (default 1,000; `0` turns referrals off), and the inviter gets a thank-you message.
- **Bonus characters** are spent only after the daily allowance is used up and never expire. Failed readings give back exactly what they took, bonus included.
- **Abuse limits:** only brand-new users count (someone who has used Ellie before can't be "invited"), nobody can invite themselves, the inviter must have used Ellie's private chat, and an inviter earns at most `REFERRAL_DAILY_LIMIT` rewards a day (default 5; their friends still get theirs).
- **First visit:** a new user sees what Ellie does, what's free each day, one message to try right away, and how to use Ellie in groups. When someone hits the daily limit, the screen offers invites.
- **Guest mode spreads the word:** every guest voice carries a "🎙 Make your own voice notes" button, and strangers who mention Ellie get a short pitch with "🎁 Start Ellie, it's free". Both links are tagged so the stats can count who joins from groups.

### Stats (admins)

`/stats` (or 📊 Stats in `/admin`) shows the last 7 days in UTC:

- active users this week and today
- readings, and readings per active user
- new users, and how many came from groups (guest mode or a group's "private chat" link) or from a friend's invite
- "Start Ellie" invites shown in other chats
- how many users hit the daily limit
- the studio-wide cap used today, how many jobs are waiting, and whether voice work is paused
- a line per day

Stats come from small events (`ellie_events`: user ID, kind, source, a count, a time), never message text, and are pruned after 90 days. Without Postgres they live in memory and start over on restart.

## What it does

| You do | Ellie does | Cartesia endpoint |
|---|---|---|
| Send any text, or `/say …` | Replies with a voice note | `POST /tts/bytes` |
| Send a voice note / audio file / video note | Replies with the transcript | `POST /stt` |
| `/voices [search]`, `/myvoices` | Paged voice library with Use and Preview buttons | `GET /voices` |
| `/voice [id]` | Shows details of, or switches to, a voice | `GET /voices/{id}` |
| `/preview [id]` | Plays the voice's free preview clip (no credits) | `GET /voices/{id}?expand[]=preview_file_url` |
| `/emotion`, `/speed`, `/volume` | Shapes delivery (58 emotions, 0.6–1.5× speed, 0.5–2× volume) | `generation_config` |
| `/model` | Picks the Sonic model | `GET /models?capability=tts` |
| `/lang en-GB` | Sets the language for TTS, STT, and cloning | `language` field |
| `/format` | Voice notes or MP3 files | — |
| `/clone <name>` (as a reply to a clip, or as the clip's caption) | Clones a voice and switches to it | `POST /voices/clone` |
| `/accents [lang]` | Lists accents (🔁 = localizable) | `GET /accents` |
| `/localize <accent> [name]` | Makes the current voice native in another accent | `POST /voices/localize` |
| `/rename <name>`, `/deletevoice [id]` | Manages owned voices (delete asks to confirm) | `PATCH`, `DELETE /voices/{id}` |
| `/dicts`, `/newdict` | Picks or creates custom pronunciations | `/pronunciation-dicts/` |
| `/detect` (as a reply) | Checks audio for a Cartesia watermark (paid plans) | `POST /v1/watermark/detect` |
| `/status` | API health and the live model list | `GET /`, `GET /models` |
| `/usage` | Credit usage over the last 4 weeks (needs `CARTESIA_ADMIN_KEY`) | `GET /usage/credits` |
| `/settings`, `/reset` | Shows or clears your preferences | — |

Creating a dictionary:

```
/newdict Brand words
Cartesia = car-TEE-zhuh
Ellie = EH-lee
```

## API reference

Before any code was written, the full Cartesia API reference was scraped from docs.cartesia.ai: all 104 English API pages for version `2026-08-14`, which is the version the live API reports.

- [docs/CARTESIA_API.md](docs/CARTESIA_API.md) has every one of the **97 REST endpoints and 4 WebSocket channels**: method, path, auth type, parameters, request fields (type, required, enum, default, description), and response fields. It also covers conventions, error codes, and concurrency limits.
- [docs/cartesia-spec.json](docs/cartesia-spec.json) is the merged machine-readable OpenAPI and AsyncAPI fragments (195 schemas).

These endpoint groups are documented but intentionally not wired into the bot, because they don't map to a chat interface:

- Managed Agents: agents, calls, batch calling, phone numbers, providers, tools, metrics, WebRTC/WebSocket sessions.
- Datasets, files, and fine-tunes.
- Organizations and API keys.
- The streaming WebSockets.

`Infill` is implemented in the client (`cartesia.Client.Infill`) but has no command yet.

## Design notes

- **Auth and versioning.** Every request sends `Authorization: Bearer` and `Cartesia-Version: 2026-08-14`. Admin-only routes use the separate admin key. The bot refuses to start if an admin key is put in `CARTESIA_API_KEY`, since the two key types are not interchangeable.
- **Audio formats.** Telegram voice notes are OGG/Opus, which `/stt` and `/voices/clone` accept directly. TTS returns MP3, which Telegram accepts for voice notes, so no ffmpeg is needed. If a user blocks voice notes in their privacy settings, the bot falls back to an audio file.
- **Limits.** `CARTESIA_CONCURRENCY` caps parallel Cartesia calls; set it to your plan's TTS limit. Requests that hit `concurrency_limited` retry with backoff. Uploads are checked against Telegram's 20 MB download cap and Cartesia's 16 MB (clone) and 32 MB (watermark) limits before any download.
- **Errors.** Cartesia's structured errors (`error_code`, `request_id`) become friendly messages that include the request ID for support.
- **Safety.** Preview downloads only send the API key to the Cartesia host. Bot-token URLs are scrubbed from error logs. File settings are written atomically with mode `0600`. Database errors never include the connection password.

## Admin panel

A web panel for studio admins, deployed as its own Render web service next to the bot (`Dockerfile.admin`). It reads the bot's Postgres database and shows:

- **Overview:** whether Ellie is live, today's studio-wide usage as a level meter, voice jobs running and waiting, the last 30 days of readings and active people, where new people came from, and the daily limits in force.
- **Users:** everyone who has used Ellie, searchable by Telegram user ID and sortable, with a page per user (allowance, lifetime totals, referrals, voices made, 30 days of activity).
- **Groups:** every chat Ellie reads in, with today's usage and status.
- **Audit log** and **Sessions** (see below).

Message text is never shown; the panel only sees the counts Ellie already stores. The bot reports its live state (the waiting line, a pause, its limits) every 15 seconds to `ellie_status`, which is how the panel knows whether Ellie is up.

**Signing in.** There are no passwords. In a browser, admins use Telegram's Login Widget; inside Telegram, `/admin` → **Open admin panel** signs them in at once (a Mini App). Either way the server checks Telegram's signature with the bot token, rejects proofs older than 10 minutes, and admits only IDs in `ADMIN_USER_IDS`.

**Security.**

- Sessions are random 256-bit tokens. The database stores only their SHA-256, and a session ends after 12 hours, or 2 hours without activity. Removing someone from `ADMIN_USER_IDS` locks them out at once.
- In a browser the token is a `__Host-` cookie (HttpOnly, Secure, SameSite=Strict). Inside Telegram, where cookies can be blocked, it's an `Authorization` header kept only in page memory.
- Writes signed in by cookie must come from the panel's own origin and carry a custom header, so cross-site forms can't forge them.
- A strict Content-Security-Policy (scripts only from the panel and telegram.org), HSTS, `nosniff`, no referrer, and frames only inside Telegram Web. Sign-in is rate-limited per IP, and request bodies are capped.
- Every sign-in, refused sign-in, sign-out and session revocation goes to the audit log (`ellie_admin_audit`). Admins can see their devices and sign any of them out.

**Setting it up on Render**

1. Create a web service from this repository with **Dockerfile path** `./Dockerfile.admin`, in the same region as the database.
2. Set `DATABASE_URL` (the database's Internal connection string), `TELEGRAM_BOT_TOKEN` and `ADMIN_USER_IDS`. `PUBLIC_URL` defaults to the service's Render URL.
3. In @BotFather, send `/setdomain`, choose the bot and enter the panel's domain (for example `ellie-admin.onrender.com`), so the Login Widget works there.
4. On the bot's service, set `ADMIN_PANEL_URL` to the panel's `https://` address. `/admin` then shows **Open admin panel**.

**Working on the UI locally:** run the panel with `PUBLIC_URL` unset (for example `go run ./cmd/ellie-admin` with a local database), then `cd admin-ui && npm install && npm run dev`, which proxies `/api` to it. `npm run build` writes the UI to `internal/admin/dist`, and `go build -tags embedui ./cmd/ellie-admin` embeds it. Without the tag, the panel serves the API alone.

## Layout

```
cmd/ellie/            the bot's entry point, env/.env config
cmd/ellie-admin/      the admin panel's entry point
internal/cartesia/    Cartesia REST client (TTS, STT, voices, accents, models, infill, watermark, dictionaries, usage)
internal/telegram/    minimal Bot API client (long polling, messages, keyboards, uploads, downloads)
internal/bot/         Ellie Studio commands, buttons, per-user settings (file or Postgres)
internal/admin/       admin panel: Telegram sign-in, sessions, audit log, JSON API
admin-ui/             admin panel UI (React + TypeScript + Vite), embedded into the panel binary
docs/                 scraped Cartesia API reference
```

## Tests

```bash
go test -race ./...
```

The bot tests run the full flow against fake Telegram and Cartesia servers: text → TTS → voice note, voice note → STT → transcript, access control, admin gating, and button handling.

To also test the Postgres backend, point it at a database. The tests apply the migrations, then only touch rows with negative user IDs or `test-` resource IDs, and delete them afterwards:

```bash
TEST_DATABASE_URL=postgres://user:pass@host:5432/db go test ./internal/bot -run 'Postgres|Migrations' 
```

### Continuous integration

Every push and pull request runs [`.github/workflows/ci.yml`](.github/workflows/ci.yml): a gofmt check, `go vet`, and the whole test suite with the race detector, including the Postgres tests against a throwaway Postgres container.
