# Cartesia API — Full Endpoint Reference

Scraped from docs.cartesia.ai (API version `2026-08-14`, the current latest). Generated from the OpenAPI/AsyncAPI fragments embedded in each of the 104 English API-reference pages. The machine-readable merge lives next to this file in `cartesia-spec.json`.

## Conventions (applies to every endpoint)

- **Base URL:** `https://api.cartesia.ai` (`wss://api.cartesia.ai` for WebSockets). HTTPS only.
- **Required header:** `Cartesia-Version: 2026-08-14` (WebSockets may use `?cartesia_version=`).
- **Auth:** `Authorization: Bearer sk_car_...`. Admin-only routes (usage, api-keys, organizations) need an admin key `sk_car_admin_...`; the two key types are not interchangeable. Client apps should use short-lived access tokens from `POST /access-token` (WebSockets: `?access_token=`).
- **GET** passes data as query params; **POST/PATCH** use JSON or `multipart/form-data`.
- **Errors** (versions >= 2026-03-01) are JSON: `{error_code, title, message, request_id, doc_url?}`. Known `error_code`s: `quota_exceeded`, `concurrency_limited`, `voice_model_mismatch`, `voice_not_found`, `model_not_found`, `language_not_supported`, `file_too_large`, `unsupported_audio_format`, `plan_upgrade_required`. WS/SSE error events add `type:"error"`, `done`, `status_code`, `context_id`.
- **Concurrency:** TTS and STT have separate limits — Free 2/8, Pro 3/12, Startup 5/20, Scale 15/60.


## Index (97 REST endpoints, 4 WebSocket channels)

| Group | Method | Path | Summary |
|---|---|---|---|
| status | GET | `/` | API Status and Version |
| models | GET | `/models` | List Models |
| tts | POST | `/tts/bytes` | Text-to-Speech (Bytes) |
| tts | POST | `/tts/sse` | Text-to-Speech (SSE) |
| stt | POST | `/stt` | Batch Speech-to-Text |
| infill | POST | `/infill/bytes` | Infill (Bytes) |
| watermark | POST | `/v1/watermark/detect` | Detect watermark |
| voices | GET | `/voices` | List Voices |
| voices | POST | `/voices/clone` | Clone Voice |
| voices | POST | `/voices/localize` | Localize Voice |
| voices | DELETE | `/voices/{id}` | Delete Voice |
| voices | GET | `/voices/{id}` | Get Voice |
| voices | PATCH | `/voices/{id}` | Update Voice |
| voices | PATCH | `/voices/{id}/accents` | Add Voice Accents |
| voices | DELETE | `/voices/{id}/accents/{accent_id}` | Delete Voice Accent |
| accents | GET | `/accents` | List Accents |
| pronunciation-dicts | GET | `/pronunciation-dicts/` |  |
| pronunciation-dicts | POST | `/pronunciation-dicts/` |  |
| pronunciation-dicts | DELETE | `/pronunciation-dicts/{id}` |  |
| pronunciation-dicts | GET | `/pronunciation-dicts/{id}` |  |
| pronunciation-dicts | PATCH | `/pronunciation-dicts/{id}` |  |
| datasets | GET | `/datasets/` |  |
| datasets | POST | `/datasets/` |  |
| datasets | DELETE | `/datasets/{id}` |  |
| datasets | GET | `/datasets/{id}` |  |
| datasets | PATCH | `/datasets/{id}` |  |
| datasets | GET | `/datasets/{id}/files` |  |
| datasets | POST | `/datasets/{id}/files` |  |
| datasets | DELETE | `/datasets/{id}/files/{fileID}` |  |
| files | GET | `/files` |  |
| files | POST | `/files` |  |
| files | DELETE | `/files/{id}` |  |
| files | GET | `/files/{id}/download` |  |
| files | GET | `/files/{id}/info` |  |
| fine-tunes | GET | `/fine-tunes/` |  |
| fine-tunes | POST | `/fine-tunes/` |  |
| fine-tunes | DELETE | `/fine-tunes/{id}` |  |
| fine-tunes | GET | `/fine-tunes/{id}` |  |
| fine-tunes | GET | `/fine-tunes/{id}/voices` |  |
| access-token | POST | `/access-token` | Generate a New Access Token |
| usage | GET | `/usage/agents` | Get Agent Usage |
| usage | GET | `/usage/credits` | Get Credit Usage |
| api-keys | GET | `/api-keys` | List API Keys |
| api-keys | GET | `/api-keys/{id}` | Get API Key |
| organizations | GET | `/organizations/invites` | List Invites |
| organizations | POST | `/organizations/invites` | Create Invites |
| organizations | DELETE | `/organizations/invites/{id}` | Cancel Invite |
| organizations | GET | `/organizations/invites/{id}` | Get Invite |
| organizations | GET | `/organizations/users` | List Users |
| organizations | DELETE | `/organizations/users/{id}` | Remove User |
| organizations | GET | `/organizations/users/{id}` | Get User |
| agents | GET | `/agents/calls` | List Calls |
| agents | POST | `/agents/calls` | Create Outbound Call |
| agents | GET | `/agents/calls/batches` | List Call Batches |
| agents | POST | `/agents/calls/batches` | Create Call Batch |
| agents | GET | `/agents/calls/batches/{batch_id}` | Get Call Batch |
| agents | POST | `/agents/calls/batches/{batch_id}/cancel` | Cancel Call Batch |
| agents | POST | `/agents/calls/batches/{batch_id}/retry` | Retry Call Batch |
| agents | DELETE | `/agents/calls/{call_id}` | Delete Call |
| agents | GET | `/agents/calls/{call_id}` | Get Call |
| agents | GET | `/agents/calls/{call_id}/audio` | Download Call Audio |
| agents | POST | `/agents/calls/{call_id}/cancel` | Cancel Call |
| agents | GET | `/agents/metrics` | List Metrics |
| agents | POST | `/agents/metrics` | Create Metric |
| agents | GET | `/agents/metrics/results` | List Metric Results |
| agents | GET | `/agents/metrics/results/export` | Export Metric Results |
| agents | DELETE | `/agents/metrics/{metric_id}` | Delete Metric |
| agents | GET | `/agents/metrics/{metric_id}` | Get Metric |
| agents | GET | `/agents/phone-numbers` | List Phone Numbers |
| agents | POST | `/agents/phone-numbers` | Import Phone Number |
| agents | GET | `/agents/phone-numbers/providers` | List Providers |
| agents | POST | `/agents/phone-numbers/providers` | Create Provider |
| agents | DELETE | `/agents/phone-numbers/providers/{id}` | Delete Provider |
| agents | GET | `/agents/phone-numbers/providers/{id}` | Get Provider |
| agents | PATCH | `/agents/phone-numbers/providers/{id}` | Update Provider |
| agents | GET | `/agents/phone-numbers/provision` | Get Phone Number Provisioning Usage |
| agents | POST | `/agents/phone-numbers/provision` | Provision Cartesia Number |
| agents | DELETE | `/agents/phone-numbers/{id}` | Delete Phone Number |
| agents | GET | `/agents/phone-numbers/{id}` | Get Phone Number |
| agents | PATCH | `/agents/phone-numbers/{id}` | Update Phone Number |
| agents | DELETE | `/agents/{agent_id}/metrics/{metric_id}` | Remove Metric from Agent |
| agents | POST | `/agents/{agent_id}/metrics/{metric_id}` | Add Metric to Agent |
| agents | GET | `/v1/agents` | List agents |
| agents | POST | `/v1/agents` | Create an agent |
| agents | GET | `/v1/agents/models` | List agent models |
| agents | GET | `/v1/agents/tools` | List tools |
| agents | POST | `/v1/agents/tools` | Create a tool |
| agents | DELETE | `/v1/agents/tools/{tool_id}` | Delete a tool |
| agents | GET | `/v1/agents/tools/{tool_id}` | Get a tool |
| agents | PATCH | `/v1/agents/tools/{tool_id}` | Update a tool |
| agents | POST | `/v1/agents/webrtc/{agent_id}/sessions` | Create a WebRTC session |
| agents | POST | `/v1/agents/websocket/{agent_id}/sessions` | Create a WebSocket session |
| agents | DELETE | `/v1/agents/{agent_id}` | Delete an agent |
| agents | GET | `/v1/agents/{agent_id}` | Get an agent |
| agents | PATCH | `/v1/agents/{agent_id}` | Update an agent |
| agents | GET | `/v1/agents/{agent_id}/versions` | List agent versions |
| agents | GET | `/v1/agents/{agent_id}/versions/{version_id}` | Get an agent version |


---

## status

### `GET /` — API Status and Version

- **Auth:** none  ·  **operationId:** `apiStatus_get`  ·  **docs page:** `api-reference/api-status/get.md`

**Responses**

- `200` `application/json` → APIInfo

| Field | Type | Req | Description |
|---|---|---|---|
| `ok` | boolean | yes |  |
| `version` | string<date> | yes | Latest API version |



---

## models

### `GET /models` — List Models

List callable Sonic (TTS) and Ink (STT) model ids.

- **Auth:** APIKeyAuth  ·  **operationId:** `models_list`  ·  **docs page:** `api-reference/models/list.md`

**Parameters**

| Name | In | Type | Req | Description |
|---|---|---|---|---|
| `capability` | query | string enum(`tts`, `stt`) |  | Filter to `tts` or `stt`. Omit to return both. |

**Responses**

- `200` `application/json` → ListModelsResponse

| Field | Type | Req | Description |
|---|---|---|---|
| `data` | ModelCatalogEntry[] | yes | Callable public model ids for TTS and STT. |
| `data[].id` | string | yes | Value clients send as TTS `model_id` or STT `model`. |
| `data[].capability` | string enum(`tts`, `stt`) | yes | Which speech API family the model belongs to. |
| `data[].modes` | string enum(`batch`, `stream`)[] |  | STT transport paths. Omitted on TTS rows. |
| `data[].supported_languages` | string[] |  | Sorted synthesis language codes supported by this TTS model. Omitted on STT rows. |



---

## tts

### `POST /tts/bytes` — Text-to-Speech (Bytes)

Stream audio from a complete transcript

- **Auth:** APIKeyAuth, AccessTokenAuth  ·  **operationId:** `tts_bytes`  ·  **docs page:** `api-reference/tts/bytes.md`

**Request body** (`application/json`; type TTSRequest)

| Field | Type | Req | Description |
|---|---|---|---|
| `model_id` | string enum(`sonic-3.6`, `sonic-3.5`, `sonic-3`, `sonic-latest`) | yes | The ID of the model to use for the generation. See [Models] all options. (default `"sonic-3.6"`) |
| `transcript` | string | yes |  (default `"Hi there, it's awesome to meet you."`) |
| `voice` | string \| TTSRequestVoiceObject | yes | The voice to use for generation. Pass either a voice ID string or an object with a required `id` (additional object fields may be added in future API versions)… |
| `language` | string |  | The transcript's language or locale (for example `en` or `en-GB`). `language` and `locale` accept the same values. Set one or the other, never both; setting bo… |
| `locale` | string |  | The transcript's language or locale (for example `en` or `en-GB`). `locale` and `language` accept the same values. Set one or the other, never both; setting bo… |
| `accent` | string |  | Usually unnecessary: Cartesia picks the closest accent the voice supports for the requested `language` or `locale`. Set it only to make a [multilingual voice] … |
| `normalization` | string |  | Text normalization. `auto` (default) runs the locale-aware normalizer, `off` skips it, or pass a language or locale code (for example `en` or `en-IN`) to pin t… (default `"auto"`) |
| `output_format` | WAVOutputFormat \| MP3OutputFormat \| RAWOutputFormat | yes |  |
| `pronunciation_dict_id` | string |  | The ID of a pronunciation dictionary to use for the generation. Pronunciation dictionaries are supported by `sonic-3` models and newer. |
| `generation_config` | GenerationConfig |  | Configure the various attributes of the generated speech. Available on `sonic-3` and newer models; not available on earlier models. See [Volume, Speed, and Emo… |
| `generation_config.volume` | number<double> |  | Adjust the volume of the generated speech between 0.5x and 2.0x the default volume. Valid values are between [0.5, 2.0] inclusive. (default `1`) |
| `generation_config.speed` | number<double> |  | Adjust the speed of the generated speech between 0.6x and 1.5x the default speed. Valid values are between [0.6, 1.5] inclusive. (default `1`) |
| `generation_config.emotion` | string enum(`neutral`, `happy`, `excited`, `enthusiastic`, `elated`, `euphoric`, `triumphant`, `amazed`, `surprised`, `flirtatious`, `curious`, `content` …(+46)) |  | The primary emotions are `neutral`, `calm`, `angry`, `content`, `sad`, `scared`. For more options, see [Volume, Speed, and Emotion]. |

**Responses**

- `200` `audio/*` → string<binary>

### `POST /tts/sse` — Text-to-Speech (SSE)

Stream audio with extra metadata from a complete transcript

- **Auth:** APIKeyAuth, AccessTokenAuth  ·  **operationId:** `tts_sse`  ·  **docs page:** `api-reference/tts/sse.md`

**Request body** (`application/json`; type TTSSSERequest)

| Field | Type | Req | Description |
|---|---|---|---|
| `model_id` | string enum(`sonic-3.6`, `sonic-3.5`, `sonic-3`, `sonic-latest`) | yes | The ID of the model to use for the generation. See [Models] all options. (default `"sonic-3.6"`) |
| `transcript` | string | yes |  (default `"Hi there, it's awesome to meet you."`) |
| `voice` | string \| TTSRequestVoiceObject | yes | The voice to use for generation. Pass either a voice ID string or an object with a required `id` (additional object fields may be added in future API versions)… |
| `output_format` | SSEOutputFormat | yes |  |
| `output_format.container` | string enum(`raw`) | yes |  (default `"raw"`) |
| `output_format.encoding` | string enum(`pcm_f32le`, `pcm_s16le`, `pcm_mulaw`, `pcm_alaw`) | yes | The encoding format for output audio. See [TTS Output Audio Format] if you're unsure what to use. |
| `output_format.sample_rate` | integer enum(`8000`, `16000`, `22050`, `24000`, `44100`, `48000`) | yes |  |
| `language` | string |  | The transcript's language or locale (for example `en` or `en-GB`). `language` and `locale` accept the same values. Set one or the other, never both; setting bo… |
| `locale` | string |  | The transcript's language or locale (for example `en` or `en-GB`). `locale` and `language` accept the same values. Set one or the other, never both; setting bo… |
| `accent` | string |  | Usually unnecessary: Cartesia picks the closest accent the voice supports for the requested `language` or `locale`. Set it only to make a [multilingual voice] … |
| `normalization` | string |  | Text normalization. `auto` (default) runs the locale-aware normalizer, `off` skips it, or pass a language or locale code (for example `en` or `en-IN`) to pin t… (default `"auto"`) |
| `add_timestamps` | boolean |  | Whether to return word-level timestamps. If `false` (default), no word timestamps will be produced at all. If `true`, the server will return timestamp events c… (default `false`) |
| `add_phoneme_timestamps` | boolean |  | Whether to return phoneme-level timestamps. If `false` (default), no phoneme timestamps will be produced. If `true`, the server will return timestamp events co… (default `false`) |
| `use_normalized_timestamps` | boolean |  | Whether to use normalized timestamps (True) or original timestamps (False). |
| `pronunciation_dict_id` | string |  | The ID of a pronunciation dictionary to use for the generation. Pronunciation dictionaries are supported by `sonic-3` models and newer. |
| `generation_config` | GenerationConfig |  | Configure the various attributes of the generated speech. Available on `sonic-3` and newer models; not available on earlier models. See [Volume, Speed, and Emo… |
| `generation_config.volume` | number<double> |  | Adjust the volume of the generated speech between 0.5x and 2.0x the default volume. Valid values are between [0.5, 2.0] inclusive. (default `1`) |
| `generation_config.speed` | number<double> |  | Adjust the speed of the generated speech between 0.6x and 1.5x the default speed. Valid values are between [0.6, 1.5] inclusive. (default `1`) |
| `generation_config.emotion` | string enum(`neutral`, `happy`, `excited`, `enthusiastic`, `elated`, `euphoric`, `triumphant`, `amazed`, `surprised`, `flirtatious`, `curious`, `content` …(+46)) |  | The primary emotions are `neutral`, `calm`, `angry`, `content`, `sad`, `scared`. For more options, see [Volume, Speed, and Emotion]. |
| `context_id` | string |  |  |

**Responses**

- `200` `text/event-stream` → TTSSSEChunkEvent \| TTSSSETimestampsEvent \| TTSSSEPhonemeTimestampsEvent \| TTSSSEDoneEvent \| TTSSSEErrorEvent


---

## stt

### `POST /stt` — Batch Speech-to-Text

Transcribes an audio file of any length

- **Auth:** APIKeyAuth, AccessTokenAuth  ·  **operationId:** `stt_transcribe`  ·  **docs page:** `api-reference/stt/transcribe.md`

**Parameters**

| Name | In | Type | Req | Description |
|---|---|---|---|---|
| `encoding` | query | string enum(`pcm_s16le`, `pcm_s32le`, `pcm_f16le`, `pcm_f32le`, `pcm_mulaw`, `pcm_alaw`) |  | Required when you upload raw PCM data without a container header. If you leave it out, the server decodes the audio automatically from its container, such as W… |
| `sample_rate` | query | integer |  | The sample rate of the audio in Hz, from 8000 to 48000 |

**Request body** (`multipart/form-data`; type object)

| Field | Type | Req | Description |
|---|---|---|---|
| `file` | string<binary> | yes | You don't need to split your audio file. The server splits long files into chunks for you. Supported audio formats: `flac`, `m4a`, `mp3`, `mp4`, `mpeg`, `mpga`… |
| `model` | string enum(`ink-whisper`) | yes | ID of the model to use for transcription. Must be in the `ink-whisper` family of models. |
| `language` | string enum(`en`, `zh`, `de`, `es`, `ru`, `ko`, `fr`, `ja`, `pt`, `tr`, `pl`, `ca` …(+88)) |  | The language code of the input audio. If you leave it out, it defaults to `en`, and speech in other languages can come back translated into English. (default `"en"`) |
| `timestamp_granularities[]` | string enum(`word`)[] |  |  |

**Responses**

- `200` `application/json` → TranscriptionResponse

| Field | Type | Req | Description |
|---|---|---|---|
| `type` | string enum(`transcript`) | yes | The message type. Always `transcript` for a batch transcription response. |
| `request_id` | string |  | Unique identifier for this transcription request |
| `text` | string | yes | The transcribed text. A batch response is always the complete transcription. |
| `language` | string |  | The specified language of the input audio |
| `duration` | number<double> |  | The duration of the input audio in seconds |
| `words` | TranscriptionWord[] |  | The start and end time of each word. Included only when you pass `[word]` in `timestamp_granularities[]`. |
| `words[].word` | string | yes | The transcribed word |
| `words[].start` | number<double> | yes | Start time of the word in seconds |
| `words[].end` | number<double> | yes | End time of the word in seconds |



---

## infill

### `POST /infill/bytes` — Infill (Bytes)

Generate audio that smoothly connects two existing audio segments

- **Auth:** APIKeyAuth  ·  **operationId:** `infill_bytes`  ·  **docs page:** `api-reference/infill/bytes.md`

**Request body** (`multipart/form-data`; type object)

| Field | Type | Req | Description |
|---|---|---|---|
| `left_audio` | string<binary> |  | Audio clip that comes before the infill transcript: `left_audio` -> `transcript` -> `right_audio` For best results, target natural pauses in the audio and clip… |
| `right_audio` | string<binary> |  | Audio clip that comes after the infill transcript: `left_audio` -> `transcript` -> `right_audio` For best results, target natural pauses in the audio and clip … |
| `model_id` | string enum(`sonic-3`, `sonic-3-2026-01-12`) |  | The ID of the model to use for generating audio |
| `language` | string enum(`en`, `fr`, `de`, `es`, `pt`, `zh`, `ja`, `hi`, `it`, `ko`, `nl`, `pl` …(+32)) |  | The language that the given voice should speak the transcript in. This may depend on the model you're using. See [Models] for details. |
| `transcript` | string |  | The infill text to generate. For best results, use longer transcripts to give the model more flexibility to adapt to the rest of the audio. |
| `voice_id` | string |  | The ID of the voice to use for generating audio |
| `output_format[container]` | string enum(`raw`, `wav`, `mp3`) |  |  |
| `output_format[sample_rate]` | integer enum(`8000`, `16000`, `22050`, `24000`, `44100`, `48000`) |  | The sample rate of the output audio |
| `output_format[encoding]` | string enum(`pcm_f32le`, `pcm_s16le`, `pcm_mulaw`, `pcm_alaw`) |  | The encoding format for output audio. See [TTS Output Audio Format] if you're unsure what to use. |
| `output_format[bit_rate]` | integer |  | Required for `mp3` containers. |

**Responses**

- `200` `audio/*` → string<binary>


---

## watermark

### `POST /v1/watermark/detect` — Detect watermark

Check audio for a Cartesia watermark. Requires a paid plan.

- **Auth:** APIKeyAuth  ·  **operationId:** `detectWatermark`  ·  **docs page:** `api-reference/watermarking/detect.md`

**Request body** (`multipart/form-data`; type object)

| Field | Type | Req | Description |
|---|---|---|---|
| `file` | string<binary> | yes | Audio file to check, between 5 seconds and 5 minutes long, up to 32 MiB. Supported formats are FLAC, M4A, MP3, MP4, MPEG, MPGA, OGA, OGG, WAV, and WebM. |

**Responses**

- `200` `application/json` → object

| Field | Type | Req | Description |
|---|---|---|---|
| `detected` | boolean | yes | Whether a Cartesia watermark was detected in the audio |



---

## voices

### `GET /voices` — List Voices

- **Auth:** APIKeyAuth  ·  **operationId:** `voices_list`  ·  **docs page:** `api-reference/voices/list.md`

**Parameters**

| Name | In | Type | Req | Description |
|---|---|---|---|---|
| `limit` | query | integer |  | The number of Voices to return per page, ranging between 1 and 100. |
| `starting_after` | query | string |  | A cursor to use in pagination. `starting_after` is a Voice ID that defines your place in the list. For example, if you make a /voices request and receive 100 o… |
| `ending_before` | query | string |  | A cursor to use in pagination. `ending_before` is a Voice ID that defines your place in the list. For example, if you make a /voices request and receive 100 ob… |
| `q` | query | string |  | Query string to search for voices by name, description, or Voice ID. |
| `is_owner` | query | boolean |  | Whether to only return voices owned your organization. |
| `gender` | query | string enum(`masculine`, `feminine`, `gender_neutral`) |  | The gender presentation of the voices to return. |
| `language` | query | string |  | Filter voices by a language or language-locale pair such as `en` or `en-GB`. A language-locale pair returns accents for that specific locale; a language alone … |
| `include_archived` | query | boolean |  | Defaults to `false`. Set to `true` to include archived voices in the response. |
| `expand[]` | query | string enum(`preview_file_url`)[] |  | Additional fields to include in the response. |

**Responses**

- `200` `application/json` → GetVoicesResponse

| Field | Type | Req | Description |
|---|---|---|---|
| `data` | Voice[] | yes | The paginated list of voices. |
| `data[].id` | string | yes | The ID of the voice. Find one in the [Voice Library] or via [List Voices]. |
| `data[].is_owner` | boolean | yes | Whether your organization owns the voice. |
| `data[].status` | string enum(`active`, `archived`) | yes | Status of the voice. Only active voices are returned in [List Voices] by default unless `include_archived=true`. |
| `data[].access` | string enum(`private`, `public`) | yes | Who can use the resource. `private` means only the owner can use the resource. `public` means everyone can use the resource. |
| `data[].name` | string | yes | The display name of the voice. Does not include the tagline. |
| `data[].tagline` | string | yes | A short descriptor for the voice (at most 32 characters). Empty string when unset. (default `""`) |
| `data[].description` | string | yes | A description for the voice, typically longer than the tagline if both are provided. |
| `data[].gender` | string enum(`masculine`, `feminine`, `gender_neutral`) |  |  |
| `data[].created_at` | string<date-time> | yes | The date and time the voice was created. |
| `data[].preview_file_url` | string |  | A URL to download a preview audio file for this voice. Useful to avoid consuming credits when looking for the right voice. The URL requires the same Authorizat… |
| `data[].language` | string | yes | The voice's language, as an ISO 639-1 code (e.g. `en`, `fr`, `zh`) |
| `data[].accents` | VoiceAccentsItem[] |  | Accents the voice sounds native in, with the original accent marked `is_native: true`. In any other [supported language], the voice speaks with that original a… |
| `data[].accents[].accent` | string enum(`emirati-arabic`, `khaleeji`, `middle-eastern-arabic`, `modern-standard-arabic`) \| string enum(`indian-bengali`, `jessore`) \| string enum(`bulgarian`) \| string enum(`mandarin`, `taiwanese-mandarin`) \| string enum(`zagreb`) \| string enum(`czech`) \| string enum(`danish`) \| string enum(`belgian-dutch`, `randstad`) \| string enum(`african-american`, `arabic-english`, `australian`, `british`, `california`, `canadian-english`, `general-american`, `indian-english`, `irish`, `midwestern-american`, `new-york`, `new-zealand` …(+5)) \| string enum(`finnish`) \| string enum(`african-french`, `belgian-french`, `canadian-french`, `parisian`) \| string enum(`tbilisi`) \| string enum(`austrian-german`, `standard-german`, `swiss-standard`) \| string enum(`thessaloniki`) \| string enum(`parsi`) \| string enum(`israeli`) \| string enum(`bagheli`, `standard-hindi`) \| string enum(`budapest`) \| string enum(`standard-indonesian`) \| string enum(`abruzzo-italian`, `campania`, `standard-italian`) \| string enum(`standard-japanese`) \| string enum(`southern-karnataka`) \| string enum(`korean`) \| string enum(`malay`) \| string enum(`north-kerala`) \| string enum(`konkani`) \| string enum(`oslo`) \| string enum(`odia`) \| string enum(`polish`) \| string enum(`brazilian-portuguese`, `european-portuguese`) \| string enum(`powadhi`) \| string enum(`moldovan`, `romanian`) \| string enum(`russian`) \| string enum(`slovak`) \| string enum(`argentinian`, `camba`, `castilian`, `chilean`, `colombian`, `dominican`, `mexican`, `panamanian`, `peruvian`, `puerto-rican`, `united-states-spanish`) \| string enum(`stockholm`) \| string enum(`manila`) \| string enum(`central-tamil`) \| string enum(`telangana`) \| string enum(`central-thai`) \| string enum(`istanbul`) \| string enum(`kyiv`) \| string enum(`indian-urdu`, `urdu`) \| string enum(`central-vietnamese`, `southern-vietnamese`) | yes | Accent id from [GET /accents] (e.g. `southern-us`). |
| `data[].accents[].locale` | string | yes | The accent's BCP-47 language-region tag (e.g. `en-US`). |
| `data[].accents[].is_native` | boolean | yes | Whether this is the accent the voice was originally recorded in. It's the accent the voice falls back to when it speaks a language it has no matching accent fo… |
| `data[].country` | string |  | The country associated with the voice, as an ISO 3166-1 alpha-2 code when available (e.g. `US`, `GB`, `FR`). |
| `data[].is_pro` | boolean |  | Whether this voice is a pro voice clone. |
| `data[].visibility` | string enum(`owner`, `all`) | yes | When the resource is returned by the list endpoint. `owner` means the resource appears for the owner only. `all` means the resource appears for all users. |
| `has_more` | boolean | yes | Whether there are more voices to fetch. |
| `next_page` | string |  | The ID of the voice. Find one in the [Voice Library] or via [List Voices]. |


### `POST /voices/clone` — Clone Voice

- **Auth:** APIKeyAuth  ·  **operationId:** `voices_clone`  ·  **docs page:** `api-reference/voices/clone.md`

**Request body** (`multipart/form-data`; type object)

| Field | Type | Req | Description |
|---|---|---|---|
| `clip` | string<binary> | yes | See [Clone Voices] for guidance on choosing a clip. Maximum upload size: `16 MB`. Supported audio formats: `flac`, `mp3`, `mpeg`, `mpga`, `oga`, `ogg`, `wav`, … |
| `name` | string | yes | The name of the voice. |
| `tagline` | string |  | A few words describing the voice. At most 32 characters. (default `""`) |
| `description` | string |  | A description for the voice, typically longer than the tagline if both are provided. |
| `language` | string enum(`en`, `fr`, `de`, `es`, `pt`, `zh`, `ja`, `hi`, `it`, `ko`, `nl`, `pl` …(+32)) | yes | The language that the given voice should speak the transcript in. This may depend on the model you're using. See [Models] for details. |
| `accent` | string enum(`emirati-arabic`, `khaleeji`, `middle-eastern-arabic`, `modern-standard-arabic`) \| string enum(`indian-bengali`, `jessore`) \| string enum(`bulgarian`) \| string enum(`mandarin`, `taiwanese-mandarin`) \| string enum(`zagreb`) \| string enum(`czech`) \| string enum(`danish`) \| string enum(`belgian-dutch`, `randstad`) \| string enum(`african-american`, `arabic-english`, `australian`, `british`, `california`, `canadian-english`, `general-american`, `indian-english`, `irish`, `midwestern-american`, `new-york`, `new-zealand` …(+5)) \| string enum(`finnish`) \| string enum(`african-french`, `belgian-french`, `canadian-french`, `parisian`) \| string enum(`tbilisi`) \| string enum(`austrian-german`, `standard-german`, `swiss-standard`) \| string enum(`thessaloniki`) \| string enum(`parsi`) \| string enum(`israeli`) \| string enum(`bagheli`, `standard-hindi`) \| string enum(`budapest`) \| string enum(`standard-indonesian`) \| string enum(`abruzzo-italian`, `campania`, `standard-italian`) \| string enum(`standard-japanese`) \| string enum(`southern-karnataka`) \| string enum(`korean`) \| string enum(`malay`) \| string enum(`north-kerala`) \| string enum(`konkani`) \| string enum(`oslo`) \| string enum(`odia`) \| string enum(`polish`) \| string enum(`brazilian-portuguese`, `european-portuguese`) \| string enum(`powadhi`) \| string enum(`moldovan`, `romanian`) \| string enum(`russian`) \| string enum(`slovak`) \| string enum(`argentinian`, `camba`, `castilian`, `chilean`, `colombian`, `dominican`, `mexican`, `panamanian`, `peruvian`, `puerto-rican`, `united-states-spanish`) \| string enum(`stockholm`) \| string enum(`manila`) \| string enum(`central-tamil`) \| string enum(`telangana`) \| string enum(`central-thai`) \| string enum(`istanbul`) \| string enum(`kyiv`) \| string enum(`indian-urdu`, `urdu`) \| string enum(`central-vietnamese`, `southern-vietnamese`) |  | Metadata for labeling the accent of the voice you're cloning, and a prerequisite for [Add Voice Accents]. See [GET /accents] for values valid for each language. |
| `access` | string enum(`private`, `public`) |  | Who can use the cloned voice. Prefer `"public"` \| `"private"`. Deprecated form field `access[type]` is still accepted. (default `"private"`) |

**Responses**

- `200` `application/json` → VoiceMetadata

| Field | Type | Req | Description |
|---|---|---|---|
| `id` | string | yes | The ID of the newly created voice. |
| `access` | string enum(`private`, `public`) | yes | Who can use the resource. `private` means only the owner can use the resource. `public` means everyone can use the resource. |
| `name` | string | yes | The name of the voice. |
| `tagline` | string | yes | A few words describing the voice. |
| `description` | string | yes | A description for the voice, typically longer than the tagline if both are provided. |
| `created_at` | string<date-time> | yes | The date and time the voice was created. |
| `language` | string | yes | The voice's language, as an ISO 639-1 code (e.g. `en`, `fr`, `zh`) |
| `user_id` | string | yes | The ID of the user who owns the voice. |
| `visibility` | string enum(`owner`, `all`) | yes | When the resource is returned by the list endpoint. `owner` means the resource appears for the owner only. `all` means the resource appears for all users. |


### `POST /voices/localize` — Localize Voice

Localize a voice to sound native in another accent. This creates a new voice with its own voice ID. The voice usually still sounds like the same speaker, though how closely it matches varies by voice and accent. Localizing a Pro Voice Clone creates an Instant Voice Clone, not a new Pro Voice Clone.

- **Auth:** APIKeyAuth  ·  **operationId:** `voices_localize`  ·  **docs page:** `api-reference/voices/localize.md`

**Request body** (`application/json`; type LocalizeVoiceRequest)

| Field | Type | Req | Description |
|---|---|---|---|
| `voice_id` | string | yes | ID of the existing voice you want to localize. |
| `name` | string | yes | Name for the newly localized voice. |
| `description` | string |  | Description for the newly localized voice. |
| `tagline` | string |  | Short tagline for the newly localized voice. |
| `accent` | string enum(`modern-standard-arabic`) \| string enum(`bulgarian`) \| string enum(`mandarin`, `taiwanese-mandarin`) \| string enum(`czech`) \| string enum(`danish`) \| string enum(`randstad`) \| string enum(`australian`, `british`, `general-american`, `indian-english`, `southern-us`) \| string enum(`canadian-french`, `parisian`) \| string enum(`standard-german`) \| string enum(`parsi`) \| string enum(`israeli`) \| string enum(`standard-hindi`) \| string enum(`standard-indonesian`) \| string enum(`standard-italian`) \| string enum(`standard-japanese`) \| string enum(`southern-karnataka`) \| string enum(`korean`) \| string enum(`north-kerala`) \| string enum(`konkani`) \| string enum(`oslo`) \| string enum(`odia`) \| string enum(`polish`) \| string enum(`brazilian-portuguese`, `european-portuguese`) \| string enum(`powadhi`) \| string enum(`russian`) \| string enum(`slovak`) \| string enum(`castilian`, `mexican`) \| string enum(`stockholm`) \| string enum(`manila`) \| string enum(`central-tamil`) \| string enum(`telangana`) \| string enum(`central-thai`) \| string enum(`istanbul`) \| string enum(`indian-urdu`) | yes | The accent to localize into. Get ids from [GET /accents] with `is_localizable=true`. |
| `access` | string enum(`private`, `public`) \| object |  | Controls who can access this resource. Prefer `"public"` \| `"private"`. Deprecated nested `{ "type": "public" \| "private" }` is still accepted. |

**Responses**

- `200` `application/json` → VoiceMetadata

| Field | Type | Req | Description |
|---|---|---|---|
| `id` | string | yes | The ID of the newly created voice. |
| `access` | string enum(`private`, `public`) | yes | Who can use the resource. `private` means only the owner can use the resource. `public` means everyone can use the resource. |
| `name` | string | yes | The name of the voice. |
| `tagline` | string | yes | A few words describing the voice. |
| `description` | string | yes | A description for the voice, typically longer than the tagline if both are provided. |
| `created_at` | string<date-time> | yes | The date and time the voice was created. |
| `language` | string | yes | The voice's language, as an ISO 639-1 code (e.g. `en`, `fr`, `zh`) |
| `user_id` | string | yes | The ID of the user who owns the voice. |
| `visibility` | string enum(`owner`, `all`) | yes | When the resource is returned by the list endpoint. `owner` means the resource appears for the owner only. `all` means the resource appears for all users. |


### `DELETE /voices/{id}` — Delete Voice

- **Auth:** APIKeyAuth  ·  **operationId:** `voices_delete`  ·  **docs page:** `api-reference/voices/delete.md`

**Parameters**

| Name | In | Type | Req | Description |
|---|---|---|---|---|
| `id` | path | string | yes |  |

**Responses**

- `204` 

### `GET /voices/{id}` — Get Voice

- **Auth:** APIKeyAuth  ·  **operationId:** `voices_get`  ·  **docs page:** `api-reference/voices/get.md`

**Parameters**

| Name | In | Type | Req | Description |
|---|---|---|---|---|
| `id` | path | string | yes |  |
| `expand[]` | query | string enum(`preview_file_url`)[] |  | Additional fields to include in the response. |

**Responses**

- `200` `application/json` → Voice

| Field | Type | Req | Description |
|---|---|---|---|
| `id` | string | yes | The ID of the voice. Find one in the [Voice Library] or via [List Voices]. |
| `is_owner` | boolean | yes | Whether your organization owns the voice. |
| `status` | string enum(`active`, `archived`) | yes | Status of the voice. Only active voices are returned in [List Voices] by default unless `include_archived=true`. |
| `access` | string enum(`private`, `public`) | yes | Who can use the resource. `private` means only the owner can use the resource. `public` means everyone can use the resource. |
| `name` | string | yes | The display name of the voice. Does not include the tagline. |
| `tagline` | string | yes | A short descriptor for the voice (at most 32 characters). Empty string when unset. (default `""`) |
| `description` | string | yes | A description for the voice, typically longer than the tagline if both are provided. |
| `gender` | string enum(`masculine`, `feminine`, `gender_neutral`) |  |  |
| `created_at` | string<date-time> | yes | The date and time the voice was created. |
| `preview_file_url` | string |  | A URL to download a preview audio file for this voice. Useful to avoid consuming credits when looking for the right voice. The URL requires the same Authorizat… |
| `language` | string | yes | The voice's language, as an ISO 639-1 code (e.g. `en`, `fr`, `zh`) |
| `accents` | VoiceAccentsItem[] |  | Accents the voice sounds native in, with the original accent marked `is_native: true`. In any other [supported language], the voice speaks with that original a… |
| `accents[].accent` | string enum(`emirati-arabic`, `khaleeji`, `middle-eastern-arabic`, `modern-standard-arabic`) \| string enum(`indian-bengali`, `jessore`) \| string enum(`bulgarian`) \| string enum(`mandarin`, `taiwanese-mandarin`) \| string enum(`zagreb`) \| string enum(`czech`) \| string enum(`danish`) \| string enum(`belgian-dutch`, `randstad`) \| string enum(`african-american`, `arabic-english`, `australian`, `british`, `california`, `canadian-english`, `general-american`, `indian-english`, `irish`, `midwestern-american`, `new-york`, `new-zealand` …(+5)) \| string enum(`finnish`) \| string enum(`african-french`, `belgian-french`, `canadian-french`, `parisian`) \| string enum(`tbilisi`) \| string enum(`austrian-german`, `standard-german`, `swiss-standard`) \| string enum(`thessaloniki`) \| string enum(`parsi`) \| string enum(`israeli`) \| string enum(`bagheli`, `standard-hindi`) \| string enum(`budapest`) \| string enum(`standard-indonesian`) \| string enum(`abruzzo-italian`, `campania`, `standard-italian`) \| string enum(`standard-japanese`) \| string enum(`southern-karnataka`) \| string enum(`korean`) \| string enum(`malay`) \| string enum(`north-kerala`) \| string enum(`konkani`) \| string enum(`oslo`) \| string enum(`odia`) \| string enum(`polish`) \| string enum(`brazilian-portuguese`, `european-portuguese`) \| string enum(`powadhi`) \| string enum(`moldovan`, `romanian`) \| string enum(`russian`) \| string enum(`slovak`) \| string enum(`argentinian`, `camba`, `castilian`, `chilean`, `colombian`, `dominican`, `mexican`, `panamanian`, `peruvian`, `puerto-rican`, `united-states-spanish`) \| string enum(`stockholm`) \| string enum(`manila`) \| string enum(`central-tamil`) \| string enum(`telangana`) \| string enum(`central-thai`) \| string enum(`istanbul`) \| string enum(`kyiv`) \| string enum(`indian-urdu`, `urdu`) \| string enum(`central-vietnamese`, `southern-vietnamese`) | yes | Accent id from [GET /accents] (e.g. `southern-us`). |
| `accents[].locale` | string | yes | The accent's BCP-47 language-region tag (e.g. `en-US`). |
| `accents[].is_native` | boolean | yes | Whether this is the accent the voice was originally recorded in. It's the accent the voice falls back to when it speaks a language it has no matching accent fo… |
| `country` | string |  | The country associated with the voice, as an ISO 3166-1 alpha-2 code when available (e.g. `US`, `GB`, `FR`). |
| `is_pro` | boolean |  | Whether this voice is a pro voice clone. |
| `visibility` | string enum(`owner`, `all`) | yes | When the resource is returned by the list endpoint. `owner` means the resource appears for the owner only. `all` means the resource appears for all users. |


### `PATCH /voices/{id}` — Update Voice

Update the name, description, gender, and accent of a voice. To set the gender back to the default, set the gender to `null`. If gender is not specified, the gender will not be updated.

- **Auth:** APIKeyAuth  ·  **operationId:** `voices_update`  ·  **docs page:** `api-reference/voices/update.md`

**Parameters**

| Name | In | Type | Req | Description |
|---|---|---|---|---|
| `id` | path | string | yes |  |

**Request body** (`application/json`; type UpdateVoiceRequest)

| Field | Type | Req | Description |
|---|---|---|---|
| `name` | string |  | The name of the voice. |
| `tagline` | string |  | A few words describing the voice. At most 32 characters. |
| `description` | string |  | A description for the voice, typically longer than the tagline if both are provided. |
| `gender` | string enum(`masculine`, `feminine`, `gender_neutral`) |  |  |
| `accent` | string enum(`emirati-arabic`, `khaleeji`, `middle-eastern-arabic`, `modern-standard-arabic`) \| string enum(`indian-bengali`, `jessore`) \| string enum(`bulgarian`) \| string enum(`mandarin`, `taiwanese-mandarin`) \| string enum(`zagreb`) \| string enum(`czech`) \| string enum(`danish`) \| string enum(`belgian-dutch`, `randstad`) \| string enum(`african-american`, `arabic-english`, `australian`, `british`, `california`, `canadian-english`, `general-american`, `indian-english`, `irish`, `midwestern-american`, `new-york`, `new-zealand` …(+5)) \| string enum(`finnish`) \| string enum(`african-french`, `belgian-french`, `canadian-french`, `parisian`) \| string enum(`tbilisi`) \| string enum(`austrian-german`, `standard-german`, `swiss-standard`) \| string enum(`thessaloniki`) \| string enum(`parsi`) \| string enum(`israeli`) \| string enum(`bagheli`, `standard-hindi`) \| string enum(`budapest`) \| string enum(`standard-indonesian`) \| string enum(`abruzzo-italian`, `campania`, `standard-italian`) \| string enum(`standard-japanese`) \| string enum(`southern-karnataka`) \| string enum(`korean`) \| string enum(`malay`) \| string enum(`north-kerala`) \| string enum(`konkani`) \| string enum(`oslo`) \| string enum(`odia`) \| string enum(`polish`) \| string enum(`brazilian-portuguese`, `european-portuguese`) \| string enum(`powadhi`) \| string enum(`moldovan`, `romanian`) \| string enum(`russian`) \| string enum(`slovak`) \| string enum(`argentinian`, `camba`, `castilian`, `chilean`, `colombian`, `dominican`, `mexican`, `panamanian`, `peruvian`, `puerto-rican`, `united-states-spanish`) \| string enum(`stockholm`) \| string enum(`manila`) \| string enum(`central-tamil`) \| string enum(`telangana`) \| string enum(`central-thai`) \| string enum(`istanbul`) \| string enum(`kyiv`) \| string enum(`indian-urdu`, `urdu`) \| string enum(`central-vietnamese`, `southern-vietnamese`) |  | Metadata for labeling the accent of this voice, and a prerequisite for [Add Voice Accents]. See [GET /accents] for values valid for each language. |
| `access` | string enum(`private`, `public`) \| object |  | Controls who can access this resource. Prefer `"public"` \| `"private"`. Deprecated nested `{ "type": "public" \| "private" }` is still accepted. |

**Responses**

- `200` `application/json` → Voice

| Field | Type | Req | Description |
|---|---|---|---|
| `id` | string | yes | The ID of the voice. Find one in the [Voice Library] or via [List Voices]. |
| `is_owner` | boolean | yes | Whether your organization owns the voice. |
| `status` | string enum(`active`, `archived`) | yes | Status of the voice. Only active voices are returned in [List Voices] by default unless `include_archived=true`. |
| `access` | string enum(`private`, `public`) | yes | Who can use the resource. `private` means only the owner can use the resource. `public` means everyone can use the resource. |
| `name` | string | yes | The display name of the voice. Does not include the tagline. |
| `tagline` | string | yes | A short descriptor for the voice (at most 32 characters). Empty string when unset. (default `""`) |
| `description` | string | yes | A description for the voice, typically longer than the tagline if both are provided. |
| `gender` | string enum(`masculine`, `feminine`, `gender_neutral`) |  |  |
| `created_at` | string<date-time> | yes | The date and time the voice was created. |
| `preview_file_url` | string |  | A URL to download a preview audio file for this voice. Useful to avoid consuming credits when looking for the right voice. The URL requires the same Authorizat… |
| `language` | string | yes | The voice's language, as an ISO 639-1 code (e.g. `en`, `fr`, `zh`) |
| `accents` | VoiceAccentsItem[] |  | Accents the voice sounds native in, with the original accent marked `is_native: true`. In any other [supported language], the voice speaks with that original a… |
| `accents[].accent` | string enum(`emirati-arabic`, `khaleeji`, `middle-eastern-arabic`, `modern-standard-arabic`) \| string enum(`indian-bengali`, `jessore`) \| string enum(`bulgarian`) \| string enum(`mandarin`, `taiwanese-mandarin`) \| string enum(`zagreb`) \| string enum(`czech`) \| string enum(`danish`) \| string enum(`belgian-dutch`, `randstad`) \| string enum(`african-american`, `arabic-english`, `australian`, `british`, `california`, `canadian-english`, `general-american`, `indian-english`, `irish`, `midwestern-american`, `new-york`, `new-zealand` …(+5)) \| string enum(`finnish`) \| string enum(`african-french`, `belgian-french`, `canadian-french`, `parisian`) \| string enum(`tbilisi`) \| string enum(`austrian-german`, `standard-german`, `swiss-standard`) \| string enum(`thessaloniki`) \| string enum(`parsi`) \| string enum(`israeli`) \| string enum(`bagheli`, `standard-hindi`) \| string enum(`budapest`) \| string enum(`standard-indonesian`) \| string enum(`abruzzo-italian`, `campania`, `standard-italian`) \| string enum(`standard-japanese`) \| string enum(`southern-karnataka`) \| string enum(`korean`) \| string enum(`malay`) \| string enum(`north-kerala`) \| string enum(`konkani`) \| string enum(`oslo`) \| string enum(`odia`) \| string enum(`polish`) \| string enum(`brazilian-portuguese`, `european-portuguese`) \| string enum(`powadhi`) \| string enum(`moldovan`, `romanian`) \| string enum(`russian`) \| string enum(`slovak`) \| string enum(`argentinian`, `camba`, `castilian`, `chilean`, `colombian`, `dominican`, `mexican`, `panamanian`, `peruvian`, `puerto-rican`, `united-states-spanish`) \| string enum(`stockholm`) \| string enum(`manila`) \| string enum(`central-tamil`) \| string enum(`telangana`) \| string enum(`central-thai`) \| string enum(`istanbul`) \| string enum(`kyiv`) \| string enum(`indian-urdu`, `urdu`) \| string enum(`central-vietnamese`, `southern-vietnamese`) | yes | Accent id from [GET /accents] (e.g. `southern-us`). |
| `accents[].locale` | string | yes | The accent's BCP-47 language-region tag (e.g. `en-US`). |
| `accents[].is_native` | boolean | yes | Whether this is the accent the voice was originally recorded in. It's the accent the voice falls back to when it speaks a language it has no matching accent fo… |
| `country` | string |  | The country associated with the voice, as an ISO 3166-1 alpha-2 code when available (e.g. `US`, `GB`, `FR`). |
| `is_pro` | boolean |  | Whether this voice is a pro voice clone. |
| `visibility` | string enum(`owner`, `all`) | yes | When the resource is returned by the list endpoint. `owner` means the resource appears for the owner only. `all` means the resource appears for all users. |


### `PATCH /voices/{id}/accents` — Add Voice Accents

Add accents to an instant voice clone you own so a single `voice_id` can speak multiple accents natively. The voice usually still sounds like the same speaker, though how closely it matches varies by voice and accent.

- **Auth:** APIKeyAuth  ·  **operationId:** `voices_add_accents`  ·  **docs page:** `api-reference/voices/add-accents.md`

**Parameters**

| Name | In | Type | Req | Description |
|---|---|---|---|---|
| `id` | path | string | yes |  |

**Request body** (`application/json`; type AddVoiceAccentsRequest)

| Field | Type | Req | Description |
|---|---|---|---|
| `accents` | string enum(`australian`, `brazilian-portuguese`, `british`, `bulgarian`, `canadian-french`, `castilian`, `central-tamil`, `central-thai`, `czech`, `danish`, `european-portuguese`, `general-american` …(+30))[] | yes | Additional accents you want this voice to speak natively. The voice must already have an accent of its own: if [Get Voice] returns no `accents` field, set one … |

**Responses**

- `200` `application/json` → Voice

| Field | Type | Req | Description |
|---|---|---|---|
| `id` | string | yes | The ID of the voice. Find one in the [Voice Library] or via [List Voices]. |
| `is_owner` | boolean | yes | Whether your organization owns the voice. |
| `status` | string enum(`active`, `archived`) | yes | Status of the voice. Only active voices are returned in [List Voices] by default unless `include_archived=true`. |
| `access` | string enum(`private`, `public`) | yes | Who can use the resource. `private` means only the owner can use the resource. `public` means everyone can use the resource. |
| `name` | string | yes | The display name of the voice. Does not include the tagline. |
| `tagline` | string | yes | A short descriptor for the voice (at most 32 characters). Empty string when unset. (default `""`) |
| `description` | string | yes | A description for the voice, typically longer than the tagline if both are provided. |
| `gender` | string enum(`masculine`, `feminine`, `gender_neutral`) |  |  |
| `created_at` | string<date-time> | yes | The date and time the voice was created. |
| `preview_file_url` | string |  | A URL to download a preview audio file for this voice. Useful to avoid consuming credits when looking for the right voice. The URL requires the same Authorizat… |
| `language` | string | yes | The voice's language, as an ISO 639-1 code (e.g. `en`, `fr`, `zh`) |
| `accents` | VoiceAccentsItem[] |  | Accents the voice sounds native in, with the original accent marked `is_native: true`. In any other [supported language], the voice speaks with that original a… |
| `accents[].accent` | string enum(`emirati-arabic`, `khaleeji`, `middle-eastern-arabic`, `modern-standard-arabic`) \| string enum(`indian-bengali`, `jessore`) \| string enum(`bulgarian`) \| string enum(`mandarin`, `taiwanese-mandarin`) \| string enum(`zagreb`) \| string enum(`czech`) \| string enum(`danish`) \| string enum(`belgian-dutch`, `randstad`) \| string enum(`african-american`, `arabic-english`, `australian`, `british`, `california`, `canadian-english`, `general-american`, `indian-english`, `irish`, `midwestern-american`, `new-york`, `new-zealand` …(+5)) \| string enum(`finnish`) \| string enum(`african-french`, `belgian-french`, `canadian-french`, `parisian`) \| string enum(`tbilisi`) \| string enum(`austrian-german`, `standard-german`, `swiss-standard`) \| string enum(`thessaloniki`) \| string enum(`parsi`) \| string enum(`israeli`) \| string enum(`bagheli`, `standard-hindi`) \| string enum(`budapest`) \| string enum(`standard-indonesian`) \| string enum(`abruzzo-italian`, `campania`, `standard-italian`) \| string enum(`standard-japanese`) \| string enum(`southern-karnataka`) \| string enum(`korean`) \| string enum(`malay`) \| string enum(`north-kerala`) \| string enum(`konkani`) \| string enum(`oslo`) \| string enum(`odia`) \| string enum(`polish`) \| string enum(`brazilian-portuguese`, `european-portuguese`) \| string enum(`powadhi`) \| string enum(`moldovan`, `romanian`) \| string enum(`russian`) \| string enum(`slovak`) \| string enum(`argentinian`, `camba`, `castilian`, `chilean`, `colombian`, `dominican`, `mexican`, `panamanian`, `peruvian`, `puerto-rican`, `united-states-spanish`) \| string enum(`stockholm`) \| string enum(`manila`) \| string enum(`central-tamil`) \| string enum(`telangana`) \| string enum(`central-thai`) \| string enum(`istanbul`) \| string enum(`kyiv`) \| string enum(`indian-urdu`, `urdu`) \| string enum(`central-vietnamese`, `southern-vietnamese`) | yes | Accent id from [GET /accents] (e.g. `southern-us`). |
| `accents[].locale` | string | yes | The accent's BCP-47 language-region tag (e.g. `en-US`). |
| `accents[].is_native` | boolean | yes | Whether this is the accent the voice was originally recorded in. It's the accent the voice falls back to when it speaks a language it has no matching accent fo… |
| `country` | string |  | The country associated with the voice, as an ISO 3166-1 alpha-2 code when available (e.g. `US`, `GB`, `FR`). |
| `is_pro` | boolean |  | Whether this voice is a pro voice clone. |
| `visibility` | string enum(`owner`, `all`) | yes | When the resource is returned by the list endpoint. `owner` means the resource appears for the owner only. `all` means the resource appears for all users. |


### `DELETE /voices/{id}/accents/{accent_id}` — Delete Voice Accent

Remove an accent your instant voice clone supports.

- **Auth:** APIKeyAuth  ·  **operationId:** `voices_delete_accent`  ·  **docs page:** `api-reference/voices/delete-accent.md`

**Parameters**

| Name | In | Type | Req | Description |
|---|---|---|---|---|
| `id` | path | string | yes |  |
| `accent_id` | path | string | yes | Pass one of the accents from this voice's [Get Voice] response, except its native accent (`is_native: true`). |

**Responses**

- `200` `application/json` → Voice

| Field | Type | Req | Description |
|---|---|---|---|
| `id` | string | yes | The ID of the voice. Find one in the [Voice Library] or via [List Voices]. |
| `is_owner` | boolean | yes | Whether your organization owns the voice. |
| `status` | string enum(`active`, `archived`) | yes | Status of the voice. Only active voices are returned in [List Voices] by default unless `include_archived=true`. |
| `access` | string enum(`private`, `public`) | yes | Who can use the resource. `private` means only the owner can use the resource. `public` means everyone can use the resource. |
| `name` | string | yes | The display name of the voice. Does not include the tagline. |
| `tagline` | string | yes | A short descriptor for the voice (at most 32 characters). Empty string when unset. (default `""`) |
| `description` | string | yes | A description for the voice, typically longer than the tagline if both are provided. |
| `gender` | string enum(`masculine`, `feminine`, `gender_neutral`) |  |  |
| `created_at` | string<date-time> | yes | The date and time the voice was created. |
| `preview_file_url` | string |  | A URL to download a preview audio file for this voice. Useful to avoid consuming credits when looking for the right voice. The URL requires the same Authorizat… |
| `language` | string | yes | The voice's language, as an ISO 639-1 code (e.g. `en`, `fr`, `zh`) |
| `accents` | VoiceAccentsItem[] |  | Accents the voice sounds native in, with the original accent marked `is_native: true`. In any other [supported language], the voice speaks with that original a… |
| `accents[].accent` | string enum(`emirati-arabic`, `khaleeji`, `middle-eastern-arabic`, `modern-standard-arabic`) \| string enum(`indian-bengali`, `jessore`) \| string enum(`bulgarian`) \| string enum(`mandarin`, `taiwanese-mandarin`) \| string enum(`zagreb`) \| string enum(`czech`) \| string enum(`danish`) \| string enum(`belgian-dutch`, `randstad`) \| string enum(`african-american`, `arabic-english`, `australian`, `british`, `california`, `canadian-english`, `general-american`, `indian-english`, `irish`, `midwestern-american`, `new-york`, `new-zealand` …(+5)) \| string enum(`finnish`) \| string enum(`african-french`, `belgian-french`, `canadian-french`, `parisian`) \| string enum(`tbilisi`) \| string enum(`austrian-german`, `standard-german`, `swiss-standard`) \| string enum(`thessaloniki`) \| string enum(`parsi`) \| string enum(`israeli`) \| string enum(`bagheli`, `standard-hindi`) \| string enum(`budapest`) \| string enum(`standard-indonesian`) \| string enum(`abruzzo-italian`, `campania`, `standard-italian`) \| string enum(`standard-japanese`) \| string enum(`southern-karnataka`) \| string enum(`korean`) \| string enum(`malay`) \| string enum(`north-kerala`) \| string enum(`konkani`) \| string enum(`oslo`) \| string enum(`odia`) \| string enum(`polish`) \| string enum(`brazilian-portuguese`, `european-portuguese`) \| string enum(`powadhi`) \| string enum(`moldovan`, `romanian`) \| string enum(`russian`) \| string enum(`slovak`) \| string enum(`argentinian`, `camba`, `castilian`, `chilean`, `colombian`, `dominican`, `mexican`, `panamanian`, `peruvian`, `puerto-rican`, `united-states-spanish`) \| string enum(`stockholm`) \| string enum(`manila`) \| string enum(`central-tamil`) \| string enum(`telangana`) \| string enum(`central-thai`) \| string enum(`istanbul`) \| string enum(`kyiv`) \| string enum(`indian-urdu`, `urdu`) \| string enum(`central-vietnamese`, `southern-vietnamese`) | yes | Accent id from [GET /accents] (e.g. `southern-us`). |
| `accents[].locale` | string | yes | The accent's BCP-47 language-region tag (e.g. `en-US`). |
| `accents[].is_native` | boolean | yes | Whether this is the accent the voice was originally recorded in. It's the accent the voice falls back to when it speaks a language it has no matching accent fo… |
| `country` | string |  | The country associated with the voice, as an ISO 3166-1 alpha-2 code when available (e.g. `US`, `GB`, `FR`). |
| `is_pro` | boolean |  | Whether this voice is a pro voice clone. |
| `visibility` | string enum(`owner`, `all`) | yes | When the resource is returned by the list endpoint. `owner` means the resource appears for the owner only. `all` means the resource appears for all users. |



---

## accents

### `GET /accents` — List Accents

Retrieve all supported accents, including which ones you can localize a voice into. Filter with `is_localizable`, `language`, and `locale`. If you pass more than one, every filter has to match.

- **Auth:** APIKeyAuth  ·  **operationId:** `accents_list`  ·  **docs page:** `api-reference/accents/list.md`

**Parameters**

| Name | In | Type | Req | Description |
|---|---|---|---|---|
| `is_localizable` | query | boolean |  | Whether you can localize a voice into this accent when calling [Add Voice Accents] or [Localize Voice] |
| `language` | query | string |  | Filter by accents belonging to a language, like `es` or `en` |
| `locale` | query | string |  | Filter by accents belonging to a locale, like `en-US` or `es-MX` |

**Responses**

- `200` `application/json` → ListAccentsResponse

| Field | Type | Req | Description |
|---|---|---|---|
| `accents` | Accent[] | yes | Official accents, sorted by id. |
| `accents[].id` | string enum(`emirati-arabic`, `khaleeji`, `middle-eastern-arabic`, `modern-standard-arabic`) \| string enum(`indian-bengali`, `jessore`) \| string enum(`bulgarian`) \| string enum(`mandarin`, `taiwanese-mandarin`) \| string enum(`zagreb`) \| string enum(`czech`) \| string enum(`danish`) \| string enum(`belgian-dutch`, `randstad`) \| string enum(`african-american`, `arabic-english`, `australian`, `british`, `california`, `canadian-english`, `general-american`, `indian-english`, `irish`, `midwestern-american`, `new-york`, `new-zealand` …(+5)) \| string enum(`finnish`) \| string enum(`african-french`, `belgian-french`, `canadian-french`, `parisian`) \| string enum(`tbilisi`) \| string enum(`austrian-german`, `standard-german`, `swiss-standard`) \| string enum(`thessaloniki`) \| string enum(`parsi`) \| string enum(`israeli`) \| string enum(`bagheli`, `standard-hindi`) \| string enum(`budapest`) \| string enum(`standard-indonesian`) \| string enum(`abruzzo-italian`, `campania`, `standard-italian`) \| string enum(`standard-japanese`) \| string enum(`southern-karnataka`) \| string enum(`korean`) \| string enum(`malay`) \| string enum(`north-kerala`) \| string enum(`konkani`) \| string enum(`oslo`) \| string enum(`odia`) \| string enum(`polish`) \| string enum(`brazilian-portuguese`, `european-portuguese`) \| string enum(`powadhi`) \| string enum(`moldovan`, `romanian`) \| string enum(`russian`) \| string enum(`slovak`) \| string enum(`argentinian`, `camba`, `castilian`, `chilean`, `colombian`, `dominican`, `mexican`, `panamanian`, `peruvian`, `puerto-rican`, `united-states-spanish`) \| string enum(`stockholm`) \| string enum(`manila`) \| string enum(`central-tamil`) \| string enum(`telangana`) \| string enum(`central-thai`) \| string enum(`istanbul`) \| string enum(`kyiv`) \| string enum(`indian-urdu`, `urdu`) \| string enum(`central-vietnamese`, `southern-vietnamese`) | yes | ID of the accent. |
| `accents[].name` | string | yes | Human-readable display name (for example `General American English`). |
| `accents[].language` | string | yes | ISO 639-1 language subtag (for example `en`). |
| `accents[].locale` | string | yes | Canonical locale for this accent (BCP-47, for example `en-US`). |
| `accents[].is_locale_default` | boolean | yes | Whether this accent is the default for its `locale`. Exactly one accent per locale is true. |
| `accents[].is_localizable` | boolean | yes | Whether POST /voices/localize can target this accent. |



---

## pronunciation-dicts

### `GET /pronunciation-dicts/` — 

List all pronunciation dictionaries for the authenticated user

- **Auth:** APIKeyAuth  ·  **operationId:** `pronunciationDicts_list`  ·  **docs page:** `api-reference/pronunciation-dicts/list.md`

**Parameters**

| Name | In | Type | Req | Description |
|---|---|---|---|---|
| `limit` | query | integer |  | The number of dictionaries to return per page, ranging between 1 and 100. |
| `starting_after` | query | string |  | A cursor to use in pagination. `starting_after` is a dictionary ID that defines your place in the list. For example, if you make a request and receive 20 objec… |
| `ending_before` | query | string |  | A cursor to use in pagination. `ending_before` is a dictionary ID that defines your place in the list. For example, if you make a request and receive 20 object… |

**Responses**

- `200` `application/json` → PaginatedPronunciationDicts

| Field | Type | Req | Description |
|---|---|---|---|
| `data` | PronunciationDict[] | yes | List of pronunciation dictionary objects |
| `data[].id` | string | yes | Unique identifier for the pronunciation dictionary |
| `data[].name` | string | yes | Name of the pronunciation dictionary |
| `data[].description` | string | yes | Description of the pronunciation dictionary |
| `data[].is_owner` | boolean | yes | Whether your organization owns the pronunciation dictionary. |
| `data[].access` | string enum(`private`, `public`) | yes | Who can use the resource. `private` means only the owner can use the resource. `public` means everyone can use the resource. |
| `data[].pinned` | boolean | yes | Whether this dictionary is pinned for the user |
| `data[].items` | PronunciationDictItem[] | yes | List of text-to-pronunciation mappings |
| `data[].items[].text` | string | yes | The original text to be replaced |
| `data[].items[].pronunciation` | string | yes | A phonetic representation or text to be said in place of the original text |
| `data[].items[].case_sensitive` | boolean |  | When false (default), match every capitalization of `text` on Sonic 3.6+. When true, a lowercase key also matches sentence-start capitalization (`cat`/`Cat`, n… (default `false`) |
| `data[].created_at` | string | yes | ISO 8601 timestamp of when the dictionary was created |
| `data[].visibility` | string enum(`owner`, `all`) | yes | When the resource is returned by the list endpoint. `owner` means the resource appears for the owner only. `all` means the resource appears for all users. |
| `has_more` | boolean | yes | Whether there are more dictionaries available |
| `next_page` | string |  | An ID that can be passed as `starting_after` or `ending_before` to get the next page of data. |


### `POST /pronunciation-dicts/` — 

Create a new pronunciation dictionary

- **Auth:** APIKeyAuth  ·  **operationId:** `pronunciationDicts_create`  ·  **docs page:** `api-reference/pronunciation-dicts/create.md`

**Request body** (`application/json`; type CreatePronunciationDictRequest)

| Field | Type | Req | Description |
|---|---|---|---|
| `name` | string | yes | Name for the new pronunciation dictionary |
| `description` | string |  | An optional description for the pronunciation dictionary. (default `""`) |
| `items` | PronunciationDictItem[] |  | Optional initial list of pronunciation mappings |
| `items[].text` | string | yes | The original text to be replaced |
| `items[].pronunciation` | string | yes | A phonetic representation or text to be said in place of the original text |
| `items[].case_sensitive` | boolean |  | When false (default), match every capitalization of `text` on Sonic 3.6+. When true, a lowercase key also matches sentence-start capitalization (`cat`/`Cat`, n… (default `false`) |
| `access` | string enum(`private`, `public`) \| object |  | Controls who can access this resource. Prefer `"public"` \| `"private"`. Deprecated nested `{ "type": "public" \| "private" }` is still accepted. |

**Responses**

- `200` `application/json` → PronunciationDict

| Field | Type | Req | Description |
|---|---|---|---|
| `id` | string | yes | Unique identifier for the pronunciation dictionary |
| `name` | string | yes | Name of the pronunciation dictionary |
| `description` | string | yes | Description of the pronunciation dictionary |
| `is_owner` | boolean | yes | Whether your organization owns the pronunciation dictionary. |
| `access` | string enum(`private`, `public`) | yes | Who can use the resource. `private` means only the owner can use the resource. `public` means everyone can use the resource. |
| `pinned` | boolean | yes | Whether this dictionary is pinned for the user |
| `items` | PronunciationDictItem[] | yes | List of text-to-pronunciation mappings |
| `items[].text` | string | yes | The original text to be replaced |
| `items[].pronunciation` | string | yes | A phonetic representation or text to be said in place of the original text |
| `items[].case_sensitive` | boolean |  | When false (default), match every capitalization of `text` on Sonic 3.6+. When true, a lowercase key also matches sentence-start capitalization (`cat`/`Cat`, n… (default `false`) |
| `created_at` | string | yes | ISO 8601 timestamp of when the dictionary was created |
| `visibility` | string enum(`owner`, `all`) | yes | When the resource is returned by the list endpoint. `owner` means the resource appears for the owner only. `all` means the resource appears for all users. |


### `DELETE /pronunciation-dicts/{id}` — 

Delete a pronunciation dictionary

- **Auth:** APIKeyAuth  ·  **operationId:** `pronunciationDicts_delete`  ·  **docs page:** `api-reference/pronunciation-dicts/delete.md`

**Parameters**

| Name | In | Type | Req | Description |
|---|---|---|---|---|
| `id` | path | string | yes | ID of the pronunciation dictionary to delete |

**Responses**

- `204` 

### `GET /pronunciation-dicts/{id}` — 

Retrieve a specific pronunciation dictionary by ID

- **Auth:** APIKeyAuth  ·  **operationId:** `pronunciationDicts_get`  ·  **docs page:** `api-reference/pronunciation-dicts/get.md`

**Parameters**

| Name | In | Type | Req | Description |
|---|---|---|---|---|
| `id` | path | string | yes | ID of the pronunciation dictionary to retrieve |

**Responses**

- `200` `application/json` → PronunciationDict

| Field | Type | Req | Description |
|---|---|---|---|
| `id` | string | yes | Unique identifier for the pronunciation dictionary |
| `name` | string | yes | Name of the pronunciation dictionary |
| `description` | string | yes | Description of the pronunciation dictionary |
| `is_owner` | boolean | yes | Whether your organization owns the pronunciation dictionary. |
| `access` | string enum(`private`, `public`) | yes | Who can use the resource. `private` means only the owner can use the resource. `public` means everyone can use the resource. |
| `pinned` | boolean | yes | Whether this dictionary is pinned for the user |
| `items` | PronunciationDictItem[] | yes | List of text-to-pronunciation mappings |
| `items[].text` | string | yes | The original text to be replaced |
| `items[].pronunciation` | string | yes | A phonetic representation or text to be said in place of the original text |
| `items[].case_sensitive` | boolean |  | When false (default), match every capitalization of `text` on Sonic 3.6+. When true, a lowercase key also matches sentence-start capitalization (`cat`/`Cat`, n… (default `false`) |
| `created_at` | string | yes | ISO 8601 timestamp of when the dictionary was created |
| `visibility` | string enum(`owner`, `all`) | yes | When the resource is returned by the list endpoint. `owner` means the resource appears for the owner only. `all` means the resource appears for all users. |


### `PATCH /pronunciation-dicts/{id}` — 

Update a pronunciation dictionary

- **Auth:** APIKeyAuth  ·  **operationId:** `pronunciationDicts_update`  ·  **docs page:** `api-reference/pronunciation-dicts/update.md`

**Parameters**

| Name | In | Type | Req | Description |
|---|---|---|---|---|
| `id` | path | string | yes | ID of the pronunciation dictionary to update |

**Request body** (`application/json`; type UpdatePronunciationDictRequest)

| Field | Type | Req | Description |
|---|---|---|---|
| `name` | string |  | New name for the pronunciation dictionary |
| `description` | string |  | New description for the pronunciation dictionary |
| `items` | PronunciationDictItem[] |  | Updated list of pronunciation mappings |
| `items[].text` | string | yes | The original text to be replaced |
| `items[].pronunciation` | string | yes | A phonetic representation or text to be said in place of the original text |
| `items[].case_sensitive` | boolean |  | When false (default), match every capitalization of `text` on Sonic 3.6+. When true, a lowercase key also matches sentence-start capitalization (`cat`/`Cat`, n… (default `false`) |
| `access` | string enum(`private`, `public`) \| object |  | Controls who can access this resource. Prefer `"public"` \| `"private"`. Deprecated nested `{ "type": "public" \| "private" }` is still accepted. |

**Responses**

- `200` `application/json` → PronunciationDict

| Field | Type | Req | Description |
|---|---|---|---|
| `id` | string | yes | Unique identifier for the pronunciation dictionary |
| `name` | string | yes | Name of the pronunciation dictionary |
| `description` | string | yes | Description of the pronunciation dictionary |
| `is_owner` | boolean | yes | Whether your organization owns the pronunciation dictionary. |
| `access` | string enum(`private`, `public`) | yes | Who can use the resource. `private` means only the owner can use the resource. `public` means everyone can use the resource. |
| `pinned` | boolean | yes | Whether this dictionary is pinned for the user |
| `items` | PronunciationDictItem[] | yes | List of text-to-pronunciation mappings |
| `items[].text` | string | yes | The original text to be replaced |
| `items[].pronunciation` | string | yes | A phonetic representation or text to be said in place of the original text |
| `items[].case_sensitive` | boolean |  | When false (default), match every capitalization of `text` on Sonic 3.6+. When true, a lowercase key also matches sentence-start capitalization (`cat`/`Cat`, n… (default `false`) |
| `created_at` | string | yes | ISO 8601 timestamp of when the dictionary was created |
| `visibility` | string enum(`owner`, `all`) | yes | When the resource is returned by the list endpoint. `owner` means the resource appears for the owner only. `all` means the resource appears for all users. |



---

## datasets

### `GET /datasets/` — 

Paginated list of datasets

- **Auth:** APIKeyAuth  ·  **operationId:** `datasets_list`  ·  **docs page:** `api-reference/datasets/list.md`

**Parameters**

| Name | In | Type | Req | Description |
|---|---|---|---|---|
| `limit` | query | integer |  | The number of Datasets to return per page, ranging between 1 and 100. |
| `starting_after` | query | string |  | A cursor to use in pagination. `starting_after` is a Dataset ID that defines your place in the list. For example, if you make a /datasets request and receive 2… |
| `ending_before` | query | string |  | A cursor to use in pagination. `ending_before` is a Dataset ID that defines your place in the list. For example, if you make a /datasets request and receive 20… |

**Responses**

- `200` `application/json` → PaginatedDatasets

| Field | Type | Req | Description |
|---|---|---|---|
| `data` | Dataset[] | yes | List of dataset objects |
| `data[].id` | string | yes | Unique identifier for the dataset |
| `data[].name` | string | yes | Name of the dataset |
| `data[].created_at` | string | yes | Timestamp when the dataset was created |
| `data[].description` | string | yes | Optional description of the dataset |
| `has_more` | boolean | yes | Whether there are more datasets available |
| `next_page` | string |  | An ID that can be passed as `starting_after` or `ending_before` to get the next page of data. |


### `POST /datasets/` — 

Create a new dataset

- **Auth:** APIKeyAuth  ·  **operationId:** `datasets_create`  ·  **docs page:** `api-reference/datasets/create.md`

**Request body** (`application/json`; type CreateDatasetRequest)

| Field | Type | Req | Description |
|---|---|---|---|
| `name` | string | yes | Name for the new dataset |
| `description` | string | yes | Optional description for the dataset |

**Responses**

- `200` `application/json` → Dataset

| Field | Type | Req | Description |
|---|---|---|---|
| `id` | string | yes | Unique identifier for the dataset |
| `name` | string | yes | Name of the dataset |
| `created_at` | string | yes | Timestamp when the dataset was created |
| `description` | string | yes | Optional description of the dataset |


### `DELETE /datasets/{id}` — 

Delete a dataset

- **Auth:** APIKeyAuth  ·  **operationId:** `datasets_delete`  ·  **docs page:** `api-reference/datasets/delete.md`

**Parameters**

| Name | In | Type | Req | Description |
|---|---|---|---|---|
| `id` | path | string | yes | ID of the dataset to delete |

**Responses**

- `204` 

### `GET /datasets/{id}` — 

Retrieve a specific dataset by ID

- **Auth:** APIKeyAuth  ·  **operationId:** `datasets_get`  ·  **docs page:** `api-reference/datasets/get.md`

**Parameters**

| Name | In | Type | Req | Description |
|---|---|---|---|---|
| `id` | path | string | yes | ID of the dataset to retrieve |

**Responses**

- `200` `application/json` → Dataset

| Field | Type | Req | Description |
|---|---|---|---|
| `id` | string | yes | Unique identifier for the dataset |
| `name` | string | yes | Name of the dataset |
| `created_at` | string | yes | Timestamp when the dataset was created |
| `description` | string | yes | Optional description of the dataset |


### `PATCH /datasets/{id}` — 

Update an existing dataset

- **Auth:** APIKeyAuth  ·  **operationId:** `datasets_update`  ·  **docs page:** `api-reference/datasets/update.md`

**Parameters**

| Name | In | Type | Req | Description |
|---|---|---|---|---|
| `id` | path | string | yes | ID of the dataset to update |

**Request body** (`application/json`; type UpdateDatasetRequest)

| Field | Type | Req | Description |
|---|---|---|---|
| `name` | string | yes | New name for the dataset |
| `description` | string | yes | New description for the dataset |

**Responses**

- `204` 

### `GET /datasets/{id}/files` — 

Paginated list of files in a dataset

- **Auth:** APIKeyAuth  ·  **operationId:** `datasets_listFiles`  ·  **docs page:** `api-reference/datasets/list-files.md`

**Parameters**

| Name | In | Type | Req | Description |
|---|---|---|---|---|
| `id` | path | string | yes | ID of the dataset to list files from |
| `limit` | query | integer |  | The number of files to return per page, ranging between 1 and 100. |
| `starting_after` | query | string |  | A cursor to use in pagination. `starting_after` is a file ID that defines your place in the list. For example, if you make a dataset files request and receive … |
| `ending_before` | query | string |  | A cursor to use in pagination. `ending_before` is a file ID that defines your place in the list. For example, if you make a dataset files request and receive 2… |

**Responses**

- `200` `application/json` → PaginatedDatasetFiles

| Field | Type | Req | Description |
|---|---|---|---|
| `data` | DatasetFile[] | yes | List of file objects |
| `data[].id` | string | yes | Unique identifier for the file |
| `data[].filename` | string | yes | Original filename |
| `data[].created_at` | string | yes | Timestamp when the file was created |
| `data[].size` | integer | yes | Size of the file in bytes |
| `has_more` | boolean | yes | Whether there are more files available |
| `next_page` | string |  | An ID that can be passed as `starting_after` or `ending_before` to get the next page of data. |


### `POST /datasets/{id}/files` — 

Upload a new file to a dataset

- **Auth:** APIKeyAuth  ·  **operationId:** `datasets_uploadFile`  ·  **docs page:** `api-reference/datasets/upload-file.md`

**Parameters**

| Name | In | Type | Req | Description |
|---|---|---|---|---|
| `id` | path | string | yes | ID of the dataset to upload to |

**Request body** (`multipart/form-data`; type object)

| Field | Type | Req | Description |
|---|---|---|---|
| `file` | string<binary> |  | Supported audio formats: `flac`, `mp3`, `mpeg`, `mpga`, `oga`, `ogg`, `wav`, `webm` |
| `purpose` | string |  | Purpose of the file |

**Responses**

- `204` 

### `DELETE /datasets/{id}/files/{fileID}` — 

Remove a file from a dataset

- **Auth:** APIKeyAuth  ·  **operationId:** `datasets_deleteFile`  ·  **docs page:** `api-reference/datasets/delete-file.md`

**Parameters**

| Name | In | Type | Req | Description |
|---|---|---|---|---|
| `id` | path | string | yes | ID of the dataset containing the file |
| `fileID` | path | string | yes | ID of the file to remove |

**Responses**

- `204` 


---

## files

### `GET /files` — 

Paginated list of your uploaded files.

- **Auth:** APIKeyAuth  ·  **operationId:** `files_list`  ·  **docs page:** `api-reference/files/list.md`

**Parameters**

| Name | In | Type | Req | Description |
|---|---|---|---|---|
| `purpose` | query | string enum(`agent_background_sound`, `fine_tune`, `tts_generation`, `voice-clone`) |  | Only return files uploaded with this purpose. |
| `q` | query | string |  | Only return files whose filename contains this substring. |
| `limit` | query | integer |  | The number of files to return per page, ranging between 1 and 100. |
| `starting_after` | query | string |  | A cursor for pagination. `starting_after` is a file ID; the response returns the page of files after it. |
| `ending_before` | query | string |  | A cursor for pagination. `ending_before` is a file ID; the response returns the page of files before it. |

**Responses**

- `200` `application/json` → PaginatedFiles

| Field | Type | Req | Description |
|---|---|---|---|
| `data` | File[] | yes | List of file objects. |
| `data[].id` | string | yes | Unique identifier for the file, prefixed with `file_`. |
| `data[].filename` | string | yes | Original filename. |
| `data[].purpose` | string enum(`agent_background_sound`, `fine_tune`, `tts_generation`, `voice-clone`) | yes |  |
| `data[].size` | integer | yes | Size of the file in bytes. |
| `data[].status` | string | yes | Processing status of the file. |
| `data[].owner_id` | string | yes | ID of the account that owns the file. |
| `data[].created_at` | string<date-time> | yes | Timestamp when the file was created. |
| `has_more` | boolean | yes | Whether there are more files available. |


### `POST /files` — 

Upload a file, tagged with what it will be used for.

- **Auth:** APIKeyAuth  ·  **operationId:** `files_upload`  ·  **docs page:** `api-reference/files/upload.md`

**Request body** (`multipart/form-data`; type object)

| Field | Type | Req | Description |
|---|---|---|---|
| `file` | string<binary> | yes | The file to upload. |
| `purpose` | string enum(`agent_background_sound`, `fine_tune`, `tts_generation`, `voice-clone`) | yes | What the file will be used for. |

**Responses**

- `201` `application/json` → File

| Field | Type | Req | Description |
|---|---|---|---|
| `id` | string | yes | Unique identifier for the file, prefixed with `file_`. |
| `filename` | string | yes | Original filename. |
| `purpose` | string enum(`agent_background_sound`, `fine_tune`, `tts_generation`, `voice-clone`) | yes |  |
| `size` | integer | yes | Size of the file in bytes. |
| `status` | string | yes | Processing status of the file. |
| `owner_id` | string | yes | ID of the account that owns the file. |
| `created_at` | string<date-time> | yes | Timestamp when the file was created. |

- `400` The file is not valid for the given purpose (for example, a non-audio file uploaded as `agent_background_sound`).
- `413` The file exceeds the maximum upload size (`error_code: file_too_large`).
- `429` Upload quota exceeded.

### `DELETE /files/{id}` — 

Delete an uploaded file.

- **Auth:** APIKeyAuth  ·  **operationId:** `files_delete`  ·  **docs page:** `api-reference/files/delete.md`

**Parameters**

| Name | In | Type | Req | Description |
|---|---|---|---|---|
| `id` | path | string | yes | ID of the file to delete. |

**Responses**

- `200` `application/json` → object

| Field | Type | Req | Description |
|---|---|---|---|
| `message` | string | yes |  |

- `404` File not found.
- `409` The file is still in use as an agent's background sound; remove it from those agents first.

### `GET /files/{id}/download` — 

Download a file's contents.

- **Auth:** APIKeyAuth  ·  **operationId:** `files_download`  ·  **docs page:** `api-reference/files/download.md`

**Parameters**

| Name | In | Type | Req | Description |
|---|---|---|---|---|
| `id` | path | string | yes | ID of the file. |

**Responses**

- `200` `application/octet-stream` → string<binary>
- `404` File not found.

### `GET /files/{id}/info` — 

Retrieve a file's metadata.

- **Auth:** APIKeyAuth  ·  **operationId:** `files_get`  ·  **docs page:** `api-reference/files/get.md`

**Parameters**

| Name | In | Type | Req | Description |
|---|---|---|---|---|
| `id` | path | string | yes | ID of the file. |

**Responses**

- `200` `application/json` → File

| Field | Type | Req | Description |
|---|---|---|---|
| `id` | string | yes | Unique identifier for the file, prefixed with `file_`. |
| `filename` | string | yes | Original filename. |
| `purpose` | string enum(`agent_background_sound`, `fine_tune`, `tts_generation`, `voice-clone`) | yes |  |
| `size` | integer | yes | Size of the file in bytes. |
| `status` | string | yes | Processing status of the file. |
| `owner_id` | string | yes | ID of the account that owns the file. |
| `created_at` | string<date-time> | yes | Timestamp when the file was created. |

- `404` File not found.


---

## fine-tunes

### `GET /fine-tunes/` — 

Paginated list of all fine-tunes for the authenticated user

- **Auth:** APIKeyAuth  ·  **operationId:** `fineTunes_list`  ·  **docs page:** `api-reference/fine-tunes/list.md`

**Parameters**

| Name | In | Type | Req | Description |
|---|---|---|---|---|
| `limit` | query | integer |  | The number of fine-tunes to return per page, ranging between 1 and 100. |
| `starting_after` | query | string |  | A cursor to use in pagination. `starting_after` is a fine-tune ID that defines your place in the list. For example, if you make a /fine-tunes request and recei… |
| `ending_before` | query | string |  | A cursor to use in pagination. `ending_before` is a fine-tune ID that defines your place in the list. For example, if you make a /fine-tunes request and receiv… |

**Responses**

- `200` `application/json` → PaginatedFineTunes

| Field | Type | Req | Description |
|---|---|---|---|
| `data` | FineTune[] | yes | List of fine-tune objects |
| `data[].id` | string | yes | Unique identifier for the fine-tune |
| `data[].name` | string | yes | Name of the fine-tune |
| `data[].description` | string | yes | Description of the fine-tune |
| `data[].language` | string | yes | Language code of the fine-tune |
| `data[].accent` | string enum(`emirati-arabic`, `khaleeji`, `middle-eastern-arabic`, `modern-standard-arabic`) \| string enum(`indian-bengali`, `jessore`) \| string enum(`bulgarian`) \| string enum(`mandarin`, `taiwanese-mandarin`) \| string enum(`zagreb`) \| string enum(`czech`) \| string enum(`danish`) \| string enum(`belgian-dutch`, `randstad`) \| string enum(`african-american`, `arabic-english`, `australian`, `british`, `california`, `canadian-english`, `general-american`, `indian-english`, `irish`, `midwestern-american`, `new-york`, `new-zealand` …(+5)) \| string enum(`finnish`) \| string enum(`african-french`, `belgian-french`, `canadian-french`, `parisian`) \| string enum(`tbilisi`) \| string enum(`austrian-german`, `standard-german`, `swiss-standard`) \| string enum(`thessaloniki`) \| string enum(`parsi`) \| string enum(`israeli`) \| string enum(`bagheli`, `standard-hindi`) \| string enum(`budapest`) \| string enum(`standard-indonesian`) \| string enum(`abruzzo-italian`, `campania`, `standard-italian`) \| string enum(`standard-japanese`) \| string enum(`southern-karnataka`) \| string enum(`korean`) \| string enum(`malay`) \| string enum(`north-kerala`) \| string enum(`konkani`) \| string enum(`oslo`) \| string enum(`odia`) \| string enum(`polish`) \| string enum(`brazilian-portuguese`, `european-portuguese`) \| string enum(`powadhi`) \| string enum(`moldovan`, `romanian`) \| string enum(`russian`) \| string enum(`slovak`) \| string enum(`argentinian`, `camba`, `castilian`, `chilean`, `colombian`, `dominican`, `mexican`, `panamanian`, `peruvian`, `puerto-rican`, `united-states-spanish`) \| string enum(`stockholm`) \| string enum(`manila`) \| string enum(`central-tamil`) \| string enum(`telangana`) \| string enum(`central-thai`) \| string enum(`istanbul`) \| string enum(`kyiv`) \| string enum(`indian-urdu`, `urdu`) \| string enum(`central-vietnamese`, `southern-vietnamese`) |  | Accent metadata for the voice. |
| `data[].model_id` | string |  | Deprecated. Use [`supported_model_ids`] instead for models compatible with this fine tune. |
| `data[].supported_model_ids` | string[] | yes | Model IDs compatible with this fine-tune |
| `data[].dataset` | string | yes | ID of the dataset used for fine-tuning |
| `data[].status` | string enum(`created`, `training`, `completed`, `failed`) | yes | Status of a fine-tune |
| `data[].user_errors` | object[] |  | Structured user-facing errors when training fails. |
| `data[].user_errors[].code` | string | yes |  |
| `data[].user_errors[].message` | string | yes |  |
| `has_more` | boolean | yes | Whether there are more fine-tunes available |
| `next_page` | string |  | An ID that can be passed as `starting_after` or `ending_before` to get the next page of data. |


### `POST /fine-tunes/` — 

Create a new fine-tune

- **Auth:** APIKeyAuth  ·  **operationId:** `fineTunes_create`  ·  **docs page:** `api-reference/fine-tunes/create.md`

**Request body** (`application/json`; type CreateFineTuneRequest)

| Field | Type | Req | Description |
|---|---|---|---|
| `name` | string | yes | Name for the new fine-tune |
| `description` | string | yes | Description for the fine-tune |
| `language` | string | yes | Language code for the fine-tune. Cannot be changed after training starts or completes. |
| `accent` | string enum(`emirati-arabic`, `khaleeji`, `middle-eastern-arabic`, `modern-standard-arabic`) \| string enum(`indian-bengali`, `jessore`) \| string enum(`bulgarian`) \| string enum(`mandarin`, `taiwanese-mandarin`) \| string enum(`zagreb`) \| string enum(`czech`) \| string enum(`danish`) \| string enum(`belgian-dutch`, `randstad`) \| string enum(`african-american`, `arabic-english`, `australian`, `british`, `california`, `canadian-english`, `general-american`, `indian-english`, `irish`, `midwestern-american`, `new-york`, `new-zealand` …(+5)) \| string enum(`finnish`) \| string enum(`african-french`, `belgian-french`, `canadian-french`, `parisian`) \| string enum(`tbilisi`) \| string enum(`austrian-german`, `standard-german`, `swiss-standard`) \| string enum(`thessaloniki`) \| string enum(`parsi`) \| string enum(`israeli`) \| string enum(`bagheli`, `standard-hindi`) \| string enum(`budapest`) \| string enum(`standard-indonesian`) \| string enum(`abruzzo-italian`, `campania`, `standard-italian`) \| string enum(`standard-japanese`) \| string enum(`southern-karnataka`) \| string enum(`korean`) \| string enum(`malay`) \| string enum(`north-kerala`) \| string enum(`konkani`) \| string enum(`oslo`) \| string enum(`odia`) \| string enum(`polish`) \| string enum(`brazilian-portuguese`, `european-portuguese`) \| string enum(`powadhi`) \| string enum(`moldovan`, `romanian`) \| string enum(`russian`) \| string enum(`slovak`) \| string enum(`argentinian`, `camba`, `castilian`, `chilean`, `colombian`, `dominican`, `mexican`, `panamanian`, `peruvian`, `puerto-rican`, `united-states-spanish`) \| string enum(`stockholm`) \| string enum(`manila`) \| string enum(`central-tamil`) \| string enum(`telangana`) \| string enum(`central-thai`) \| string enum(`istanbul`) \| string enum(`kyiv`) \| string enum(`indian-urdu`, `urdu`) \| string enum(`central-vietnamese`, `southern-vietnamese`) |  | Metadata for labeling the accent of this fine-tune. See [GET /accents] for values valid for each language. Cannot be changed after training starts or completes. |
| `dataset` | string | yes | Dataset ID containing training files |

**Responses**

- `200` `application/json` → FineTune

| Field | Type | Req | Description |
|---|---|---|---|
| `id` | string | yes | Unique identifier for the fine-tune |
| `name` | string | yes | Name of the fine-tune |
| `description` | string | yes | Description of the fine-tune |
| `language` | string | yes | Language code of the fine-tune |
| `accent` | string enum(`emirati-arabic`, `khaleeji`, `middle-eastern-arabic`, `modern-standard-arabic`) \| string enum(`indian-bengali`, `jessore`) \| string enum(`bulgarian`) \| string enum(`mandarin`, `taiwanese-mandarin`) \| string enum(`zagreb`) \| string enum(`czech`) \| string enum(`danish`) \| string enum(`belgian-dutch`, `randstad`) \| string enum(`african-american`, `arabic-english`, `australian`, `british`, `california`, `canadian-english`, `general-american`, `indian-english`, `irish`, `midwestern-american`, `new-york`, `new-zealand` …(+5)) \| string enum(`finnish`) \| string enum(`african-french`, `belgian-french`, `canadian-french`, `parisian`) \| string enum(`tbilisi`) \| string enum(`austrian-german`, `standard-german`, `swiss-standard`) \| string enum(`thessaloniki`) \| string enum(`parsi`) \| string enum(`israeli`) \| string enum(`bagheli`, `standard-hindi`) \| string enum(`budapest`) \| string enum(`standard-indonesian`) \| string enum(`abruzzo-italian`, `campania`, `standard-italian`) \| string enum(`standard-japanese`) \| string enum(`southern-karnataka`) \| string enum(`korean`) \| string enum(`malay`) \| string enum(`north-kerala`) \| string enum(`konkani`) \| string enum(`oslo`) \| string enum(`odia`) \| string enum(`polish`) \| string enum(`brazilian-portuguese`, `european-portuguese`) \| string enum(`powadhi`) \| string enum(`moldovan`, `romanian`) \| string enum(`russian`) \| string enum(`slovak`) \| string enum(`argentinian`, `camba`, `castilian`, `chilean`, `colombian`, `dominican`, `mexican`, `panamanian`, `peruvian`, `puerto-rican`, `united-states-spanish`) \| string enum(`stockholm`) \| string enum(`manila`) \| string enum(`central-tamil`) \| string enum(`telangana`) \| string enum(`central-thai`) \| string enum(`istanbul`) \| string enum(`kyiv`) \| string enum(`indian-urdu`, `urdu`) \| string enum(`central-vietnamese`, `southern-vietnamese`) |  | Accent metadata for the voice. |
| `model_id` | string |  | Deprecated. Use [`supported_model_ids`] instead for models compatible with this fine tune. |
| `supported_model_ids` | string[] | yes | Model IDs compatible with this fine-tune |
| `dataset` | string | yes | ID of the dataset used for fine-tuning |
| `status` | string enum(`created`, `training`, `completed`, `failed`) | yes | Status of a fine-tune |
| `user_errors` | object[] |  | Structured user-facing errors when training fails. |
| `user_errors[].code` | string | yes |  |
| `user_errors[].message` | string | yes |  |


### `DELETE /fine-tunes/{id}` — 

Delete a fine-tune

- **Auth:** APIKeyAuth  ·  **operationId:** `fineTunes_delete`  ·  **docs page:** `api-reference/fine-tunes/delete.md`

**Parameters**

| Name | In | Type | Req | Description |
|---|---|---|---|---|
| `id` | path | string | yes | ID of the fine-tune to delete |

**Responses**

- `204` 

### `GET /fine-tunes/{id}` — 

Retrieve a specific fine-tune by ID

- **Auth:** APIKeyAuth  ·  **operationId:** `fineTunes_get`  ·  **docs page:** `api-reference/fine-tunes/get.md`

**Parameters**

| Name | In | Type | Req | Description |
|---|---|---|---|---|
| `id` | path | string | yes | ID of the fine-tune to retrieve |

**Responses**

- `200` `application/json` → FineTune

| Field | Type | Req | Description |
|---|---|---|---|
| `id` | string | yes | Unique identifier for the fine-tune |
| `name` | string | yes | Name of the fine-tune |
| `description` | string | yes | Description of the fine-tune |
| `language` | string | yes | Language code of the fine-tune |
| `accent` | string enum(`emirati-arabic`, `khaleeji`, `middle-eastern-arabic`, `modern-standard-arabic`) \| string enum(`indian-bengali`, `jessore`) \| string enum(`bulgarian`) \| string enum(`mandarin`, `taiwanese-mandarin`) \| string enum(`zagreb`) \| string enum(`czech`) \| string enum(`danish`) \| string enum(`belgian-dutch`, `randstad`) \| string enum(`african-american`, `arabic-english`, `australian`, `british`, `california`, `canadian-english`, `general-american`, `indian-english`, `irish`, `midwestern-american`, `new-york`, `new-zealand` …(+5)) \| string enum(`finnish`) \| string enum(`african-french`, `belgian-french`, `canadian-french`, `parisian`) \| string enum(`tbilisi`) \| string enum(`austrian-german`, `standard-german`, `swiss-standard`) \| string enum(`thessaloniki`) \| string enum(`parsi`) \| string enum(`israeli`) \| string enum(`bagheli`, `standard-hindi`) \| string enum(`budapest`) \| string enum(`standard-indonesian`) \| string enum(`abruzzo-italian`, `campania`, `standard-italian`) \| string enum(`standard-japanese`) \| string enum(`southern-karnataka`) \| string enum(`korean`) \| string enum(`malay`) \| string enum(`north-kerala`) \| string enum(`konkani`) \| string enum(`oslo`) \| string enum(`odia`) \| string enum(`polish`) \| string enum(`brazilian-portuguese`, `european-portuguese`) \| string enum(`powadhi`) \| string enum(`moldovan`, `romanian`) \| string enum(`russian`) \| string enum(`slovak`) \| string enum(`argentinian`, `camba`, `castilian`, `chilean`, `colombian`, `dominican`, `mexican`, `panamanian`, `peruvian`, `puerto-rican`, `united-states-spanish`) \| string enum(`stockholm`) \| string enum(`manila`) \| string enum(`central-tamil`) \| string enum(`telangana`) \| string enum(`central-thai`) \| string enum(`istanbul`) \| string enum(`kyiv`) \| string enum(`indian-urdu`, `urdu`) \| string enum(`central-vietnamese`, `southern-vietnamese`) |  | Accent metadata for the voice. |
| `model_id` | string |  | Deprecated. Use [`supported_model_ids`] instead for models compatible with this fine tune. |
| `supported_model_ids` | string[] | yes | Model IDs compatible with this fine-tune |
| `dataset` | string | yes | ID of the dataset used for fine-tuning |
| `status` | string enum(`created`, `training`, `completed`, `failed`) | yes | Status of a fine-tune |
| `user_errors` | object[] |  | Structured user-facing errors when training fails. |
| `user_errors[].code` | string | yes |  |
| `user_errors[].message` | string | yes |  |


### `GET /fine-tunes/{id}/voices` — 

List all voices created from a fine-tune

- **Auth:** APIKeyAuth  ·  **operationId:** `fineTunes_listVoices`  ·  **docs page:** `api-reference/fine-tunes/list-voices.md`

**Parameters**

| Name | In | Type | Req | Description |
|---|---|---|---|---|
| `id` | path | string | yes | ID of the fine-tune to list voices from |
| `limit` | query | integer |  | The number of voices to return per page, ranging between 1 and 100. |
| `starting_after` | query | string |  | A cursor to use in pagination. `starting_after` is a voice ID that defines your place in the list. For example, if you make a fine-tune voices request and rece… |
| `ending_before` | query | string |  | A cursor to use in pagination. `ending_before` is a voice ID that defines your place in the list. For example, if you make a fine-tune voices request and recei… |

**Responses**

- `200` `application/json` → PaginatedVoices

| Field | Type | Req | Description |
|---|---|---|---|
| `data` | Voice[] | yes | List of voice objects |
| `data[].id` | string | yes | The ID of the voice. Find one in the [Voice Library] or via [List Voices]. |
| `data[].is_owner` | boolean | yes | Whether your organization owns the voice. |
| `data[].status` | string enum(`active`, `archived`) | yes | Status of the voice. Only active voices are returned in [List Voices] by default unless `include_archived=true`. |
| `data[].access` | string enum(`private`, `public`) | yes | Who can use the resource. `private` means only the owner can use the resource. `public` means everyone can use the resource. |
| `data[].name` | string | yes | The display name of the voice. Does not include the tagline. |
| `data[].tagline` | string | yes | A short descriptor for the voice (at most 32 characters). Empty string when unset. (default `""`) |
| `data[].description` | string | yes | A description for the voice, typically longer than the tagline if both are provided. |
| `data[].gender` | string enum(`masculine`, `feminine`, `gender_neutral`) |  |  |
| `data[].created_at` | string<date-time> | yes | The date and time the voice was created. |
| `data[].preview_file_url` | string |  | A URL to download a preview audio file for this voice. Useful to avoid consuming credits when looking for the right voice. The URL requires the same Authorizat… |
| `data[].language` | string | yes | The voice's language, as an ISO 639-1 code (e.g. `en`, `fr`, `zh`) |
| `data[].accents` | VoiceAccentsItem[] |  | Accents the voice sounds native in, with the original accent marked `is_native: true`. In any other [supported language], the voice speaks with that original a… |
| `data[].accents[].accent` | string enum(`emirati-arabic`, `khaleeji`, `middle-eastern-arabic`, `modern-standard-arabic`) \| string enum(`indian-bengali`, `jessore`) \| string enum(`bulgarian`) \| string enum(`mandarin`, `taiwanese-mandarin`) \| string enum(`zagreb`) \| string enum(`czech`) \| string enum(`danish`) \| string enum(`belgian-dutch`, `randstad`) \| string enum(`african-american`, `arabic-english`, `australian`, `british`, `california`, `canadian-english`, `general-american`, `indian-english`, `irish`, `midwestern-american`, `new-york`, `new-zealand` …(+5)) \| string enum(`finnish`) \| string enum(`african-french`, `belgian-french`, `canadian-french`, `parisian`) \| string enum(`tbilisi`) \| string enum(`austrian-german`, `standard-german`, `swiss-standard`) \| string enum(`thessaloniki`) \| string enum(`parsi`) \| string enum(`israeli`) \| string enum(`bagheli`, `standard-hindi`) \| string enum(`budapest`) \| string enum(`standard-indonesian`) \| string enum(`abruzzo-italian`, `campania`, `standard-italian`) \| string enum(`standard-japanese`) \| string enum(`southern-karnataka`) \| string enum(`korean`) \| string enum(`malay`) \| string enum(`north-kerala`) \| string enum(`konkani`) \| string enum(`oslo`) \| string enum(`odia`) \| string enum(`polish`) \| string enum(`brazilian-portuguese`, `european-portuguese`) \| string enum(`powadhi`) \| string enum(`moldovan`, `romanian`) \| string enum(`russian`) \| string enum(`slovak`) \| string enum(`argentinian`, `camba`, `castilian`, `chilean`, `colombian`, `dominican`, `mexican`, `panamanian`, `peruvian`, `puerto-rican`, `united-states-spanish`) \| string enum(`stockholm`) \| string enum(`manila`) \| string enum(`central-tamil`) \| string enum(`telangana`) \| string enum(`central-thai`) \| string enum(`istanbul`) \| string enum(`kyiv`) \| string enum(`indian-urdu`, `urdu`) \| string enum(`central-vietnamese`, `southern-vietnamese`) | yes | Accent id from [GET /accents] (e.g. `southern-us`). |
| `data[].accents[].locale` | string | yes | The accent's BCP-47 language-region tag (e.g. `en-US`). |
| `data[].accents[].is_native` | boolean | yes | Whether this is the accent the voice was originally recorded in. It's the accent the voice falls back to when it speaks a language it has no matching accent fo… |
| `data[].country` | string |  | The country associated with the voice, as an ISO 3166-1 alpha-2 code when available (e.g. `US`, `GB`, `FR`). |
| `data[].is_pro` | boolean |  | Whether this voice is a pro voice clone. |
| `data[].visibility` | string enum(`owner`, `all`) | yes | When the resource is returned by the list endpoint. `owner` means the resource appears for the owner only. `all` means the resource appears for all users. |
| `has_more` | boolean | yes | Whether there are more voices available |
| `next_page` | string |  | The ID of the voice. Find one in the [Voice Library] or via [List Voices]. |



---

## access-token

### `POST /access-token` — Generate a New Access Token

Generates a short-lived access token to make API requests from authenticated clients.

- **Auth:** APIKeyAuth  ·  **operationId:** `auth_access-token`  ·  **docs page:** `api-reference/auth/access-token.md`

**Request body** (`application/json`; type TokenRequest)

| Field | Type | Req | Description |
|---|---|---|---|
| `grants` | TokenGrant |  |  |
| `grants.tts` | boolean |  | The `tts` grant allows the token to be used to access any TTS endpoint. |
| `grants.stt` | boolean |  | The `stt` grant allows the token to be used to access any STT endpoint. |
| `grants.agent` | boolean |  | The `agent` grant allows the token to be used to access the Agent websocket calling [endpoint]. |
| `expires_in` | integer |  | The number of seconds the token will be valid for since the time of generation. The maximum is 1 hour (3600 seconds). |

**Responses**

- `200` `application/json` → TokenResponse

| Field | Type | Req | Description |
|---|---|---|---|
| `token` | string | yes | The generated Access Token. |



---

## usage

### `GET /usage/agents` — Get Agent Usage

Returns your agent usage over time, bucketed by the requested interval.

- **Auth:** AdminAPIKeyAuth  ·  **operationId:** `usage_agents`  ·  **docs page:** `api-reference/usage/agents.md`

**Parameters**

| Name | In | Type | Req | Description |
|---|---|---|---|---|
| `start_ts` | query | string<date-time> |  | Start of the time window, as an RFC 3339 datetime (with timezone). Rounded down to the start of the UTC day. `start_ts` to `end_ts` cannot span more than a yea… |
| `end_ts` | query | string<date-time> |  | End of the time window, as an RFC 3339 datetime (with timezone). Rounded up to the start of the next UTC day, unless already at midnight. `start_ts` to `end_ts… |
| `interval` | query | string enum(`day`, `week`, `month`) |  | Omit to receive a single bucket spanning from `start_ts` to `end_ts`. |

**Responses**

- `200` `application/json` → UsageAgentsResponse

| Field | Type | Req | Description |
|---|---|---|---|
| `data` | UsageAgentsBucket[] | yes | Agent usage buckets in chronological order. Buckets with no recorded activity are returned with zero values. |
| `data[].start_ts` | string<date-time> | yes | RFC 3339 timestamp for the start of the usage bucket. |
| `data[].end_ts` | string<date-time> | yes | RFC 3339 timestamp for the end of the usage bucket. |
| `data[].cents` | integer | yes | Total cost in US cents accrued within the bucket. |
| `data[].minutes` | number | yes | Total minutes of agent runtime within the bucket. |
| `data[].calls` | integer | yes | Number of agent calls that started within the bucket. |


### `GET /usage/credits` — Get Credit Usage

Returns your credit usage over time, bucketed by the requested interval.

- **Auth:** AdminAPIKeyAuth  ·  **operationId:** `usage_credits`  ·  **docs page:** `api-reference/usage/credits.md`

**Parameters**

| Name | In | Type | Req | Description |
|---|---|---|---|---|
| `start_ts` | query | string<date-time> |  | Start of the time window, as an RFC 3339 datetime (with timezone). Rounded down to the start of the UTC day. `start_ts` to `end_ts` cannot span more than a yea… |
| `end_ts` | query | string<date-time> |  | End of the time window, as an RFC 3339 datetime (with timezone). Rounded up to the start of the next UTC day, unless already at midnight. `start_ts` to `end_ts… |
| `interval` | query | string enum(`day`, `week`, `month`) |  | Omit to receive a single bucket spanning from `start_ts` to `end_ts`. When `group_by` is set, must be `day` if provided. |
| `api_key_id` | query | string<uuid> |  | Filter usage to a single API key. When set, only credits attributable to that key are returned. Can be combined with `group_by` for other dimensions (`capabili… |
| `group_by` | query | string enum(`capability`, `model`, `voice`, `api_key`) |  | Break down credit usage by capability, model, voice, or API key. When `interval` is set it must be `day`. Cannot be combined with `api_key_id` when `group_by=a… |

**Responses**

- `200` `application/json` → UsageCreditsResponse

| Field | Type | Req | Description |
|---|---|---|---|
| `group_by` | string enum(`capability`, `model`, `voice`, `api_key`) |  | Dimension to break credit usage down by. |
| `data` | UsageCreditsBucket \| UsageCreditsBreakdownSeries[] | yes | Without `group_by`, flat credit usage buckets in chronological order. With `group_by`, one entry per dimension value; each item includes `id`, optional `label`… |



---

## api-keys

### `GET /api-keys` — List API Keys

Returns a paginated list of standard API keys owned by the authenticating organization. Only metadata is returned, not the keys themselves. Admin API keys are not included.

- **Auth:** AdminAPIKeyAuth  ·  **operationId:** `api-keys_list`  ·  **docs page:** `api-reference/api-keys/list.md`

**Parameters**

| Name | In | Type | Req | Description |
|---|---|---|---|---|
| `limit` | query | integer |  | The number of API keys to return per page. Defaults to 20. Values above 100 are silently capped at 100. |
| `starting_after` | query | string |  | A cursor for forward pagination. `starting_after` is an API key ID that defines your place in the list; the response will include keys that come after this ID.… |
| `ending_before` | query | string |  | A cursor for backward pagination. `ending_before` is an API key ID that defines your place in the list; the response will include keys that come before this ID… |
| `q` | query | string |  | Free-text query that matches against the key's description, creator name, and creator email. |

**Responses**

- `200` `application/json` → ListApiKeysResponse

| Field | Type | Req | Description |
|---|---|---|---|
| `data` | ApiKey[] | yes | The paginated list of API keys. |
| `data[].id` | string | yes | Stable identifier for the API key. Safe to expose. |
| `data[].description` | string | yes | Human-readable description set when the key was created. |
| `data[].created_at` | string<date-time> | yes | When the API key was created. |
| `data[].creator_email` | string |  | Email of the user who created the key. `null` for older keys with no recorded creator. |
| `data[].creator_still_in_org` | boolean |  | Whether the creator is still a member of the organization. `null` for older keys with no recorded creator. |
| `data[].creator_image_url` | string |  | URL of the creator's avatar. Returned as `null` when the creator is no longer in the organization, and for older keys with no recorded creator. |
| `has_more` | boolean | yes | Whether there are more pages of API keys. |
| `next_page` | string |  | An ID that can be passed as `starting_after` or `ending_before` to get the next page of data. |

- `400` Returned when `starting_after` and `ending_before` are both provided.

### `GET /api-keys/{id}` — Get API Key

Returns metadata for a single API key.

- **Auth:** AdminAPIKeyAuth  ·  **operationId:** `api-keys_get`  ·  **docs page:** `api-reference/api-keys/get.md`

**Parameters**

| Name | In | Type | Req | Description |
|---|---|---|---|---|
| `id` | path | string | yes | The ID of the API key. This does not start with `sk_`. |

**Responses**

- `200` `application/json` → ApiKey

| Field | Type | Req | Description |
|---|---|---|---|
| `id` | string | yes | Stable identifier for the API key. Safe to expose. |
| `description` | string | yes | Human-readable description set when the key was created. |
| `created_at` | string<date-time> | yes | When the API key was created. |
| `creator_email` | string |  | Email of the user who created the key. `null` for older keys with no recorded creator. |
| `creator_still_in_org` | boolean |  | Whether the creator is still a member of the organization. `null` for older keys with no recorded creator. |
| `creator_image_url` | string |  | URL of the creator's avatar. Returned as `null` when the creator is no longer in the organization, and for older keys with no recorded creator. |

- `404` API key not found, or not owned by the authenticating organization.


---

## organizations

### `GET /organizations/invites` — List Invites

Lists organization invites, filtered by status

- **Auth:** AdminAPIKeyAuth  ·  **operationId:** `getOrganizationsInvites`  ·  **docs page:** `api-reference/organizations/list-invites.md`

**Parameters**

| Name | In | Type | Req | Description |
|---|---|---|---|---|
| `limit` | query | integer |  | The maximum number of items to return. Defaults to 20; values above 100 are capped at 100. |
| `starting_after` | query | string |  | An opaque pagination cursor from `next_page`. Pass it back to fetch the next page. Cannot be combined with `ending_before`. |
| `ending_before` | query | string |  | An opaque pagination cursor from `next_page`. Pass it back to fetch the previous page. Cannot be combined with `starting_after`. |
| `status` | query | string enum(`pending`, `accepted`, `revoked`, `expired`) |  | Filter invites by status. Defaults to `pending`. |

**Responses**

- `200` `application/json` → object

| Field | Type | Req | Description |
|---|---|---|---|
| `data` | OrganizationInvite[] | yes | The requested page of results |
| `data[].created_at` | string<date-time> | yes | When the invite was created |
| `data[].email` | string | yes | Email address the invite was sent to |
| `data[].expires_at` | string<date-time> | yes | When the invite expires, if it has an expiry |
| `data[].id` | string | yes | Unique identifier for the invite |
| `data[].role` | string enum(`admin`, `member`) | yes | Role the invited user will receive when they accept |
| `data[].status` | string enum(`pending`, `accepted`, `revoked`, `expired`) | yes | Status of the invite |
| `has_more` | boolean | yes | Whether more results are available after this page |
| `next_page` | string |  | An ID that can be passed as `starting_after` or `ending_before` to get the next page of data |

- `400` `application/json` → PublicErrorResponse

| Field | Type | Req | Description |
|---|---|---|---|
| `doc_url` | string |  | URL to relevant documentation for the error |
| `error_code` | string |  | Machine-readable error code |
| `message` | string | yes | Detailed human-readable error message |
| `request_id` | string | yes | Unique identifier for this request. Include this when contacting support. |
| `title` | string | yes | Short human-readable error summary, like "File too large" |

- `500` `application/json` → PublicErrorResponse

| Field | Type | Req | Description |
|---|---|---|---|
| `doc_url` | string |  | URL to relevant documentation for the error |
| `error_code` | string |  | Machine-readable error code |
| `message` | string | yes | Detailed human-readable error message |
| `request_id` | string | yes | Unique identifier for this request. Include this when contacting support. |
| `title` | string | yes | Short human-readable error summary, like "File too large" |


### `POST /organizations/invites` — Create Invites

Invites email addresses to the organization as members. If any email is rejected, the whole request fails and no invites are created. Rate limited to 10 requests per hour.

- **Auth:** AdminAPIKeyAuth  ·  **operationId:** `postOrganizationsInvites`  ·  **docs page:** `api-reference/organizations/create-invites.md`

**Request body** (`application/json`; type CreateOrganizationInvitesRequest)

| Field | Type | Req | Description |
|---|---|---|---|
| `emails` | string[] | yes | Email addresses to invite, up to 10 per request. Each address receives a `member` invite. |

**Responses**

- `201` `application/json` → OrganizationInvite[]

| Field | Type | Req | Description |
|---|---|---|---|
| `[].created_at` | string<date-time> | yes | When the invite was created |
| `[].email` | string | yes | Email address the invite was sent to |
| `[].expires_at` | string<date-time> | yes | When the invite expires, if it has an expiry |
| `[].id` | string | yes | Unique identifier for the invite |
| `[].role` | string enum(`admin`, `member`) | yes | Role the invited user will receive when they accept |
| `[].status` | string enum(`pending`, `accepted`, `revoked`, `expired`) | yes | Status of the invite |

- `400` `application/json` → PublicErrorResponse

| Field | Type | Req | Description |
|---|---|---|---|
| `doc_url` | string |  | URL to relevant documentation for the error |
| `error_code` | string |  | Machine-readable error code |
| `message` | string | yes | Detailed human-readable error message |
| `request_id` | string | yes | Unique identifier for this request. Include this when contacting support. |
| `title` | string | yes | Short human-readable error summary, like "File too large" |

- `500` `application/json` → PublicErrorResponse

| Field | Type | Req | Description |
|---|---|---|---|
| `doc_url` | string |  | URL to relevant documentation for the error |
| `error_code` | string |  | Machine-readable error code |
| `message` | string | yes | Detailed human-readable error message |
| `request_id` | string | yes | Unique identifier for this request. Include this when contacting support. |
| `title` | string | yes | Short human-readable error summary, like "File too large" |


### `DELETE /organizations/invites/{id}` — Cancel Invite

Cancels a pending invite. If it has already been accepted, [remove the user] instead.

- **Auth:** AdminAPIKeyAuth  ·  **operationId:** `deleteOrganizationsInvitesById`  ·  **docs page:** `api-reference/organizations/cancel-invite.md`

**Parameters**

| Name | In | Type | Req | Description |
|---|---|---|---|---|
| `id` | path | string | yes | Invite ID returned by [List Invites] |

**Responses**

- `204` Cancel Invite
- `400` Invalid request
- `404` Not found
- `500` Unexpected error

### `GET /organizations/invites/{id}` — Get Invite

- **Auth:** AdminAPIKeyAuth  ·  **operationId:** `getOrganizationsInvitesById`  ·  **docs page:** `api-reference/organizations/get-invite.md`

**Parameters**

| Name | In | Type | Req | Description |
|---|---|---|---|---|
| `id` | path | string | yes | Invite ID returned by [List Invites] |

**Responses**

- `200` `application/json` → OrganizationInvite

| Field | Type | Req | Description |
|---|---|---|---|
| `created_at` | string<date-time> | yes | When the invite was created |
| `email` | string | yes | Email address the invite was sent to |
| `expires_at` | string<date-time> | yes | When the invite expires, if it has an expiry |
| `id` | string | yes | Unique identifier for the invite |
| `role` | string enum(`admin`, `member`) | yes | Role the invited user will receive when they accept |
| `status` | string enum(`pending`, `accepted`, `revoked`, `expired`) | yes | Status of the invite |

- `404` `application/json` → PublicErrorResponse

| Field | Type | Req | Description |
|---|---|---|---|
| `doc_url` | string |  | URL to relevant documentation for the error |
| `error_code` | string |  | Machine-readable error code |
| `message` | string | yes | Detailed human-readable error message |
| `request_id` | string | yes | Unique identifier for this request. Include this when contacting support. |
| `title` | string | yes | Short human-readable error summary, like "File too large" |

- `500` `application/json` → PublicErrorResponse

| Field | Type | Req | Description |
|---|---|---|---|
| `doc_url` | string |  | URL to relevant documentation for the error |
| `error_code` | string |  | Machine-readable error code |
| `message` | string | yes | Detailed human-readable error message |
| `request_id` | string | yes | Unique identifier for this request. Include this when contacting support. |
| `title` | string | yes | Short human-readable error summary, like "File too large" |


### `GET /organizations/users` — List Users

Lists users in the organization, ordered by when they joined (newest first)

- **Auth:** AdminAPIKeyAuth  ·  **operationId:** `getOrganizationsUsers`  ·  **docs page:** `api-reference/organizations/list-users.md`

**Parameters**

| Name | In | Type | Req | Description |
|---|---|---|---|---|
| `limit` | query | integer |  | The maximum number of items to return. Defaults to 20; values above 100 are capped at 100. |
| `starting_after` | query | string |  | An opaque pagination cursor from `next_page`. Pass it back to fetch the next page. Cannot be combined with `ending_before`. |
| `ending_before` | query | string |  | An opaque pagination cursor from `next_page`. Pass it back to fetch the previous page. Cannot be combined with `starting_after`. |
| `q` | query | string |  | Free-text search over user name and email |

**Responses**

- `200` `application/json` → object

| Field | Type | Req | Description |
|---|---|---|---|
| `data` | OrganizationUser[] | yes | The requested page of results |
| `data[].created_at` | string<date-time> | yes | When the user joined the organization |
| `data[].email` | string<email> | yes | Primary email address of the user, if available |
| `data[].id` | string | yes | Stable identifier for the user. Safe to pass to [Get User] and [Remove User]. |
| `data[].image_url` | string | yes | Profile image URL for the user, if available |
| `data[].name` | string | yes | Full name of the user, if available |
| `data[].role` | string enum(`admin`, `member`) | yes | Role of a user in the organization |
| `has_more` | boolean | yes | Whether more results are available after this page |
| `next_page` | string |  | An ID that can be passed as `starting_after` or `ending_before` to get the next page of data |

- `400` `application/json` → PublicErrorResponse

| Field | Type | Req | Description |
|---|---|---|---|
| `doc_url` | string |  | URL to relevant documentation for the error |
| `error_code` | string |  | Machine-readable error code |
| `message` | string | yes | Detailed human-readable error message |
| `request_id` | string | yes | Unique identifier for this request. Include this when contacting support. |
| `title` | string | yes | Short human-readable error summary, like "File too large" |

- `500` `application/json` → PublicErrorResponse

| Field | Type | Req | Description |
|---|---|---|---|
| `doc_url` | string |  | URL to relevant documentation for the error |
| `error_code` | string |  | Machine-readable error code |
| `message` | string | yes | Detailed human-readable error message |
| `request_id` | string | yes | Unique identifier for this request. Include this when contacting support. |
| `title` | string | yes | Short human-readable error summary, like "File too large" |


### `DELETE /organizations/users/{id}` — Remove User

Removes a non-admin user from the organization without deleting their account

- **Auth:** AdminAPIKeyAuth  ·  **operationId:** `deleteOrganizationsUsersById`  ·  **docs page:** `api-reference/organizations/remove-user.md`

**Parameters**

| Name | In | Type | Req | Description |
|---|---|---|---|---|
| `id` | path | string | yes | User ID returned by [List Users] |

**Responses**

- `204` Remove User
- `400` Invalid request
- `404` Not found
- `500` Unexpected error

### `GET /organizations/users/{id}` — Get User

- **Auth:** AdminAPIKeyAuth  ·  **operationId:** `getOrganizationsUsersById`  ·  **docs page:** `api-reference/organizations/get-user.md`

**Parameters**

| Name | In | Type | Req | Description |
|---|---|---|---|---|
| `id` | path | string | yes | User ID returned by [List Users] |

**Responses**

- `200` `application/json` → OrganizationUser

| Field | Type | Req | Description |
|---|---|---|---|
| `created_at` | string<date-time> | yes | When the user joined the organization |
| `email` | string<email> | yes | Primary email address of the user, if available |
| `id` | string | yes | Stable identifier for the user. Safe to pass to [Get User] and [Remove User]. |
| `image_url` | string | yes | Profile image URL for the user, if available |
| `name` | string | yes | Full name of the user, if available |
| `role` | string enum(`admin`, `member`) | yes | Role of a user in the organization |

- `404` `application/json` → PublicErrorResponse

| Field | Type | Req | Description |
|---|---|---|---|
| `doc_url` | string |  | URL to relevant documentation for the error |
| `error_code` | string |  | Machine-readable error code |
| `message` | string | yes | Detailed human-readable error message |
| `request_id` | string | yes | Unique identifier for this request. Include this when contacting support. |
| `title` | string | yes | Short human-readable error summary, like "File too large" |

- `500` `application/json` → PublicErrorResponse

| Field | Type | Req | Description |
|---|---|---|---|
| `doc_url` | string |  | URL to relevant documentation for the error |
| `error_code` | string |  | Machine-readable error code |
| `message` | string | yes | Detailed human-readable error message |
| `request_id` | string | yes | Unique identifier for this request. Include this when contacting support. |
| `title` | string | yes | Short human-readable error summary, like "File too large" |



---

## agents

### `GET /agents/calls` — List Calls

Lists an agent's calls, or the calls placed for one outbound call batch. One of `agent_id` or `batch_id` is required.

- **Auth:** APIKeyAuth  ·  **operationId:** `agents_list-calls`  ·  **docs page:** `api-reference/agents/calls/list-calls.md`

**Parameters**

| Name | In | Type | Req | Description |
|---|---|---|---|---|
| `agent_id` | query | string |  | The ID of the agent. Required unless `batch_id` is supplied. |
| `batch_id` | query | string |  | The ID of the outbound call batch. Required unless `agent_id` is supplied. Returns one entry per call attempt, so a retried recipient appears more than once. R… |
| `start_time_gte` | query | string<date-time> |  | Only return calls that started at or after this RFC 3339 timestamp, inclusive. |
| `start_time_lte` | query | string<date-time> |  | Only return calls that started at or before this RFC 3339 timestamp, inclusive. |
| `expand` | query | string |  | The fields to expand in the response. Currently, the only supported value is `transcript`. |
| `starting_after` | query | string |  | (Pagination option)The ID of the call to start after. |
| `ending_before` | query | string |  | (Pagination option) The ID of the call to end before. |
| `limit` | query | integer |  | (Pagination option) The number of calls to return per page, ranging between 1 and 100. |

**Responses**

- `200` `application/json` → GetCallsResponse

| Field | Type | Req | Description |
|---|---|---|---|
| `data` | AgentCall[] | yes | The list of agent calls. |
| `data[].id` | string | yes | The unique identifier for the agent call. |
| `data[].agent_id` | string | yes | The identifier of the agent associated with the call. |
| `data[].agent_name` | string |  | The name of the agent associated with the call. |
| `data[].agent_version_id` | string | yes | The [agent version] used for the call, which is the version published when the call started. |
| `data[].zdr_enabled` | boolean | yes | Whether [Zero Data Retention] was on for the call. When `true`, the transcript, recording, summary, and logs are not retained. |
| `data[].start_time` | string<date-time> |  | The start time of the agent call. |
| `data[].end_time` | string<date-time> |  | The end time of the agent call. |
| `data[].redacted_at` | string<date-time> |  | When the call's sensitive data was deleted, if it has been. Set by `DELETE /agents/calls/{call_id}`; deleted fields are omitted from responses. |
| `data[].transcript` | AgentTranscript[] |  | The transcript of the agent call. |
| `data[].transcript[].role` | string | yes | The role of the participant in the conversation. Roles are `user`, `assistant`, or `system`. `assistant` is the agent. |
| `data[].transcript[].text` | string |  | The text content of the transcript. This is the text that was spoken by the user or the agent. |
| `data[].transcript[].ignored` | boolean |  | Present and `true` on a `user` turn that ended while the agent's `initial_message` was playing with `initial_message_interruptions` set to `ignore`. The turn i… |
| `data[].transcript[].start_timestamp` | number<float> | yes | The start timestamp in seconds relative to the start of the call. |
| `data[].transcript[].end_timestamp` | number<float> | yes | The end timestamp in seconds relative to the start of the call. |
| `data[].transcript[].tool_calls` | ToolCall[] |  | The tool calls made during the turn. |
| `data[].transcript[].stt_ttfb` | number<float> |  | The time to first byte in seconds for speech-to-text. |
| `data[].transcript[].tts_ttfb` | number<float> |  | The time to first byte in seconds from the agent for text-to-speech. |
| `data[].telephony_params` | TelephonyParams |  | The telephony parameters associated with the call when the call is made via a phone. More details can also be returned depending on the provider. |
| `data[].telephony_params.to` | string |  | The phone number of the caller. Present for phone calls; omitted for websocket connections. |
| `data[].telephony_params.from` | string |  | The phone number of the agent. Present for phone calls; omitted for websocket connections. |
| `data[].telephony_params.call_sid` | string |  | The SID of the telephony call from the provider. |
| `data[].telephony_params.direction` | string |  | The direction of the call, e.g., `inbound` or `outbound`. |
| `data[].telephony_params.parameters` | object |  | Additional parameters associated with the telephony call. |
| `data[].telephony_params.headers` | object |  | Additional headers associated with the telephony call. |
| `data[].telephony_params.connection_type` | string enum(`websocket`, `phone`) |  | The type of connection used for the call. |
| `data[].telephony_account_type` | string enum(`cartesia`, `twilio`, `sip_trunk`) |  | The telephony account type backing the call. `cartesia` is the default Cartesia-hosted path; `twilio` indicates a customer-owned Twilio subaccount; `sip_trunk`… |
| `data[].summary` | string |  | A brief summary of the call, generated by Cartesia. Omitted when no summary was generated, e.g. when nobody spoke because the call was not answered or failed t… |
| `data[].status` | string enum(`created`, `started`, `completed`, `failed`) | yes | The status of an agent call. |
| `data[].error_message` | string |  | The error message, if any, associated with the call. |
| `data[].end_reason` | string enum(`agent_hangup`, `voicemail_detected`, `client_hangup`, `api_cancelled`, `max_duration`, `call_inactivity`, `client_inactivity`, `client_disconnected`, `dial_no_answer`, `dial_busy`, `dial_failed`, `dial_timeout` …(+5)) |  | A machine-readable enum indicating why a call ended. |
| `data[].dynamic_variables` | object |  | Initial custom values while the call is active; final values after it ends, including webhook assignments. System variables are not included. Omitted from ZDR … |
| `next_page` | string |  | An ID that can be passed as `starting_after` or `ending_before` to get the next page of calls. |
| `has_more` | boolean | yes | Whether there are more results available. |

- `400` Neither `agent_id` nor `batch_id` was supplied, an `agent_id` that does not match the batch's agent, an invalid pagination option, or a `start_time_gte` / `sta…
- `404` Agent or batch not found.

### `POST /agents/calls` — Create Outbound Call

- **Auth:** APIKeyAuth  ·  **operationId:** `agents_create-outbound-call`  ·  **docs page:** `api-reference/agents/calls/create-outbound-call.md`

**Request body** (`application/json`; type AgentOutboundCallRequest)

| Field | Type | Req | Description |
|---|---|---|---|
| `from_number_id` | string | yes | Phone number ID to place calls from. The attached provider handles outbound calling for this number. |
| `agent_id` | string | yes | Agent that handles the outbound calls. |
| `outbound_calls` | AgentOutboundCallItem[] | yes | Destinations and optional per-call dynamic variables |
| `outbound_calls[].to_number` | string | yes | Destination phone number for this outbound call, in E.164 format (e.g., `+14155559876`). |
| `outbound_calls[].dynamic_variables` | AgentDynamicVariableInputs |  | Custom values for this call. Supply values for variables used in the initial message. Names are case-sensitive, up to 64 characters, and use letters, digits, o… |
| `ringing_timeout_seconds` | integer |  | Seconds to wait for the callee to answer before giving up. Omit to use the default (60 seconds). |
| `max_call_duration_minutes` | integer |  | Optional maximum call duration in minutes. |

**Responses**

- `200` `application/json` → AgentOutboundCallResponse

| Field | Type | Req | Description |
|---|---|---|---|
| `calls` | object \| object[] | yes |  |
| `calls[].number` | string | yes | Destination phone number for this outbound call, in E.164 format (e.g., `+14155559876`). |
| `calls[].agent_call_id` | string |  | Agent call ID for tracking per-call information. This is absent when the request fails before Cartesia creates the call record. |
| `calls[].error` | object |  | Validation or dialing error for this destination. Other valid calls still proceed. |

- `400` Invalid request or call settings. Per-destination errors are returned in the HTTP 200 response.
- `403` The phone number does not belong to the same organization as the agent.
- `404` Agent, phone number, or deployment not found.
- `500` Outbound calling is temporarily unavailable, or provider setup failed.

### `GET /agents/calls/batches` — List Call Batches

- **Auth:** APIKeyAuth  ·  **operationId:** `agents_list-call-batches`  ·  **docs page:** `api-reference/agents/call-batches/list-call-batches.md`

**Parameters**

| Name | In | Type | Req | Description |
|---|---|---|---|---|
| `agent_id` | query | string |  | Filter to batches for this agent. |
| `starting_after` | query | string |  | (Pagination option) The ID of the batch to start after. |
| `ending_before` | query | string |  | (Pagination option) The ID of the batch to end before. |
| `limit` | query | integer |  | (Pagination option) The number of batches to return per page, ranging between 1 and 100. |

**Responses**

- `200` `application/json` → ListAgentCallBatchesResponse

| Field | Type | Req | Description |
|---|---|---|---|
| `data` | AgentCallBatch[] | yes | The list of batches, newest first. |
| `data[].id` | string | yes | The unique identifier for the batch. |
| `data[].name` | string | yes | The batch's label. |
| `data[].agent_id` | string | yes | The identifier of the agent that handles the batch's calls. |
| `data[].from_number_id` | string | yes | The identifier of the phone number the batch dials from. |
| `data[].region` | string enum(`US`, `EU`, `APAC`) | yes | The region that handles the batch. |
| `data[].target_concurrency_limit` | integer | yes | Maximum number of calls from this batch dialed concurrently. |
| `data[].status` | string enum(`pending`, `in_progress`, `completed`, `failed`, `cancelled`) | yes | The lifecycle status of a batch, derived at read time from dispatch progress. |
| `data[].total_calls_scheduled` | integer | yes | Total recipients queued in the batch. |
| `data[].total_calls_dispatched` | integer | yes | Recipients handed to the dialer so far, including those that failed before a call could be placed. |
| `data[].total_calls_finished` | integer | yes | Recipients whose latest call attempt reached a terminal state (completed or failed), including pre-dial failures. |
| `data[].retry_count` | integer | yes | Number of times the batch has been retried. `0` until the first retry. |
| `data[].created_at` | string<date-time> | yes | When the batch was created. |
| `data[].last_updated_at` | string<date-time> | yes | When the batch was last updated. |
| `data[].scheduled_at` | string<date-time> |  | The scheduled dispatch time, in RFC3339 UTC format. Omitted for batches that dispatch immediately. |
| `data[].admitted_at` | string<date-time> |  | The actual dispatch time, in RFC3339 UTC format. The batch may stay unadmitted in the queue due to scheduling or unavailable concurrency. |
| `data[].recipients` | AgentCallBatchRecipient[] |  | The batch's recipients. Returned only on `GET /agents/calls/batches/{batch_id}`. |
| `data[].recipients[].id` | string | yes | The unique identifier for the recipient within the batch. |
| `data[].recipients[].to_number` | string | yes | Destination phone number, in E.164 format (e.g., `+14155559876`). |
| `data[].recipients[].status` | string | yes | The status of a recipient within a batch. Before dispatch it reflects the queue state; after dispatch it reflects the latest call attempt. |
| `data[].recipients[].agent_call_id` | string |  | The agent call ID, used to fetch per-call information via [Get Call]. Set once the call has been dispatched. |
| `data[].recipients[].end_reason` | string enum(`agent_hangup`, `voicemail_detected`, `client_hangup`, `api_cancelled`, `max_duration`, `call_inactivity`, `client_inactivity`, `client_disconnected`, `dial_no_answer`, `dial_busy`, `dial_failed`, `dial_timeout` …(+5)) |  | A machine-readable enum indicating why a call ended. |
| `data[].recipients[].error_message` | string |  | Why the request failed before a call could be placed. Returned in place of `agent_call_id` when the recipient never reached a dial attempt. |
| `data[].recipients[].created_at` | string<date-time> | yes | When the recipient was queued. |
| `next_page` | string |  | An ID that can be passed as `starting_after` or `ending_before` to get the next page of batches. |
| `has_more` | boolean | yes | Whether there are more results available. |


### `POST /agents/calls/batches` — Create Call Batch

Queues a batch of outbound calls. Calls are issued gradually, subject to the organization's and the batch's concurrency limits.

- **Auth:** APIKeyAuth  ·  **operationId:** `agents_create-call-batch`  ·  **docs page:** `api-reference/agents/call-batches/create-call-batch.md`

**Request body** (`application/json`; type AgentCallBatchRequest)

| Field | Type | Req | Description |
|---|---|---|---|
| `name` | string | yes | A label for the batch. |
| `agent_id` | string | yes | The identifier of the agent that handles the batch's calls. |
| `from_number_id` | string | yes | The identifier of the phone number to place calls from. The attached provider handles outbound calling for this number. |
| `recipients` | AgentOutboundCallItem[] | yes | Destinations and optional per-call dynamic variables |
| `recipients[].to_number` | string | yes | Destination phone number for this outbound call, in E.164 format (e.g., `+14155559876`). |
| `recipients[].dynamic_variables` | AgentDynamicVariableInputs |  | Custom values for this call. Supply values for variables used in the initial message. Names are case-sensitive, up to 64 characters, and use letters, digits, o… |
| `target_concurrency_limit` | integer |  | Maximum number of calls from this batch to dial concurrently. Must not exceed the organization's concurrency limit. Omit to default to half of the organization… |
| `ringing_timeout_seconds` | integer |  | Seconds to wait for the callee to answer before giving up. Omit to use the default (60 seconds). |
| `max_call_duration_minutes` | integer |  | Maximum call duration in minutes. Omit to use the default (480 minutes). |
| `scheduled_at` | string<date-time> |  | When to start dispatching the batch, as an RFC3339 timestamp with a timezone offset (e.g. `2026-06-15T16:00:00Z`). Must be in the future and within 30 days. Om… |
| `region` | string enum(`US`, `EU`, `APAC`) |  | The region from which the batch's outbound calls are dispatched. Valid only when `from_number_id` is a SIP-trunk number; rejected for other telephony account t… |

**Responses**

- `201` `application/json` → AgentCallBatch

| Field | Type | Req | Description |
|---|---|---|---|
| `id` | string | yes | The unique identifier for the batch. |
| `name` | string | yes | The batch's label. |
| `agent_id` | string | yes | The identifier of the agent that handles the batch's calls. |
| `from_number_id` | string | yes | The identifier of the phone number the batch dials from. |
| `region` | string enum(`US`, `EU`, `APAC`) | yes | The region that handles the batch. |
| `target_concurrency_limit` | integer | yes | Maximum number of calls from this batch dialed concurrently. |
| `status` | string enum(`pending`, `in_progress`, `completed`, `failed`, `cancelled`) | yes | The lifecycle status of a batch, derived at read time from dispatch progress. |
| `total_calls_scheduled` | integer | yes | Total recipients queued in the batch. |
| `total_calls_dispatched` | integer | yes | Recipients handed to the dialer so far, including those that failed before a call could be placed. |
| `total_calls_finished` | integer | yes | Recipients whose latest call attempt reached a terminal state (completed or failed), including pre-dial failures. |
| `retry_count` | integer | yes | Number of times the batch has been retried. `0` until the first retry. |
| `created_at` | string<date-time> | yes | When the batch was created. |
| `last_updated_at` | string<date-time> | yes | When the batch was last updated. |
| `scheduled_at` | string<date-time> |  | The scheduled dispatch time, in RFC3339 UTC format. Omitted for batches that dispatch immediately. |
| `admitted_at` | string<date-time> |  | The actual dispatch time, in RFC3339 UTC format. The batch may stay unadmitted in the queue due to scheduling or unavailable concurrency. |
| `recipients` | AgentCallBatchRecipient[] |  | The batch's recipients. Returned only on `GET /agents/calls/batches/{batch_id}`. |
| `recipients[].id` | string | yes | The unique identifier for the recipient within the batch. |
| `recipients[].to_number` | string | yes | Destination phone number, in E.164 format (e.g., `+14155559876`). |
| `recipients[].status` | string | yes | The status of a recipient within a batch. Before dispatch it reflects the queue state; after dispatch it reflects the latest call attempt. |
| `recipients[].agent_call_id` | string |  | The agent call ID, used to fetch per-call information via [Get Call]. Set once the call has been dispatched. |
| `recipients[].end_reason` | string enum(`agent_hangup`, `voicemail_detected`, `client_hangup`, `api_cancelled`, `max_duration`, `call_inactivity`, `client_inactivity`, `client_disconnected`, `dial_no_answer`, `dial_busy`, `dial_failed`, `dial_timeout` …(+5)) |  | A machine-readable enum indicating why a call ended. |
| `recipients[].error_message` | string |  | Why the request failed before a call could be placed. Returned in place of `agent_call_id` when the recipient never reached a dial attempt. |
| `recipients[].created_at` | string<date-time> | yes | When the recipient was queued. |

- `400` Invalid request or recipient. Dynamic variable errors identify the first failing recipient as `recipients[index]`. No batch is created.
- `402` The organization has reached its agents usage limit. Upgrade the subscription or enable overages.
- `404` Agent or phone number not found.
- `500` The batch could not be created.

### `GET /agents/calls/batches/{batch_id}` — Get Call Batch

Retrieves a batch and the current status of each of its recipients.

- **Auth:** APIKeyAuth  ·  **operationId:** `agents_get-call-batch`  ·  **docs page:** `api-reference/agents/call-batches/get-call-batch.md`

**Parameters**

| Name | In | Type | Req | Description |
|---|---|---|---|---|
| `batch_id` | path | string | yes | The ID of the batch. |

**Responses**

- `200` `application/json` → AgentCallBatch

| Field | Type | Req | Description |
|---|---|---|---|
| `id` | string | yes | The unique identifier for the batch. |
| `name` | string | yes | The batch's label. |
| `agent_id` | string | yes | The identifier of the agent that handles the batch's calls. |
| `from_number_id` | string | yes | The identifier of the phone number the batch dials from. |
| `region` | string enum(`US`, `EU`, `APAC`) | yes | The region that handles the batch. |
| `target_concurrency_limit` | integer | yes | Maximum number of calls from this batch dialed concurrently. |
| `status` | string enum(`pending`, `in_progress`, `completed`, `failed`, `cancelled`) | yes | The lifecycle status of a batch, derived at read time from dispatch progress. |
| `total_calls_scheduled` | integer | yes | Total recipients queued in the batch. |
| `total_calls_dispatched` | integer | yes | Recipients handed to the dialer so far, including those that failed before a call could be placed. |
| `total_calls_finished` | integer | yes | Recipients whose latest call attempt reached a terminal state (completed or failed), including pre-dial failures. |
| `retry_count` | integer | yes | Number of times the batch has been retried. `0` until the first retry. |
| `created_at` | string<date-time> | yes | When the batch was created. |
| `last_updated_at` | string<date-time> | yes | When the batch was last updated. |
| `scheduled_at` | string<date-time> |  | The scheduled dispatch time, in RFC3339 UTC format. Omitted for batches that dispatch immediately. |
| `admitted_at` | string<date-time> |  | The actual dispatch time, in RFC3339 UTC format. The batch may stay unadmitted in the queue due to scheduling or unavailable concurrency. |
| `recipients` | AgentCallBatchRecipient[] |  | The batch's recipients. Returned only on `GET /agents/calls/batches/{batch_id}`. |
| `recipients[].id` | string | yes | The unique identifier for the recipient within the batch. |
| `recipients[].to_number` | string | yes | Destination phone number, in E.164 format (e.g., `+14155559876`). |
| `recipients[].status` | string | yes | The status of a recipient within a batch. Before dispatch it reflects the queue state; after dispatch it reflects the latest call attempt. |
| `recipients[].agent_call_id` | string |  | The agent call ID, used to fetch per-call information via [Get Call]. Set once the call has been dispatched. |
| `recipients[].end_reason` | string enum(`agent_hangup`, `voicemail_detected`, `client_hangup`, `api_cancelled`, `max_duration`, `call_inactivity`, `client_inactivity`, `client_disconnected`, `dial_no_answer`, `dial_busy`, `dial_failed`, `dial_timeout` …(+5)) |  | A machine-readable enum indicating why a call ended. |
| `recipients[].error_message` | string |  | Why the request failed before a call could be placed. Returned in place of `agent_call_id` when the recipient never reached a dial attempt. |
| `recipients[].created_at` | string<date-time> | yes | When the recipient was queued. |

- `404` Batch not found.

### `POST /agents/calls/batches/{batch_id}/cancel` — Cancel Call Batch

Stops dialing the batch's queued calls. Calls already in flight are left to finish. Idempotent.

- **Auth:** APIKeyAuth  ·  **operationId:** `agents_cancel-call-batch`  ·  **docs page:** `api-reference/agents/call-batches/cancel-call-batch.md`

**Parameters**

| Name | In | Type | Req | Description |
|---|---|---|---|---|
| `batch_id` | path | string | yes | The ID of the batch. |

**Responses**

- `200` `application/json` → AgentCallBatch

| Field | Type | Req | Description |
|---|---|---|---|
| `id` | string | yes | The unique identifier for the batch. |
| `name` | string | yes | The batch's label. |
| `agent_id` | string | yes | The identifier of the agent that handles the batch's calls. |
| `from_number_id` | string | yes | The identifier of the phone number the batch dials from. |
| `region` | string enum(`US`, `EU`, `APAC`) | yes | The region that handles the batch. |
| `target_concurrency_limit` | integer | yes | Maximum number of calls from this batch dialed concurrently. |
| `status` | string enum(`pending`, `in_progress`, `completed`, `failed`, `cancelled`) | yes | The lifecycle status of a batch, derived at read time from dispatch progress. |
| `total_calls_scheduled` | integer | yes | Total recipients queued in the batch. |
| `total_calls_dispatched` | integer | yes | Recipients handed to the dialer so far, including those that failed before a call could be placed. |
| `total_calls_finished` | integer | yes | Recipients whose latest call attempt reached a terminal state (completed or failed), including pre-dial failures. |
| `retry_count` | integer | yes | Number of times the batch has been retried. `0` until the first retry. |
| `created_at` | string<date-time> | yes | When the batch was created. |
| `last_updated_at` | string<date-time> | yes | When the batch was last updated. |
| `scheduled_at` | string<date-time> |  | The scheduled dispatch time, in RFC3339 UTC format. Omitted for batches that dispatch immediately. |
| `admitted_at` | string<date-time> |  | The actual dispatch time, in RFC3339 UTC format. The batch may stay unadmitted in the queue due to scheduling or unavailable concurrency. |
| `recipients` | AgentCallBatchRecipient[] |  | The batch's recipients. Returned only on `GET /agents/calls/batches/{batch_id}`. |
| `recipients[].id` | string | yes | The unique identifier for the recipient within the batch. |
| `recipients[].to_number` | string | yes | Destination phone number, in E.164 format (e.g., `+14155559876`). |
| `recipients[].status` | string | yes | The status of a recipient within a batch. Before dispatch it reflects the queue state; after dispatch it reflects the latest call attempt. |
| `recipients[].agent_call_id` | string |  | The agent call ID, used to fetch per-call information via [Get Call]. Set once the call has been dispatched. |
| `recipients[].end_reason` | string enum(`agent_hangup`, `voicemail_detected`, `client_hangup`, `api_cancelled`, `max_duration`, `call_inactivity`, `client_inactivity`, `client_disconnected`, `dial_no_answer`, `dial_busy`, `dial_failed`, `dial_timeout` …(+5)) |  | A machine-readable enum indicating why a call ended. |
| `recipients[].error_message` | string |  | Why the request failed before a call could be placed. Returned in place of `agent_call_id` when the recipient never reached a dial attempt. |
| `recipients[].created_at` | string<date-time> | yes | When the recipient was queued. |

- `404` Batch not found.

### `POST /agents/calls/batches/{batch_id}/retry` — Retry Call Batch

Re-queues the batch's recipients whose latest call attempt failed or went unanswered, so the dispatcher dials them again. Recipients that successfully completed their call are left untouched. A cancelled batch cannot be retried.

- **Auth:** APIKeyAuth  ·  **operationId:** `agents_retry-call-batch`  ·  **docs page:** `api-reference/agents/call-batches/retry-call-batch.md`

**Parameters**

| Name | In | Type | Req | Description |
|---|---|---|---|---|
| `batch_id` | path | string | yes | The ID of the batch. |

**Responses**

- `200` `application/json` → AgentCallBatch

| Field | Type | Req | Description |
|---|---|---|---|
| `id` | string | yes | The unique identifier for the batch. |
| `name` | string | yes | The batch's label. |
| `agent_id` | string | yes | The identifier of the agent that handles the batch's calls. |
| `from_number_id` | string | yes | The identifier of the phone number the batch dials from. |
| `region` | string enum(`US`, `EU`, `APAC`) | yes | The region that handles the batch. |
| `target_concurrency_limit` | integer | yes | Maximum number of calls from this batch dialed concurrently. |
| `status` | string enum(`pending`, `in_progress`, `completed`, `failed`, `cancelled`) | yes | The lifecycle status of a batch, derived at read time from dispatch progress. |
| `total_calls_scheduled` | integer | yes | Total recipients queued in the batch. |
| `total_calls_dispatched` | integer | yes | Recipients handed to the dialer so far, including those that failed before a call could be placed. |
| `total_calls_finished` | integer | yes | Recipients whose latest call attempt reached a terminal state (completed or failed), including pre-dial failures. |
| `retry_count` | integer | yes | Number of times the batch has been retried. `0` until the first retry. |
| `created_at` | string<date-time> | yes | When the batch was created. |
| `last_updated_at` | string<date-time> | yes | When the batch was last updated. |
| `scheduled_at` | string<date-time> |  | The scheduled dispatch time, in RFC3339 UTC format. Omitted for batches that dispatch immediately. |
| `admitted_at` | string<date-time> |  | The actual dispatch time, in RFC3339 UTC format. The batch may stay unadmitted in the queue due to scheduling or unavailable concurrency. |
| `recipients` | AgentCallBatchRecipient[] |  | The batch's recipients. Returned only on `GET /agents/calls/batches/{batch_id}`. |
| `recipients[].id` | string | yes | The unique identifier for the recipient within the batch. |
| `recipients[].to_number` | string | yes | Destination phone number, in E.164 format (e.g., `+14155559876`). |
| `recipients[].status` | string | yes | The status of a recipient within a batch. Before dispatch it reflects the queue state; after dispatch it reflects the latest call attempt. |
| `recipients[].agent_call_id` | string |  | The agent call ID, used to fetch per-call information via [Get Call]. Set once the call has been dispatched. |
| `recipients[].end_reason` | string enum(`agent_hangup`, `voicemail_detected`, `client_hangup`, `api_cancelled`, `max_duration`, `call_inactivity`, `client_inactivity`, `client_disconnected`, `dial_no_answer`, `dial_busy`, `dial_failed`, `dial_timeout` …(+5)) |  | A machine-readable enum indicating why a call ended. |
| `recipients[].error_message` | string |  | Why the request failed before a call could be placed. Returned in place of `agent_call_id` when the recipient never reached a dial attempt. |
| `recipients[].created_at` | string<date-time> | yes | When the recipient was queued. |

- `400` Batch is cancelled and cannot be retried.
- `404` Batch not found.

### `DELETE /agents/calls/{call_id}` — Delete Call

Deletes the sensitive data for a call, including the transcript, audio recordings, and logs. The call record is retained with non-sensitive operational metadata; deleted fields are omitted from subsequent API responses, and `redacted_at` marks when deletion occurred. Deletion cannot be undone. Only completed or failed calls can be deleted. Cancel an active call first via `POST /agents/calls/{call_id}/cancel`.

- **Auth:** APIKeyAuth  ·  **operationId:** `agents_delete-call`  ·  **docs page:** `api-reference/agents/calls/delete-call.md`

**Parameters**

| Name | In | Type | Req | Description |
|---|---|---|---|---|
| `call_id` | path | string | yes | The ID of the call. |

**Responses**

- `200` `application/json` → AgentCall

| Field | Type | Req | Description |
|---|---|---|---|
| `id` | string | yes | The unique identifier for the agent call. |
| `agent_id` | string | yes | The identifier of the agent associated with the call. |
| `agent_name` | string |  | The name of the agent associated with the call. |
| `agent_version_id` | string | yes | The [agent version] used for the call, which is the version published when the call started. |
| `zdr_enabled` | boolean | yes | Whether [Zero Data Retention] was on for the call. When `true`, the transcript, recording, summary, and logs are not retained. |
| `start_time` | string<date-time> |  | The start time of the agent call. |
| `end_time` | string<date-time> |  | The end time of the agent call. |
| `redacted_at` | string<date-time> |  | When the call's sensitive data was deleted, if it has been. Set by `DELETE /agents/calls/{call_id}`; deleted fields are omitted from responses. |
| `transcript` | AgentTranscript[] |  | The transcript of the agent call. |
| `transcript[].role` | string | yes | The role of the participant in the conversation. Roles are `user`, `assistant`, or `system`. `assistant` is the agent. |
| `transcript[].text` | string |  | The text content of the transcript. This is the text that was spoken by the user or the agent. |
| `transcript[].ignored` | boolean |  | Present and `true` on a `user` turn that ended while the agent's `initial_message` was playing with `initial_message_interruptions` set to `ignore`. The turn i… |
| `transcript[].start_timestamp` | number<float> | yes | The start timestamp in seconds relative to the start of the call. |
| `transcript[].end_timestamp` | number<float> | yes | The end timestamp in seconds relative to the start of the call. |
| `transcript[].tool_calls` | ToolCall[] |  | The tool calls made during the turn. |
| `transcript[].tool_calls[].id` | string |  | The unique identifier for the tool call. |
| `transcript[].tool_calls[].name` | string | yes | The name of the tool that was called. |
| `transcript[].tool_calls[].arguments` | object | yes | Arguments supplied by the model. For webhook tools, this excludes values filled from dynamic variables or constants. |
| `transcript[].tool_calls[].result` | string |  | The result returned by the tool. |
| `transcript[].tool_calls[].dynamic_variable_updates` | DynamicVariableUpdate[] |  | Values assigned by this tool. Omitted when the tool assigned no values. |
| `transcript[].stt_ttfb` | number<float> |  | The time to first byte in seconds for speech-to-text. |
| `transcript[].tts_ttfb` | number<float> |  | The time to first byte in seconds from the agent for text-to-speech. |
| `telephony_params` | TelephonyParams |  | The telephony parameters associated with the call when the call is made via a phone. More details can also be returned depending on the provider. |
| `telephony_params.to` | string |  | The phone number of the caller. Present for phone calls; omitted for websocket connections. |
| `telephony_params.from` | string |  | The phone number of the agent. Present for phone calls; omitted for websocket connections. |
| `telephony_params.call_sid` | string |  | The SID of the telephony call from the provider. |
| `telephony_params.direction` | string |  | The direction of the call, e.g., `inbound` or `outbound`. |
| `telephony_params.parameters` | object |  | Additional parameters associated with the telephony call. |
| `telephony_params.headers` | object |  | Additional headers associated with the telephony call. |
| `telephony_params.connection_type` | string enum(`websocket`, `phone`) |  | The type of connection used for the call. |
| `telephony_account_type` | string enum(`cartesia`, `twilio`, `sip_trunk`) |  | The telephony account type backing the call. `cartesia` is the default Cartesia-hosted path; `twilio` indicates a customer-owned Twilio subaccount; `sip_trunk`… |
| `summary` | string |  | A brief summary of the call, generated by Cartesia. Omitted when no summary was generated, e.g. when nobody spoke because the call was not answered or failed t… |
| `status` | string enum(`created`, `started`, `completed`, `failed`) | yes | The status of an agent call. |
| `error_message` | string |  | The error message, if any, associated with the call. |
| `end_reason` | string enum(`agent_hangup`, `voicemail_detected`, `client_hangup`, `api_cancelled`, `max_duration`, `call_inactivity`, `client_inactivity`, `client_disconnected`, `dial_no_answer`, `dial_busy`, `dial_failed`, `dial_timeout` …(+5)) |  | A machine-readable enum indicating why a call ended. |
| `dynamic_variables` | object |  | Initial custom values while the call is active; final values after it ends, including webhook assignments. System variables are not included. Omitted from ZDR … |

- `400` Call is not in a terminal state. Only completed or failed calls can be deleted; cancel an active call first via `POST /agents/calls/{call_id}/cancel`.
- `404` Call not found.
- `500` Deletion did not complete. Retry the request; deletion is idempotent.

### `GET /agents/calls/{call_id}` — Get Call

- **Auth:** APIKeyAuth  ·  **operationId:** `agents_get-call`  ·  **docs page:** `api-reference/agents/calls/get-call.md`

**Parameters**

| Name | In | Type | Req | Description |
|---|---|---|---|---|
| `call_id` | path | string | yes | The ID of the call. |

**Responses**

- `200` `application/json` → AgentCall

| Field | Type | Req | Description |
|---|---|---|---|
| `id` | string | yes | The unique identifier for the agent call. |
| `agent_id` | string | yes | The identifier of the agent associated with the call. |
| `agent_name` | string |  | The name of the agent associated with the call. |
| `agent_version_id` | string | yes | The [agent version] used for the call, which is the version published when the call started. |
| `zdr_enabled` | boolean | yes | Whether [Zero Data Retention] was on for the call. When `true`, the transcript, recording, summary, and logs are not retained. |
| `start_time` | string<date-time> |  | The start time of the agent call. |
| `end_time` | string<date-time> |  | The end time of the agent call. |
| `redacted_at` | string<date-time> |  | When the call's sensitive data was deleted, if it has been. Set by `DELETE /agents/calls/{call_id}`; deleted fields are omitted from responses. |
| `transcript` | AgentTranscript[] |  | The transcript of the agent call. |
| `transcript[].role` | string | yes | The role of the participant in the conversation. Roles are `user`, `assistant`, or `system`. `assistant` is the agent. |
| `transcript[].text` | string |  | The text content of the transcript. This is the text that was spoken by the user or the agent. |
| `transcript[].ignored` | boolean |  | Present and `true` on a `user` turn that ended while the agent's `initial_message` was playing with `initial_message_interruptions` set to `ignore`. The turn i… |
| `transcript[].start_timestamp` | number<float> | yes | The start timestamp in seconds relative to the start of the call. |
| `transcript[].end_timestamp` | number<float> | yes | The end timestamp in seconds relative to the start of the call. |
| `transcript[].tool_calls` | ToolCall[] |  | The tool calls made during the turn. |
| `transcript[].tool_calls[].id` | string |  | The unique identifier for the tool call. |
| `transcript[].tool_calls[].name` | string | yes | The name of the tool that was called. |
| `transcript[].tool_calls[].arguments` | object | yes | Arguments supplied by the model. For webhook tools, this excludes values filled from dynamic variables or constants. |
| `transcript[].tool_calls[].result` | string |  | The result returned by the tool. |
| `transcript[].tool_calls[].dynamic_variable_updates` | DynamicVariableUpdate[] |  | Values assigned by this tool. Omitted when the tool assigned no values. |
| `transcript[].stt_ttfb` | number<float> |  | The time to first byte in seconds for speech-to-text. |
| `transcript[].tts_ttfb` | number<float> |  | The time to first byte in seconds from the agent for text-to-speech. |
| `telephony_params` | TelephonyParams |  | The telephony parameters associated with the call when the call is made via a phone. More details can also be returned depending on the provider. |
| `telephony_params.to` | string |  | The phone number of the caller. Present for phone calls; omitted for websocket connections. |
| `telephony_params.from` | string |  | The phone number of the agent. Present for phone calls; omitted for websocket connections. |
| `telephony_params.call_sid` | string |  | The SID of the telephony call from the provider. |
| `telephony_params.direction` | string |  | The direction of the call, e.g., `inbound` or `outbound`. |
| `telephony_params.parameters` | object |  | Additional parameters associated with the telephony call. |
| `telephony_params.headers` | object |  | Additional headers associated with the telephony call. |
| `telephony_params.connection_type` | string enum(`websocket`, `phone`) |  | The type of connection used for the call. |
| `telephony_account_type` | string enum(`cartesia`, `twilio`, `sip_trunk`) |  | The telephony account type backing the call. `cartesia` is the default Cartesia-hosted path; `twilio` indicates a customer-owned Twilio subaccount; `sip_trunk`… |
| `summary` | string |  | A brief summary of the call, generated by Cartesia. Omitted when no summary was generated, e.g. when nobody spoke because the call was not answered or failed t… |
| `status` | string enum(`created`, `started`, `completed`, `failed`) | yes | The status of an agent call. |
| `error_message` | string |  | The error message, if any, associated with the call. |
| `end_reason` | string enum(`agent_hangup`, `voicemail_detected`, `client_hangup`, `api_cancelled`, `max_duration`, `call_inactivity`, `client_inactivity`, `client_disconnected`, `dial_no_answer`, `dial_busy`, `dial_failed`, `dial_timeout` …(+5)) |  | A machine-readable enum indicating why a call ended. |
| `dynamic_variables` | object |  | Initial custom values while the call is active; final values after it ends, including webhook assignments. System variables are not included. Omitted from ZDR … |


### `GET /agents/calls/{call_id}/audio` — Download Call Audio

- **Auth:** APIKeyAuth  ·  **operationId:** `agents_download-call-audio`  ·  **docs page:** `api-reference/agents/calls/download-call-audio.md`

**Parameters**

| Name | In | Type | Req | Description |
|---|---|---|---|---|
| `call_id` | path | string | yes | The ID of the call. |

**Responses**

- `200` `audio/wav` → string<binary>

### `POST /agents/calls/{call_id}/cancel` — Cancel Call

Terminates an active call. Only calls in the `started` status can be cancelled; the call ends with end reason `api_cancelled`. Cancelling ends the call but does not delete its data. To delete a call's sensitive data after it ends, use `DELETE /agents/calls/{call_id}`.

- **Auth:** APIKeyAuth  ·  **operationId:** `agents_cancel-call`  ·  **docs page:** `api-reference/agents/calls/cancel-call.md`

**Parameters**

| Name | In | Type | Req | Description |
|---|---|---|---|---|
| `call_id` | path | string | yes | The ID of the call. |

**Responses**

- `200` `application/json` → object

| Field | Type | Req | Description |
|---|---|---|---|
| `cancelled` | boolean | yes | Always `true` when the cancel is confirmed. |

- `400` The call is not currently active and cannot be cancelled.
- `404` Call not found, or no active connection was found for the call.
- `502` The cancel request could not be delivered. Retry the request.

### `GET /agents/metrics` — List Metrics

List of all LLM-as-a-Judge metrics owned by your account.

- **Auth:** APIKeyAuth  ·  **operationId:** `agents_list-metrics`  ·  **docs page:** `api-reference/agents/metrics/list-metrics.md`

**Parameters**

| Name | In | Type | Req | Description |
|---|---|---|---|---|
| `starting_after` | query | string |  | (Pagination option) The ID of the last Metric in the current response as a cursor for the next page of results. |
| `limit` | query | integer |  | (Pagination option) The number of metrics to return per page, ranging between 1 and 100. The default page limit is 10. |

**Responses**

- `200` `application/json` → ListMetricsResponse

| Field | Type | Req | Description |
|---|---|---|---|
| `data` | Metric[] | yes | List of metrics. |
| `data[].id` | string | yes | The unique identifier for the metric. |
| `data[].name` | string | yes | The name of the metric. This is a unique name that you can use to identify the metric in the CLI. |
| `data[].display_name` | string |  | The display name of the metric, if available. This is the name that is displayed in the Playground. |
| `data[].prompt` | string | yes | The prompt associated with the metric, detailing the task and evaluation criteria. |
| `data[].created_at` | string<date-time> | yes | The timestamp when the metric was created. |
| `has_more` | boolean | yes | Whether there are more metrics to fetch. |
| `next_page` | string |  | An ID that can be passed as `starting_after` or `ending_before` to get the next page of metrics. |


### `POST /agents/metrics` — Create Metric

Create a new metric.

- **Auth:** APIKeyAuth  ·  **operationId:** `agents_create-metric`  ·  **docs page:** `api-reference/agents/metrics/create-metric.md`

**Request body** (`application/json`; type CreateMetricRequest)

| Field | Type | Req | Description |
|---|---|---|---|
| `name` | string | yes | The name of the metric. This must be a unique name that only allows lower case letters, numbers, and the characters _, -, and . |
| `display_name` | string |  | The display name of the metric. |
| `prompt` | string | yes | The prompt associated with the metric, detailing the task and evaluation criteria. |

**Responses**

- `200` `application/json` → Metric

| Field | Type | Req | Description |
|---|---|---|---|
| `id` | string | yes | The unique identifier for the metric. |
| `name` | string | yes | The name of the metric. This is a unique name that you can use to identify the metric in the CLI. |
| `display_name` | string |  | The display name of the metric, if available. This is the name that is displayed in the Playground. |
| `prompt` | string | yes | The prompt associated with the metric, detailing the task and evaluation criteria. |
| `created_at` | string<date-time> | yes | The timestamp when the metric was created. |


### `GET /agents/metrics/results` — List Metric Results

- **Auth:** APIKeyAuth  ·  **operationId:** `agents_list-metric-results`  ·  **docs page:** `api-reference/agents/metrics/list-metric-results.md`

**Parameters**

| Name | In | Type | Req | Description |
|---|---|---|---|---|
| `agent_id` | query | string |  | The ID of the agent. |
| `deployment_id` | query | string |  | The ID of the deployment. |
| `metric_id` | query | string |  | The ID of the metric. |
| `start_date` | query | string<date-time> |  | Filter metric results created at or after this RFC 3339 datetime. |
| `end_date` | query | string<date-time> |  | Filter metric results created before or at this RFC 3339 datetime. |
| `call_id` | query | string |  | The ID of the call. |
| `starting_after` | query | string |  | A cursor to use in pagination. `starting_after` is a metric result ID that defines your place in the list. For example, if you make a /metrics/results request … |
| `ending_before` | query | string |  | A cursor to use in pagination. `ending_before` is a metric result ID that defines your place in the list. For example, if you make a /metrics/results request a… |
| `limit` | query | integer |  | The number of metric results to return per page, ranging between 1 and 100. |

**Responses**

- `200` `application/json` → ListMetricResultsResponse

| Field | Type | Req | Description |
|---|---|---|---|
| `data` | MetricResult[] | yes | List of metric results. |
| `data[].id` | string | yes | The unique identifier for the metric result. |
| `data[].metricId` | string | yes | The identifier of the metric being measured. |
| `data[].metricName` | string | yes | The name of the metric being measured. |
| `data[].metricDisplayName` | string |  | The display name of the metric, if available. |
| `data[].summary` | string | yes | A summary of the transcript of the call. Null when no summary was generated. |
| `data[].transcript` | AgentTranscript[] |  | The transcript of the call. |
| `data[].transcript[].role` | string | yes | The role of the participant in the conversation. Roles are `user`, `assistant`, or `system`. `assistant` is the agent. |
| `data[].transcript[].text` | string |  | The text content of the transcript. This is the text that was spoken by the user or the agent. |
| `data[].transcript[].ignored` | boolean |  | Present and `true` on a `user` turn that ended while the agent's `initial_message` was playing with `initial_message_interruptions` set to `ignore`. The turn i… |
| `data[].transcript[].start_timestamp` | number<float> | yes | The start timestamp in seconds relative to the start of the call. |
| `data[].transcript[].end_timestamp` | number<float> | yes | The end timestamp in seconds relative to the start of the call. |
| `data[].transcript[].tool_calls` | ToolCall[] |  | The tool calls made during the turn. |
| `data[].transcript[].stt_ttfb` | number<float> |  | The time to first byte in seconds for speech-to-text. |
| `data[].transcript[].tts_ttfb` | number<float> |  | The time to first byte in seconds from the agent for text-to-speech. |
| `data[].agentId` | string | yes | The identifier of the agent associated with the metric result. |
| `data[].callId` | string | yes | The identifier of the call associated with the metric result. |
| `data[].result` | string | yes | The raw result of the metric in a string format. |
| `data[].jsonResult` | object |  | The structured JSON result of the metric. |
| `data[].value` | object |  | The value of the metric result. |
| `data[].status` | string enum(`completed`, `failed`) | yes | The status of an agent metric result. `failed` indicates that the metric result was not computed or parsed properly. |
| `data[].runId` | string |  | The identifier of the run associated with the metric result, if applicable. |
| `data[].createdAt` | string<date-time> | yes | The UTC timestamp when the metric result was created. |
| `has_more` | boolean | yes | Whether there are more metric results to fetch. |
| `next_page` | string |  | An ID that can be passed as `starting_after` or `ending_before` to get the next page of metric results. |


### `GET /agents/metrics/results/export` — Export Metric Results

Exports metric results as a CSV file. By default, with no pagination parameters, the endpoint streams the entire matching result set as a single CSV. To page through results instead, pass `limit` and `starting_after` / `ending_before`. Paginated responses include the `X-Has-More` and `X-Next-Cursor` headers. Up to 100,000 rows can be exported per request in either mode; requests whose total matching set exceeds this cap return a `400` error.

- **Auth:** APIKeyAuth  ·  **operationId:** `agents_export-metric-results`  ·  **docs page:** `api-reference/agents/metrics/export-metric-results.md`

**Parameters**

| Name | In | Type | Req | Description |
|---|---|---|---|---|
| `agent_id` | query | string |  | The ID of the agent. |
| `deployment_id` | query | string |  | The ID of the deployment. |
| `metric_id` | query | string |  | The ID of the metric. |
| `call_id` | query | string |  | The ID of the call. |
| `start_date` | query | string<date-time> |  | Filter metric results created at or after this RFC 3339 datetime. |
| `end_date` | query | string<date-time> |  | Filter metric results created before or at this RFC 3339 datetime. |
| `limit` | query | integer |  | The maximum number of rows to return in this CSV page, between 1 and 1000. Passing this parameter enables paginated mode; omit it (along with `starting_after` … |
| `starting_after` | query | string |  | An opaque pagination cursor. Returns the page of results after this cursor. Pass the `X-Next-Cursor` value from the previous page to fetch the next page. Treat… |
| `ending_before` | query | string |  | An opaque pagination cursor. Returns the page of results before this cursor. Treat the cursor as an opaque string. Cannot be combined with `starting_after`. |

**Responses**

- `200` `text/csv` → string<binary>
- `400` More than 100k metric results found for the given filters. Please narrow down the filters to export less than 100k results.
- `404` No metric results found for the given filters.

### `DELETE /agents/metrics/{metric_id}` — Delete Metric

Delete a metric by its ID. The metric is removed from all agents it is attached to. Existing metric results are retained.

- **Auth:** APIKeyAuth  ·  **operationId:** `agents_delete-metric`  ·  **docs page:** `api-reference/agents/metrics/delete-metric.md`

**Parameters**

| Name | In | Type | Req | Description |
|---|---|---|---|---|
| `metric_id` | path | string | yes | The ID of the metric. |

**Responses**

- `204` 

### `GET /agents/metrics/{metric_id}` — Get Metric

Get a metric by its ID.

- **Auth:** APIKeyAuth  ·  **operationId:** `agents_get-metric`  ·  **docs page:** `api-reference/agents/metrics/get-metric.md`

**Parameters**

| Name | In | Type | Req | Description |
|---|---|---|---|---|
| `metric_id` | path | string | yes | The ID of the metric. |

**Responses**

- `200` `application/json` → Metric

| Field | Type | Req | Description |
|---|---|---|---|
| `id` | string | yes | The unique identifier for the metric. |
| `name` | string | yes | The name of the metric. This is a unique name that you can use to identify the metric in the CLI. |
| `display_name` | string |  | The display name of the metric, if available. This is the name that is displayed in the Playground. |
| `prompt` | string | yes | The prompt associated with the metric, detailing the task and evaluation criteria. |
| `created_at` | string<date-time> | yes | The timestamp when the metric was created. |


### `GET /agents/phone-numbers` — List Phone Numbers

- **Auth:** APIKeyAuth  ·  **operationId:** `phone_numbers_list`  ·  **docs page:** `api-reference/agents/phone-numbers/list.md`

**Parameters**

| Name | In | Type | Req | Description |
|---|---|---|---|---|
| `agent_id` | query | string |  | Filter by assigned agent ID. |
| `provider_id` | query | string |  | Filter by provider ID. |
| `type` | query | string enum(`twilio`, `cartesia`, `sip_trunk`) |  | Filter by provider type. |
| `q` | query | string |  | Search by phone number, label, or agent name. |
| `starting_after` | query | string |  | A cursor to use in pagination. `starting_after` is a phone number ID that defines your place in the list. For example, if you make a `/agents/phone-numbers` re… |
| `ending_before` | query | string |  | A cursor to use in pagination. `ending_before` is a phone number ID that defines your place in the list. For example, if you make a `/agents/phone-numbers` req… |
| `limit` | query | integer |  | (Pagination option) The number of phone numbers to return per page, ranging between 1 and 100. The default page limit is 10. |

**Responses**

- `200` `application/json` → object

| Field | Type | Req | Description |
|---|---|---|---|
| `data` | PhoneNumberResponse[] | yes |  |
| `data[].id` | string | yes | Unique identifier for the phone number. |
| `data[].label` | string |  | A human-readable name for the phone number. |
| `data[].number` | string | yes | The phone number in E.164 format (e.g. +14155551234). |
| `data[].agent` | object | yes | The agent that answers inbound calls to this number, or `null` if unassigned |
| `data[].agent.id` | string | yes |  |
| `data[].agent.name` | string | yes |  |
| `data[].created_at` | string<date-time> | yes | UTC timestamp when the phone number was created. |
| `data[].updated_at` | string<date-time> | yes | UTC timestamp when the phone number was last updated. |
| `data[].provider` | Twilio \| Cartesia \| SIP Trunk | yes | The telephony provider associated with a phone number. One of `Twilio`, `Cartesia`, or `SIP Trunk`, determined by the `type` field. For SIP trunk numbers, `inb… |
| `has_more` | boolean | yes |  |


### `POST /agents/phone-numbers` — Import Phone Number

- **Auth:** APIKeyAuth  ·  **operationId:** `phone_numbers_import`  ·  **docs page:** `api-reference/agents/phone-numbers/import.md`

**Request body** (`application/json`; type ImportPhoneNumberBody)

| Field | Type | Req | Description |
|---|---|---|---|
| `label` | string | yes | A human-readable name for the phone number. |
| `number` | string | yes | The phone number in E.164 format. |
| `provider` | By ID \| By Twilio Account | yes | Reference to the provider. Pass `{ "id": "..." }` for an existing provider, or `{ "type": "twilio", "account_sid": "...", "region": "..." }` to look up by acco… |
| `agent_id` | string |  | Optionally assign an agent to answer inbound calls to this number. Omit to keep the number outbound-only. |

**Responses**

- `201` `application/json` → PhoneNumberResponse

| Field | Type | Req | Description |
|---|---|---|---|
| `id` | string | yes | Unique identifier for the phone number. |
| `label` | string |  | A human-readable name for the phone number. |
| `number` | string | yes | The phone number in E.164 format (e.g. +14155551234). |
| `agent` | object | yes | The agent that answers inbound calls to this number, or `null` if unassigned |
| `agent.id` | string | yes |  |
| `agent.name` | string | yes |  |
| `created_at` | string<date-time> | yes | UTC timestamp when the phone number was created. |
| `updated_at` | string<date-time> | yes | UTC timestamp when the phone number was last updated. |
| `provider` | Twilio \| Cartesia \| SIP Trunk | yes | The telephony provider associated with a phone number. One of `Twilio`, `Cartesia`, or `SIP Trunk`, determined by the `type` field. For SIP trunk numbers, `inb… |

- `400` Import failed. Possible reasons: phone number not found in the provider account, provider lookup failed, provider credentials incomplete, webhook configuration…
- `404` Provider or agent not found.
- `409` Phone number has already been imported.

### `GET /agents/phone-numbers/providers` — List Providers

List telephony provider accounts linked to your account. Supports Twilio and SIP Trunk providers.

- **Auth:** APIKeyAuth  ·  **operationId:** `providers_list`  ·  **docs page:** `api-reference/agents/providers/list.md`

**Parameters**

| Name | In | Type | Req | Description |
|---|---|---|---|---|
| `type` | query | string enum(`twilio`, `sip_trunk`) |  | Filter by provider type. |
| `account_sid` | query | string |  | Filter by account SID (Twilio providers only). |

**Responses**

- `200` `application/json` → Twilio \| SIP Trunk[]

### `POST /agents/phone-numbers/providers` — Create Provider

Link a telephony provider account. For Twilio, credentials are validated before the provider is created. Each account SID + region combination can only be linked once. For SIP trunks, configure a trunk with an inbound and/or outbound configuration.

- **Auth:** APIKeyAuth  ·  **operationId:** `providers_create`  ·  **docs page:** `api-reference/agents/providers/create.md`

**Request body** (`application/json`; type Twilio \| object \| object)

_Variant: Twilio_

| Field | Type | Req | Description |
|---|---|---|---|
| `type` | string enum(`twilio`) | yes | Must be `"twilio"`. |
| `account_sid` | string | yes | Your Twilio account SID. |
| `api_key_sid` | string | yes | A Twilio API key SID. |
| `api_key_secret` | string<password> | yes | The corresponding API key secret. |
| `region` | string enum(`us1`, `ie1`, `au1`) |  | The Twilio region the phone number and API key are configured for. `us1` (US), `ie1` (Ireland), `au1` (Australia). Default is `us1`. (default `"us1"`) |
| `label` | string |  | An optional human-readable label for the provider. |

_Variant: object \| object_

| Field | Type | Req | Description |
|---|---|---|---|
| `type` | string enum(`sip_trunk`) | yes | Must be `"sip_trunk"`. |
| `label` | string | yes | A human-readable label for the provider. |
| `inbound` | InboundSIPTrunkConfig |  | Inbound SIP configuration for creating a trunk. |
| `inbound.credentials` | SIPCredentials |  | SIP digest authentication credentials. |
| `inbound.credentials.username` | string | yes |  |
| `inbound.credentials.password` | string<password> | yes |  |
| `inbound.allowed_addresses` | string[] |  | Source IP addresses or CIDR ranges allowed to send SIP traffic. |
| `inbound.allowed_numbers` | string[] |  | Caller numbers allowed on inbound calls. |
| `inbound.media_encryption` | string enum(`disabled`, `allowed`, `required`) |  | Media encryption policy. Defaults to `allowed`. |
| `outbound` | OutboundSIPTrunkConfig |  | Outbound SIP configuration for creating a trunk. |
| `outbound.address` | string | yes | Carrier host that receives Cartesia's SIP INVITE, as `host` or `host:port`, without a `sip:` prefix. When the port is omitted, Cartesia uses 5060 for `tcp` and… |
| `outbound.transport` | string enum(`tcp`, `tls`) | yes | Transport for outbound SIP signaling. |
| `outbound.destination_country` | string |  | Two-letter country code for the country the call terminates in (e.g. `US`, `GB`, `DE`). See [ISO 3166-1 alpha-2]. |
| `outbound.media_encryption` | string enum(`disabled`, `allowed`, `required`) |  | Media encryption policy. Defaults to `disabled`. |
| `outbound.credentials` | SIPCredentials |  | SIP digest authentication credentials. |
| `outbound.credentials.username` | string | yes |  |
| `outbound.credentials.password` | string<password> | yes |  |

**Responses**

- `201` `application/json` → Twilio \| SIP Trunk
- `400` Invalid provider credentials or SIP trunk configuration.
- `409` A Twilio provider already exists for this account and region. Use PATCH to update the provider.

### `DELETE /agents/phone-numbers/providers/{id}` — Delete Provider

Delete a provider account

- **Auth:** APIKeyAuth  ·  **operationId:** `providers_delete`  ·  **docs page:** `api-reference/agents/providers/delete.md`

**Parameters**

| Name | In | Type | Req | Description |
|---|---|---|---|---|
| `id` | path | string | yes | The provider ID. Delete linked phone numbers before deleting the provider. |

**Responses**

- `204` Provider deleted.
- `404` Provider not found.
- `409` Provider has phone numbers. Remove all phone numbers using this provider before deleting it.

### `GET /agents/phone-numbers/providers/{id}` — Get Provider

Returns the details of a specific provider account.

- **Auth:** APIKeyAuth  ·  **operationId:** `providers_get`  ·  **docs page:** `api-reference/agents/providers/get.md`

**Parameters**

| Name | In | Type | Req | Description |
|---|---|---|---|---|
| `id` | path | string | yes | The provider ID. |

**Responses**

- `200` `application/json` → Twilio \| SIP Trunk
- `404` Provider not found.

### `PATCH /agents/phone-numbers/providers/{id}` — Update Provider

Update a provider. To ensure assigned numbers are reachable, Twilio credentials are validated before saving. If the new credentials are invalid, the request fails and returns a list of unreachable numbers currently assigned to agents. For SIP trunks, omitted fields remain unchanged. Set `credentials` or `destination_country` to `null` to clear that field. Send `[]` to clear an inbound allowlist.

- **Auth:** APIKeyAuth  ·  **operationId:** `providers_update`  ·  **docs page:** `api-reference/agents/providers/update.md`

**Parameters**

| Name | In | Type | Req | Description |
|---|---|---|---|---|
| `id` | path | string | yes | The provider ID. |

**Request body** (`application/json`; type Twilio \| SIP Trunk)

_Variant: Twilio_

| Field | Type | Req | Description |
|---|---|---|---|
| `type` | string enum(`twilio`) | yes | Must be `"twilio"`. |
| `api_key_sid` | string | yes | New Twilio API key SID. |
| `api_key_secret` | string<password> | yes | New API key secret. |
| `region` | string enum(`us1`, `ie1`, `au1`) |  | The Twilio region the phone number and API key are configured for. `us1` (US), `ie1` (Ireland), `au1` (Australia). Default is `us1`. (default `"us1"`) |

_Variant: SIP Trunk_

| Field | Type | Req | Description |
|---|---|---|---|
| `type` | string enum(`sip_trunk`) | yes | Must be `"sip_trunk"`. |
| `label` | string |  | A new human-readable label for the provider. Omit to leave unchanged. |
| `inbound` | UpdateInboundSIPTrunkConfig |  | Inbound SIP configuration for updating a trunk. Omitted fields remain unchanged. Send `[]` to clear an allowlist. |
| `inbound.credentials` | UpdateSIPCredentials |  | SIP digest authentication credentials for an update. The username is required when credentials are provided. Omit the password to keep the stored secret, or se… |
| `inbound.credentials.username` | string | yes |  |
| `inbound.credentials.password` | string<password> |  |  |
| `inbound.allowed_addresses` | string[] |  | Source IP addresses or CIDR ranges allowed to send SIP traffic. |
| `inbound.allowed_numbers` | string[] |  | Caller numbers allowed on inbound calls. |
| `inbound.media_encryption` | string enum(`disabled`, `allowed`, `required`) |  | Media encryption policy. |
| `outbound` | UpdateOutboundSIPTrunkConfig |  | Outbound SIP configuration for updating a trunk. `address` and `transport` are immutable on an existing outbound trunk, but required when adding outbound to an… |
| `outbound.address` | string |  | Required only when adding outbound to an inbound-only provider. |
| `outbound.transport` | string enum(`tcp`, `tls`) |  | Required only when adding outbound to an inbound-only provider. |
| `outbound.destination_country` | string |  | Send `null` to clear. |
| `outbound.media_encryption` | string enum(`disabled`, `allowed`, `required`) |  | Media encryption policy. Omit to leave unchanged. |
| `outbound.credentials` | UpdateSIPCredentials |  | SIP digest authentication credentials for an update. The username is required when credentials are provided. Omit the password to keep the stored secret, or se… |
| `outbound.credentials.username` | string | yes |  |
| `outbound.credentials.password` | string<password> |  |  |

**Responses**

- `200` `application/json` → Twilio \| SIP Trunk
- `400` Invalid credentials, invalid SIP trunk configuration, or Twilio numbers are unreachable with the new credentials.
- `404` Provider not found.

### `GET /agents/phone-numbers/provision` — Get Phone Number Provisioning Usage

Returns Cartesia-managed phone number provisioning usage for the current month. Active numbers and numbers deleted during the current month count toward usage.

- **Auth:** APIKeyAuth  ·  **operationId:** `phone_numbers_provisioning_usage`  ·  **docs page:** `api-reference/agents/phone-numbers/provisioning-usage.md`

**Responses**

- `200` `application/json` → PhoneNumberProvisioningUsageResponse

| Field | Type | Req | Description |
|---|---|---|---|
| `monthly_usage` | integer | yes | Number of phone numbers provisioned during the current month. This includes active numbers and numbers deleted during the month. |
| `monthly_limit` | integer | yes | Maximum phone numbers that can be provisioned during the current month. `null` means the plan has no monthly limit. |
| `remaining` | integer | yes | Phone numbers that can still be provisioned during the current month. `null` means the plan has no monthly limit. |


### `POST /agents/phone-numbers/provision` — Provision Cartesia Number

Provision a new Cartesia-managed US phone number

- **Auth:** APIKeyAuth  ·  **operationId:** `phone_numbers_provision`  ·  **docs page:** `api-reference/agents/phone-numbers/provision.md`

**Request body** (`application/json`; type ProvisionPhoneNumberBody)

| Field | Type | Req | Description |
|---|---|---|---|
| `label` | string | yes | A human-readable name for the phone number. |
| `agent_id` | string |  | Optionally assign an agent to answer inbound calls to this number. Omit to keep the number outbound-only. |

**Responses**

- `201` `application/json` → PhoneNumberBaseResponse

| Field | Type | Req | Description |
|---|---|---|---|
| `id` | string | yes | Unique identifier for the phone number. |
| `label` | string |  | A human-readable name for the phone number. |
| `number` | string | yes | The phone number in E.164 format (e.g. +14155551234). |
| `agent` | object | yes | The agent that answers inbound calls to this number, or `null` if unassigned |
| `agent.id` | string | yes |  |
| `agent.name` | string | yes |  |
| `created_at` | string<date-time> | yes | UTC timestamp when the phone number was created. |
| `updated_at` | string<date-time> | yes | UTC timestamp when the phone number was last updated. |

- `402` Subscription phone number limit reached (`error_code: quota_exceeded`). Upgrade your plan or wait until the next month.
- `404` Agent not found.

### `DELETE /agents/phone-numbers/{id}` — Delete Phone Number

- **Auth:** APIKeyAuth  ·  **operationId:** `phone_numbers_delete`  ·  **docs page:** `api-reference/agents/phone-numbers/delete.md`

**Parameters**

| Name | In | Type | Req | Description |
|---|---|---|---|---|
| `id` | path | string | yes | The phone number ID. Twilio numbers can be deleted while assigned. Cartesia disconnects the agent and attempts to clear the Twilio webhook, but cleanup failure… |

**Responses**

- `204` Phone number deleted.
- `400` A Cartesia-managed or SIP trunk number is still assigned, or provider cleanup failed.
- `404` Phone number not found.

### `GET /agents/phone-numbers/{id}` — Get Phone Number

Returns the details of a specific phone number

- **Auth:** APIKeyAuth  ·  **operationId:** `phone_numbers_get`  ·  **docs page:** `api-reference/agents/phone-numbers/get.md`

**Parameters**

| Name | In | Type | Req | Description |
|---|---|---|---|---|
| `id` | path | string | yes | The phone number ID. |

**Responses**

- `200` `application/json` → PhoneNumberResponse

| Field | Type | Req | Description |
|---|---|---|---|
| `id` | string | yes | Unique identifier for the phone number. |
| `label` | string |  | A human-readable name for the phone number. |
| `number` | string | yes | The phone number in E.164 format (e.g. +14155551234). |
| `agent` | object | yes | The agent that answers inbound calls to this number, or `null` if unassigned |
| `agent.id` | string | yes |  |
| `agent.name` | string | yes |  |
| `created_at` | string<date-time> | yes | UTC timestamp when the phone number was created. |
| `updated_at` | string<date-time> | yes | UTC timestamp when the phone number was last updated. |
| `provider` | Twilio \| Cartesia \| SIP Trunk | yes | The telephony provider associated with a phone number. One of `Twilio`, `Cartesia`, or `SIP Trunk`, determined by the `type` field. For SIP trunk numbers, `inb… |

- `404` Phone number not found.

### `PATCH /agents/phone-numbers/{id}` — Update Phone Number

Update a phone number's label, agent assignment, or provider

- **Auth:** APIKeyAuth  ·  **operationId:** `phone_numbers_update`  ·  **docs page:** `api-reference/agents/phone-numbers/update.md`

**Parameters**

| Name | In | Type | Req | Description |
|---|---|---|---|---|
| `id` | path | string | yes | The phone number ID. |

**Request body** (`application/json`; type UpdatePhoneNumberBody)

| Field | Type | Req | Description |
|---|---|---|---|
| `label` | string |  | A new human-readable name. |
| `agent_id` | string |  | Assign an agent to answer inbound calls to this number, or `null` to unassign. |
| `provider` | By ID \| By Twilio Account |  | Change the provider. Pass `{ "id": "..." }` for an existing provider, or pass Twilio account fields for lookup convenience. For Twilio, the phone number must e… |

**Responses**

- `200` `application/json` → PhoneNumberResponse

| Field | Type | Req | Description |
|---|---|---|---|
| `id` | string | yes | Unique identifier for the phone number. |
| `label` | string |  | A human-readable name for the phone number. |
| `number` | string | yes | The phone number in E.164 format (e.g. +14155551234). |
| `agent` | object | yes | The agent that answers inbound calls to this number, or `null` if unassigned |
| `agent.id` | string | yes |  |
| `agent.name` | string | yes |  |
| `created_at` | string<date-time> | yes | UTC timestamp when the phone number was created. |
| `updated_at` | string<date-time> | yes | UTC timestamp when the phone number was last updated. |
| `provider` | Twilio \| Cartesia \| SIP Trunk | yes | The telephony provider associated with a phone number. One of `Twilio`, `Cartesia`, or `SIP Trunk`, determined by the `type` field. For SIP trunk numbers, `inb… |

- `400` Update failed. Possible reasons: phone number not found in the target provider account, or webhook configuration failed.
- `404` Phone number, agent, or provider not found.

### `DELETE /agents/{agent_id}/metrics/{metric_id}` — Remove Metric from Agent

- **Auth:** APIKeyAuth  ·  **operationId:** `agents_remove-metric-from-agent`  ·  **docs page:** `api-reference/agents/metrics/remove-metric-from-agent.md`

**Parameters**

| Name | In | Type | Req | Description |
|---|---|---|---|---|
| `agent_id` | path | string | yes |  |
| `metric_id` | path | string | yes | The ID of the metric. |

**Responses**

- `204` 

### `POST /agents/{agent_id}/metrics/{metric_id}` — Add Metric to Agent

- **Auth:** APIKeyAuth  ·  **operationId:** `agents_add-metric-to-agent`  ·  **docs page:** `api-reference/agents/metrics/add-metric-to-agent.md`

**Parameters**

| Name | In | Type | Req | Description |
|---|---|---|---|---|
| `agent_id` | path | string | yes | The ID of the agent. |
| `metric_id` | path | string | yes | The ID of the metric. |

**Responses**

- `204` 

### `GET /v1/agents` — List agents

Returns agent summaries for your account. Full configurations are omitted.

- **Auth:** APIKeyAuth  ·  **operationId:** `getV1Agents`  ·  **docs page:** `api-reference/agents/list.md`

**Parameters**

| Name | In | Type | Req | Description |
|---|---|---|---|---|
| `starting_after` | query | string |  |  |
| `ending_before` | query | string |  |  |
| `limit` | query | integer |  |  |
| `q` | query | string |  |  |

**Responses**

- `200` `application/json` → ListManagedAgentsResponse

| Field | Type | Req | Description |
|---|---|---|---|
| `data` | ManagedAgentSummary[] | yes | The requested page of results. |
| `data[].created_at` | string<date-time> | yes | An RFC 3339 / ISO 8601 date-time string with timezone (e.g. `2025-04-16T12:34:56.789Z`). |
| `data[].description` | string | yes | Description of the agent. |
| `data[].id` | string | yes | Unique identifier for the agent. |
| `data[].name` | string | yes | Name shown for the agent. |
| `data[].updated_at` | string<date-time> | yes | An RFC 3339 / ISO 8601 date-time string with timezone (e.g. `2025-04-16T12:34:56.789Z`). |
| `data[].version_id` | string | yes | Unique identifier for an immutable agent configuration version. |
| `has_more` | boolean | yes | Whether more results are available after this page. |
| `next_page` | string |  | An ID that can be passed as `starting_after` or `ending_before` to get the next page of data. |


### `POST /v1/agents` — Create an agent

Creates an agent and its initial configuration version

- **Auth:** APIKeyAuth  ·  **operationId:** `postV1Agents`  ·  **docs page:** `api-reference/agents/create.md`

**Request body** (`application/json`; type CreateManagedAgentRequest)

| Field | Type | Req | Description |
|---|---|---|---|
| `config` | object |  |  |
| `config.audio` | object |  |  |
| `config.audio.input` | object |  |  |
| `config.audio.output` | object |  |  |
| `config.dynamic_variable_placeholders` | Dynamic variable placeholders |  | Sample values for Playground test calls. Production calls never use them. Replaces saved samples on update. Omit to keep them, or send `{}` to clear them. Vari… |
| `config.initial_message` | string |  | Message the agent says when the conversation begins. Set to `null` to wait for the user to speak first. |
| `config.initial_message_interruptions` | string enum(`allow`, `ignore`, `respond_after`) |  | Controls how the agent handles interruptions while `initial_message` plays. `allow` lets the caller interrupt it, `ignore` plays it in full and adds what the c… |
| `config.instructions` | string |  | Instructions that define the agent's behavior. Supports dynamic variables such as `{{name}}`. |
| `config.language` | object |  |  |
| `config.language.primary` | string enum(`ar`, `bg`, `bn`, `cs`, `da`, `de`, `el`, `en`, `es`, `fi`, `fr`, `gu` …(+96)) \| string enum(`None`) |  | The language the agent starts each call in. Use a language code such as `en`, or a locale such as `en-US` for a regional accent. Write `initial_message` in thi… |
| `config.language.additional` | string enum(`ar`, `bg`, `bn`, `cs`, `da`, `de`, `el`, `en`, `es`, `fi`, `fr`, `gu` …(+96))[] |  | Other languages the caller can switch to, in order. Each is a language, such as `fr`, or a locale, such as `es-MX`. Omit or send `[]` for a single-language age… |
| `config.language.presets` | object |  | Settings per language, keyed by a language or locale such as `es-MX`. A preset applies only when the agent speaks that exact language or locale, including the … |
| `config.model` | object |  |  |
| `config.model.id` | string |  | Model ID from `GET /v1/agents/models`. |
| `config.model.fallback_strategy` | Auto \| Explicit \| object enum(`None`) |  | Models to fall back to when the primary model fails before it starts responding. `auto` uses Cartesia-managed fallback models. `explicit` uses up to three mode… |
| `config.model.max_output_tokens` | integer |  | Maximum number of tokens the model can generate, from `1` to `4096`. Set to `null` for no limit. |
| `config.model.temperature` | number |  | Controls the randomness of model responses, from `0` to `1`. |
| `config.prompt_optimization` | boolean |  | Optimizes the system prompt with guidelines for natural and realistic speech. Defaults to `true`. |
| `config.speculative_generation` | boolean |  | Starts LLM generation on an eager end-of-turn prediction to reduce response latency. Defaults to `true`. |
| `config.system_tools` | object |  | Built-in tools available to the agent. |
| `config.system_tools.end_call` | object |  | Configuration for ending the call. Omit or set to `null` to disable this tool. |
| `config.system_tools.send_dtmf` | object |  | Configuration for sending DTMF tones. Omit or set to `null` to disable this tool. |
| `config.system_tools.skip_turn` | object |  | Configuration for skipping the agent's turn. Omit or set to `null` to disable this tool. |
| `config.system_tools.voicemail_detection` | object |  | Configuration for detecting voicemail. Omit or set to `null` to disable this tool. |
| `config.system_tools.transfer_to_number` | object |  | Configuration for transferring the call. Omit or set to `null` to disable this tool. |
| `config.timezone` | string |  | IANA time zone for `{{system__time}}`. Defaults to `UTC`. |
| `config.tools` | ManagedAgentToolReference[] |  | Tools available to the agent. |
| `config.tools[].id` | string | yes | ID of a tool from the Tools API. |
| `config.turn` | object |  |  |
| `config.turn.inactivity_end_call_secs` | integer |  | Seconds of inactivity before the agent says a farewell message and ends the call, from `20` to `240`. Defaults to `240`; cannot be `null`. |
| `config.turn.inactivity_check_in_secs` | integer |  | Seconds of inactivity before the agent checks in, from `5` to `60`. Defaults to `8`. Set to `null` to disable check-ins. When enabled, it must be less than `in… |
| `description` | string |  | Description of the agent. |
| `name` | string |  | Name shown for the agent. It does not need to be unique. |

**Responses**

- `201` `application/json` → ManagedAgent

| Field | Type | Req | Description |
|---|---|---|---|
| `config` | ManagedAgentConfig | yes | Complete configuration for the agent's model, language, audio, and tools. |
| `config.audio` | object | yes |  |
| `config.audio.input` | object | yes |  |
| `config.audio.output` | object | yes |  |
| `config.dynamic_variable_placeholders` | Dynamic variable placeholders | yes | Sample values for Playground test calls. Production calls never use them. Replaces saved samples on update. Omit to keep them, or send `{}` to clear them. Vari… |
| `config.initial_message` | string | yes | Message the agent says when the conversation begins. Set to `null` to wait for the user to speak first. |
| `config.initial_message_interruptions` | string enum(`allow`, `ignore`, `respond_after`) | yes | Controls how the agent handles interruptions while `initial_message` plays. `allow` lets the caller interrupt it, `ignore` plays it in full and adds what the c… |
| `config.instructions` | string | yes | Instructions that define the agent's behavior. Supports dynamic variables such as `{{name}}`. |
| `config.language` | object | yes |  |
| `config.language.primary` | string enum(`en`, `fr`, `de`, `es`, `pt`, `zh`, `ja`, `hi`, `it`, `ko`, `nl`, `pl` …(+88)) | yes | The language the agent starts each call in: a language, such as `en`, or a locale, such as `en-US`. See [supported languages and locales]. |
| `config.language.additional` | string[] | yes | Other languages the caller can switch to, in order. Each is a language, such as `fr`, or a locale, such as `es-MX`. Returns `[]` when none are configured. Tags… |
| `config.language.presets` | object | yes | Settings per language, keyed by a language or locale such as `es-MX`. A preset applies only when the agent speaks that exact language or locale, including the … |
| `config.model` | object | yes |  |
| `config.model.id` | string | yes | Model ID from `GET /v1/agents/models`. |
| `config.model.fallback_strategy` | Auto \| Explicit \| object enum(`None`) | yes | Models to fall back to when the primary model fails before it starts responding. `auto` uses Cartesia-managed fallback models. `explicit` uses up to three mode… |
| `config.model.max_output_tokens` | integer | yes | Maximum number of tokens the model can generate, from `1` to `4096`. Set to `null` for no limit. |
| `config.model.temperature` | number | yes | Controls the randomness of model responses, from `0` to `1`. |
| `config.prompt_optimization` | boolean | yes | Optimizes the system prompt with guidelines for natural and realistic speech. Defaults to `true`. |
| `config.speculative_generation` | boolean | yes | Starts LLM generation on an eager end-of-turn prediction to reduce response latency |
| `config.system_tools` | ManagedAgentSystemTools | yes |  |
| `config.system_tools.end_call` | object | yes | Configuration for ending the call. Set to `null` to disable this tool. |
| `config.system_tools.send_dtmf` | object | yes | Configuration for sending DTMF tones. Set to `null` to disable this tool. |
| `config.system_tools.skip_turn` | object | yes | Configuration for skipping the agent's turn. Set to `null` to disable this tool. |
| `config.system_tools.voicemail_detection` | object | yes | Configuration for detecting voicemail. Set to `null` to disable this tool. |
| `config.system_tools.transfer_to_number` | object | yes | Configuration for transferring the call. Set to `null` to disable this tool. |
| `config.timezone` | string | yes | IANA time zone for `{{system__time}}`. Defaults to `UTC`. |
| `config.tools` | ManagedAgentToolReference[] | yes | Tools available to the agent. |
| `config.tools[].id` | string | yes | ID of a tool from the Tools API. |
| `config.turn` | object | yes |  |
| `config.turn.inactivity_end_call_secs` | integer | yes | Seconds of inactivity before the agent says a farewell message and ends the call, from `20` to `240`. Defaults to `240`; cannot be `null`. |
| `config.turn.inactivity_check_in_secs` | integer | yes | Seconds of inactivity before the agent checks in, from `5` to `60`. Defaults to `8`. Set to `null` to disable check-ins. When enabled, it must be less than `in… |
| `created_at` | string<date-time> | yes | An RFC 3339 / ISO 8601 date-time string with timezone (e.g. `2025-04-16T12:34:56.789Z`). |
| `description` | string | yes | Description of the agent. |
| `id` | string | yes | Unique identifier for the agent. |
| `name` | string | yes | Name shown for the agent. |
| `updated_at` | string<date-time> | yes | An RFC 3339 / ISO 8601 date-time string with timezone (e.g. `2025-04-16T12:34:56.789Z`). |
| `version` | object | yes |  |
| `version.created_at` | string<date-time> | yes | Time when this version was created. |
| `version.created_by` | string | yes | ID of the user or API key that created this version. `null` when no creator was recorded. |
| `version.description` | string | yes | Description supplied when this version was created. `null` if none was provided. |
| `version.id` | string | yes | Unique identifier for an immutable agent configuration version. |


### `GET /v1/agents/models` — List agent models

Returns the LLMs available for Managed Agents, including latency and pricing metadata

- **Auth:** APIKeyAuth  ·  **operationId:** `getV1AgentsModels`  ·  **docs page:** `api-reference/agents/models/list.md`

**Parameters**

| Name | In | Type | Req | Description |
|---|---|---|---|---|
| `starting_after` | query | string |  |  |
| `ending_before` | query | string |  |  |
| `limit` | query | integer |  |  |

**Responses**

- `200` `application/json` → ListManagedAgentModelsResponse

| Field | Type | Req | Description |
|---|---|---|---|
| `data` | ManagedAgentModel[] | yes | The requested page of results. |
| `data[].average_latency_ms` | integer | yes | Average response latency in milliseconds. |
| `data[].description` | string | yes | Description of the model. |
| `data[].display_name` | string | yes | Display name of the model. |
| `data[].id` | string | yes | Model ID used in agent configuration. |
| `data[].pricing` | object | yes |  |
| `data[].pricing.cache_read_per_million_tokens` | string | yes | Price in USD per one million tokens, represented as a decimal string. |
| `data[].pricing.cache_write_per_million_tokens` | string | yes | Price in USD per one million tokens, represented as a decimal string. |
| `data[].pricing.currency` | string enum(`USD`) | yes |  |
| `data[].pricing.input_per_million_tokens` | string | yes | Price in USD per one million tokens, represented as a decimal string. |
| `data[].pricing.output_per_million_tokens` | string | yes | Price in USD per one million tokens, represented as a decimal string. |
| `data[].provider` | string | yes | Company that provides the model. |
| `has_more` | boolean | yes | Whether more results are available after this page. |
| `next_page` | string |  | An ID that can be passed as `starting_after` or `ending_before` to get the next page of data. |


### `GET /v1/agents/tools` — List tools

Returns shared webhook and client tools for your account

- **Auth:** APIKeyAuth  ·  **operationId:** `getV1AgentsTools`  ·  **docs page:** `api-reference/agents/tools/list.md`

**Parameters**

| Name | In | Type | Req | Description |
|---|---|---|---|---|
| `starting_after` | query | string |  |  |
| `ending_before` | query | string |  |  |
| `limit` | query | integer |  |  |
| `type` | query | string enum(`webhook`, `client`) |  |  |
| `agent_id` | query | string |  |  |

**Responses**

- `200` `application/json` → ListManagedToolsResponse

| Field | Type | Req | Description |
|---|---|---|---|
| `data` | Webhook tool \| Client tool[] | yes | The requested page of results. |
| `has_more` | boolean | yes | Whether more results are available after this page. |
| `next_page` | string |  | An ID that can be passed as `starting_after` or `ending_before` to get the next page of data. |


### `POST /v1/agents/tools` — Create a tool

Creates a shared webhook or client tool that can be attached to agents

- **Auth:** APIKeyAuth  ·  **operationId:** `postV1AgentsTools`  ·  **docs page:** `api-reference/agents/tools/create.md`

**Request body** (`application/json`; type Webhook tool \| Client tool)

_Variant: Webhook tool_

| Field | Type | Req | Description |
|---|---|---|---|
| `api_schema` | ManagedWebhookApiSchema | yes |  |
| `api_schema.authentication` | object \| object |  | Authentication sent in the `Authorization` header. |
| `api_schema.method` | string enum(`GET`, `POST`, `PUT`, `PATCH`, `DELETE`) | yes | The HTTP method. |
| `api_schema.path_params_schema` | object |  | Path parameters keyed by placeholder name. |
| `api_schema.query_params_schema` | WebhookQuerySchema |  | Parameters sent in the request query string. |
| `api_schema.query_params_schema.properties` | object | yes | Query parameters keyed by name. |
| `api_schema.query_params_schema.required` | string[] |  | Parameters the model must provide. Exclude parameters filled by `constant_value` or `dynamic_variable`. |
| `api_schema.request_body_schema` | WebhookBodyParam |  | A value sent in the JSON request body. The model supplies it unless you set `constant_value` or `dynamic_variable`. |
| `api_schema.request_body_schema.constant_value` | string \| number \| boolean |  | A fixed value sent on every request. It must match the parameter's `type`. The agent does not see or set it. |
| `api_schema.request_body_schema.description` | string |  | What the agent should provide when it chooses this parameter's value. |
| `api_schema.request_body_schema.dynamic_variable` | string |  | Name of the variable to use when the tool runs. The parameter is hidden from the model. A missing value or type mismatch returns a tool error before execution.… |
| `api_schema.request_body_schema.enum` | string \| number \| boolean[] |  | Allowed values for the model to choose from. Use only when `constant_value` and `dynamic_variable` are omitted. |
| `api_schema.request_body_schema.items` | WebhookBodyParam |  | A value sent in the JSON request body. The model supplies it unless you set `constant_value` or `dynamic_variable`. |
| `api_schema.request_body_schema.properties` | object |  | Fields within this object, listed by name |
| `api_schema.request_body_schema.required` | string[] |  | Fields the model must provide. Exclude fields filled by `constant_value` or `dynamic_variable`. |
| `api_schema.request_body_schema.type` | string enum(`string`, `integer`, `number`, `boolean`, `object`, `array`) | yes |  |
| `api_schema.request_headers` | object |  | Headers sent with every request. |
| `api_schema.url` | string | yes | The HTTPS endpoint to call. Use `{name}` for path parameters defined in `path_params_schema`. |
| `assignments` | WebhookAssignment[] |  | Response fields to save as dynamic variables after a successful JSON response |
| `assignments[].dynamic_variable` | string | yes | Name of the custom variable to update. `system__` variables cannot be updated. Assign each variable at most once per tool. |
| `assignments[].value_path` | string | yes | Response path, such as `order.status` or `items[0].name`. List positions start at zero; `items.0.name` also selects the first item. Field names cannot contain … |
| `description` | string | yes | What the tool does and when the agent should use it. |
| `execution_mode` | string enum(`immediate`, `async`) | yes | Controls how the tool runs relative to the conversation. `immediate` runs in the current turn and can be interrupted by the caller, while `async` lets the turn… |
| `name` | string | yes | Name the agent uses to call the tool. Names are case-sensitive. |
| `pre_tool_speech` | string enum(`auto`, `force`, `off`) | yes | Controls whether the agent speaks before using the tool. `auto` lets the agent decide, `force` asks the agent to speak first, and `off` doesn't ask it to. |
| `response_timeout_secs` | integer |  | Maximum time to wait for the endpoint to respond. Defaults to 20 seconds. (default `20`) |
| `type` | string enum(`webhook`) | yes | Runs an HTTP request from Cartesia's servers. |

_Variant: Client tool_

| Field | Type | Req | Description |
|---|---|---|---|
| `description` | string | yes | What the tool does and when the agent should use it. |
| `execution_mode` | string enum(`immediate`, `async`) | yes | Controls how the tool runs relative to the conversation. `immediate` runs in the current turn and can be interrupted by the caller, while `async` lets the turn… |
| `expects_response` | boolean | yes | Whether the client application must return a result. When `false`, the tool call is dispatched without waiting for a result. |
| `name` | string | yes | Name the agent uses to call the tool. Names are case-sensitive. |
| `parameters` | ManagedClientToolParameters | yes |  |
| `parameters.properties` | object |  | Parameters keyed by name. |
| `parameters.required` | string[] |  | Names of parameters the agent must provide. Other parameters are optional. |
| `parameters.type` | string enum(`object`) | yes | The parameters schema type. Always `object`. |
| `pre_tool_speech` | string enum(`auto`, `force`, `off`) | yes | Controls whether the agent speaks before using the tool. `auto` lets the agent decide, `force` asks the agent to speak first, and `off` doesn't ask it to. |
| `response_timeout_secs` | integer |  | Maximum time to wait for the client application to return a result. Defaults to 20 seconds. Applies only when `expects_response` is `true`. If no result arrive… (default `20`) |
| `type` | string enum(`client`) | yes | Runs a function in the connected client application. |

**Responses**

- `201` `application/json` → Webhook tool \| Client tool

### `DELETE /v1/agents/tools/{tool_id}` — Delete a tool

Deletes an unused tool. Detach the tool from current agent configurations first.

- **Auth:** APIKeyAuth  ·  **operationId:** `deleteV1AgentsToolsByTool_id`  ·  **docs page:** `api-reference/agents/tools/delete.md`

**Parameters**

| Name | In | Type | Req | Description |
|---|---|---|---|---|
| `tool_id` | path | string | yes | Unique identifier for the tool. |

**Responses**

- `204` Delete a tool
- `404` `application/json` → PublicErrorResponse

| Field | Type | Req | Description |
|---|---|---|---|
| `doc_url` | string |  | URL to relevant documentation for the error |
| `error_code` | string |  | Machine-readable error code |
| `message` | string | yes | Detailed human-readable error message |
| `request_id` | string | yes | Unique identifier for this request. Include this when contacting support. |
| `title` | string | yes | Short human-readable error summary, like "File too large" |


### `GET /v1/agents/tools/{tool_id}` — Get a tool

Returns one shared tool. Expand `agents` to include agents that currently reference it.

- **Auth:** APIKeyAuth  ·  **operationId:** `getV1AgentsToolsByTool_id`  ·  **docs page:** `api-reference/agents/tools/get.md`

**Parameters**

| Name | In | Type | Req | Description |
|---|---|---|---|---|
| `tool_id` | path | string | yes | Unique identifier for the tool. |
| `expand[]` | query | string enum(`agents`)[] |  |  |

**Responses**

- `200` `application/json` → Webhook tool \| Client tool
- `404` `application/json` → PublicErrorResponse

| Field | Type | Req | Description |
|---|---|---|---|
| `doc_url` | string |  | URL to relevant documentation for the error |
| `error_code` | string |  | Machine-readable error code |
| `message` | string | yes | Detailed human-readable error message |
| `request_id` | string | yes | Unique identifier for this request. Include this when contacting support. |
| `title` | string | yes | Short human-readable error summary, like "File too large" |


### `PATCH /v1/agents/tools/{tool_id}` — Update a tool

Sparsely updates a tool.

- **Auth:** APIKeyAuth  ·  **operationId:** `patchV1AgentsToolsByTool_id`  ·  **docs page:** `api-reference/agents/tools/update.md`

**Parameters**

| Name | In | Type | Req | Description |
|---|---|---|---|---|
| `tool_id` | path | string | yes | Unique identifier for the tool. |

**Request body** (`application/json`; type Webhook tool update \| Client tool update)

_Variant: Webhook tool update_

| Field | Type | Req | Description |
|---|---|---|---|
| `api_schema` | object |  |  |
| `api_schema.authentication` | object \| object |  |  |
| `api_schema.method` | string enum(`GET`, `POST`, `PUT`, `PATCH`, `DELETE`) |  | The HTTP method. |
| `api_schema.path_params_schema` | object |  | Path parameters keyed by placeholder name. |
| `api_schema.query_params_schema` | WebhookQuerySchema |  |  |
| `api_schema.query_params_schema.properties` | object | yes | Query parameters keyed by name. |
| `api_schema.query_params_schema.required` | string[] |  | Parameters the model must provide. Exclude parameters filled by `constant_value` or `dynamic_variable`. |
| `api_schema.request_body_schema` | WebhookBodyParam |  | The schema for the JSON request body. The root must be an `object`. Only `POST`, `PUT`, and `PATCH` requests can include a body. |
| `api_schema.request_body_schema.constant_value` | string \| number \| boolean |  | A fixed value sent on every request. It must match the parameter's `type`. The agent does not see or set it. |
| `api_schema.request_body_schema.description` | string |  | What the agent should provide when it chooses this parameter's value. |
| `api_schema.request_body_schema.dynamic_variable` | string |  | Name of the variable to use when the tool runs. The parameter is hidden from the model. A missing value or type mismatch returns a tool error before execution.… |
| `api_schema.request_body_schema.enum` | string \| number \| boolean[] |  | Allowed values for the model to choose from. Use only when `constant_value` and `dynamic_variable` are omitted. |
| `api_schema.request_body_schema.items` | WebhookBodyParam |  | A value sent in the JSON request body. The model supplies it unless you set `constant_value` or `dynamic_variable`. |
| `api_schema.request_body_schema.properties` | object |  | Fields within this object, listed by name |
| `api_schema.request_body_schema.required` | string[] |  | Fields the model must provide. Exclude fields filled by `constant_value` or `dynamic_variable`. |
| `api_schema.request_body_schema.type` | string enum(`string`, `integer`, `number`, `boolean`, `object`, `array`) | yes |  |
| `api_schema.request_headers` | object |  |  |
| `api_schema.url` | string |  | The HTTPS endpoint to call. Use `{name}` for path parameters defined in `path_params_schema`. |
| `assignments` | WebhookAssignment[] |  | Replaces the tool's assignments. Send `null` or `[]` to remove them all. |
| `assignments[].dynamic_variable` | string | yes | Name of the custom variable to update. `system__` variables cannot be updated. Assign each variable at most once per tool. |
| `assignments[].value_path` | string | yes | Response path, such as `order.status` or `items[0].name`. List positions start at zero; `items.0.name` also selects the first item. Field names cannot contain … |
| `description` | string |  | What the tool does and when the agent should use it. |
| `execution_mode` | string enum(`immediate`, `async`) |  | Controls how the tool runs relative to the conversation. `immediate` runs in the current turn and can be interrupted by the caller. `async` lets the turn finis… |
| `name` | string |  | Name the agent uses to call the tool. Names are case-sensitive. |
| `pre_tool_speech` | string enum(`auto`, `force`, `off`) |  | Controls whether the agent speaks before using the tool. `auto` lets the agent decide, `force` asks the agent to speak first, and `off` doesn't ask it to. |
| `response_timeout_secs` | integer |  | Maximum time to wait for the endpoint to respond. Defaults to 20 seconds. |

_Variant: Client tool update_

| Field | Type | Req | Description |
|---|---|---|---|
| `description` | string |  | What the tool does and when the agent should use it. |
| `execution_mode` | string enum(`immediate`, `async`) |  | Controls how the tool runs relative to the conversation. `immediate` runs in the current turn and can be interrupted by the caller. `async` lets the turn finis… |
| `expects_response` | boolean |  | Whether the client application must return a result. When `false`, the tool call is dispatched without waiting for a result. |
| `name` | string |  | Name the agent uses to call the tool. Names are case-sensitive. |
| `parameters` | ManagedClientToolParameters |  |  |
| `parameters.properties` | object |  | Parameters keyed by name. |
| `parameters.required` | string[] |  | Names of parameters the agent must provide. Other parameters are optional. |
| `parameters.type` | string enum(`object`) | yes | The parameters schema type. Always `object`. |
| `pre_tool_speech` | string enum(`auto`, `force`, `off`) |  | Controls whether the agent speaks before using the tool. `auto` lets the agent decide, `force` asks the agent to speak first, and `off` doesn't ask it to. |
| `response_timeout_secs` | integer |  | Maximum time to wait for the client application to return a result. Defaults to 20 seconds. Applies only when `expects_response` is `true`. If no result arrive… |

**Responses**

- `200` `application/json` → Webhook tool \| Client tool
- `404` `application/json` → PublicErrorResponse

| Field | Type | Req | Description |
|---|---|---|---|
| `doc_url` | string |  | URL to relevant documentation for the error |
| `error_code` | string |  | Machine-readable error code |
| `message` | string | yes | Detailed human-readable error message |
| `request_id` | string | yes | Unique identifier for this request. Include this when contacting support. |
| `title` | string | yes | Short human-readable error summary, like "File too large" |


### `POST /v1/agents/webrtc/{agent_id}/sessions` — Create a WebRTC session

Creates a WebRTC session on your trusted server and returns credentials for the browser to join. Authenticate this request with an API key; access tokens, including returned session tokens, cannot create sessions.

- **Auth:** APIKeyAuth  ·  **operationId:** `createAgentWebrtcSession`  ·  **docs page:** `api-reference/agents/create-webrtc-session.md`

**Parameters**

| Name | In | Type | Req | Description |
|---|---|---|---|---|
| `agent_id` | path | string | yes | Unique identifier for the agent. |

**Request body** (`application/json`; type CreateAgentWebSessionRequest)

| Field | Type | Req | Description |
|---|---|---|---|
| `dynamic_variables` | object |  | Trusted values that Cartesia stores on the call and supplies to the agent. |
| `config_overrides` | AgentSessionConfig |  | Per-call overrides using the same field structure as agent configuration. Omitted fields use the agent's published configuration. |
| `config_overrides.instructions` | string |  | Instructions that define the agent's behavior for this call. |
| `config_overrides.initial_message` | string |  | Message spoken when the call begins. Set to `null` to wait for the caller to speak first. |
| `config_overrides.language` | object |  |  |
| `config_overrides.language.primary` | string enum(`en`, `fr`, `de`, `es`, `pt`, `zh`, `ja`, `hi`, `it`, `ko`, `nl`, `pl` …(+88)) |  | Primary language used for this call. Omit to inherit the published language or set to `null` to use English (`en`). |
| `config_overrides.audio` | object |  |  |
| `config_overrides.audio.output` | object |  |  |

**Responses**

- `200` `application/json` → CreateAgentWebrtcSessionResponse

| Field | Type | Req | Description |
|---|---|---|---|
| `call_id` | string | yes | ID of the pre-created call. |
| `url` | string<uri> | yes | LiveKit server URL the browser connects to. |
| `token` | string | yes | Short-lived LiveKit participant token for the browser connection. Do not log it. |
| `room` | string | yes | LiveKit room the browser joins. |
| `expires_at` | string<date-time> | yes | Connection deadline. Create a new session if the browser does not join before this time. |

- `400` `application/json` → PublicErrorResponse

| Field | Type | Req | Description |
|---|---|---|---|
| `doc_url` | string |  | URL to relevant documentation for the error |
| `error_code` | string |  | Machine-readable error code |
| `message` | string | yes | Detailed human-readable error message |
| `request_id` | string | yes | Unique identifier for this request. Include this when contacting support. |
| `title` | string | yes | Short human-readable error summary, like "File too large" |

- `401` `application/json` → PublicErrorResponse

| Field | Type | Req | Description |
|---|---|---|---|
| `doc_url` | string |  | URL to relevant documentation for the error |
| `error_code` | string |  | Machine-readable error code |
| `message` | string | yes | Detailed human-readable error message |
| `request_id` | string | yes | Unique identifier for this request. Include this when contacting support. |
| `title` | string | yes | Short human-readable error summary, like "File too large" |

- `404` `application/json` → PublicErrorResponse

| Field | Type | Req | Description |
|---|---|---|---|
| `doc_url` | string |  | URL to relevant documentation for the error |
| `error_code` | string |  | Machine-readable error code |
| `message` | string | yes | Detailed human-readable error message |
| `request_id` | string | yes | Unique identifier for this request. Include this when contacting support. |
| `title` | string | yes | Short human-readable error summary, like "File too large" |

- `429` `application/json` → PublicErrorResponse

| Field | Type | Req | Description |
|---|---|---|---|
| `doc_url` | string |  | URL to relevant documentation for the error |
| `error_code` | string |  | Machine-readable error code |
| `message` | string | yes | Detailed human-readable error message |
| `request_id` | string | yes | Unique identifier for this request. Include this when contacting support. |
| `title` | string | yes | Short human-readable error summary, like "File too large" |


### `POST /v1/agents/websocket/{agent_id}/sessions` — Create a WebSocket session

Creates a call-bound WebSocket session on your trusted server and returns a session token for the browser to connect. Authenticate this request with an API key; access tokens, including returned session tokens, cannot create sessions.

- **Auth:** APIKeyAuth  ·  **operationId:** `createAgentWebsocketSession`  ·  **docs page:** `api-reference/agents/create-websocket-session.md`

**Parameters**

| Name | In | Type | Req | Description |
|---|---|---|---|---|
| `agent_id` | path | string | yes | Unique identifier for the agent. |

**Request body** (`application/json`; type CreateAgentWebSessionRequest)

| Field | Type | Req | Description |
|---|---|---|---|
| `dynamic_variables` | object |  | Trusted values that Cartesia stores on the call and supplies to the agent. |
| `config_overrides` | AgentSessionConfig |  | Per-call overrides using the same field structure as agent configuration. Omitted fields use the agent's published configuration. |
| `config_overrides.instructions` | string |  | Instructions that define the agent's behavior for this call. |
| `config_overrides.initial_message` | string |  | Message spoken when the call begins. Set to `null` to wait for the caller to speak first. |
| `config_overrides.language` | object |  |  |
| `config_overrides.language.primary` | string enum(`en`, `fr`, `de`, `es`, `pt`, `zh`, `ja`, `hi`, `it`, `ko`, `nl`, `pl` …(+88)) |  | Primary language used for this call. Omit to inherit the published language or set to `null` to use English (`en`). |
| `config_overrides.audio` | object |  |  |
| `config_overrides.audio.output` | object |  |  |

**Responses**

- `200` `application/json` → CreateAgentWebsocketSessionResponse

| Field | Type | Req | Description |
|---|---|---|---|
| `call_id` | string | yes | ID of the pre-created call. The `session_ready` event returns the same value. |
| `url` | string<uri> | yes | WebSocket URL for this agent. Append the returned session token as the `access_token` query parameter. |
| `token` | string | yes | Opaque, short-lived, single-use session token. Do not inspect or log it. |
| `expires_at` | string<date-time> | yes | Connection deadline, currently four minutes after creation. Use this timestamp rather than calculating the deadline. |

- `400` `application/json` → PublicErrorResponse

| Field | Type | Req | Description |
|---|---|---|---|
| `doc_url` | string |  | URL to relevant documentation for the error |
| `error_code` | string |  | Machine-readable error code |
| `message` | string | yes | Detailed human-readable error message |
| `request_id` | string | yes | Unique identifier for this request. Include this when contacting support. |
| `title` | string | yes | Short human-readable error summary, like "File too large" |

- `401` `application/json` → PublicErrorResponse

| Field | Type | Req | Description |
|---|---|---|---|
| `doc_url` | string |  | URL to relevant documentation for the error |
| `error_code` | string |  | Machine-readable error code |
| `message` | string | yes | Detailed human-readable error message |
| `request_id` | string | yes | Unique identifier for this request. Include this when contacting support. |
| `title` | string | yes | Short human-readable error summary, like "File too large" |

- `404` `application/json` → PublicErrorResponse

| Field | Type | Req | Description |
|---|---|---|---|
| `doc_url` | string |  | URL to relevant documentation for the error |
| `error_code` | string |  | Machine-readable error code |
| `message` | string | yes | Detailed human-readable error message |
| `request_id` | string | yes | Unique identifier for this request. Include this when contacting support. |
| `title` | string | yes | Short human-readable error summary, like "File too large" |

- `429` `application/json` → PublicErrorResponse

| Field | Type | Req | Description |
|---|---|---|---|
| `doc_url` | string |  | URL to relevant documentation for the error |
| `error_code` | string |  | Machine-readable error code |
| `message` | string | yes | Detailed human-readable error message |
| `request_id` | string | yes | Unique identifier for this request. Include this when contacting support. |
| `title` | string | yes | Short human-readable error summary, like "File too large" |


### `DELETE /v1/agents/{agent_id}` — Delete an agent

Deletes the agent. Future reads and new calls fail.

- **Auth:** APIKeyAuth  ·  **operationId:** `deleteV1AgentsByAgent_id`  ·  **docs page:** `api-reference/agents/delete.md`

**Parameters**

| Name | In | Type | Req | Description |
|---|---|---|---|---|
| `agent_id` | path | string | yes | Unique identifier for the agent. |

**Responses**

- `204` Delete an agent
- `404` `application/json` → PublicErrorResponse

| Field | Type | Req | Description |
|---|---|---|---|
| `doc_url` | string |  | URL to relevant documentation for the error |
| `error_code` | string |  | Machine-readable error code |
| `message` | string | yes | Detailed human-readable error message |
| `request_id` | string | yes | Unique identifier for this request. Include this when contacting support. |
| `title` | string | yes | Short human-readable error summary, like "File too large" |


### `GET /v1/agents/{agent_id}` — Get an agent

Returns the published agent with its complete normalized configuration

- **Auth:** APIKeyAuth  ·  **operationId:** `getV1AgentsByAgent_id`  ·  **docs page:** `api-reference/agents/get.md`

**Parameters**

| Name | In | Type | Req | Description |
|---|---|---|---|---|
| `agent_id` | path | string | yes | Unique identifier for the agent. |

**Responses**

- `200` `application/json` → ManagedAgent

| Field | Type | Req | Description |
|---|---|---|---|
| `config` | ManagedAgentConfig | yes | Complete configuration for the agent's model, language, audio, and tools. |
| `config.audio` | object | yes |  |
| `config.audio.input` | object | yes |  |
| `config.audio.output` | object | yes |  |
| `config.dynamic_variable_placeholders` | Dynamic variable placeholders | yes | Sample values for Playground test calls. Production calls never use them. Replaces saved samples on update. Omit to keep them, or send `{}` to clear them. Vari… |
| `config.initial_message` | string | yes | Message the agent says when the conversation begins. Set to `null` to wait for the user to speak first. |
| `config.initial_message_interruptions` | string enum(`allow`, `ignore`, `respond_after`) | yes | Controls how the agent handles interruptions while `initial_message` plays. `allow` lets the caller interrupt it, `ignore` plays it in full and adds what the c… |
| `config.instructions` | string | yes | Instructions that define the agent's behavior. Supports dynamic variables such as `{{name}}`. |
| `config.language` | object | yes |  |
| `config.language.primary` | string enum(`en`, `fr`, `de`, `es`, `pt`, `zh`, `ja`, `hi`, `it`, `ko`, `nl`, `pl` …(+88)) | yes | The language the agent starts each call in: a language, such as `en`, or a locale, such as `en-US`. See [supported languages and locales]. |
| `config.language.additional` | string[] | yes | Other languages the caller can switch to, in order. Each is a language, such as `fr`, or a locale, such as `es-MX`. Returns `[]` when none are configured. Tags… |
| `config.language.presets` | object | yes | Settings per language, keyed by a language or locale such as `es-MX`. A preset applies only when the agent speaks that exact language or locale, including the … |
| `config.model` | object | yes |  |
| `config.model.id` | string | yes | Model ID from `GET /v1/agents/models`. |
| `config.model.fallback_strategy` | Auto \| Explicit \| object enum(`None`) | yes | Models to fall back to when the primary model fails before it starts responding. `auto` uses Cartesia-managed fallback models. `explicit` uses up to three mode… |
| `config.model.max_output_tokens` | integer | yes | Maximum number of tokens the model can generate, from `1` to `4096`. Set to `null` for no limit. |
| `config.model.temperature` | number | yes | Controls the randomness of model responses, from `0` to `1`. |
| `config.prompt_optimization` | boolean | yes | Optimizes the system prompt with guidelines for natural and realistic speech. Defaults to `true`. |
| `config.speculative_generation` | boolean | yes | Starts LLM generation on an eager end-of-turn prediction to reduce response latency |
| `config.system_tools` | ManagedAgentSystemTools | yes |  |
| `config.system_tools.end_call` | object | yes | Configuration for ending the call. Set to `null` to disable this tool. |
| `config.system_tools.send_dtmf` | object | yes | Configuration for sending DTMF tones. Set to `null` to disable this tool. |
| `config.system_tools.skip_turn` | object | yes | Configuration for skipping the agent's turn. Set to `null` to disable this tool. |
| `config.system_tools.voicemail_detection` | object | yes | Configuration for detecting voicemail. Set to `null` to disable this tool. |
| `config.system_tools.transfer_to_number` | object | yes | Configuration for transferring the call. Set to `null` to disable this tool. |
| `config.timezone` | string | yes | IANA time zone for `{{system__time}}`. Defaults to `UTC`. |
| `config.tools` | ManagedAgentToolReference[] | yes | Tools available to the agent. |
| `config.tools[].id` | string | yes | ID of a tool from the Tools API. |
| `config.turn` | object | yes |  |
| `config.turn.inactivity_end_call_secs` | integer | yes | Seconds of inactivity before the agent says a farewell message and ends the call, from `20` to `240`. Defaults to `240`; cannot be `null`. |
| `config.turn.inactivity_check_in_secs` | integer | yes | Seconds of inactivity before the agent checks in, from `5` to `60`. Defaults to `8`. Set to `null` to disable check-ins. When enabled, it must be less than `in… |
| `created_at` | string<date-time> | yes | An RFC 3339 / ISO 8601 date-time string with timezone (e.g. `2025-04-16T12:34:56.789Z`). |
| `description` | string | yes | Description of the agent. |
| `id` | string | yes | Unique identifier for the agent. |
| `name` | string | yes | Name shown for the agent. |
| `updated_at` | string<date-time> | yes | An RFC 3339 / ISO 8601 date-time string with timezone (e.g. `2025-04-16T12:34:56.789Z`). |
| `version` | object | yes |  |
| `version.created_at` | string<date-time> | yes | Time when this version was created. |
| `version.created_by` | string | yes | ID of the user or API key that created this version. `null` when no creator was recorded. |
| `version.description` | string | yes | Description supplied when this version was created. `null` if none was provided. |
| `version.id` | string | yes | Unique identifier for an immutable agent configuration version. |

- `404` `application/json` → PublicErrorResponse

| Field | Type | Req | Description |
|---|---|---|---|
| `doc_url` | string |  | URL to relevant documentation for the error |
| `error_code` | string |  | Machine-readable error code |
| `message` | string | yes | Detailed human-readable error message |
| `request_id` | string | yes | Unique identifier for this request. Include this when contacting support. |
| `title` | string | yes | Short human-readable error summary, like "File too large" |


### `PATCH /v1/agents/{agent_id}` — Update an agent

Sparsely updates agent metadata or configuration. Configuration changes create a new immutable version.

- **Auth:** APIKeyAuth  ·  **operationId:** `patchV1AgentsByAgent_id`  ·  **docs page:** `api-reference/agents/update.md`

**Parameters**

| Name | In | Type | Req | Description |
|---|---|---|---|---|
| `agent_id` | path | string | yes | Unique identifier for the agent. |

**Request body** (`application/json`; type UpdateManagedAgentRequest)

| Field | Type | Req | Description |
|---|---|---|---|
| `config` | object |  |  |
| `config.audio` | object |  |  |
| `config.audio.input` | object |  |  |
| `config.audio.output` | object |  |  |
| `config.dynamic_variable_placeholders` | Dynamic variable placeholders |  | Sample values for Playground test calls. Production calls never use them. Replaces saved samples on update. Omit to keep them, or send `{}` to clear them. Vari… |
| `config.initial_message` | string |  | Message the agent says when the conversation begins. Set to `null` to wait for the user to speak first. |
| `config.initial_message_interruptions` | string enum(`allow`, `ignore`, `respond_after`) |  | Controls how the agent handles interruptions while `initial_message` plays. `allow` lets the caller interrupt it, `ignore` plays it in full and adds what the c… |
| `config.instructions` | string |  | Instructions that define the agent's behavior. Supports dynamic variables such as `{{name}}`. |
| `config.language` | object |  |  |
| `config.language.primary` | string enum(`ar`, `bg`, `bn`, `cs`, `da`, `de`, `el`, `en`, `es`, `fi`, `fr`, `gu` …(+96)) \| string enum(`None`) |  | The language the agent starts each call in. Use a language code such as `en`, or a locale such as `en-US` for a regional accent. Write `initial_message` in thi… |
| `config.language.additional` | string enum(`ar`, `bg`, `bn`, `cs`, `da`, `de`, `el`, `en`, `es`, `fi`, `fr`, `gu` …(+96))[] |  | Other languages the caller can switch to, in order. Each is a language, such as `fr`, or a locale, such as `es-MX`. Omit to preserve the current list; `[]` cle… |
| `config.language.presets` | object |  | Settings per language, keyed by a language or locale such as `es-MX`. A preset applies only when the agent speaks that exact language or locale, including the … |
| `config.model` | object |  |  |
| `config.model.id` | string |  | Model ID from `GET /v1/agents/models`. |
| `config.model.fallback_strategy` | Auto \| Explicit \| object enum(`None`) |  | Models to fall back to when the primary model fails before it starts responding. `auto` uses Cartesia-managed fallback models. `explicit` uses up to three mode… |
| `config.model.max_output_tokens` | integer |  | Maximum number of tokens the model can generate, from `1` to `4096`. Set to `null` for no limit. |
| `config.model.temperature` | number |  | Controls the randomness of model responses, from `0` to `1`. |
| `config.prompt_optimization` | boolean |  | Optimizes the system prompt with guidelines for natural and realistic speech. Defaults to `true`. |
| `config.speculative_generation` | boolean |  | Starts LLM generation on an eager end-of-turn prediction to reduce response latency. Omit from an update to preserve the current setting. |
| `config.system_tools` | object |  | Built-in tools available to the agent. |
| `config.system_tools.end_call` | object |  | Configuration for ending the call. Omit to keep the current setting or set to `null` to disable this tool. |
| `config.system_tools.send_dtmf` | object |  | Configuration for sending DTMF tones. Omit to keep the current setting or set to `null` to disable this tool. |
| `config.system_tools.skip_turn` | object |  | Configuration for skipping the agent's turn. Omit to keep the current setting or set to `null` to disable this tool. |
| `config.system_tools.voicemail_detection` | object |  | Configuration for detecting voicemail. Omit to keep the current setting or set to `null` to disable this tool. |
| `config.system_tools.transfer_to_number` | object |  | Configuration for transferring the call. Omit to keep the current setting or set to `null` to disable this tool. |
| `config.timezone` | string |  | IANA time zone for `{{system__time}}`. Defaults to `UTC`. |
| `config.tools` | ManagedAgentToolReference[] |  | Tools available to the agent. |
| `config.tools[].id` | string | yes | ID of a tool from the Tools API. |
| `config.turn` | object |  |  |
| `config.turn.inactivity_end_call_secs` | integer |  | Seconds of inactivity before the agent says a farewell message and ends the call, from `20` to `240`. Defaults to `240`; cannot be `null`. |
| `config.turn.inactivity_check_in_secs` | integer |  | Seconds of inactivity before the agent checks in, from `5` to `60`. Defaults to `8`. Set to `null` to disable check-ins. When enabled, it must be less than `in… |
| `description` | string |  | Description of the agent. |
| `name` | string |  | Name shown for the agent. It does not need to be unique. |
| `version_description` | string |  | Description to attach to the new configuration version. Allowed only when this request changes `config`. |

**Responses**

- `200` `application/json` → ManagedAgent

| Field | Type | Req | Description |
|---|---|---|---|
| `config` | ManagedAgentConfig | yes | Complete configuration for the agent's model, language, audio, and tools. |
| `config.audio` | object | yes |  |
| `config.audio.input` | object | yes |  |
| `config.audio.output` | object | yes |  |
| `config.dynamic_variable_placeholders` | Dynamic variable placeholders | yes | Sample values for Playground test calls. Production calls never use them. Replaces saved samples on update. Omit to keep them, or send `{}` to clear them. Vari… |
| `config.initial_message` | string | yes | Message the agent says when the conversation begins. Set to `null` to wait for the user to speak first. |
| `config.initial_message_interruptions` | string enum(`allow`, `ignore`, `respond_after`) | yes | Controls how the agent handles interruptions while `initial_message` plays. `allow` lets the caller interrupt it, `ignore` plays it in full and adds what the c… |
| `config.instructions` | string | yes | Instructions that define the agent's behavior. Supports dynamic variables such as `{{name}}`. |
| `config.language` | object | yes |  |
| `config.language.primary` | string enum(`en`, `fr`, `de`, `es`, `pt`, `zh`, `ja`, `hi`, `it`, `ko`, `nl`, `pl` …(+88)) | yes | The language the agent starts each call in: a language, such as `en`, or a locale, such as `en-US`. See [supported languages and locales]. |
| `config.language.additional` | string[] | yes | Other languages the caller can switch to, in order. Each is a language, such as `fr`, or a locale, such as `es-MX`. Returns `[]` when none are configured. Tags… |
| `config.language.presets` | object | yes | Settings per language, keyed by a language or locale such as `es-MX`. A preset applies only when the agent speaks that exact language or locale, including the … |
| `config.model` | object | yes |  |
| `config.model.id` | string | yes | Model ID from `GET /v1/agents/models`. |
| `config.model.fallback_strategy` | Auto \| Explicit \| object enum(`None`) | yes | Models to fall back to when the primary model fails before it starts responding. `auto` uses Cartesia-managed fallback models. `explicit` uses up to three mode… |
| `config.model.max_output_tokens` | integer | yes | Maximum number of tokens the model can generate, from `1` to `4096`. Set to `null` for no limit. |
| `config.model.temperature` | number | yes | Controls the randomness of model responses, from `0` to `1`. |
| `config.prompt_optimization` | boolean | yes | Optimizes the system prompt with guidelines for natural and realistic speech. Defaults to `true`. |
| `config.speculative_generation` | boolean | yes | Starts LLM generation on an eager end-of-turn prediction to reduce response latency |
| `config.system_tools` | ManagedAgentSystemTools | yes |  |
| `config.system_tools.end_call` | object | yes | Configuration for ending the call. Set to `null` to disable this tool. |
| `config.system_tools.send_dtmf` | object | yes | Configuration for sending DTMF tones. Set to `null` to disable this tool. |
| `config.system_tools.skip_turn` | object | yes | Configuration for skipping the agent's turn. Set to `null` to disable this tool. |
| `config.system_tools.voicemail_detection` | object | yes | Configuration for detecting voicemail. Set to `null` to disable this tool. |
| `config.system_tools.transfer_to_number` | object | yes | Configuration for transferring the call. Set to `null` to disable this tool. |
| `config.timezone` | string | yes | IANA time zone for `{{system__time}}`. Defaults to `UTC`. |
| `config.tools` | ManagedAgentToolReference[] | yes | Tools available to the agent. |
| `config.tools[].id` | string | yes | ID of a tool from the Tools API. |
| `config.turn` | object | yes |  |
| `config.turn.inactivity_end_call_secs` | integer | yes | Seconds of inactivity before the agent says a farewell message and ends the call, from `20` to `240`. Defaults to `240`; cannot be `null`. |
| `config.turn.inactivity_check_in_secs` | integer | yes | Seconds of inactivity before the agent checks in, from `5` to `60`. Defaults to `8`. Set to `null` to disable check-ins. When enabled, it must be less than `in… |
| `created_at` | string<date-time> | yes | An RFC 3339 / ISO 8601 date-time string with timezone (e.g. `2025-04-16T12:34:56.789Z`). |
| `description` | string | yes | Description of the agent. |
| `id` | string | yes | Unique identifier for the agent. |
| `name` | string | yes | Name shown for the agent. |
| `updated_at` | string<date-time> | yes | An RFC 3339 / ISO 8601 date-time string with timezone (e.g. `2025-04-16T12:34:56.789Z`). |
| `version` | object | yes |  |
| `version.created_at` | string<date-time> | yes | Time when this version was created. |
| `version.created_by` | string | yes | ID of the user or API key that created this version. `null` when no creator was recorded. |
| `version.description` | string | yes | Description supplied when this version was created. `null` if none was provided. |
| `version.id` | string | yes | Unique identifier for an immutable agent configuration version. |

- `404` `application/json` → PublicErrorResponse

| Field | Type | Req | Description |
|---|---|---|---|
| `doc_url` | string |  | URL to relevant documentation for the error |
| `error_code` | string |  | Machine-readable error code |
| `message` | string | yes | Detailed human-readable error message |
| `request_id` | string | yes | Unique identifier for this request. Include this when contacting support. |
| `title` | string | yes | Short human-readable error summary, like "File too large" |


### `GET /v1/agents/{agent_id}/versions` — List agent versions

Returns the agent's configuration history, ordered from newest to oldest. Each version is an immutable snapshot of the complete configuration.

- **Auth:** APIKeyAuth  ·  **operationId:** `getV1AgentsByAgent_idVersions`  ·  **docs page:** `api-reference/agents/versions/list.md`

**Parameters**

| Name | In | Type | Req | Description |
|---|---|---|---|---|
| `agent_id` | path | string | yes | Unique identifier for the agent. |
| `starting_after` | query | string |  |  |
| `ending_before` | query | string |  |  |
| `limit` | query | integer |  |  |

**Responses**

- `200` `application/json` → ListManagedAgentVersionsResponse

| Field | Type | Req | Description |
|---|---|---|---|
| `data` | ManagedAgentVersion[] | yes | The requested page of results. |
| `data[].config` | ManagedAgentConfig | yes | Complete configuration for the agent's model, language, audio, and tools. |
| `data[].config.audio` | object | yes |  |
| `data[].config.dynamic_variable_placeholders` | Dynamic variable placeholders | yes | Sample values for Playground test calls. Production calls never use them. Replaces saved samples on update. Omit to keep them, or send `{}` to clear them. Vari… |
| `data[].config.initial_message` | string | yes | Message the agent says when the conversation begins. Set to `null` to wait for the user to speak first. |
| `data[].config.initial_message_interruptions` | string enum(`allow`, `ignore`, `respond_after`) | yes | Controls how the agent handles interruptions while `initial_message` plays. `allow` lets the caller interrupt it, `ignore` plays it in full and adds what the c… |
| `data[].config.instructions` | string | yes | Instructions that define the agent's behavior. Supports dynamic variables such as `{{name}}`. |
| `data[].config.language` | object | yes |  |
| `data[].config.model` | object | yes |  |
| `data[].config.prompt_optimization` | boolean | yes | Optimizes the system prompt with guidelines for natural and realistic speech. Defaults to `true`. |
| `data[].config.speculative_generation` | boolean | yes | Starts LLM generation on an eager end-of-turn prediction to reduce response latency |
| `data[].config.system_tools` | ManagedAgentSystemTools | yes |  |
| `data[].config.timezone` | string | yes | IANA time zone for `{{system__time}}`. Defaults to `UTC`. |
| `data[].config.tools` | ManagedAgentToolReference[] | yes | Tools available to the agent. |
| `data[].config.turn` | object | yes |  |
| `data[].created_at` | string<date-time> | yes | Time when this version was created. |
| `data[].created_by` | string | yes | ID of the user or API key that created this version. `null` when no creator was recorded. |
| `data[].description` | string | yes | Description supplied when this version was created. `null` if none was provided. |
| `data[].id` | string | yes | Unique identifier for an immutable agent configuration version. |
| `has_more` | boolean | yes | Whether more results are available after this page. |
| `next_page` | string |  | An ID that can be passed as `starting_after` or `ending_before` to get the next page of data. |

- `404` `application/json` → PublicErrorResponse

| Field | Type | Req | Description |
|---|---|---|---|
| `doc_url` | string |  | URL to relevant documentation for the error |
| `error_code` | string |  | Machine-readable error code |
| `message` | string | yes | Detailed human-readable error message |
| `request_id` | string | yes | Unique identifier for this request. Include this when contacting support. |
| `title` | string | yes | Short human-readable error summary, like "File too large" |


### `GET /v1/agents/{agent_id}/versions/{version_id}` — Get an agent version

Returns one immutable configuration snapshot. To restore it, pass its `config` to `PATCH /v1/agents/{agent_id}`. Restoring creates a new version and does not change existing history.

- **Auth:** APIKeyAuth  ·  **operationId:** `getV1AgentsByAgent_idVersionsByVersion_id`  ·  **docs page:** `api-reference/agents/versions/get.md`

**Parameters**

| Name | In | Type | Req | Description |
|---|---|---|---|---|
| `agent_id` | path | string | yes | Unique identifier for the agent. |
| `version_id` | path | string | yes | Unique identifier for an immutable agent configuration version. |

**Responses**

- `200` `application/json` → ManagedAgentVersion

| Field | Type | Req | Description |
|---|---|---|---|
| `config` | ManagedAgentConfig | yes | Complete configuration for the agent's model, language, audio, and tools. |
| `config.audio` | object | yes |  |
| `config.audio.input` | object | yes |  |
| `config.audio.output` | object | yes |  |
| `config.dynamic_variable_placeholders` | Dynamic variable placeholders | yes | Sample values for Playground test calls. Production calls never use them. Replaces saved samples on update. Omit to keep them, or send `{}` to clear them. Vari… |
| `config.initial_message` | string | yes | Message the agent says when the conversation begins. Set to `null` to wait for the user to speak first. |
| `config.initial_message_interruptions` | string enum(`allow`, `ignore`, `respond_after`) | yes | Controls how the agent handles interruptions while `initial_message` plays. `allow` lets the caller interrupt it, `ignore` plays it in full and adds what the c… |
| `config.instructions` | string | yes | Instructions that define the agent's behavior. Supports dynamic variables such as `{{name}}`. |
| `config.language` | object | yes |  |
| `config.language.primary` | string enum(`en`, `fr`, `de`, `es`, `pt`, `zh`, `ja`, `hi`, `it`, `ko`, `nl`, `pl` …(+88)) | yes | The language the agent starts each call in: a language, such as `en`, or a locale, such as `en-US`. See [supported languages and locales]. |
| `config.language.additional` | string[] | yes | Other languages the caller can switch to, in order. Each is a language, such as `fr`, or a locale, such as `es-MX`. Returns `[]` when none are configured. Tags… |
| `config.language.presets` | object | yes | Settings per language, keyed by a language or locale such as `es-MX`. A preset applies only when the agent speaks that exact language or locale, including the … |
| `config.model` | object | yes |  |
| `config.model.id` | string | yes | Model ID from `GET /v1/agents/models`. |
| `config.model.fallback_strategy` | Auto \| Explicit \| object enum(`None`) | yes | Models to fall back to when the primary model fails before it starts responding. `auto` uses Cartesia-managed fallback models. `explicit` uses up to three mode… |
| `config.model.max_output_tokens` | integer | yes | Maximum number of tokens the model can generate, from `1` to `4096`. Set to `null` for no limit. |
| `config.model.temperature` | number | yes | Controls the randomness of model responses, from `0` to `1`. |
| `config.prompt_optimization` | boolean | yes | Optimizes the system prompt with guidelines for natural and realistic speech. Defaults to `true`. |
| `config.speculative_generation` | boolean | yes | Starts LLM generation on an eager end-of-turn prediction to reduce response latency |
| `config.system_tools` | ManagedAgentSystemTools | yes |  |
| `config.system_tools.end_call` | object | yes | Configuration for ending the call. Set to `null` to disable this tool. |
| `config.system_tools.send_dtmf` | object | yes | Configuration for sending DTMF tones. Set to `null` to disable this tool. |
| `config.system_tools.skip_turn` | object | yes | Configuration for skipping the agent's turn. Set to `null` to disable this tool. |
| `config.system_tools.voicemail_detection` | object | yes | Configuration for detecting voicemail. Set to `null` to disable this tool. |
| `config.system_tools.transfer_to_number` | object | yes | Configuration for transferring the call. Set to `null` to disable this tool. |
| `config.timezone` | string | yes | IANA time zone for `{{system__time}}`. Defaults to `UTC`. |
| `config.tools` | ManagedAgentToolReference[] | yes | Tools available to the agent. |
| `config.tools[].id` | string | yes | ID of a tool from the Tools API. |
| `config.turn` | object | yes |  |
| `config.turn.inactivity_end_call_secs` | integer | yes | Seconds of inactivity before the agent says a farewell message and ends the call, from `20` to `240`. Defaults to `240`; cannot be `null`. |
| `config.turn.inactivity_check_in_secs` | integer | yes | Seconds of inactivity before the agent checks in, from `5` to `60`. Defaults to `8`. Set to `null` to disable check-ins. When enabled, it must be less than `in… |
| `created_at` | string<date-time> | yes | Time when this version was created. |
| `created_by` | string | yes | ID of the user or API key that created this version. `null` when no creator was recorded. |
| `description` | string | yes | Description supplied when this version was created. `null` if none was provided. |
| `id` | string | yes | Unique identifier for an immutable agent configuration version. |

- `404` `application/json` → PublicErrorResponse

| Field | Type | Req | Description |
|---|---|---|---|
| `doc_url` | string |  | URL to relevant documentation for the error |
| `error_code` | string |  | Machine-readable error code |
| `message` | string | yes | Detailed human-readable error message |
| `request_id` | string | yes | Unique identifier for this request. Include this when contacting support. |
| `title` | string | yes | Short human-readable error summary, like "File too large" |



---

## WebSocket channels

### `WS /v1/agents/websocket/{agent_id}` — /v1/agents/websocket/{agent_id}

- **docs page:** `api-reference/agents/agent-websocket.md`  ·  **URL:** `wss://api.cartesia.ai/v1/agents/websocket/{agent_id}`

Stream audio and events between an application and a Managed Agent. Server applications authenticate with `X-API-Key`. Browser and mobile applications should create a trusted session on their server and pass the returned single-use token in the `access_token` query parameter. That parameter also accepts existing generic agent tokens, but only the call-bound session credential carries trusted session values. See the [WebSocket guide] for setup, limits, and audio handling.

**Query params**

| Name | Type | Req | Description |
|---|---|---|---|
| `agent_id` | string | yes | ID of the managed agent to connect to. |
| `cartesia_version` | string | yes | API version, e.g. `2026-08-14` |

**Client → server: `agentSessionCreateEvent`** — Session Create. Configures the session's audio. **This must be the first message sent.** The server closes the connection if any other event arrives first, or if no event arrives within 10 seconds. Unknown fields ar…

| Field | Type | Req | Description |
|---|---|---|---|
| `type` | string enum(`session_create`) | yes | Event type identifier. |
| `audio` | object | yes |  |
| `audio.input_format` | string enum(`mulaw_8000`, `pcm_16000`, `pcm_24000`, `pcm_44100`) | yes | Format of the audio your client sends. Agent audio is returned in the same format. Required, because a sample rate that mismatches your capture format fails si… |
| `audio.output_delivery` | string enum(`speaking_pace`, `as_available`) |  | How the server delivers agent audio. `speaking_pace` paces audio to playback speed. `as_available` sends it as fast as the model produces it, so a client that … |
| `audio.continuous_input` | boolean |  | Set to `true` if you stream audio continuously, including silence, such as from a phone line or an always-on microphone. The agent then waits for delayed audio… |
| `dynamic_variables` | object |  | Custom [dynamic variable] values for this session. Names are case-sensitive, up to 64 characters, and use letters, digits, or underscores. `system__` and `secr… |

**Client → server: `agentAudioInputEvent`** — Audio Input. Streams user audio to the agent. Send chunks continuously (for example every 20–100 ms) for the best latency. `audio` must be base64-encoded audio in the format declared in `session_create.audio.inpu…

| Field | Type | Req | Description |
|---|---|---|---|
| `type` | string enum(`audio_input`) | yes | Event type identifier. |
| `audio` | string | yes | Base64-encoded audio in the format declared in `session_create.audio.input_format`. |

**Client → server: `agentDtmfInputEvent`** — DTMF Input. Sends a DTMF (dual-tone multi-frequency) digit to the agent.

| Field | Type | Req | Description |
|---|---|---|---|
| `type` | string enum(`dtmf_input`) | yes | Event type identifier. |
| `digit` | string | yes | A single DTMF digit, one of `0`-`9`, `*`, or `#`. |

**Client → server: `agentClientToolResultEvent`** — Client Tool Result. Answers a `client_tool_call` whose `expects_response` is `true`. `tool_call_id` must match the call being answered. The decoded `result` may be at most 16 KiB; larger results are replaced with a `res…

| Field | Type | Req | Description |
|---|---|---|---|
| `type` | string enum(`client_tool_result`) | yes | Event type identifier. |
| `tool_call_id` | string | yes | The `tool_call_id` from the `client_tool_call` being answered. |
| `result` | string | yes | The tool's result, as a string the model can read. At most 16 KiB decoded. |
| `is_error` | boolean |  | Set to `true` when the tool failed. The LLM receives `result` as a normal string; Cartesia logs the failure and uses a generic message if `result` is empty. |
| `error_type` | string |  | Optional free-form string, at most 256 UTF-8 bytes. Cartesia logs this; the LLM does not see it. |

**Server → client: `agentSessionReadyEvent`** — Session Ready. Sent once after `session_create`, when the agent pipeline can accept audio. Reports the call record created for this session and the agent version the session is pinned to. Use `call_id` with the [ca…

| Field | Type | Req | Description |
|---|---|---|---|
| `type` | string enum(`session_ready`) | yes | Event type identifier. |
| `call_id` | string | yes | Identifier of the call record created for this session. Use it with the calls API to fetch the recording and transcript after the call. |
| `agent_id` | string | yes | The connected agent. |
| `agent_version_id` | string | yes | The agent version this session is pinned to, which is the version published when the connection opened. |
| `audio` | object | yes |  |
| `audio.input_format` | string |  | The accepted input format, echoed from `session_create`. |
| `audio.output_delivery` | string |  | The resolved output delivery mode. |
| `audio.continuous_input` | boolean |  | Echoed from `session_create` when `true`. |

**Server → client: `agentAudioOutputEvent`** — Audio Output. The agent's speech. `audio` is base64-encoded audio in the format declared in `session_create.audio.input_format`.

| Field | Type | Req | Description |
|---|---|---|---|
| `type` | string enum(`audio_output`) | yes | Event type identifier. |
| `audio` | string | yes | Base64-encoded agent audio in the format declared in `session_create.audio.input_format`. |

**Server → client: `agentAudioOutputClearEvent`** — Audio Output Clear. Sent when the user starts speaking. Discard buffered agent audio and stop playback. The event can arrive while no agent audio is playing, in which case there is nothing to clear.

| Field | Type | Req | Description |
|---|---|---|---|
| `type` | string enum(`audio_output_clear`) | yes | Event type identifier. |

**Server → client: `agentDtmfOutputEvent`** — DTMF Output. A DTMF digit the agent sends, for clients bridging a telephony system.

| Field | Type | Req | Description |
|---|---|---|---|
| `type` | string enum(`dtmf_output`) | yes | Event type identifier. |
| `digit` | string | yes | A single DTMF digit, one of `0`-`9`, `*`, or `#`. |

**Server → client: `agentClientToolCallEvent`** — Client Tool Call. The agent asks this client to run a client tool. When `expects_response` is `true`, answer with a `client_tool_result` carrying the same `tool_call_id`; the invocation stays open until a result, erro…

| Field | Type | Req | Description |
|---|---|---|---|
| `type` | string enum(`client_tool_call`) | yes | Event type identifier. |
| `tool_call_id` | string | yes | Identifies this invocation. Echo it in `client_tool_result`. |
| `tool_name` | string | yes | Name of the client tool to run. |
| `parameters` | object | yes | Arguments for the tool, matching the tool's parameter schema. |
| `expects_response` | boolean | yes | Whether the agent waits for a `client_tool_result`. Comes from the tool's definition. |

**Server → client: `agentTurnStartedEvent`** — Turn Started. Sent when the user starts speaking or the agent starts responding. `turn` comes from a single counter shared by both roles, starting at 1 and strictly increasing over the call, so `turn` alone identi…

| Field | Type | Req | Description |
|---|---|---|---|
| `type` | string enum(`turn_started`) | yes | Event type identifier. |
| `turn` | integer | yes | Turn index, unique across the call. |
| `role` | string enum(`user`, `assistant`) | yes | Who is speaking. |
| `start_time` | number | yes | Seconds since the start of the session. |

**Server → client: `agentTurnOutputTextDeltaEvent`** — Turn Text Delta. Sent as the agent speaks, carrying the text spoken since the previous delta, typically one word at a time. Deltas include separator spaces, so build the running text by appending `text` verbatim. The…

| Field | Type | Req | Description |
|---|---|---|---|
| `type` | string enum(`turn_output_text_delta`) | yes | Event type identifier. |
| `turn` | integer | yes | Matches the `turn` of the turn the text belongs to. |
| `role` | string enum(`assistant`) | yes | Always `assistant`. |
| `text` | string | yes | The exact substring to append. Separator spaces are included, so build the running text with `buffer[turn] += text`. |

**Server → client: `agentTurnEndedEvent`** — Turn Ended. Sent when a turn finishes, with the complete final text for the turn. This is the version to store and display. The events for a single turn always arrive in order: `turn_started`, then any `turn_out…

| Field | Type | Req | Description |
|---|---|---|---|
| `type` | string enum(`turn_ended`) | yes | Event type identifier. |
| `turn` | integer | yes | Matches the `turn` of the corresponding `turn_started` event. |
| `role` | string enum(`user`, `assistant`) | yes | Who spoke. |
| `text` | string | yes | The finalized text of the turn. For a user turn, the transcribed speech. For an assistant turn, the text the agent actually spoke. |
| `interrupted` | boolean | yes | For assistant turns, `true` when the user interrupted the turn. For user turns, `true` only when the call ended mid-turn. |
| `ignored` | boolean |  | Set to `true` on a user turn that ended while the agent's `initial_message` was playing with `initial_message_interruptions` set to `ignore`. |
| `start_time` | number | yes | Seconds since the start of the session. |
| `end_time` | number | yes | Seconds since the start of the session. |
| `eager_end_timestamp` | number |  | Seconds since session start when the speech-to-text model last predicted the caller had finished speaking. Included on user turns when available, even if specu… |
| `tool_calls` | array | yes | Tools called during this turn. |
| `tool_calls.name` | string | yes | Name of the tool. |
| `tool_calls.arguments` | object | yes | Arguments the model supplied. |
| `tool_calls.result` | string |  | The tool's result, when one was recorded. |
| `tool_calls.id` | string |  | Identifier of the invocation. |
| `tool_calls.dynamic_variable_updates` | array |  | Values assigned by this tool. Omitted when the tool assigned no values. |
| `tool_calls.dynamic_variable_updates.variable_name` | string | yes | Custom variable that changed |
| `tool_calls.dynamic_variable_updates.old_value` | anyOf | yes | Previous value, or null if the variable had no value |
| `tool_calls.dynamic_variable_updates.new_value` | anyOf | yes | Assigned value |
| `tool_calls.dynamic_variable_updates.updated_at` | number | yes | Time of the update, in seconds since the call started |
| `tool_calls.dynamic_variable_updates.tool_id` | string | yes | ID of the configured tool that assigned the value |
| `tool_calls.dynamic_variable_updates.tool_call_id` | string | yes | ID of the tool call that assigned the value, matching `tool_calls[].id` |

**Server → client: `agentErrorEvent`** — Error. Reports a problem with the session or an event you sent. When `fatal` is `false`, the server ignores that event and keeps the connection open. When `fatal` is `true`, it closes the connection with co…

| Field | Type | Req | Description |
|---|---|---|---|
| `type` | string enum(`error`) | yes | Event type identifier. |
| `code` | string enum(`invalid_event`, `agent_failed`, `missing_dynamic_variables`, `invalid_dynamic_variables`, `dynamic_variables_not_allowed`) | yes | `invalid_event` means a client event was rejected; `agent_failed` means the agent failed. The following errors prevent the session from starting: - `missing_dy… |
| `message` | string | yes | Human-readable explanation. |
| `fatal` | boolean | yes | Whether the server closes the connection after this error. |


### `WS /stt/turns/websocket` — /stt/turns/websocket

- **docs page:** `api-reference/stt/turns/websocket.md`  ·  **URL:** `wss://api.cartesia.ai/stt/turns/websocket`

Realtime speech transcription with built-in turn detection <Note> `ink-2` supports English, French, Hindi, Japanese, and Spanish. `ink-preview` supports [44 languages]. Both detect the language automatically. </Note>

**Client → server: `sttTurnsAudioData`** — Send Audio Data. Send raw audio as binary WebSocket messages, in small chunks such as 100 ms. The audio must match the `encoding` and `sample_rate` query parameters.

_(no fields)_

**Client → server: `sttTurnsCloseCommand`** — Close Command. Send a JSON formatted `close` command as a WebSocket text message to close the session cleanly. The model processes all buffered audio into events.

| Field | Type | Req | Description |
|---|---|---|---|
| `type` | string enum(`close`) | yes | Command type. Send this as a JSON formatted WebSocket text message to close the session. |

**Client → server: `sttTurnsConfigCommand`** — Config Command. Change turn detection settings, keyterms, or language hints mid-stream

| Field | Type | Req | Description |
|---|---|---|---|
| `type` | string enum(`config`) | yes | Command type. Send this as a JSON formatted WebSocket text message to update model settings. Changes take effect right away, including for words not returned y… |
| `turn` | object |  | Turn detection settings |
| `turn.start_threshold` | number |  | Threshold above which the turn starts. Default: 0.8. Range: 0.5–0.9. Must stay above `eager_end_threshold`. See [Configuring turn detection] for details. |
| `turn.eager_end_threshold` | number |  | Threshold below which `turn.eager_end` fires. Default: 0.6. Range: 0.3–0.8. Must stay between `end_threshold` and `start_threshold`. See [Configuring turn dete… |
| `turn.end_threshold` | number |  | Threshold below which the turn ends. Default: 0.3. Range: 0.05–0.5. Must stay below `eager_end_threshold`. See [Configuring turn detection] for details. |
| `turn.end_timeout_ms` | number |  | The longest the model waits after the user stops speaking before it ends the turn, in milliseconds. Default: 5600. Range: 640–11200. See [Configuring turn dete… |
| `keyterms` | array |  | `ink-preview` only. Replaces the session's keyterms. `[]` clears them, and leaving the field out keeps them. The limits are the same as at connect time. See [C… |
| `keyterms.item` | string |  |  |
| `language_hints` | array |  | `ink-preview` only. Replaces the session's language hints, up to 6. `[]` clears them, and leaving the field out keeps them. See [Change hints mid-stream]. |
| `language_hints.item` | string |  |  |

**Server → client: `sttTurnsConnected`** — Connected. Fires once when the WebSocket connection opens. You don't need to wait for this event before sending audio.

| Field | Type | Req | Description |
|---|---|---|---|
| `type` | string enum(`connected`) | yes | Event type identifier |
| `request_id` | string | yes | Unique identifier for this connection |

**Server → client: `sttTurnsTurnStart`** — Turn Start. Marks the start of a user turn. Fires soon after the user starts speaking. Use this event to interrupt your agent so it doesn't talk over the user.

| Field | Type | Req | Description |
|---|---|---|---|
| `type` | string enum(`turn.start`) | yes | Event type identifier |
| `request_id` | string | yes | Unique identifier for this connection. Does not change between turns. |
| `turn_id` | string |  | Identifies the turn. Every event in a turn has the same `turn_id`, and each new turn gets a new one. |

**Server → client: `sttTurnsTurnUpdate`** — Turn Update. Fires repeatedly as the model transcribes the current user turn.

| Field | Type | Req | Description |
|---|---|---|---|
| `type` | string enum(`turn.update`) | yes | Event type identifier |
| `transcript` | string | yes | All text transcribed so far in the current turn, not just the new text |
| `words` | array |  | `ink-preview` only. Every complete word in the turn so far, with timestamps. A word joins the list once the model has heard all of it, and it never changes aft… |
| `words.word` | string | yes | The word with any punctuation attached. `ink-whisper` words can start with a space. |
| `words.start` | number | yes | Start time in seconds, counted from the start of the audio sent on the connection |
| `words.end` | number | yes | End time in seconds, counted from the start of the audio sent on the connection |
| `request_id` | string | yes | Unique identifier for this connection. Does not change between turns. |
| `turn_id` | string |  | Identifies the turn. Every event in a turn has the same `turn_id`, and each new turn gets a new one. |

**Server → client: `sttTurnsTurnEagerEnd`** — Turn Eager End. Fires when the model predicts that the user might be done speaking.

| Field | Type | Req | Description |
|---|---|---|---|
| `type` | string enum(`turn.eager_end`) | yes | Event type identifier |
| `transcript` | string | yes | All text transcribed so far in the current turn. It can be empty. Don't start a reply for an empty transcript. See [Turn detection]. |
| `words` | array |  | `ink-preview` only. Every word in the turn, with timestamps. Left out when there are none. See [Word timestamps]. |
| `words.word` | string | yes | The word with any punctuation attached. `ink-whisper` words can start with a space. |
| `words.start` | number | yes | Start time in seconds, counted from the start of the audio sent on the connection |
| `words.end` | number | yes | End time in seconds, counted from the start of the audio sent on the connection |
| `languages` | array |  | `ink-preview` only. The languages spoken in the turn, highest confidence first. It lists the languages the model is confident about, or only the most likely on… |
| `languages.language` | string | yes | Language code. See [Languages]. |
| `languages.confidence` | number | yes | How confident the model is that this language was spoken, from 0 (not confident) to 1 (very confident). |
| `request_id` | string | yes | Unique identifier for this connection. Does not change between turns. |
| `turn_id` | string |  | Identifies the turn. Every event in a turn has the same `turn_id`, and each new turn gets a new one. |

**Server → client: `sttTurnsTurnResume`** — Turn Resume. Fires after `turn.eager_end` if the user turn has not actually ended.

| Field | Type | Req | Description |
|---|---|---|---|
| `type` | string enum(`turn.resume`) | yes | Event type identifier |
| `request_id` | string | yes | Unique identifier for this connection. Does not change between turns. |
| `turn_id` | string |  | Identifies the turn. Every event in a turn has the same `turn_id`, and each new turn gets a new one. |

**Server → client: `sttTurnsTurnEnd`** — Turn End. Marks the end of a user turn.

| Field | Type | Req | Description |
|---|---|---|---|
| `type` | string enum(`turn.end`) | yes | Event type identifier |
| `transcript` | string | yes | The final transcript for the completed turn. It can be empty. Discard turns that end with an empty transcript. See [Turn detection]. |
| `words` | array |  | `ink-preview` only. Every word in the turn, with timestamps. Left out when there are none. See [Word timestamps]. |
| `words.word` | string | yes | The word with any punctuation attached. `ink-whisper` words can start with a space. |
| `words.start` | number | yes | Start time in seconds, counted from the start of the audio sent on the connection |
| `words.end` | number | yes | End time in seconds, counted from the start of the audio sent on the connection |
| `languages` | array |  | `ink-preview` only. The languages spoken in the turn, highest confidence first. It lists the languages the model is confident about, or only the most likely on… |
| `languages.language` | string | yes | Language code. See [Languages]. |
| `languages.confidence` | number | yes | How confident the model is that this language was spoken, from 0 (not confident) to 1 (very confident). |
| `request_id` | string | yes | Unique identifier for this connection. Does not change between turns. |
| `turn_id` | string |  | Identifies the turn. Every event in a turn has the same `turn_id`, and each new turn gets a new one. |

**Server → client: `sttErrorResponse`** — Error Response. Error information for STT WebSocket connections

| Field | Type | Req | Description |
|---|---|---|---|
| `type` | string enum(`error`) | yes | Event type identifier |
| `error_code` | string |  | Machine-readable error code |
| `status_code` | number | yes | An HTTP response status code |
| `title` | string | yes | Human-readable error title |
| `message` | string | yes | Human-readable error message |
| `doc_url` | string |  | URL to relevant documentation |
| `request_id` | string |  | Unique identifier for this WebSocket connection |


### `WS /stt/websocket` — /stt/websocket

- **docs page:** `api-reference/stt/websocket.md`  ·  **URL:** `wss://api.cartesia.ai/stt/websocket`

Realtime speech transcription without turn detection <Note> Send the `finalize` command to trigger transcription. See [Compare STT Endpoints] for details. </Note>

**Client → server: `sttAudioData`** — Send Audio Data. Send raw audio as binary WebSocket messages, in small chunks such as 100 ms. The audio must match the `encoding` and `sample_rate` query parameters.

_(no fields)_

**Client → server: `sttFinalizeCommand`** — Finalize Command. Send `finalize` as a text message when the user has finished speaking. You get the transcript for any audio still buffered. The JSON form `{"type": "finalize"}` also works.

_(no fields)_

**Client → server: `sttCloseCommand`** — Close Command. Send `close` as a text message to flush the remaining audio and close the session. The server replies with `done`. The JSON form `{"type": "close"}` also works.

_(no fields)_

**Client → server: `sttConfigCommand`** — Config Command. Change keyterms or language hints mid-stream (`ink-preview` only)

| Field | Type | Req | Description |
|---|---|---|---|
| `type` | string enum(`config`) | yes | Command type. Send this as a JSON formatted WebSocket text message. `ink-preview` only. Changes take effect right away, including for words not returned yet. T… |
| `keyterms` | array |  | `ink-preview` only. Replaces the session's keyterms. `[]` clears them, and leaving the field out keeps them. The limits are the same as at connect time. See [C… |
| `keyterms.item` | string |  |  |
| `language_hints` | array |  | `ink-preview` only. Replaces the session's language hints, up to 6. `[]` clears them, and leaving the field out keeps them. See [Change hints mid-stream]. |
| `language_hints.item` | string |  |  |

**Server → client: `sttTranscriptResponse`** — Transcript Response. Transcript chunks. Send the `finalize` command when the user has finished speaking to get these chunks. The API can also send chunks before you send `finalize`.

| Field | Type | Req | Description |
|---|---|---|---|
| `type` | string enum(`transcript`) | yes | Response type identifier |
| `is_final` | boolean | yes | Whether `text` is finalized |
| `request_id` | string | yes | Unique identifier for this WebSocket connection |
| `text` | string | yes | Text transcribed since the last chunk with `"is_final": true`. To build the full transcript, concatenate the `text` of every chunk where `"is_final": true`. Do… |
| `duration` | number |  | Duration of the audio in seconds |
| `words` | array |  | Word-level timestamps, from `ink-preview` and `ink-whisper`. `ink-2` doesn't return them. With `ink-preview`, each event lists only the words completed since t… |
| `words.word` | string | yes | The word with any punctuation attached. `ink-whisper` words can start with a space. |
| `words.start` | number | yes | Start time in seconds, counted from the start of the audio sent on the connection |
| `words.end` | number | yes | End time in seconds, counted from the start of the audio sent on the connection |

**Server → client: `sttFlushDoneResponse`** — Flush Done Response. Acknowledgment for the `finalize` command

| Field | Type | Req | Description |
|---|---|---|---|
| `type` | string enum(`flush_done`) | yes | Response type identifier |
| `request_id` | string | yes | Unique identifier for this WebSocket connection |
| `languages` | array |  | `ink-preview` only. Languages detected at this `finalize`, highest confidence first. Detection looks at the most recent audio, so this can include audio from b… |
| `languages.language` | string | yes | Language code. See [Languages]. |
| `languages.confidence` | number | yes | How confident the model is that this language was spoken, from 0 (not confident) to 1 (very confident). |
| `is_final` | boolean |  | Has no meaning for this message |

**Server → client: `sttDoneResponse`** — Done Response. Acknowledgment for the `close` command

| Field | Type | Req | Description |
|---|---|---|---|
| `type` | string enum(`done`) | yes | Response type identifier |
| `request_id` | string | yes | Unique identifier for this WebSocket connection |
| `languages` | array |  | `ink-preview` only. Languages detected in audio sent after the last `finalize`, highest confidence first. Detection looks at the most recent audio, so this can… |
| `languages.language` | string | yes | Language code. See [Languages]. |
| `languages.confidence` | number | yes | How confident the model is that this language was spoken, from 0 (not confident) to 1 (very confident). |
| `is_final` | boolean |  | Has no meaning for this message |

**Server → client: `sttErrorResponse`** — Error Response. Error information for STT WebSocket connections

| Field | Type | Req | Description |
|---|---|---|---|
| `type` | string enum(`error`) | yes | Event type identifier |
| `error_code` | string |  | Machine-readable error code |
| `status_code` | number | yes | An HTTP response status code |
| `title` | string | yes | Human-readable error title |
| `message` | string | yes | Human-readable error message |
| `doc_url` | string |  | URL to relevant documentation |
| `request_id` | string |  | Unique identifier for this WebSocket connection |


### `WS /tts/websocket` — /tts/websocket

- **docs page:** `api-reference/tts/websocket.md`  ·  **URL:** `wss://api.cartesia.ai/tts/websocket`

Generate audio in realtime with contexts

**Query params**

| Name | Type | Req | Description |
|---|---|---|---|
| `cartesia_version` | string | yes | API version, e.g. `2026-08-14` |

**Client → server: `generationRequest`** — Generation Request. Use this to generate speech for a transcript.

| Field | Type | Req | Description |
|---|---|---|---|
| `model_id` | string enum(`sonic-3.6`, `sonic-3.5`, `sonic-3`, `sonic-latest`) | yes | The ID of the model to use for the generation. See [Models] for all options. |
| `transcript` | string | yes | Transcript chunk to add to the audio being generated by this context. |
| `voice` | oneOf | yes | The voice to use for generation. Pass either a voice ID string or an object with a required `id` (additional object fields may be added in future API versions)… |
| `voice.id` | string | yes | The ID of the voice. |
| `output_format` | object | yes | Audio output format configuration. Use the same value for all generation requests made to the same context. |
| `output_format.container` | string enum(`raw`) | yes | Audio container format |
| `output_format.encoding` | string enum(`pcm_f32le`, `pcm_s16le`, `pcm_mulaw`, `pcm_alaw`) | yes | Audio encoding format. See [TTS Output Audio Format] if you're unsure what to use. |
| `output_format.sample_rate` | integer enum(`8000`, `16000`, `22050`, `24000`, `44100`, `48000`) | yes | Audio sample rate in Hz. |
| `language` | string enum(`en`, `fr`, `de`, `es`, `pt`, `zh`, `ja`, `hi`, `it`, `ko`, `nl`, `pl`) |  | The transcript's language or locale. Use a base language code such as `en` or a regional locale code such as `en-GB`. `language` and `locale` accept the same v… |
| `locale` | string enum(`en`, `fr`, `de`, `es`, `pt`, `zh`, `ja`, `hi`, `it`, `ko`, `nl`, `pl`) |  | The transcript's language or locale. Use a base language code such as `en` or a regional locale code such as `en-GB`. `language` and `locale` accept the same v… |
| `accent` | string |  | Usually unnecessary: Cartesia picks the closest accent the voice supports for the requested `language` or `locale`. Set it only to make a [multilingual voice] … |
| `normalization` | string |  | Text normalization. `auto` (initial default) runs the locale-aware normalizer, `off` skips it, or pass a language or locale code (for example `en` or `en-IN`) … |
| `context_id` | string | yes | A unique identifier for the context. You can use any unique identifier, like a UUID or human ID. See [Contexts] for details. |
| `continue` | boolean |  | Whether `transcript` may be followed by more text: - Set to `true` if you will add more `transcript` chunks to this context - Set to `false` on the last `trans… |
| `max_buffer_delay_ms` | integer |  | The maximum time in milliseconds to buffer text before starting generation. Values between [0, 5000]ms are supported. Defaults to 3000ms. See [Buffering] for d… |
| `flush` | boolean |  | Whether to flush the context See [Context Flushing] for details |
| `add_timestamps` | boolean |  | Whether to include word-level timestamps for the generated audio |
| `add_phoneme_timestamps` | boolean |  | Whether to include phoneme-level timestamps for the generated audio |
| `use_normalized_timestamps` | boolean |  | Whether to use normalized timestamps (`true`) or original timestamps (`false`) |
| `pronunciation_dict_id` | string |  | The ID of a pronunciation dictionary to use for the generation See [Custom Pronunciations] for details |
| `generation_config` | object |  | Configure the various attributes of the generated speech See [Volume, Speed, and Emotion] for details |
| `generation_config.volume` | number |  | Adjust the volume of the generated speech between 0.5x and 2.0x the default volume. Valid values are between [0.5, 2.0] inclusive. |
| `generation_config.speed` | number |  | Adjust the speed of the generated speech between 0.6x and 1.5x the default speed. Valid values are between [0.6, 1.5] inclusive. |
| `generation_config.emotion` | string enum(`neutral`, `calm`, `angry`, `content`, `sad`) |  | Optional. If omitted, the model interprets the emotional subtext of the transcript. Must match a valid option exactly. A complete list can be found on the [Vol… |

**Client → server: `cancelRequest`** — Cancel Context Request. Use this to cancel a context, so that no more messages are generated for that context.

| Field | Type | Req | Description |
|---|---|---|---|
| `context_id` | string | yes | A unique identifier for the context. You can use any unique identifier, like a UUID or human ID. See [Contexts] for details. |
| `cancel` | boolean enum(`True`) | yes | Whether to cancel the context, so that no more messages are generated for that context. |

**Server → client: `chunkResponse`** — Audio Chunk Response. Audio data chunk

| Field | Type | Req | Description |
|---|---|---|---|
| `type` | string enum(`chunk`) | yes | Response type identifier |
| `data` | string | yes | Base64-encoded audio data |
| `done` | boolean | yes | Whether this is the final chunk for this context |
| `status_code` | integer | yes | HTTP-style status code |
| `step_time` | number | yes | Server-side processing time for this chunk in milliseconds |
| `context_id` | string | yes | A unique identifier for the context. You can use any unique identifier, like a UUID or human ID. See [Contexts] for details. |

**Server → client: `flushDoneResponse`** — Flush Done Response. Acknowledgment that flush command was received. See [Context Flushing] for details.

| Field | Type | Req | Description |
|---|---|---|---|
| `type` | string enum(`flush_done`) | yes | Response type identifier |
| `done` | boolean | yes | Whether generation is complete |
| `flush_done` | boolean | yes | Whether the flush is complete |
| `flush_id` | integer | yes | An identifier corresponding to the number of flush commands that have been sent for this context. Starts at 1. This can be used to map chunks of audio to certa… |
| `status_code` | integer | yes | HTTP-style status code |
| `context_id` | string | yes | A unique identifier for the context. You can use any unique identifier, like a UUID or human ID. See [Contexts] for details. |

**Server → client: `doneResponse`** — Done Response. Generation completion signal

| Field | Type | Req | Description |
|---|---|---|---|
| `type` | string enum(`done`) | yes | Response type identifier |
| `done` | boolean | yes | Whether generation is complete |
| `status_code` | integer | yes | HTTP-style status code |
| `context_id` | string | yes | A unique identifier for the context. You can use any unique identifier, like a UUID or human ID. See [Contexts] for details. |

**Server → client: `timestampsResponse`** — Word Timestamps Response. Word-level timing information

| Field | Type | Req | Description |
|---|---|---|---|
| `type` | string enum(`timestamps`) | yes | Response type identifier |
| `done` | boolean | yes | Whether generation is complete |
| `status_code` | integer | yes | HTTP-style status code |
| `context_id` | string | yes | A unique identifier for the context. You can use any unique identifier, like a UUID or human ID. See [Contexts] for details. |
| `word_timestamps` | object |  | Word-level timing information |
| `word_timestamps.words` | array |  | List of words in order |
| `word_timestamps.words.item` | string |  |  |
| `word_timestamps.start` | array |  | Start times in seconds for each word |
| `word_timestamps.start.item` | number |  |  |
| `word_timestamps.end` | array |  | End times in seconds for each word |
| `word_timestamps.end.item` | number |  |  |

**Server → client: `phonemeTimestampsResponse`** — Phoneme Timestamps Response. Phoneme-level timing information

| Field | Type | Req | Description |
|---|---|---|---|
| `type` | string enum(`phoneme_timestamps`) | yes | Response type identifier |
| `done` | boolean | yes | Whether generation is complete |
| `status_code` | integer | yes | HTTP-style status code |
| `context_id` | string | yes | A unique identifier for the context. You can use any unique identifier, like a UUID or human ID. See [Contexts] for details. |
| `phoneme_timestamps` | object |  | Phoneme-level timing information |
| `phoneme_timestamps.phonemes` | array |  | List of phonemes in order |
| `phoneme_timestamps.phonemes.item` | string |  |  |
| `phoneme_timestamps.start` | array |  | Start times in seconds for each phoneme |
| `phoneme_timestamps.start.item` | number |  |  |
| `phoneme_timestamps.end` | array |  | End times in seconds for each phoneme |
| `phoneme_timestamps.end.item` | number |  |  |

**Server → client: `ttsErrorResponse`** — Error Response. Error information for TTS WebSocket connections.

| Field | Type | Req | Description |
|---|---|---|---|
| `type` | string enum(`error`) | yes | Response type identifier |
| `done` | boolean |  | Whether generation is complete |
| `error_code` | string |  | Machine-readable error code. |
| `status_code` | number | yes | An HTTP response status code. |
| `title` | string | yes | Human-readable error title. |
| `message` | string | yes | Human-readable error message. |
| `doc_url` | string |  | URL to relevant documentation |
| `request_id` | string |  | Unique identifier for this websocket connection |
| `context_id` | string |  | A unique identifier for the context. You can use any unique identifier, like a UUID or human ID. See [Contexts] for details. |

