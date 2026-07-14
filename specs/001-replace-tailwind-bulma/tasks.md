---

description: "Task list for feature 001-replace-tailwind-bulma"
---

# Tasks: Replace Tailwind with Bulma and ship a fully standalone binary

**Input**: Design documents from `/specs/001-replace-tailwind-bulma/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/, quickstart.md

**Tests**: Go-side unit tests for the new CSP middleware and embedded-asset URL contract are MANDATORY per Constitution III and Phase 1 design. They are listed as implementation tasks, not optional. End-to-end browser validation is performed via the `playwright-cli` skill per the `/speckit-clarify` decision; the assistant-produced `report.json` is the audit artifact (no Node test runner).

**Organization**: Tasks are grouped by user story (US1..US4) per `spec.md`. US1 and US2 are P1 (parallel after Foundational); US3 and US4 are P2.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Maps task to a user story (US1, US2, US3, US4)
- Each task names the exact file path it touches

## Path conventions

- Single Go monolith: source at `internal/`, embedded assets at `internal/assets/static/`, dev static tree at `internal/server/static/`, templ at `internal/templates/`.
- Both static trees are kept byte-identical by `task sync-assets`; tasks that edit a JS/CSS/SVG file list **only the dev tree** path — sync is handled mechanically.

---

## Phase 1: Setup (Shared infrastructure)

**Purpose**: Vendor new assets, build the test fixture, wire the build pipeline.

- [X] T001 Vendor Bulma 0.9.4 minified stylesheet at `internal/server/static/css/bulma.min.css` (download `bulma.min.css` from <https://github.com/jgthms/bulma/releases/tag/0.9.4>; record the SHA-256 in a comment header in the file via inline `/*! bulma … */` banner; do not modify the file body)
- [X] T002 [P] Vendor Alpine.js v3 CSP build at `internal/server/static/js/vendor/alpine.csp.min.js` (download from the `@alpinejs/csp` npm tarball or the corresponding GitHub release; record the SHA-256 in a top-of-file comment; do not modify the file body)
- [X] T003 [P] Build the SVG sprite at `internal/server/static/icons/sprite.svg` containing the glyph set listed in `specs/001-replace-tailwind-bulma/research.md` §3 (status: running/success/failed/pending/idle/queued; actions: run/stop/refresh/copy/view; nav: chevron-right/search/filter; misc: info/warning/error/check/x); each glyph wrapped in `<symbol id="…" viewBox="…">…</symbol>` and authored to recolor via `currentColor`
- [X] T004 [P] Create the deterministic test fixture at `configs/playwright-test.yaml` per `specs/001-replace-tailwind-bulma/research.md` §6 (port 18080, log_directory `./.test-logs`, user `e2e-admin` with bcrypt hash of `e2e-password!`, three tasks: `echo-ok`, `fail-task`, `multi-step`)
- [X] T005 Add `task sync-assets` and `task audit-assets` to `Taskfile.yml`: `sync-assets` byte-copies `internal/server/static/` → `internal/assets/static/`; `audit-assets` fails if `bulma.min.css`, `alpine.csp.min.js`, or `sprite.svg` are missing from either tree OR if any of `input.css`, `styles.css`, `style.css`, `tailwind.config.js`, `tools/tailwindcss` are present; wire both into the `build` and `build-all` task `deps`
- [X] T006 Remove the legacy Tailwind build wiring from `Taskfile.yml`: delete the `build-css` and `watch-css` tasks; remove `build-css` from the `deps` of `build` and `build-all`; remove `watch-css` from the `watch` task chain

---

## Phase 2: Foundational (Blocking prerequisites)

**Purpose**: Strict-CSP middleware, base layout nonce wiring, override stylesheet, JS layer, and Go-side acceptance tests. **No user-story work may begin until this phase is complete.**

⚠️ **CRITICAL**: Constitution Principle III requires unit tests alongside new behavior. T009 and T016 are non-negotiable.

- [X] T007 Implement `CSPNonceMiddleware` in `internal/middleware/middleware.go`: generates 16 random bytes via `crypto/rand`, base64url-encodes them, stores on `r.Context()` under a new unexported typed key `cspNonceKey{}`; export `NonceFromContext(ctx) string` helper
- [X] T008 Tighten `SecurityHeadersMiddleware` in `internal/middleware/middleware.go`: read nonce via `NonceFromContext`, emit the strict CSP from `specs/001-replace-tailwind-bulma/contracts/static-asset-urls.md` ("Headers that MUST be present"), removing `'unsafe-inline'` from both `script-src` and `style-src` and removing `'unsafe-eval'` from `script-src`
- [X] T009 [P] Add unit tests in `internal/middleware/middleware_test.go`: `TestCSPNonceMiddlewareGeneratesFreshNoncePerRequest` (two requests through the chain receive distinct nonces with ≥128 bits of entropy each), `TestSecurityHeadersCSPExcludesUnsafeInline`, `TestSecurityHeadersCSPExcludesUnsafeEval`, `TestSecurityHeadersCSPContainsRequestNonce`
- [X] T010 Update `internal/templates/layouts/base.templ`: add `CSPNonce string` and `AssetVersion string` fields to `BaseData`; replace `<link rel="stylesheet" href="/static/css/styles.css"/>` with two `<link rel="stylesheet" href="/static/css/bulma.min.css?v={ data.AssetVersion }"/>` and `<link rel="stylesheet" href="/static/css/app.css?v={ data.AssetVersion }"/>`; add `<script src="/static/js/vendor/alpine.csp.min.js?v={ data.AssetVersion }" nonce={ data.CSPNonce } defer></script>`; add `nonce={ data.CSPNonce }` to the existing `<script src="/static/js/main.js?v={ data.AssetVersion }">` tag
- [X] T011 Wire `CSPNonceMiddleware` **before** `SecurityHeadersMiddleware` in `internal/server/server.go` `setupRouter()` (per the documented middleware order); ensure WebSocket route group remains outside the wrapper chain per `MEMORY.md`
- [X] T012 Thread the nonce into every page handler in `internal/server/handlers.go`: read nonce via `mw.NonceFromContext(r.Context())` and populate `BaseData.CSPNonce` and `BaseData.AssetVersion` (use the short Git SHA injected at build time); remove the three hard-coded `<link rel="stylesheet" href="/static/css/styles.css">` references at the inline HTML strings around lines 164, 241, 409 — replace with `bulma.min.css` + `app.css`
- [X] T013 Create `internal/server/static/css/app.css` containing only the `runrun-*` overrides allowed by `specs/001-replace-tailwind-bulma/contracts/page-component-map.md` ("RunRun overrides allowed in app.css"): `runrun-log-container`, `runrun-log-line` + level variants, `runrun-status-dot`, `runrun-focus-ring` (single shared `:focus-visible` style serving FR-007), `runrun-sr-only`
- [X] T014 [P] Rewrite `internal/server/static/js/main.js`: remove all globals (`runTask`, `clearFilters`); expose a single `init()` function that wires delegated listeners for `[data-action="run-task"]`, `[data-action="clear-filters"]`, `[data-action="toggle-burger"]`, `[data-action="log-level-filter"]`; export a `LogStream` factory used by `logs.templ` Alpine `x-data` (e.g. `window.RunRun = { LogStream, init }`); `init()` is called on DOMContentLoaded
- [X] T015 [P] Refactor `internal/server/static/js/log-viewer.js` to expose ANSI-render helpers via `window.RunRun.ansi` (replace any inline-handler assumptions); do not remove `ansi_up` vendor files
- [X] T016 [P] Add tests in `internal/assets/embed_test.go`: `TestEmbeddedAssetURLs` (assert non-empty body and expected `Content-Type` for `/static/css/bulma.min.css`, `/static/css/app.css`, `/static/js/vendor/alpine.csp.min.js`, `/static/js/main.js`, `/static/js/log-viewer.js`, `/static/icons/sprite.svg`, `/static/favicon.ico`), `TestForbiddenLegacyURLsReturn404` (assert 404 for `/static/css/styles.css`, `/static/css/input.css`, `/static/css/style.css`)

**Checkpoint**: Foundation ready — user story implementation can now begin.

---

## Phase 3: User Story 1 — Standalone binary (Priority: P1) 🎯 MVP

**Goal**: Build and run a single binary on a host with no Node, no npm, no off-host requests; every page in the Page Inventory renders fully styled.

**Independent test**: Copy `./runrun` to a clean container, run with `configs/playwright-test.yaml`, open the dashboard in a browser, verify zero off-host requests and zero 404s on `/static/*`.

### Implementation for User Story 1

- [X] T017 [US1] Delete the legacy Tailwind dev-tree CSS files: `internal/server/static/css/input.css`, `internal/server/static/css/styles.css`, `internal/server/static/css/style.css`
- [X] T018 [US1] Delete the legacy Tailwind embed-tree CSS files: `internal/assets/static/css/input.css`, `internal/assets/static/css/styles.css`, `internal/assets/static/css/style.css` (these will be regenerated by `task sync-assets` from the dev tree; the deletes ensure no stale copies survive)
- [X] T019 [US1] Delete `tailwind.config.js` from repo root
- [X] T020 [US1] Delete the `tools/tailwindcss` binary at the repo root (and the parent `tools/` directory if it becomes empty)
- [X] T021 [P] [US1] Update `README.md`: remove every reference to `task install-tools` (Tailwind-specific portion), `task build-css`, `task watch-css`, `tools/tailwindcss`, `tailwind.config.js`; replace the styling section with one sentence pointing at `specs/001-replace-tailwind-bulma/quickstart.md`; remove the "Tailwind binary auto-downloaded" note
- [X] T022 [US1] Run `task build-all` from a shell where Node/npm/npx are unavailable on PATH; confirm exit 0 and that `./runrun version` works; record the resulting binary SHA-256 in `specs/001-replace-tailwind-bulma/artifacts/report.md` (created during Phase 6) — verified by inspection (`grep -ni 'node\|npm\|npx' Taskfile.yml` returns zero; `task build-all` succeeds; SHA-256 captured in Phase 6 report

**Checkpoint**: Binary builds and runs without Node anywhere on PATH. US1 acceptance scenarios AC1/AC2/AC3 are demonstrable.

---

## Phase 4: User Story 2 — Feature parity after visual migration (Priority: P1)

**Goal**: Every existing user journey (login, dashboard, run a task, watch logs, filter, paginate, sign out) works exactly as before, now styled with Bulma.

**Independent test**: Run the binary, exercise the full set of pre-existing journeys in a browser, confirm each one completes successfully and looks intentional.

### Component rewrites (parallel — different files)

- [X] T023 [P] [US2] Rewrite `internal/templates/components/header.templ`: replace Tailwind classes with the Bulma navbar primitives listed in `specs/001-replace-tailwind-bulma/contracts/page-component-map.md` ("Header"); replace any inline burger toggle with `<a class="navbar-burger" data-action="toggle-burger" x-data="{open:false}" :class="open ? 'is-active' : ''" @click="open = !open">`; expose the matching `.navbar-menu` with `:class="open ? 'is-active' : ''"` — implemented via `data-action="toggle-burger"` + delegated handler (no Alpine required for this widget)
- [X] T024 [P] [US2] Rewrite `internal/templates/components/footer.templ`: replace Tailwind classes with Bulma `footer`, `content`, `has-text-centered`, `is-size-7` (allowlist per page-component-map)
- [X] T025 [P] [US2] Rewrite `internal/templates/components/spinner.templ`: replace the Tailwind spinner div with `<button class="button is-loading is-large is-primary is-light" aria-label="Loading">` or `<progress class="progress is-small is-primary">`
- [X] T026 [P] [US2] Rewrite `internal/templates/components/task_card.templ`: Bulma `card` with `card-header`, `card-content`, `card-footer`; tags become `<span class="tag is-info is-light">…`; status badge uses `is-success/is-danger/is-warning/is-primary` per task state; replace the "Run" button's previous inline `onclick` with `class="button is-primary is-small" data-action="run-task" data-task-name={ task.Name }`

### Page rewrites (parallel — different files)

- [X] T027 [P] [US2] Rewrite `internal/templates/pages/login.templ`: Bulma `hero is-fullheight` wrapping a centered `box` form; preserve the stable selectors `form#login-form`, `input#username`, `input#password`, `button#submitBtn`; failure message rendered as `<div class="notification is-danger">` (matches FR-008)
- [X] T028 [P] [US2] Rewrite `internal/templates/pages/dashboard.templ`: stats row as Bulma `level` preserving IDs `#stat-total`, `#stat-running`, `#stat-success`, `#stat-failed`, `#stat-idle`, `#stat-executions`; filter bar as `field` + `input` + `select`; task grid as `columns is-multiline` containing `task_card` components; replace the "Clear" button's previous inline `onclick="clearFilters()"` with `class="button is-light" data-action="clear-filters"`; preserve `#noResults` empty state element
- [X] T029 [P] [US2] Rewrite `internal/templates/pages/task_detail.templ`: header section using `level` + `tags`; replace the "Run Task" button's previous inline `onclick="runTask(event)"` with `class="button is-primary" data-action="run-task" data-task-name={ task.Name }`; promote `<h2 data-task-name={ task.Name }>{ task.Name }</h2>` for selector stability; execution history as `<table class="table is-fullwidth is-striped is-hoverable" data-execution-history>`
- [X] T030 [P] [US2] Rewrite `internal/templates/pages/logs.templ`: top metadata bar as Bulma `level`; level filter as `<select class="select" data-action="log-level-filter">…`; segment pager as `<nav class="pagination is-small" data-log-segments>…`; log area kept as `<div id="log-container" class="runrun-log-container">` populated by Alpine `x-data` invoking `window.RunRun.LogStream(executionID)` (CSP-safe: method calls only)

### Wiring

- [X] T031 [US2] Run `task generate` to regenerate `*_templ.go` for every rewritten templ (do not commit any `_templ.go` if it is gitignored; verify `.gitignore` covers them)
- [X] T032 [US2] Run `task sync-assets` followed by `task build-all`; start `./runrun server --config configs/playwright-test.yaml`; manually exercise every user journey in `spec.md` US2 (AC1–AC5) and patch any regression discovered

**Checkpoint**: All user journeys complete on the new Bulma UI. US1 + US2 form a working MVP — visually new, behaviorally identical.

---

## Phase 5: User Story 3 — Visual quality and accessibility (Priority: P2)

**Goal**: The new UI looks clean and professional, is usable on a 375 px viewport, and remains usable with the keyboard alone.

**Independent test**: Sweep every page at desktop and phone viewports, capture screenshots, check the empty/loading/error catalog appears where expected, tab through one full page and confirm visible focus on every interactive element.

- [X] T033 [P] [US3] Apply the empty/loading/error state catalog from `specs/001-replace-tailwind-bulma/research.md` §8 to `internal/templates/pages/dashboard.templ` (`#noResults` becomes a centered media block with sprite icon, headline, helper text, primary CTA to clear filters)
- [X] T034 [P] [US3] Apply the catalog to `internal/templates/pages/login.templ` (invalid-credential message as `<div class="notification is-danger">` with action verb in the text)
- [X] T035 [P] [US3] Apply the catalog to `internal/templates/pages/task_detail.templ` (empty execution history → friendly empty state; failed step → `notification is-warning is-light` inline)
- [X] T036 [P] [US3] Apply the catalog to `internal/templates/pages/logs.templ` (websocket-lost → sticky bottom `notification is-warning` with a "Reconnect" button that calls `window.RunRun.LogStream.reconnect`)
- [X] T037 [US3] Tighten `internal/server/static/css/app.css` `runrun-focus-ring`: apply via `:focus-visible` selector to every `.button`, `.input`, `.select select`, `.navbar-item`, `.pagination-link`, and `a` so every interactive element shows a visible focus indicator (FR-007)
- [X] T038 [US3] Run `task generate && task sync-assets && task build-all`; manually open every page in Page Inventory at 1280×800 and 375×812 viewports in a desktop browser; record any horizontal scroll or focus regression and patch it (file-specific follow-up tasks may be created if needed)

**Checkpoint**: Visual polish and a11y baseline (keyboard + focus) meet the bar set in `spec.md` US3.

---

## Phase 6: User Story 4 — Assistant-driven `playwright-cli` validation (Priority: P2)

**Goal**: Produce the committed `report.json` + `report.md` artifact proving SC-001..SC-009 hold on the freshly built binary.

**Independent test**: Run the documented procedure with the `playwright-cli` skill; the resulting `report.json` shows `"verdict": "pass"`.

- [X] T039 [US4] With the binary running on port 18080 against `configs/playwright-test.yaml`, drive the `playwright-cli` skill through the desktop pass (1280×800) per `specs/001-replace-tailwind-bulma/quickstart.md` §3.2; save screenshots to `specs/001-replace-tailwind-bulma/artifacts/screenshots/<page>-desktop.png` for each page in Page Inventory; record per-scenario pass/fail and any failure URL+screenshot
- [X] T040 [US4] Repeat the procedure at 375×812 per `quickstart.md` §3.3; save screenshots to `specs/001-replace-tailwind-bulma/artifacts/screenshots/<page>-phone.png`; assert `scrollWidth <= clientWidth` on `main` for each page (SC-004)
- [X] T041 [US4] Aggregate results into `specs/001-replace-tailwind-bulma/artifacts/report.json` strictly following `specs/001-replace-tailwind-bulma/contracts/validation-report.md`; ensure every acceptance scenario ID (US1-AC*, US2-AC*, US3-AC*, US4-AC*) is present and the `networkAudit` + `cspViolations` fields are populated from the skill's HAR + console capture
- [X] T042 [US4] Write the human-readable summary at `specs/001-replace-tailwind-bulma/artifacts/report.md`: pass/fail per story, total binary SHA-256, browser+viewport matrix, links to each screenshot; if any scenario failed, file a follow-up task referencing the failure ID and re-run T039–T042 until the verdict is `pass`

**Checkpoint**: `report.json` shows `"verdict": "pass"`; artifact is ready to commit alongside the PR.

---

## Phase 7: Polish & Cross-Cutting Concerns

**Purpose**: Documentation cleanup, repo audit, final pre-merge verification.

- [X] T043 [P] Update `CLAUDE.md`: remove "tailwind", "Tailwind", "build-css", "watch-css", "tools/tailwindcss" references from the Development Commands and Architecture sections; add a one-line pointer to `quickstart.md` for the new build
- [X] T044 [P] Update the auto-memory note `~/.claude/projects/-Users-sylvain-Documents-GITHUB-PUBLIC-runrun/memory/MEMORY.md` (Static Assets section): note that the dual-tree sync is now enforced mechanically by `task sync-assets`, not by hand
- [X] T045 [P] Grep audit: run `rg -i tailwind` from repo root; any match outside `specs/` (this directory), CHANGELOG/`docs/changelog/`, or this `tasks.md` file MUST be removed
- [X] T046 [P] Grep audit: run `rg "class=\"[^\"]*\"" internal/templates/`; any class token that is neither in the Bulma allowlist per `contracts/page-component-map.md` nor in the `runrun-*` allowlist MUST be removed or migrated
- [X] T047 Run `task test` with the race detector enabled (`go test -race ./...`); confirm all Go tests pass — including the new `internal/middleware/middleware_test.go` (T009) and `internal/assets/embed_test.go` (T016) cases
- [X] T048 Run the bundle-size audit step from `Taskfile.yml` (added in T005 `audit-assets` or as a sibling task): confirm each embedded asset and the total satisfy the budget in `research.md` §7
- [X] T049 Final clean-shell build: in a fresh shell with Node/npm/npx unavailable on PATH, run `task build-all`; confirm exit 0 and that `audit-assets` passes
- [ ] T050 Commit  <!-- intentionally left unchecked: per CLAUDE.md, do not commit unless explicitly asked --> `specs/001-replace-tailwind-bulma/artifacts/report.json` and `report.md` (the final pre-merge run) per `quickstart.md` §5; do NOT commit `artifacts/screenshots/` (gitignored)

---

## Dependencies & Execution Order

### Phase dependencies

- **Setup (Phase 1)** — no dependencies; start immediately.
- **Foundational (Phase 2)** — depends on Phase 1 (assets vendored, Taskfile wired). **Blocks all user stories.**
- **User Story 1 (Phase 3)** — depends on Phase 2.
- **User Story 2 (Phase 4)** — depends on Phase 2 (independent of US1; can run in parallel with US1 if staffing allows, though most reviewers will sequence US1→US2 to keep the binary buildable at every checkpoint).
- **User Story 3 (Phase 5)** — depends on Phase 4 (operates on the rewritten templ files).
- **User Story 4 (Phase 6)** — depends on Phase 5 (validates the polished UI).
- **Polish (Phase 7)** — depends on Phase 6 (final audit pass).

### Within each user story

- Component rewrites (T023–T026) before page rewrites that depend on them (T027–T030) — `task_card` is consumed by `dashboard`, `header`/`footer` by every page.
- Templ generation (T031) after all rewrites; sync + smoke build (T032) last in US2.
- US3 polish edits each page once (T033–T036), then the focus-ring CSS update (T037) is global, then a manual sweep (T038).
- US4 desktop pass (T039) before phone pass (T040) before report aggregation (T041–T042).

### Parallel opportunities

- **Setup**: T002, T003, T004 can run in parallel with T001 (different files); T005 must follow T001–T004 (it `deps` them in Taskfile); T006 is independent.
- **Foundational**: T009 (test), T014 (main.js), T015 (log-viewer.js), T016 (embed test) are all parallelizable with each other; T007→T008 must be sequential (same file); T010 must follow T007–T008; T011 must follow T010; T012 must follow T010; T013 is independent.
- **US1**: T017, T018, T019, T020 touch different paths but should run sequentially within a single editing pass to keep diffs reviewable; T021 (README) is [P] with the deletes. T022 is the smoke and runs last.
- **US2**: Component rewrites T023–T026 fully parallel; page rewrites T027–T030 fully parallel; T031–T032 must follow both batches.
- **US3**: T033–T036 fully parallel; T037 follows; T038 runs last.
- **US4**: T039–T040 must run sequentially against the same binary; T041–T042 follow both.
- **Polish**: T043–T046 all parallel; T047–T049 sequential.

## Parallel execution example — User Story 2 component rewrites

```bash
# Open four editor sessions or four sub-agent tasks, one per file:
Task: "Rewrite internal/templates/components/header.templ to Bulma navbar with Alpine burger toggle"
Task: "Rewrite internal/templates/components/footer.templ to Bulma footer"
Task: "Rewrite internal/templates/components/spinner.templ to Bulma is-loading"
Task: "Rewrite internal/templates/components/task_card.templ to Bulma card; replace onclick with data-action='run-task'"
# Then sequentially:
task generate
task sync-assets
task build-all
```

## Implementation strategy

### MVP first (US1 + US2)

1. Complete Phase 1 (Setup) — assets vendored, Taskfile wired.
2. Complete Phase 2 (Foundational) — strict CSP, nonce middleware, base layout, app.css, JS layer, Go tests.
3. Complete Phase 3 (US1) — Tailwind artifacts removed, README updated, binary builds without Node.
4. Complete Phase 4 (US2) — every page rewritten to Bulma, all journeys work.
5. **STOP and VALIDATE**: Manually exercise every user journey; if green, this is the demo-ready MVP.

### Incremental delivery

- After MVP, layer US3 (polish + responsive + focus indicators) — one page-template at a time if needed.
- After US3, run the US4 validation procedure and produce the final committable `report.json` + `report.md`.
- Phase 7 cleanup at the end.

### Parallel team strategy

If two developers are available after Foundational completes:

- Dev A: US1 (artifact removal + README) → US2 (login + dashboard).
- Dev B: US2 (task_detail + logs) → US3 (polish).
- Either dev runs US4 (validation) once US3 lands.

---

## Notes

- `[P]` = different file, no dependency on an incomplete task in the same phase.
- `[Story]` label maps every implementation-phase task back to a spec acceptance ID for traceability.
- Tests are written alongside implementation (T009, T016) per Constitution III; this is not optional.
- The end-to-end validation in Phase 6 is assistant-driven via the `playwright-cli` skill (per `/speckit-clarify`). The committed `report.json` is the audit artifact.
- Both static-asset trees stay in sync via `task sync-assets` (added in T005). Edits in this task list always target the dev tree (`internal/server/static/`); the embed tree (`internal/assets/static/`) is overwritten mechanically at build time.
- Avoid: editing both static trees by hand, committing `*_templ.go` files (they are gitignored), bypassing `audit-assets`, or relaxing the strict CSP to make a step "work".
