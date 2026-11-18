# Ellie Studio

A Telegram voice studio powered by [Cartesia](https://cartesia.ai). Send text and get it back as speech. Send a voice note and get a transcript. Browse, clone, and localize voices, all from chat.

Written in Go with **no third-party dependencies**: both the Cartesia client and the Telegram client are standard library only.

## Quick start

```bash
cp .env.example .env
```

Fill in `TELEGRAM_BOT_TOKEN` (from @BotFather) and `CARTESIA_API_KEY` (from play.cartesia.ai/keys). Set `ALLOWED_USER_IDS` too, so strangers can't spend your credits. Then:

```bash
go run ./cmd/ellie
```

Don't know your Telegram ID? Start the bot with `ALLOWED_USER_IDS` set to anything (for example `1`), message it, and it replies with your ID.

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
- **Safety.** Preview downloads only send the API key to the Cartesia host. Bot-token URLs are scrubbed from error logs. Settings are written atomically to `data/settings.json` with mode `0600`.

## Layout

```
cmd/ellie/            entry point, env/.env config
internal/cartesia/    Cartesia REST client (TTS, STT, voices, accents, models, infill, watermark, dictionaries, usage)
internal/telegram/    minimal Bot API client (long polling, messages, keyboards, uploads, downloads)
internal/bot/         Ellie Studio commands, buttons, per-user settings
docs/                 scraped Cartesia API reference
```

## Tests

```bash
go test -race ./...
```

The bot tests run the full flow against fake Telegram and Cartesia servers: text → TTS → voice note, voice note → STT → transcript, access control, admin gating, and button handling.
