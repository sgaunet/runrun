# Implementation Plan: Replace Tailwind with Bulma and ship a fully standalone binary

**Branch**: `001-replace-tailwind-bulma` | **Date**: 2026-05-19 | **Spec**: [`spec.md`](./spec.md)
**Input**: Feature specification from `/specs/001-replace-tailwind-bulma/spec.md`

## Summary

Replace TailwindCSS — including its Node-dependent build pipeline,
`tools/tailwindcss` binary, and `tailwind.config.js` — with vendored
**Bulma 0.9.4** plus a small `app.css` of RunRun overrides. Add
**Alpine.js v3 (CSP build)** for the few interactive widgets Bulma
cannot drive on its own (navbar burger, dropdowns, log filter
panel). Bundle a hand-picked **SVG sprite** (Heroicons-derived) for
status and action glyphs. Embed every CSS, JS, and icon asset into
the Go binary via `//go:embed all:static`, so a single binary plus
its config file is the only artifact a host needs. Tighten the
existing `SecurityHeadersMiddleware` to emit a **strict CSP** with
no `'unsafe-inline'` and no `'unsafe-eval'`, gated by a per-request
nonce. Validate the result by driving a real browser through the
acceptance scenarios with the `playwright-cli` skill and committing
the resulting `report.json`/`report.md` alongside the PR.

## Technical Context

- **Language/Version**: Go 1.25+; HTML emitted via Templ.
- **Primary Dependencies**: Bulma 0.9.4 (single vendored CSS),
  Alpine.js v3 CSP build (single vendored JS), existing
  `ansi_up` (already vendored). No Tailwind, no Node toolchain,
  no npm/npx.
- **Storage**: N/A (no data-model changes).
- **Testing**: `go test -race ./...` for Go code (new tests in
  `internal/middleware/` and `internal/assets/`).
  End-to-end validation via the `playwright-cli` skill, output
  committed under `specs/001-replace-tailwind-bulma/artifacts/`.
- **Target Platform**: Linux server binary; UI on current stable
  Chromium, Firefox, and WebKit.
- **Project Type**: Modular monolith (single Go binary, embedded
  static assets) — unchanged.
- **Performance Goals**: P95 < 200 ms for the existing core HTTP
  endpoints (Constitution V). Embedded asset bundle ≤ 65 KB gzipped
  total (`research.md` §7).
- **Constraints**: Strict CSP with per-request nonce, no
  `'unsafe-inline'`, no `'unsafe-eval'`. Single self-contained
  binary — no off-host requests at runtime. Build MUST work with no
  Node/npm/npx on PATH.
- **Scale/Scope**: Four pages (login, dashboard, task detail, logs)
  and four templ components (header, footer, spinner, task card).
  No new endpoints, no new entities.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Gate | Status | Evidence |
|---|---|---|---|
| I. Modular Monolith & Explicit Boundaries | No new processes, frameworks, or transport-coupling inside business logic. No new heavy abstractions. | **PASS** | Two vendored static files (Bulma, Alpine CSP) plus a small hand-written `app.css`. No DI container, no SSR framework, no node toolchain. |
| II. Idiomatic Go & Code Quality | Code passes `gofmt`/`go vet`, error wrapping is explicit, no god-packages. | **PASS** | New Go is one middleware (`CSPNonceMiddleware`) ~30 LoC + a context key + 2 small test files. Plain `errors` semantics. |
| III. Test-First Discipline | Unit tests for new behavior + race detector in CI. | **PASS (with caveat)** | New middleware and embed checks have unit tests written alongside the implementation (test-complete). End-to-end coverage is assistant-driven (FR-009/SC-006) per the `/speckit-clarify` decision — documented as an exception to the "CI runs `go test ./...`" gate, because the validation procedure produces a committed artifact instead. |
| IV. Predictable, Actionable UX | Consistent error/empty/loading states; primary actions distinct; actionable errors; keyboard focus visible. | **PASS** | Empty/loading/error state catalog (`research.md` §8) standardizes presentation; Bulma `notification is-danger/-warning` carries every user-visible error; `runrun-focus-ring` enforces FR-007. |
| V. Performance, Reliability, Observability | Outbound I/O has timeouts; structured logs; measurable performance budget. | **PASS** | No new I/O paths. Bundle-size budget (`research.md` §7) is the measurable backstop. Existing structured logging and request-id middleware untouched. |

**Complexity-tracking exceptions** (recorded for visibility — see also the table below):

1. **Two static-asset trees kept in sync mechanically**. The
   project already maintains a dual `internal/server/static/` ⇄
   `internal/assets/static/` layout (per `MEMORY.md`). Rather than
   collapse it (out of scope), this feature adds a build-time
   `task sync-assets` step that enforces byte-equality. Documented,
   automated; not a violation.
2. **Per-request CSP nonce** adds a middleware and a context key.
   This is the minimum machinery required to remove
   `'unsafe-inline'` from `script-src`; rejecting it would force
   us to relax the CSP. Documented in `research.md` §4.
3. **End-to-end validation via the `playwright-cli` skill** rather
   than a CI-pinned headless test runner. This is the user's
   explicit clarification, not a slip — the committed `report.json`
   is the audit artifact. The path to a future Playwright-Node
   suite is preserved (it would consume the same selectors documented
   in `contracts/page-component-map.md`).

### Re-evaluated post Phase 1 design

| Principle | Status | Notes |
|---|---|---|
| I  | **PASS** | Phase 1 added one middleware, one context key, one Taskfile step, and three contract docs. No new transport coupling. |
| II | **PASS** | Files touched are all in `internal/templates/`, `internal/middleware/`, `internal/assets/`, `internal/server/static/`, plus the Taskfile and CLAUDE.md pointer. |
| III| **PASS (caveat)** | Unit-test additions enumerated in `quickstart.md` §4; assistant-driven end-to-end gate documented in `contracts/validation-report.md`. |
| IV | **PASS** | Page-component-map (`contracts/page-component-map.md`) pins selectors and class allowlist — predictability gate cleared. |
| V  | **PASS** | Bundle-size budget enforced by Taskfile helper invoked from `task build-all`. |

## Project Structure

### Documentation (this feature)

```text
specs/001-replace-tailwind-bulma/
├── spec.md                     # Phase -1 (/speckit-specify) + clarifications (/speckit-clarify)
├── plan.md                     # this file
├── research.md                 # Phase 0
├── data-model.md               # Phase 1
├── quickstart.md               # Phase 1
├── contracts/
│   ├── static-asset-urls.md
│   ├── page-component-map.md
│   └── validation-report.md
├── checklists/
│   └── requirements.md         # quality checklist (filled by /speckit-specify)
├── tasks.md                    # Phase 2 (/speckit-tasks — NOT created here)
└── artifacts/                  # validation outputs (created at run time)
    ├── report.md
    ├── report.json
    └── screenshots/
```

### Source Code (repository root)

This feature edits only the existing modular-monolith layout. No
new top-level directories.

```text
runrun/
├── cmd/
│   └── runrun/                  # entrypoint (unchanged)
├── internal/
│   ├── assets/
│   │   ├── embed.go             # //go:embed all:static (unchanged)
│   │   ├── embed_test.go        # add: required URL + forbidden URL assertions
│   │   └── static/              # mirror; written by `task sync-assets`
│   ├── auth/                    # unchanged by this feature
│   ├── config/                  # unchanged by this feature
│   ├── csrf/                    # unchanged by this feature
│   ├── executor/                # unchanged by this feature
│   ├── middleware/
│   │   ├── middleware.go        # add CSPNonceMiddleware; tighten SecurityHeadersMiddleware
│   │   └── middleware_test.go   # add tests for nonce + CSP excludes unsafe-*
│   ├── ratelimit/               # unchanged
│   ├── security/                # unchanged
│   ├── server/
│   │   ├── handlers.go          # replace hard-coded HTML referring to styles.css
│   │   ├── routes.go            # insert CSPNonceMiddleware before SecurityHeadersMiddleware
│   │   ├── server.go            # unchanged (router wiring already supports new MW)
│   │   ├── static.go            # serves /static/*; unchanged
│   │   └── static/              # canonical dev tree (source of truth)
│   │       ├── css/
│   │       │   ├── bulma.min.css         (new, vendored)
│   │       │   └── app.css               (new, hand-written overrides)
│   │       ├── js/
│   │       │   ├── main.js               (rewritten: delegated handlers, no globals)
│   │       │   ├── log-viewer.js         (rewritten: small refactor for Alpine integration)
│   │       │   └── vendor/
│   │       │       ├── alpine.csp.min.js (new, vendored)
│   │       │       ├── ansi_up.js        (unchanged)
│   │       │       ├── ansi_up.min.js    (unchanged)
│   │       │       └── ansi_up_loader.js (unchanged)
│   │       ├── icons/
│   │       │   └── sprite.svg            (new, ~15–25 glyphs)
│   │       ├── favicon.ico               (unchanged)
│   │       ├── css/input.css             (DELETED)
│   │       ├── css/styles.css            (DELETED)
│   │       └── css/style.css             (DELETED)
│   ├── templates/
│   │   ├── components/
│   │   │   ├── footer.templ              (rewritten with Bulma classes)
│   │   │   ├── header.templ              (rewritten; navbar w/ Alpine burger)
│   │   │   ├── spinner.templ             (rewritten)
│   │   │   └── task_card.templ           (rewritten)
│   │   ├── layouts/
│   │   │   └── base.templ                (nonce-aware script tags, Bulma+app links)
│   │   └── pages/
│   │       ├── dashboard.templ           (rewritten)
│   │       ├── login.templ               (rewritten)
│   │       ├── logs.templ                (rewritten)
│   │       └── task_detail.templ        (rewritten; data-action replaces onclick)
│   └── websocket/                # unchanged by this feature
├── configs/
│   ├── example.yaml              # unchanged
│   └── playwright-test.yaml      # new test fixture
├── Taskfile.yml                  # remove build-css/watch-css; add sync-assets, audit-assets
├── tailwind.config.js            # DELETED
├── tools/tailwindcss             # DELETED
├── README.md                     # remove Tailwind sections; add quickstart link
├── CLAUDE.md                     # SPECKIT pointer → this plan
└── docs/                         # unchanged unless a doc references Tailwind
```

**Structure Decision**: keep the existing monolith layout. All work
fits inside `internal/{templates,middleware,server,assets}` plus
build files at repo root. No new packages, no new processes.

## Complexity Tracking

> Filled because the Constitution Check above flagged three exceptions
> requiring explicit justification.

| Violation | Why Needed | Simpler Alternative Rejected Because |
|---|---|---|
| Dual static-asset tree (`internal/server/static/` ⇄ `internal/assets/static/`) | Pre-existing layout per `MEMORY.md`; this feature does not refactor it. | Collapsing the trees is a separate change with its own risk surface (handlers/test paths). Out of scope; mitigated by adding a mechanical `task sync-assets` enforcer. |
| Per-request CSP nonce + new middleware | Required to remove `'unsafe-inline'` from `script-src` while still allowing the small handful of legitimate inline `<script>` tags emitted by templ. | Hash-based CSP would work for fully static scripts but breaks when the script body changes; nonces survive template edits with no CSP churn. |
| End-to-end gate is assistant-driven (no CI Playwright suite) | User clarification (Q3 in `spec.md` Clarifications). Validation artifact (`report.json`) is committed per PR, providing audit trail. | A pinned Node Playwright suite would add a Node toolchain and a non-trivial CI surface; declined by the user and out of scope here. Door is left open via the selectors documented in `contracts/page-component-map.md`. |

## Phase 0 — outline & research (DONE)

Output: [`research.md`](./research.md). Resolves every
NEEDS-CLARIFICATION from Technical Context and pins:

- Bulma 0.9.4 vendored as a single file
- Alpine.js v3 CSP build vendored as a single file
- Heroicons-derived SVG sprite
- Strict CSP with per-request nonce middleware
- `playwright-cli` skill validation procedure
- Test configuration fixture at `configs/playwright-test.yaml`
- Bundle-size budget (≤ 65 KB gzipped total)
- Empty/loading/error state catalog
- `task sync-assets` build step + asset audit
- Inline-handler migration to `data-action` + Alpine
- Cache busting via `?v=<git-sha>` query string
- Validation browser matrix

## Phase 1 — design & contracts (DONE)

Outputs:

- [`data-model.md`](./data-model.md) — Embedded Asset Bundle,
  Page Inventory, Component Inventory, CSP Nonce, Test
  Configuration Fixture, Validation Report.
- [`contracts/static-asset-urls.md`](./contracts/static-asset-urls.md)
  — required URLs, forbidden URLs, cache headers, CSP header
  contract, Go-side acceptance checks.
- [`contracts/page-component-map.md`](./contracts/page-component-map.md)
  — stable DOM selectors, per-component Bulma class allowlist,
  RunRun override allowlist in `app.css`.
- [`contracts/validation-report.md`](./contracts/validation-report.md)
  — schema for the assistant-produced report, mandatory scenario
  coverage, failure-artifact requirements.
- [`quickstart.md`](./quickstart.md) — build, run, validate, and
  pre-merge checklist; what the build MUST fail on.

Agent context update: see "Agent context" below.

## Phase 2 — task generation (NOT done here)

`/speckit-tasks` will turn these contracts and the page/component
allowlist into an ordered task list. Suggested groupings:

- **Setup**: add Bulma + Alpine CSP + SVG sprite to
  `internal/server/static/`; add `task sync-assets` and
  `task audit-assets`; remove Tailwind artifacts.
- **Foundational**: `CSPNonceMiddleware` + tightened
  `SecurityHeadersMiddleware`; `BaseData.CSPNonce`; base layout
  nonce wiring; `app.css` overrides; tests.
- **User Story 2** (parity): rewrite each templ file (page-by-page,
  starting with `login.templ`, then `dashboard.templ`, then
  `task_detail.templ`, then `logs.templ`); migrate inline handlers
  to `data-action`/Alpine.
- **User Story 3** (visual polish): apply the empty/loading/error
  state catalog; verify responsive behavior at 375 px.
- **User Story 4** (validation): produce `configs/playwright-test.yaml`;
  run the `playwright-cli` procedure; commit `report.json`+`report.md`.
- **Cleanup**: delete `tailwind.config.js`, `tools/tailwindcss`,
  legacy CSS files; update `README.md` and `MEMORY.md` reference.

## Agent context

`CLAUDE.md`'s `<!-- SPECKIT START -->` … `<!-- SPECKIT END -->`
block is updated to point at this plan (see end of plan workflow).
