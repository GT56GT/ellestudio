# Store package extraction Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move all persistence (Postgres and file backends, migrations, data types) out of `internal/bot` into a new `internal/store` package, with no behaviour change, so the admin panel stops importing the bot.

**Architecture:** Pure move-and-rename refactor. Types and backends move to `internal/store`; `internal/bot` keeps its existing names through Go type aliases in one file (`internal/bot/aliases.go`) so the ~14k lines of bot code compile unchanged. `internal/admin` and `cmd/*` switch their imports to `internal/store`. Every move keeps its tests and the full suite stays green after each task.

**Tech Stack:** Go 1.26, pgx v5, GitHub Actions CI with a Postgres 17 service (`TEST_DATABASE_URL`).

**Spec:** `docs/superpowers/specs/2026-11-28-web-app-redesign-design.md` §8.2 and §9 (ships first, as its own pull request from branch `refactor/store`).

## Global Constraints

- No behaviour change: same SQL, the same migrations 1–9 in the same order (byte for byte), same JSON field names.
- After every task: `go build ./...`, `go vet ./...`, `gofmt -l .` prints nothing, `go test -race -count=1 ./...` passes. CI (with Postgres) passes before merge.
- `internal/store` must not import `internal/bot`, `internal/admin` or `internal/telegram`.
- Moved exported names keep their names (`store.Settings`, `store.ChatInfo`, `store.DB`, `store.OpenDatabase`, …). One rename: the settings store type `bot.Store` becomes `store.SettingsStore`.

## Review Focus

- **Old and new binaries run side by side during a deploy** → migrations unchanged; `TestMigrationsAreIdempotent` passes and `git diff` of the moved `migrations` slice shows only the package line.
- **JSON written by the old binary is read by the new one** → field tags of `Settings`, `Ban`, `LimitOverrides`, `Status`, `GroupSettings` unchanged; pinned by `TestSettingsJSONUnchanged` (Task 2).
- **Unexported helpers crossing the package line** (`pgTimeout`, kinds, statuses) → exported with identical values; `PgTimeout` stays `5 * time.Second`.
- **The panel still compiles against the bot** → after Task 4, `go list -deps ./internal/admin | grep -c ellestudio/internal/bot` prints `0`.
- **File-backed local runs** (no `DATABASE_URL`) → `TestFileStorePersists` and the file owners/chats tests move and still pass.

---

### Task 1: Package skeleton, DB and migrations

**Files:**
- Create: `internal/store/db.go` (from `internal/bot/db.go`: `DB`, `OpenDatabase`, `migrate`, `migrations`, `migrationLock`, `pgTimeout`, `(*DB).Close`, `(*DB).Pool` from `status.go`)
- Create: `internal/store/storetest/storetest.go` (the `testDB` helper from `internal/bot/store_test.go`, exported)
- Create: `internal/store/db_test.go` (`TestMigrationsAreIdempotent`)
- Create: `internal/bot/aliases.go`
- Modify: `internal/bot/db.go`, `internal/bot/status.go`, `internal/bot/store_test.go` (remove what moved; use `storetest.TestDB`)

**Interfaces:**
- Produces: `store.DB`; `store.OpenDatabase(ctx context.Context, url string) (*store.DB, error)`; `(*store.DB).Pool() *pgxpool.Pool`; `store.PgTimeout = 5 * time.Second`; `storetest.TestDB(t *testing.T) *store.DB` (skips without `TEST_DATABASE_URL`).
- `internal/bot/aliases.go` starts with: `type DB = store.DB`, `var OpenDatabase = store.OpenDatabase`, `const pgTimeout = store.PgTimeout`.

- [ ] **Step 1:** Move the code and tests above.
- [ ] **Step 2:** Run `go build ./... && go test -race -count=1 ./...` — Expected: every package `ok`.
- [ ] **Step 3:** Commit `refactor(store): move DB and migrations into internal/store`.

### Task 2: Settings store

**Files:**
- Create: `internal/store/settings.go` (from `internal/bot/settings.go`: `Settings`, `Ban`, `Usage`, `usageToday`, `overLimit`, the store type, `OpenStore`, `persister`, `sharedStore`, `userTx`, the file backend, `writeFileAtomic`, `ref`/`voiceRef`/`dictRef`)
- Create: `internal/store/pgsettings.go` (from `internal/bot/db.go`: `pgSettings`, `(*DB).SettingsStore`, `userLockSQL`)
- Create: `internal/store/settings_test.go` (moved: `exerciseStore`, `TestFileStorePersists`, `TestPostgresStorePersists`, `TestFileStoreConcurrentSpends`, `TestPostgresStoreSharedByTwoInstances`, plus the new JSON test)
- Modify: `internal/bot/aliases.go`

**Interfaces:**
- Produces: `store.Settings`, `store.Ban`, `store.Usage`, `store.SettingsStore` (every method unchanged), `store.OpenStore(path string, defaults Settings) (*SettingsStore, error)`, `(*DB).SettingsStore(ctx context.Context, defaults Settings) (*SettingsStore, error)`, `store.OverLimit(used, limit int) int`.
- `aliases.go` adds: `type Settings = store.Settings`, `type Ban = store.Ban`, `type Usage = store.Usage`, `type Store = store.SettingsStore`, `var OpenStore = store.OpenStore`, `func overLimit(used, limit int) int { return store.OverLimit(used, limit) }`.

- [ ] **Step 1: Pin today's JSON.** In `internal/bot`, before moving anything, write `TestSettingsJSONUnchanged`: build a `Settings` with every field non-zero (including `Ban` with `Until`, `NoNews`, `Referrals`, `TipDay`), `json.Marshal` it and compare to a string literal captured from one run.
- [ ] **Step 2:** Run `go test -run TestSettingsJSONUnchanged ./internal/bot` — Expected: PASS.
- [ ] **Step 3:** Move the code and tests (the JSON test moves to `internal/store/settings_test.go` unchanged).
- [ ] **Step 4:** Run `go test -race -count=1 ./...` — Expected: every package `ok`.
- [ ] **Step 5:** Commit `refactor(store): move the settings store`.

### Task 3: Registries, metrics, budget

**Files:**
- Create: `internal/store/owners.go` (`Owners`, `Owned`, `Cursor`, file owners, `pgOwners`; `errAlreadyOwned` → `ErrAlreadyOwned`; `kindVoice`/`kindDict` → `KindVoice`/`KindDict`)
- Create: `internal/store/chats.go` (`Chats`, `ChatInfo`, `GroupSettings`, file chats, `pgChats`; `chatMember`/`chatLeft` → `ChatMember`/`ChatLeft`)
- Create: `internal/store/metrics.go` (`Event`, `DayStats`, `Metrics`, `memMetrics`, `pgMetrics`; event kinds → `EvStart`, `EvReading`, `EvTranscribe`, `EvLimit`, `EvGuestInvite`)
- Create: `internal/store/budget.go` (`Budget`, `memBudget`, `pgBudget`, `guardedBudget`, `fits`, `ErrBudgetUnknown`)
- Move matching storage tests (`exerciseOwners`, chats storage tests, `exerciseMetrics`, `exerciseBudget`, the `guardedBudget` part of `TestStudioCapHoldsDuringDatabaseOutage`); tests that drive the bot through fixtures stay in `internal/bot`.
- Modify: `internal/bot/aliases.go` (an alias for every moved name, e.g. `const kindVoice = store.KindVoice`)

**Interfaces:**
- Produces: the names above; `store.OpenFileOwners(path string) (Owners, error)`, `store.OpenFileChats(path string) (Chats, error)`, `store.NewMemMetrics() Metrics`, `store.NewMemBudget() Budget`, `store.GuardBudget(b Budget) Budget`; `(*DB).Owners()`, `(*DB).Chats()`, `(*DB).Metrics()`, `(*DB).Budget()`.
- `bot.SetBudget(bu)` stores `store.GuardBudget(bu)`.

- [ ] **Step 1:** Move code and tests; touch bot call sites only where an unexported name became exported and no alias covers it.
- [ ] **Step 2:** Run `go test -race -count=1 ./...` — Expected: every package `ok`.
- [ ] **Step 3:** Commit `refactor(store): move owners, chats, metrics and budget`.

### Task 4: Panel-facing data and the admin switch-over

**Files:**
- Create: `internal/store/status.go` (`Status`, `StatusLimits`, `(*DB).ReportStatus`, `LatestStatus`)
- Create: `internal/store/limits.go` (`Limits`, `LimitOverrides` with `Apply`/`Validate`, `LoadOverrides`, `SaveOverrides`, `(*DB).LoadOverrides`)
- Create: `internal/store/adminqueue.go` (`AdminJob`, `AdminQueue`, `JobDeleteVoice`, `JobDeleteDict`, `pgAdminQueue`, `(*DB).AdminQueue`)
- Create: `internal/store/broadcasts.go` (`Broadcast`, `BroadcastQueue`, `AudienceEveryone`, `AudienceActive`, `recipientsWhere`, `CountRecipients`, `BroadcastHTML`, `pgBroadcasts`, `(*DB).Broadcasts`)
- Modify: every file in `internal/admin/` and both `cmd/*/main.go`: import `ellestudio/internal/store` instead of `ellestudio/internal/bot` (`cmd/ellie` still imports `bot` for `bot.New`).
- Move store-level Postgres tests out of `internal/admin/pg_test.go` (`ClaimBroadcast`, `Recipients`, overrides round trip) into `internal/store/`; panel tests stay.

**Interfaces:**
- Produces: the names above with unchanged signatures. `internal/bot` keeps the heartbeat, job worker, broadcast sender and ban handling, using these types through aliases.

- [ ] **Step 1:** Move code and tests; switch the imports.
- [ ] **Step 2:** Run `go list -deps ./internal/admin | grep -c ellestudio/internal/bot` — Expected: `0`.
- [ ] **Step 3:** Run `go vet ./... && go vet -tags embedui ./internal/admin ./cmd/ellie-admin && go test -race -count=1 ./...` — Expected: every package `ok`.
- [ ] **Step 4:** In README "Layout", add the line `internal/store/       persistence: Postgres and file backends, migrations, data types`.
- [ ] **Step 5:** Commit `refactor(store): move panel data; the admin panel no longer imports the bot`.

### Task 5: Ship

- [ ] **Step 1:** Push branch `refactor/store` and open a pull request — Expected: CI jobs `test` and `admin-ui` both `success`.
- [ ] **Step 2:** After the user approves merging: merge, deploy the bot then the panel (`render deploys create <service> --commit <sha> --wait --confirm`), and check both logs show `settings stored in Postgres` and `admin panel listening` with no `level=ERROR`.
