# Web app redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rebuild the admin web app (and its Telegram Mini App mode) on the approved design system and EasyUI components, keeping every Phase 1–3 feature.

**Architecture:** Branch `redesign/web-app`, cut after the `refactor/store` pull request (`docs/superpowers/plans/2026-11-28-store-package.md`) is merged. `admin-ui/` moves to React 19 + Tailwind v4 + shadcn config; EasyUI components are installed as source into `src/components/ui/`; features live in `src/features/<name>/`; role tokens in `src/styles/tokens.css`; a Telegram bridge in `src/lib/telegram.ts`. The Go server and `go:embed` build are unchanged except one API filter.

**Tech Stack:** React 19, TypeScript, Vite, Tailwind CSS v4 (`@tailwindcss/vite`), shadcn CLI, EasyUI (`Surajmaurya1/easyui`), Framer Motion, lucide-react, TanStack Query, Vitest + Testing Library + jsdom, Go 1.26.

**Spec:** `docs/superpowers/specs/2026-11-28-web-app-redesign-design.md` (approved). Executors read it alongside this plan; section numbers below refer to it.

## Global Constraints

- Palette and role tokens exactly as spec §5.1–§5.2; feature code uses role utilities only (`bg-page`, `bg-surface`, `text-fg`, `text-muted`, `border-line`, `border-input`, `bg-action`, `text-on-action`, `brand`, `text-accent`, status roles, `chart-1`…`chart-6`). No hex colours outside `src/styles/`.
- Type sizes only `text-xs` 12/16, `text-sm` 13/18, `text-base` 15/22, `text-lg` 18/24, `text-xl` 22/28, `text-2xl` 28/32, `text-hero` 44/44 (34 below 768 px). Weights 400, 550, 650.
- Spacing only Tailwind steps 1, 2, 3, 4, 5, 6, 8, 10, 12, 16; radii `rounded-sm` 6, `rounded-lg` 12, `rounded-2xl` 20, `rounded-full`; no arbitrary values (`-[`) and no `style={{` outside `src/components/ui/` and chart code — enforced by `scripts/check-tokens.mjs` in CI.
- Touch targets ≥ 44 × 44 px in the phone shell. Motion 120/200/320 ms; respect `prefers-reduced-motion`.
- CSP unchanged (scripts self + telegram.org). Initial JS ≤ 200 KB gzipped. Browsers Safari 16.4+ / Chrome 111+.
- Every Phase 1–3 behaviour keeps working: sign-in (widget and Mini App), and every endpoint the old UI called.
- Copy: plain, sentence case, no apologies; buttons say what happens ("Save limits", "Send to 270 people").

## Review Focus

- **Telegram sends missing or swapped theme colours** (old clients; iOS dark since 10.2) → roles fall back to the browser palette for the scheme, and the swap guard fixes `bg`/`secondary_bg`; pinned in Task 2 by `themeFromTelegram` tests for "no params", "partial params" and "swapped on dark".
- **Telegram Desktop in a wide window** → always the phone shell; pinned in Task 4 by `useShell` test "telegram at 1400px returns mobile".
- **Drag-to-confirm with a keyboard or screen reader** (WCAG 2.5.7 needs a non-drag alternative) → `ConfirmAction` also completes with Enter held on the focused handle and exposes a "Confirm" button to assistive tech; pinned in Task 5 by "confirms from keyboard".
- **The bot hasn't sent a heartbeat yet** (fresh deploy) → Overview, Settings and the status pill render with "No word from Ellie yet" and empty defaults, never crash; pinned in Task 7 and Task 12 tests with `bot: null`.
- **Long content on phone cards** (60-char group titles, 13-digit IDs, 1,000,000+ numbers) → stacked cards wrap or truncate with full text in `title`; pinned in Task 9 by a long-title render test asserting no horizontal overflow class (`overflow-x-auto` absent on the card list).

---

### Task 1: Foundation stack

**Files:**
- Modify: `admin-ui/package.json` (deps: `react@19`, `react-dom@19`, `@types/react@19`, `@types/react-dom@19`, `tailwindcss@4`, `@tailwindcss/vite@4`, `framer-motion`, `lucide-react`, `@tanstack/react-query@5`, `clsx`, `tailwind-merge`; dev: `vitest`, `@testing-library/react`, `@testing-library/user-event`, `jsdom`; scripts: `"test": "vitest run"`, `"lint:tokens": "node scripts/check-tokens.mjs"`, `"build": "tsc -b && vite build"`), `admin-ui/vite.config.ts` (add `@tailwindcss/vite`, `resolve.alias` `@` → `src`, `test: { environment: 'jsdom' }`; keep `outDir: '../internal/admin/dist'`), `admin-ui/tsconfig.json` (`baseUrl`, `paths: {"@/*": ["src/*"]}`)
- Create: `admin-ui/components.json` (shadcn: `style: "new-york"`, `tsx: true`, `tailwind.css: "src/styles/globals.css"`, `aliases.components: "@/components"`, `aliases.ui: "@/components/ui"`, `aliases.lib: "@/lib"`), `admin-ui/src/lib/cn.ts`, `admin-ui/scripts/check-tokens.mjs`, `admin-ui/src/lib/cn.test.ts`
- Modify: `.github/workflows/ci.yml` admin-ui job: run `npm run lint:tokens`, `npm test`, `npm run build`.

**Interfaces:**
- Produces: `cn(...inputs: ClassValue[]): string` (clsx + tailwind-merge); `check-tokens.mjs` exits 1 listing `file:line` for any `-[` in `className`, any `style={{` and any `#[0-9a-f]{3,8}` outside `src/components/ui/`, `src/styles/`, `src/features/*/charts/`.

- [ ] **Step 1:** Write `cn.test.ts`: `cn('p-2', 'p-4') === 'p-4'`; and a fixture test for `check-tokens.mjs` (spawn it on a temp dir with `className="p-[13px]"`, expect exit code 1 and the path in stdout).
- [ ] **Step 2:** Run `npm test` — Expected: FAIL (modules missing).
- [ ] **Step 3:** Install deps, add configs, implement `cn` and the checker.
- [ ] **Step 4:** Run `npm test && npm run build` — Expected: tests pass; build writes `internal/admin/dist/index.html`.
- [ ] **Step 5:** Commit `build(web): React 19, Tailwind v4, shadcn config, Vitest, token checker`.

### Task 2: Tokens and theme

**Files:**
- Create: `admin-ui/src/styles/tokens.css` (primitives from spec §5.1 as `--color-<ramp>-<stop>`; roles from §5.2 in `@theme` as `--color-page`, `--color-surface`, `--color-fg`, `--color-muted`, `--color-line`, `--color-input`, `--color-action`, `--color-on-action`, `--color-brand`, `--color-accent`, `--color-success|warning|danger|info` (text) and `-tint`/`-on-tint` pairs, `--color-chart-1..6`; light values on `:root`, dark under `[data-theme=dark]` and `@media (prefers-color-scheme: dark) :root:not([data-theme=light])`; Telegram values under `[data-tg]` reading `--tg-*` variables written by `applyTheme`), `admin-ui/src/styles/globals.css` (imports Tailwind, tokens; type scale §5.3 as `--text-*` theme values; radii §5.4), `admin-ui/src/lib/theme.ts`, `admin-ui/src/lib/theme.test.ts`

**Interfaces:**
- Produces: `type TelegramThemeParams = Partial<Record<'bg_color'|'secondary_bg_color'|'section_bg_color'|'text_color'|'hint_color'|'link_color'|'button_color'|'button_text_color'|'accent_text_color'|'destructive_text_color'|'header_bg_color', string>>`; `themeFromTelegram(p: TelegramThemeParams, scheme: 'light'|'dark'): Record<string, string>` (CSS variable name → value, only for params present, after the swap guard); `applyTheme(vars: Record<string,string>, scheme: 'light'|'dark'): void` (sets them on `document.documentElement`, sets `data-tg` and `data-theme`).

- [ ] **Step 1:** Write `theme.test.ts`: (a) all params present → `--tg-bg` = `secondary_bg_color` mapping per §5.2 table (page ← secondary, surface ← section ?? bg); (b) no params → `{}`; (c) dark scheme with `bg_color:'#ffffff'`, `secondary_bg_color:'#000000'` → values swapped; (d) light scheme with the reverse → swapped.
- [ ] **Step 2:** Run `npx vitest run src/lib/theme.test.ts` — Expected: FAIL.
- [ ] **Step 3:** Implement `theme.ts` (relative luminance for the guard) and the CSS files.
- [ ] **Step 4:** Run `npm test && npm run build` — Expected: PASS.
- [ ] **Step 5:** Commit `feat(web): design tokens and Telegram theme mapping`.

### Task 3: Telegram bridge and data layer

**Files:**
- Create: `admin-ui/src/lib/telegram.ts`, `admin-ui/src/lib/telegram.test.ts`, `admin-ui/src/app/providers.tsx`
- Move: `src/api.ts` → `src/lib/api.ts`, `src/format.ts` → `src/lib/format.ts`, `src/router.ts` → `src/app/router.ts`, `src/types.ts` → `src/lib/types.ts` (unchanged behaviour)

**Interfaces:**
- Produces: `launchData(): string | null` (from `#tgWebAppData`); `isTelegram(): boolean`; `loadTelegram(): Promise<TelegramWebApp | null>` (injects `https://telegram.org/js/telegram-web-app.js` once, only when `isTelegram()`; calls `ready()`, `expand()`; wires `themeChanged` → `applyTheme(themeFromTelegram(...))`); `useBackButton(onBack: (() => void) | null): void`; `useMainButton(cfg: { text: string; onClick: () => void; enabled?: boolean } | null): void`; `haptic(kind: 'success' | 'error' | 'light'): void` (no-op outside Telegram). `<Providers>` wraps the app in `QueryClientProvider` (`staleTime: 15_000`, `retry: 1`) and a toast provider slot.

- [ ] **Step 1:** Write `telegram.test.ts` with a fake `window.Telegram.WebApp` (BackButton/MainButton/HapticFeedback spies): `useBackButton(fn)` shows the button and calls `fn` on click, hides on unmount; `useMainButton(null)` hides it; `haptic` outside Telegram does nothing; `launchData()` returns the hash param.
- [ ] **Step 2:** Run the test — Expected: FAIL.
- [ ] **Step 3:** Implement; move the four files and fix imports.
- [ ] **Step 4:** Run `npm test && npm run build` — Expected: PASS.
- [ ] **Step 5:** Commit `feat(web): Telegram bridge and TanStack Query providers`.

### Task 4: Shells and navigation

**Files:**
- Install (`npx shadcn@latest add Surajmaurya1/easyui/<name>`): `sidebar`, `small-floating-dock`, `sheet`, `dynamic-island`, `command-menu`, `breadcrumb`
- Create: `admin-ui/src/app/useShell.ts`, `admin-ui/src/app/useShell.test.ts`, `admin-ui/src/app/DesktopShell.tsx`, `admin-ui/src/app/MobileShell.tsx`, `admin-ui/src/app/nav.ts`, `admin-ui/src/components/StatusPill.tsx`, `admin-ui/src/components/Page.tsx`
- Modify: `admin-ui/src/App.tsx` (pick the shell; sign-in gate unchanged)

**Interfaces:**
- Produces: `useShell(): 'desktop' | 'tablet' | 'mobile'` (≥1024 desktop, 768–1023 tablet, else mobile; always `mobile` when `isTelegram()`); `NAV: { path: string; label: string; icon: LucideIcon; dock: boolean }[]` (dock items: Overview, Users, Groups, Voices; others in the More sheet: Announcements, Settings, Audit log, Sessions); `<Page title lede actions? back?>` renders the header (title hidden inside Telegram, BackButton used for `back`) and its content on the spec §6.2 grid (`grid grid-cols-12 gap-6` on desktop, single column with `px-4` on phone); `MobileShell` pads with `env(safe-area-inset-*)` and, inside Telegram, `var(--tg-content-safe-area-inset-*)`; `<StatusPill />` (from `/api/status`, states live / paused by an admin / paused (breaker) / offline / no word yet).

- [ ] **Step 1:** Write `useShell.test.ts` (resize to 1400/900/375; Telegram at 1400 → `mobile`) and a `StatusPill` test for each state including `bot: null`.
- [ ] **Step 2:** Run — Expected: FAIL.
- [ ] **Step 3:** Implement shells with EasyUI components restyled to role tokens; ⌘K `command-menu` on desktop jumps to pages and to `#/users/<id>` for numeric input.
- [ ] **Step 4:** Run `npm test && npm run lint:tokens && npm run build` — Expected: PASS.
- [ ] **Step 5:** Commit `feat(web): desktop and phone shells, status pill`.

### Task 5: Shared feedback: toasts and confirmations

**Files:**
- Install: `velocity-toast`, `settle-modal`, `drag-to-confirm`, `morphing-button`, `skeleton`, `alert`, `not-found`
- Create: `admin-ui/src/components/Toast.tsx`, `admin-ui/src/components/ConfirmAction.tsx`, `admin-ui/src/components/ConfirmAction.test.tsx`, `admin-ui/src/components/States.tsx` (replaces old `States`)

**Interfaces:**
- Produces: `useToast(): (message: string, tone?: 'success' | 'error' | 'info') => void`; `<ConfirmAction<T> title summary confirmLabel path body? fields? danger? onDone(result: T) onClose>` — same contract as today's `ActionDialog` (reason textarea, 3–300 chars, POST via `lib/api`), rendered as `settle-modal` on desktop and `sheet` on phone; when `danger`, the confirm control is `drag-to-confirm` with a keyboard path (hold Enter 600 ms on the focused handle) and an `sr-only` button; calls `haptic('success'|'error')`.

- [ ] **Step 1:** Write `ConfirmAction.test.tsx`: confirm disabled until reason ≥ 3 chars; POST body includes `reason` and `body`; API error shown in `role="alert"`; danger variant "confirms from keyboard" (focus handle, hold Enter → POST sent).
- [ ] **Step 2:** Run — Expected: FAIL.
- [ ] **Step 3:** Implement.
- [ ] **Step 4:** Run `npm test` — Expected: PASS.
- [ ] **Step 5:** Commit `feat(web): toasts and responsive confirmations`.

### Task 6: Sign-in and not-found

**Files:** Install `login`. Create `admin-ui/src/features/auth/SignIn.tsx`, `admin-ui/src/features/auth/SignIn.test.tsx`; route unknown paths to `not-found`.

**Interfaces:** Consumes `launchData`, `signInWithTelegramApp` (lib/api). Produces `<SignIn botUsername error>` with the same error copy as today.

- [ ] **Step 1:** Test: renders the Telegram widget script with `data-auth-url` = `<origin>/api/auth/telegram`; shows the `not-admin` message for `error=not-admin`.
- [ ] **Step 2–4:** Run (FAIL), implement, run (PASS).
- [ ] **Step 5:** Commit `feat(web): sign-in and not-found`.

### Task 7: Overview

**Files:** Install `stat-card`, `animated-number`, `interactive-area-chart`, `bar-chart`, `pull-to-refresh`. Create `admin-ui/src/features/overview/{OverviewPage.tsx, Meter.tsx, useOverview.ts, OverviewPage.test.tsx}`.

**Interfaces:** `useOverview()` = `useQuery(['overview'], …, { refetchInterval: 20_000 })` on `/api/overview`.

- [ ] **Step 1:** Test: with sample overview → hero shows `24,180` and "of 35,000"; four stat cards; with `bot: null` → "No word from Ellie yet" and no meter.
- [ ] **Step 2–4:** FAIL, implement (meter: 40 segments, `brand` fill, warning past 70 %, danger past 90 %; area chart readings vs active; bar chart of sources), PASS.
- [ ] **Step 5:** Commit `feat(web): overview`.

### Task 8: Users and user detail

**Files:** Install `data-table`, `expandable-search`, `spring-select`, `slide-pagination`, `badge`, `split-button`. Create `admin-ui/src/features/users/{UsersPage.tsx, UserPage.tsx, useUsers.ts, UsersPage.test.tsx, UserPage.test.tsx}`.

**Interfaces:** Actions use `ConfirmAction` with the existing endpoints (`/api/users/{id}/bonus|reset-today|ban|unban`); after success `invalidateQueries(['user', id])` and toast.

- [ ] **Step 1:** Tests: list renders stacked cards when `useShell()` is `mobile`; search non-numeric → error message from API shown; user page with `banned` shows the ban banner and "Lift ban"; Ban hidden for admin IDs from `/api/me`.
- [ ] **Step 2–4:** FAIL, implement, PASS.
- [ ] **Step 5:** Commit `feat(web): users and user detail`.

### Task 9: Groups

**Files:** Install `expandable-data-row`, `stretch-switch`. Create `admin-ui/src/features/groups/{GroupsPage.tsx, GroupSettingsForm.tsx, GroupsPage.test.tsx}`.

- [ ] **Step 1:** Tests: toggling "Pause Ellie here" then saving POSTs `settings.paused: true`; Block uses the danger `ConfirmAction`; a 60-character title renders inside the card without the list gaining `overflow-x-auto`.
- [ ] **Step 2–4:** FAIL, implement, PASS.
- [ ] **Step 5:** Commit `feat(web): groups`.

### Task 10: Voices

**Files:** Install `activity-feed`. Create `admin-ui/src/features/voices/{VoicesPage.tsx, VoicesPage.test.tsx}`.

- [ ] **Step 1:** Tests: rows with `job_status: 'pending'` show "Deleting…" and no Delete; while any job is pending the query polls every 4 s (`refetchInterval` 4000); Delete is a danger `ConfirmAction` to `/api/voices/{kind}/{id}/delete`.
- [ ] **Step 2–4:** FAIL, implement, PASS.
- [ ] **Step 5:** Commit `feat(web): voices`.

### Task 11: Announcements

**Files:** Install `stepper`. Create `admin-ui/src/features/announcements/{AnnouncementsPage.tsx, Composer.tsx, AnnouncementsPage.test.tsx}`.

**Interfaces:** Steps: Write (textarea ≤ 3500 chars, audience) → Test (`POST /api/broadcasts/test`) → Send (danger `ConfirmAction` to `/api/broadcasts`); inside Telegram, `useMainButton` shows the current step's action.

- [ ] **Step 1:** Tests: Send step disabled until a test was sent for the current text; 3501 chars blocks progress; a `sending` broadcast shows progress and Cancel; 409 from the API shows its message.
- [ ] **Step 2–4:** FAIL, implement, PASS.
- [ ] **Step 5:** Commit `feat(web): announcements`.

### Task 12: Settings

**Files:** Install `animated-tabs`, `form`. Create `admin-ui/src/features/settings/{SettingsPage.tsx, LimitsForm.tsx, PauseCard.tsx, SettingsPage.test.tsx}`.

**Interfaces:** Field list and minute/second conversion as today's `Settings.tsx` (keys from `OverrideKey`); Save via `ConfirmAction` to `/api/settings`; pause via danger `ConfirmAction` to `/api/pause`, resume via a normal one; `useMainButton({ text: 'Save limits', … })` when there are unsaved changes inside Telegram.

- [ ] **Step 1:** Tests: placeholders show defaults from `/api/settings`; clearing a field sends it absent (back to default); `defaults: null` renders the "hasn't reported" notice; minutes field `10` sends `600`.
- [ ] **Step 2–4:** FAIL, implement, PASS.
- [ ] **Step 5:** Commit `feat(web): settings and emergency pause`.

### Task 13: Audit log (with action filter) and sessions

**Files:**
- Go: modify `internal/admin/server.go` (`auditLog` reads `action` query param) and the store's `AuditLog(ctx, before int64, limit int, action string)`; test in `internal/admin/admin_test.go` (`TestAuditFilterByAction`) and Postgres test.
- Install `recovery-ledger`. Create `admin-ui/src/features/audit/{AuditPage.tsx, describe.ts, describe.test.ts}`, `admin-ui/src/features/sessions/{SessionsPage.tsx, SessionsPage.test.tsx}`.

**Interfaces:** `describe(e: AuditEntry): string` keeps every sentence from today's `Audit.tsx`; before/after shown as a diff for entries whose `detail` has both.

- [ ] **Step 1:** Tests: Go `TestAuditFilterByAction` (`?action=user_banned` returns only those); `describe` for each action key; sessions "Sign out other devices" calls `/api/sessions/revoke-others`.
- [ ] **Step 2:** Run `go test ./internal/admin -run AuditFilter` and `npm test` — Expected: FAIL.
- [ ] **Step 3:** Implement.
- [ ] **Step 4:** Run `go test -race ./... && npm test` — Expected: PASS.
- [ ] **Step 5:** Commit `feat: audit log filter and redesigned audit and sessions pages`.

### Task 14: Remove the old UI and QA

**Files:** Delete `admin-ui/src/pages/`, old `components/` files and `styles.css` superseded by Tasks 2–13.

- [ ] **Step 1:** Run `npm run lint:tokens && npm test && npm run build`; check the gzipped size of `internal/admin/dist/assets/*.js` — Expected: all pass; JS ≤ 200 KB gzipped.
- [ ] **Step 2:** Serve the build with a mock API (`node` script in the session scratchpad, not committed) and run the ux-ui-audit probes (`probe-core`, `probe-heuristics`, `probe-perception`, `probe-ltr`, `probe-focus`) at 375 × 812 and 1440 × 900 in light and dark. Fix every Critical/Major/Accessibility finding; record the results in the pull request description.
- [ ] **Step 3:** Update README "Admin panel": the new stack (React 19, Tailwind v4, EasyUI), the design tokens file, the Telegram behaviour (theme, BackButton, MainButton, haptics) and the token checker.
- [ ] **Step 4:** Commit `chore(web): remove the old UI; document the redesign`.

### Task 15: Switch-over

- [ ] **Step 1:** Push `redesign/web-app`, open the pull request — Expected: CI `test` and `admin-ui` jobs `success`.
- [ ] **Step 2:** With the user's go-ahead, deploy the branch's head commit to the panel (`render deploys create srv-dc4tg8m1egvs739pj7p0 --commit <sha> --wait --confirm`); confirm the log line `admin panel listening … ui=true` and that `/api/me` without a session returns 401.
- [ ] **Step 3:** The user checks it inside Telegram on their phone and on desktop. If anything is wrong, redeploy the previous commit (`render deploys create … --commit <previous sha>`) and fix on the branch.
- [ ] **Step 4:** After the user approves, merge the pull request (the deployed commit is already live).
