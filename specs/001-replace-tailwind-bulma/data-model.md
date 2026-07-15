# Phase 1 — Data Model: Replace Tailwind with Bulma

**Feature**: `001-replace-tailwind-bulma`
**Date**: 2026-05-19

This feature is primarily a UI/asset migration; it does not introduce
domain entities (no DB tables, no new task fields). The "data model"
documented here is the **shape of artifacts and runtime values that
the build and request pipeline manipulate** — sufficient to derive
tasks, contracts, and acceptance checks.

---

## Entities

### 1. Embedded Asset Bundle

The set of files compiled into the binary via `//go:embed all:static`.

| Field | Type | Constraints |
|---|---|---|
| `path` | string (POSIX, relative to `static/`) | MUST start with `css/`, `js/`, `icons/`, or be the literal `favicon.ico`. |
| `contentType` | string | Derived from extension (`.css`, `.js`, `.svg`, `.ico`). |
| `etag` | string | Strong ETag generated at build time from file content (already done by existing `NewStaticFileServer`). |
| `cacheControl` | string | Constant: `public, max-age=31536000, immutable`. |
| `gzippedSizeBytes` | int | MUST satisfy the budget table in `research.md` §7. |

Required files (post-migration):

```text
static/
├── css/
│   ├── bulma.min.css          # vendored Bulma 0.9.4
│   └── app.css                # RunRun overrides (hand-written, small)
├── js/
│   ├── main.js                # log-stream + delegated handlers
│   ├── log-viewer.js          # ansi rendering helpers
│   └── vendor/
│       ├── alpine.csp.min.js  # Alpine.js v3, CSP build
│       ├── ansi_up.js
│       ├── ansi_up.min.js
│       └── ansi_up_loader.js
├── icons/
│   └── sprite.svg             # ~15–25 Heroicons-derived glyphs
└── favicon.ico
```

Forbidden after migration (validated by a make/test step):

```text
static/css/input.css          # Tailwind source
static/css/styles.css         # Tailwind output
static/css/style.css          # legacy stray file (remove)
```

Validation:

- The build MUST fail if `bulma.min.css`, `alpine.csp.min.js`, or
  `sprite.svg` is missing from either tree.
- The build MUST fail if any forbidden file is present in either tree.
- `task sync-assets` enforces `internal/server/static/` ⇄
  `internal/assets/static/` are byte-identical.

### 2. Page Inventory

The full set of user-visible pages whose styling and behavior are
migrated.

| Field | Type | Notes |
|---|---|---|
| `name` | string | One of: `login`, `dashboard`, `task_detail`, `logs`. |
| `route` | string | `/login`, `/dashboard`, `/tasks/{name}`, `/logs/{executionID}`. |
| `auth` | enum {`public`, `authenticated`} | `login` is public; the other three are authenticated. |
| `components` | list of component refs | See "Component Inventory". |
| `responsive_breakpoints` | list of viewport widths | At minimum: 375, 768, 1280. |
| `interactive_widgets` | list of widget refs | Used to know which Alpine directives to emit. |

### 3. Component Inventory

| Component | File | Responsibilities | Bulma primitives used |
|---|---|---|---|
| Base layout | `layouts/base.templ` | HTML scaffold, `<head>`, nonce script tag injection | `navbar`, `container`, `section`, `footer` |
| Header | `components/header.templ` | Branded navbar + sign-out link | `navbar`, `navbar-brand`, `navbar-burger`, `navbar-end` |
| Footer | `components/footer.templ` | Static footer | `footer`, `content` |
| Spinner | `components/spinner.templ` | Inline loader | `button is-loading` (or `progress is-small is-primary`) |
| Task Card | `components/task_card.templ` | Task summary card on dashboard | `card`, `tag`, `button is-primary`, `button is-light` |
| Dashboard page | `pages/dashboard.templ` | Stats row + filter bar + task grid | `level`, `box`, `columns`, `field`, `input`, `select`, `tag` |
| Login page | `pages/login.templ` | Centered login form | `hero is-fullheight`, `box`, `field`, `input`, `button is-primary is-fullwidth` |
| Task Detail page | `pages/task_detail.templ` | Task header + execution history | `card`, `tags`, `button`, `table is-fullwidth is-striped` |
| Logs page | `pages/logs.templ` | Live log stream + level filter + segment pager | `box`, `tabs`, `field`, `pagination`, custom `log-container` styles in `app.css` |

### 4. CSP Nonce (request-scoped)

| Field | Type | Lifecycle |
|---|---|---|
| `value` | string (base64url-encoded, 16 random bytes) | Generated per HTTP request by `CSPNonceMiddleware`; attached to `request.Context()` under a typed key `cspNonceKey{}`. |
| Read by | `SecurityHeadersMiddleware` (sets header), `BaseData.CSPNonce` (rendered on `<script>` tags) | Same request only; never logged. |
| Validation | MUST be at least 128 bits of entropy; MUST be different on every response. Asserted by middleware unit test. |

### 5. Test Configuration Fixture

| Field | Type | Notes |
|---|---|---|
| `path` | string | `configs/playwright-test.yaml` (checked in). |
| `users[0].username` | string | `e2e-admin`. |
| `users[0].passwordHash` | bcrypt hash | Hash of `e2e-password!`. |
| `tasks[]` | list | At minimum: `echo-ok`, `fail-task`, `multi-step` (see `research.md` §6). |
| `server.port` | int | `18080` (test-only, avoids 8080 clash). |
| `server.log_directory` | path | `./.test-logs`. |
| `jwt_secret` | string | 32+ chars; documented test-only. |

### 6. Validation Report

The artifact produced by an assistant-driven `playwright-cli`
validation run.

| Field | Type | Notes |
|---|---|---|
| `runTimestamp` | RFC3339 string | UTC. |
| `binarySHA` | string | SHA-256 of the runrun binary under test. |
| `scenarios[]` | list | One per user-story acceptance scenario. |
| `scenarios[].id` | string | E.g. `US1-AC1`. |
| `scenarios[].result` | enum {`pass`, `fail`} | |
| `scenarios[].pageURL` | string | URL at point of pass/fail. |
| `scenarios[].screenshotPath` | string | Relative to `artifacts/`. |
| `scenarios[].notes` | string | Failure reason or "ok". |
| `networkAudit.offHostRequests` | int | MUST equal 0 to pass SC-001. |
| `networkAudit.embeddedAsset404s` | int | MUST equal 0 to pass SC-001. |
| `cspViolations` | int | MUST equal 0 to pass SC-009. |

---

## Relationships

```text
Page Inventory ──uses──> Component Inventory ──depends on──> Embedded Asset Bundle
                                  │
                                  └──renders with──> CSP Nonce (per request)

Test Configuration Fixture ──fed to──> running binary ──observed by──>
   playwright-cli skill ──produces──> Validation Report
```

## Lifecycle / state transitions

This feature introduces no new state machines. The existing task
execution states (idle, pending, running, success, failed) are
unchanged; only their visual presentation (badges, colors) is
restyled to Bulma classes.

## Out of scope (explicitly excluded)

- New API entities or persistence layer changes.
- Dark-mode preferences entity.
- A11y-conformance metadata.
- User-preference theming.
