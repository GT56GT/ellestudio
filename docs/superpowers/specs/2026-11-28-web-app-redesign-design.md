# Ellie Studio web app redesign

Date: 2026-11-28 · Status: draft for approval

The web app is the admin panel at https://ellie-admin.onrender.com, which also
opens as a Telegram Mini App from the bot's `/admin` button. This document
describes rebuilding it on a real design system and the EasyUI component
library, and restructuring the code underneath.

## 1. Intent

The panel works but looks and feels sloppy. Measured causes (admin-ui after Phase 3):

- about 30 different pixel values for spacing and sizes, no spacing scale;
- 9 font sizes with no type scale;
- 53 inline `style={{…}}` props, so layout lives in pages, not a system;
- inside Telegram it ignores the user's theme, Telegram's back button and
  safe areas; the side menu becomes a sideways-scrolling strip and 7–8
  column tables scroll sideways on a phone;
- 30px "small" buttons, below the 44px touch target phones expect;
- a violet accent that reads as the generic "AI purple".

Why now: the panel is used on both phone (inside Telegram) and desktop, and
Ellie is a portfolio project, so it should look deliberate and be built the
way a production team would build it.

## 2. Goals, non-goals, anti-goals

Goals

1. One design system: approved palette, type scale, spacing grid, radii,
   elevation and motion, as tokens. No raw colors or arbitrary sizes in
   feature code.
2. Two first-class layouts from one codebase: desktop (browser) and phone
   (browser or inside Telegram).
3. Inside Telegram it behaves like a native Mini App: Telegram theme
   colors, BackButton, MainButton for the page's primary action, haptics,
   safe areas, no sideways scrolling.
4. Every existing feature of Phases 1–3 keeps working: sign-in, overview,
   users, user actions, groups, voices, announcements, settings, pause,
   audit log, sessions.
5. A maintainable structure: features in their own folders; on the Go side
   the database code moves out of the bot package.
6. WCAG 2.2 AA throughout (contrast already audited for the palette).

Non-goals

- The bot's chat screens in Telegram (separate project, later).
- A user-facing Mini App for Ellie's users (later; this system should make
  it easy).
- New admin features. The only API additions are the small ones listed in
  §7.3 that the new screens need.

Anti-goals (would make this a failure even if it works)

- Animation everywhere. EasyUI animates nearly everything; here motion is
  used only where it shows what changed.
- A half-old, half-new panel in production at any point.
- Hover-only interactions on a touch screen.
- Looking like a template: no purple gradients, no decorative effects.

## 3. Constraints

- Served by the Go binary via `go:embed` (`-tags embedui`), built in
  `Dockerfile.admin` (Node 22 stage). Plain `go build`/`go test` must keep
  working without Node.
- Content-Security-Policy stays strict: scripts only from self and
  telegram.org; styles self plus inline (needed by Framer Motion);
  `connect-src 'self'`. Therefore no CDN fonts or CDN scripts other than
  telegram.org.
- EasyUI's stack: React 19, Tailwind CSS v4, TypeScript, Framer Motion
  (126/140 components), lucide-react (67/140), installed as source through
  `npx shadcn@latest add Surajmaurya1/easyui/<name>`. MIT licence.
- Telegram theme parameters arrive at launch and on change; on iOS dark
  theme since Telegram 10.2, `bg_color` and `secondary_bg_color` can arrive
  swapped.
- Budget: initial JS ≤ 200 KB gzipped (today 54 KB); first contentful paint
  on a mid-range phone ≤ 1.5 s from the Singapore region.
- Browsers: Tailwind v4 needs Safari 16.4+ / Chrome 111+ (Telegram on
  iOS 16.4+ and current Android WebView). Acceptable: only admins use the
  panel.

## 4. Approaches considered

- **A. Rebuild on EasyUI's stack on a branch, switch over in one deploy.**
  Chosen by the user. One consistent system at the end; the live panel
  never shows a mixed state. Cost: the largest single change.
- B. Migrate page by page on the live app. Rejected: weeks of a mixed look
  and two styling systems fighting.
- C. Keep hand-written CSS and re-implement EasyUI's ideas. Rejected: we
  would rewrite ~100 components ourselves and lose upstream updates.

## 5. Design system

### 5.1 Palette (approved)

OKLCH ramps, Tailwind stops 50–950, generated with the ui-color-palette
tooling from base `#EFA21B` (Saffron studio). Hex values:

| ramp | 50 | 100 | 200 | 300 | 400 | 500 | 600 | 700 | 800 | 900 | 950 |
|---|---|---|---|---|---|---|---|---|---|---|---|
| saffron | ffeccc | f7d0a3 | e4b479 | d1984c | bd7d03 | a96200 | 8d4c00 | 703700 | 552400 | 3a1300 | 200400 |
| slate | eef2f8 | d2d8e0 | b7beca | 9ca5b4 | 828d9e | 687588 | 535e70 | 3e4858 | 2a3341 | 18202a | 080e15 |
| info | d2f6ff | addcff | 88c1ff | 65a5ff | 4587f9 | 2b68ee | 134ed1 | 0034b3 | 001a92 | 000070 | 02004b |
| success | dbfaec | b6e4d0 | 91cdb3 | 69b798 | 3da17e | 008b64 | 00724e | 005b39 | 004325 | 002d13 | 001705 |
| warning | f1f6ce | d7dda5 | bdc579 | a5ad49 | 8d9500 | 767d00 | 606500 | 4b4d00 | 363700 | 222200 | 100f00 |
| danger | ffe2e1 | ffc1bf | ff9f9c | f27c78 | e25855 | d02b2f | b30015 | 950000 | 760000 | 560000 | 350000 |
| violet | fae8ff | e3c9ff | cdaaff | b78af6 | a369ea | 8e45dc | 7729c0 | 6000a3 | 490084 | 330063 | 1d0041 |
| magenta | ffe1f7 | ffbfdf | f99dc7 | ea79b0 | d95499 | c72683 | aa006b | 8d0054 | 6e003e | 500029 | 300014 |
| cyan | d8f9ff | b2e1ec | 8bc9d8 | 61b2c5 | 2d9cb1 | 00859d | 006d85 | 00556c | 003e53 | 00283a | 001322 |

Brand fill: `#EFA21B` (the source colour itself), always with dark label
`slate-950` (9.1:1).

### 5.2 Role tokens

Two layers. Primitives above; feature code uses only these roles (Tailwind
v4 `@theme` names in brackets):

| role | light | dark (browser) | inside Telegram |
|---|---|---|---|
| page [`bg-page`] | slate-50 | slate-950 | `--tg-theme-secondary-bg-color` |
| surface [`bg-surface`] | white | slate-900 | `--tg-theme-section-bg-color` (fallback `bg_color`) |
| text [`text-fg`] | slate-900 | slate-50 | `--tg-theme-text-color` |
| muted [`text-muted`] | slate-600 | slate-200 | `--tg-theme-hint-color` |
| border [`border-line`] (decorative) | slate-100 | slate-800 | text colour at 12% |
| input border [`border-input`] | slate-400 | slate-500 | hint colour |
| primary action [`bg-action`, `text-on-action`] | `#EFA21B` / slate-950 | `#EFA21B` / slate-950 | `--tg-theme-button-color` / `button-text-color` |
| brand (meaning-bearing: meter, active tab, chart line, focus) [`brand`] | saffron-500 | `#EFA21B` | saffron-500 on light schemes, `#EFA21B` on dark |
| accent text / links [`text-accent`] | saffron-600 | saffron-200 | `--tg-theme-accent-text-color` |
| success / warning / danger / info text | each 600 | each 200 | same as browser for the scheme |
| status tint | each 50 + 700 text | each 950 + 200 text | same as browser |
| destructive text | danger-600 | danger-200 | `--tg-theme-destructive-text-color` |
| chart series 1–6 | saffron-500, info-500, success-500, violet-500, magenta-500, cyan-500 | `#EFA21B`, then the same 500s | per scheme |

Contrast audit (done): every text pair ≥ 4.5:1 (body 16.4:1, APCA Lc 103);
input borders ≥ 3.36:1; charts ≥ 3.15:1 on both themes. Warning (citron) is
never colour-only: always icon + word.

Theme selection: browser follows `prefers-color-scheme` with a manual
override (`data-theme`), as today. Inside Telegram, `data-tg` is set and
Telegram's `colorScheme` picks light/dark ramps for the roles Telegram does
not supply. iOS swap guard: if `colorScheme` is dark and `bg_color` is
lighter than `secondary_bg_color` (or the reverse for light), swap them.

### 5.3 Type

System font stack (`-apple-system, "Segoe UI", Roboto, …`): native inside
Telegram, zero bytes, no CSP change. Tabular numerals for all figures.

| token | size / line | use |
|---|---|---|
| `text-xs` | 12 / 16 | meta, axis labels |
| `text-sm` | 13 / 18 | table cells, captions |
| `text-base` | 15 / 22 | body |
| `text-lg` | 18 / 24 | section titles |
| `text-xl` | 22 / 28 | page titles |
| `text-2xl` | 28 / 32 | stat figures |
| `text-hero` | 44 / 44 (phone 34) | the one hero figure per page |

Seven sizes, nothing else. Weights: 400 body, 550 labels, 650 titles and
figures.

### 5.4 Spacing, radius, elevation

- 4 px grid; only Tailwind's spacing scale (1 = 4 px). Allowed steps:
  1, 2, 3, 4, 5, 6, 8, 10, 12, 16. No arbitrary values (`p-[13px]`) in
  feature code; a CI grep fails the build on `-\[` and on `style={{` outside
  `components/ui/` and chart code.
- Radius: `rounded-sm` 6 px (inputs, buttons), `rounded-lg` 12 px (cards),
  `rounded-2xl` 20 px (sheets, dialogs), `rounded-full` (pills).
- Elevation: flat surfaces with a 1 px border; one shadow token for
  overlays (sheets, menus, dialogs). Dark mode uses borders, no shadows.
- Touch targets: ≥ 44 × 44 px on phone layouts (WCAG 2.5.8 minimum is 24;
  44 is the platform norm).

### 5.5 Motion

- Durations: 120 ms (press), 200 ms (state change), 320 ms (sheets).
  EasyUI's spring presets for sheets and pills only.
- Motion answers an action (open, close, save, select). No idle or
  decorative motion; one entrance moment allowed: the level meter filling on
  Overview.
- `prefers-reduced-motion`: springs become 0 ms fades (EasyUI supports it).

## 6. Layout

### 6.1 Shells

| width / context | shell |
|---|---|
| ≥ 1024 px, browser | `sidebar` (EasyUI) 240 px + content max 1200 px, 12-column grid, 24 px gaps; ⌘K `command-menu` |
| 768–1023 px | sidebar collapsed to 72 px icons |
| < 768 px, or any width inside Telegram | single column, 16 px gutters, safe-area insets; bottom `small-floating-dock` with Overview, Users, Groups, Voices, More (More opens a `sheet` with Announcements, Settings, Audit log, Sessions) |

Header on every page: title + one-line lede on the left; status pill
(`dynamic-island`, replacing today's tally) on the right. Inside Telegram
the title moves into Telegram's header area: our header shows only the
status pill, and Telegram's BackButton replaces the "← All users" link on
detail pages.

### 6.2 Grid rules

- Desktop content uses the 12-column grid: Overview figures 4 × 3 columns;
  charts 12 or 8 + 4; detail pages 8 + 4 (facts + actions).
- Phone: everything stacks; tables become stacked cards (EasyUI
  `data-table` responsive mode); dialogs become bottom `sheet`s.

## 7. Screens

### 7.1 Component choices per screen

| screen | components |
|---|---|
| Sign-in | `login` card layout re-skinned for Telegram sign-in (widget in browser; automatic inside Telegram) |
| Overview | hero: our level meter (kept, restyled) + `animated-number`; `stat-card` × 4; `interactive-area-chart` (30 days, readings vs active, touch scrubbing; replaces the waveform); sources of new people as a `bar-chart` (from groups, from invites, direct; 7 days); daily limits as a definition list; `skeleton` while loading; `pull-to-refresh` in Telegram |
| Users | `data-table` (stacked cards on phone), `expandable-search` (ID), `spring-select` (filter, sort), `slide-pagination`, `badge` (Banned, Groups only) |
| User | `breadcrumb` (desktop) / BackButton (Telegram); facts grid; `interactive-area-chart` for activity; `split-button` (Give bonus ▾ Reset today · Ban); dialogs: `settle-modal` on desktop, `sheet` on phone; `drag-to-confirm` for Ban |
| Groups | `data-table`; `expandable-data-row` holds the group's settings with `stretch-switch` toggles and numeric fields; `drag-to-confirm` for Block |
| Voices | `data-table`; `badge` for Deleting / Failed; `drag-to-confirm` for Delete; recent deletions as `activity-feed` |
| Announcements | `stepper` (Write → Test → Send); composer from `form` textarea with live preview bubble; `morphing-button` for "Send a test to me"; `drag-to-confirm` for Send; progress list with cancel |
| Settings | emergency pause card on top with `drag-to-confirm` (pause) / `button` (resume); limits in `animated-tabs` (Each person · Everyone · Groups · Invites) using `form` inputs; `morphing-button` Save → Saving → Saved; Telegram MainButton = Save inside Telegram |
| Audit log | `recovery-ledger` styled as a read-only log with before/after diff (no rollback button); filter by action with `spring-select` |
| Sessions | list + `button`; "Sign out other devices" |
| Everywhere | `velocity-toast` replaces the Flash notice; `alert` for errors; `not-found` for unknown routes; `directional-tooltip` only on desktop, never hover-only |

Components not listed are out of scope for this project (see the 140-item
review in the conversation: 38 skipped as decorative, 15 kept for later).

### 7.2 Telegram behaviour

- Load `https://telegram.org/js/telegram-web-app.js` only when launched
  inside Telegram (launch data present in the URL hash). Call `ready()` and
  `expand()`; listen to `themeChanged` and `viewportChanged`.
- BackButton: shown on detail pages and open sheets; goes back.
- MainButton: the page's one primary action (Save on Settings, Send on
  Announcements step 3, nothing elsewhere).
- Haptics: `notificationOccurred('success'|'error')` after an action;
  `impactOccurred('light')` on drag-to-confirm completion.
- Respect `safeAreaInset` and `contentSafeAreaInset` CSS variables.
- Sign-in as today (initData → bearer token in memory).

### 7.3 API additions (small)

- `GET /api/audit?action=` filter (Audit log filter).
- Nothing else; every other screen uses today's endpoints.

## 8. Code structure

### 8.1 Front end (`admin-ui/`)

```
src/
  app/            App, routes, providers (query client, theme, telegram), shells
  components/ui/  EasyUI components exactly as installed (edited only to use role tokens)
  components/     our shared pieces: Meter, StatusPill, Page, Section, EmptyState
  features/
    overview/ users/ groups/ voices/ announcements/ settings/ audit/ sessions/ auth/
      (each: page component(s), hooks, API calls, types)
  lib/            api client, format, telegram bridge, theme
  styles/         tokens.css (generated primitives + roles), globals.css
```

- Routing: keep the small hash router (deep links already work; no new
  dependency).
- Data: TanStack Query replaces `useLoad` (caching, polling while jobs run,
  invalidation after actions).
- Tests: Vitest + Testing Library for `lib/` (theme mapping incl. the iOS
  swap guard, format) and for each feature's main actions; CI job runs them
  with the build.
- Lint: the CI grep in §5.4 plus `tsc --noEmit`.

### 8.2 Back end (Go)

Move Postgres code out of `internal/bot` into a new `internal/store`
package: migrations, settings store backend, owners, chats, metrics,
budget, status, admin queue, broadcasts, limits persistence. `internal/bot`
and `internal/admin` both depend on `internal/store`; `internal/admin` no
longer imports `internal/bot` except for shared types that move to
`internal/store` too (Settings, Ban, LimitOverrides, Status, DayStats). No
behaviour change; all existing tests move with their code and must pass
unchanged in substance.

## 9. Delivery

- Branch `redesign/web-app`; the live panel is untouched until the
  switch-over deploy.
- The Go extraction (§8.2) ships first as its own pull request
  (`refactor/store`), merged and deployed on its own; the redesign branch
  builds on it.
- Order: (1) foundation — React 19, Tailwind v4, shadcn config, tokens,
  Telegram bridge, both shells, toast, sheet/dialog; (2) screens in the order
  Overview, Users + User, Groups, Voices, Announcements, Settings, Audit,
  Sessions, Sign-in; (3) QA.
- QA before switch-over: ux-ui-audit probes at 375 × 812 and 1440 × 900 in
  both themes; keyboard pass; manual run inside Telegram on iOS, Android and
  Desktop (by the user); all Go and front-end tests green in CI.
- Switch-over: merge, deploy bot then panel, as today.

## 10. Decided by the user

- Scope is the web app (admin panel + its Mini App mode).
- Used on both phone (inside Telegram) and desktop; both are first class.
- Use EasyUI; approach A (rebuild on its stack on a branch).
- Palette: Saffron studio, as in §5.1, approved.

## 11. Calls made in this document (check these)

- System font stack instead of a brand typeface (native in Telegram, no
  bytes, CSP unchanged).
- Inside Telegram, primary buttons use Telegram's button colour; saffron is
  the identity layer.
- Interactive area chart replaces the mirrored waveform; the level meter
  stays the one memorable element.
- TanStack Query added; router stays hand-written.
- Audit log uses `recovery-ledger` read-only (no rollback).
- `drag-to-confirm` for ban, block, delete voice, send announcement and
  pause; normal buttons for reversible actions.
- Bar chart of sources instead of a funnel: the stats don't record a true
  sequence of stages, so a funnel would mislead.
- The Go store extraction ships first, as its own pull request.
- Telegram Desktop always gets the phone shell, whatever its window width.

## 12. Left to the builder

- Exact EasyUI prop values and any small edits to installed components
  (they must use role tokens, not primitives).
- Copy for empty and error states, following the existing voice (plain,
  sentence case, no apologies).
- Icon choices from lucide-react.
