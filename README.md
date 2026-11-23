# Ellie Studio

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

The bot's database is one of:

- **Postgres** (production): set `DATABASE_URL`. On Render, use the database's **Internal** connection string. Tables are created and upgraded on startup by versioned migrations (`ellie_schema_migrations`), which are safe when two instances start at once during a deploy.
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
- **What Ellie sees:** only the message that mentions it and the message it replies to. Not the chat's history or members.
- **One reply per mention:** the voice, or a short notice (limit reached, text too long, pick a voice first) with a link to the private chat. Texts are capped at 500 characters (or `MAX_TTS_CHARS` if lower) so the reply arrives in time.
- **Same limits as groups:** the caller's own daily allowance, plus the chat's shared `GROUP_DAILY_CHAR_LIMIT` and the all-groups total. `GROUP_MODE=off` turns guest replies off too, and `admins` limits them to `ADMIN_USER_IDS`. `/blockgroup <id>` silences a chat; guest chats appear in `/groups`.
- **How the voice gets there:** a guest reply can only point at a file already on Telegram, so each reading is uploaded silently to a storage chat, used, and deleted again. That chat is `GUEST_STORAGE_CHAT_ID`, or by default the lowest ID in `ADMIN_USER_IDS` (your own chat with the bot; you may glimpse the upload for a moment). A private channel with the bot as admin keeps it out of sight.

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

## Layout

```
cmd/ellie/            entry point, env/.env config
internal/cartesia/    Cartesia REST client (TTS, STT, voices, accents, models, infill, watermark, dictionaries, usage)
internal/telegram/    minimal Bot API client (long polling, messages, keyboards, uploads, downloads)
internal/bot/         Ellie Studio commands, buttons, per-user settings (file or Postgres)
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
