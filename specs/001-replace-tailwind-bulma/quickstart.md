# Quickstart — Build, run, and validate the Bulma migration

**Feature**: `001-replace-tailwind-bulma`
**Audience**: a developer (human or AI assistant) who needs to build
the new binary, exercise the UI, and produce a validation report.

This is the single source of truth for both `task build-all` and the
assistant-driven `playwright-cli` validation procedure.

---

## 0. Prerequisites

- Go 1.25+ installed (`go version`)
- `task` installed (`task --version`)
- `templ` CLI installed (`templ version`) — run `task install-tools`
  if missing
- **No Node.js, no npm, no npx required.** If any of these are
  installed they MUST NOT be invoked by the build.

The `tools/tailwindcss` binary and `tailwind.config.js` are not used
by this build. Removing them is part of the implementation tasks.

## 1. Build

From repo root:

```bash
task build-all
```

Expected behavior:

- `task install-tools` (idempotent) ensures `templ` is present.
- `task generate` runs `templ generate` and produces `*_templ.go`.
- `task sync-assets` (new) copies
  `internal/server/static/` → `internal/assets/static/`, ensuring
  byte-identical trees.
- `task build` compiles `./runrun`.

The build MUST fail (non-zero exit) if any of the following hold:

- `internal/server/static/css/bulma.min.css` is missing
- `internal/server/static/js/vendor/alpine.csp.min.js` is missing
- `internal/server/static/icons/sprite.svg` is missing
- Any forbidden Tailwind artifact (`input.css`, `styles.css`,
  `tailwind.config.js`, `tools/tailwindcss`) is present
- `internal/server/static/` and `internal/assets/static/` disagree
  byte-for-byte after `task sync-assets`

> **Why `task sync-assets`**: the `MEMORY.md` note about the two
> directories is enforced mechanically by the build, not relied upon
> as a human reminder.

Confirm the build produced `./runrun` and the embedded assets:

```bash
./runrun version
```

## 2. Run with the test configuration

```bash
./runrun server --config configs/playwright-test.yaml
```

Expected:

- Server listens on `http://localhost:18080`.
- Three deterministic tasks (`echo-ok`, `fail-task`, `multi-step`)
  are visible on `/dashboard` after login.
- Credentials: `e2e-admin` / `e2e-password!`.

## 3. Assistant-driven `playwright-cli` validation procedure

Run by invoking the `playwright-cli` skill against the running
binary. The procedure produces
`specs/001-replace-tailwind-bulma/artifacts/report.json` (and
matching screenshots and `report.md`).

### 3.1 Setup

1. Start the binary on port 18080 (step 2).
2. Open a fresh browser context with the `playwright-cli` skill.
3. Set viewport to `1280×800` (`desktop`) for the first pass.
4. Open the browser's network panel and CSP-violation logger; pipe
   both into the report (the skill records HAR + console events).

### 3.2 Scenarios (covers every acceptance scenario in `spec.md`)

For each scenario, the skill asserts: page status code 200 (or the
expected redirect), all embedded assets resolve, zero off-host
requests, zero CSP violations in the console.

| ID | What to do | What to assert |
|---|---|---|
| US1-AC1 | GET `/login` → log in as `e2e-admin` → land on `/dashboard` | Page fully styled (compute style on `body` shows Bulma's font + bg), navbar visible, six stat tiles present |
| US1-AC2 | Click each top-nav link, exercise every page in the Page Inventory | All pages render; zero 404s; zero off-host requests |
| US1-AC3 | (build-time, not browser): verify Taskfile + repo contain no Tailwind refs | `rg -i tailwind` returns zero matches outside `specs/` and changelogs |
| US2-AC1 | Submit login form with valid creds | Lands on dashboard with task list populated |
| US2-AC2 | Click "Run Task" on `echo-ok` → confirm dialog if any | Navigates to `/logs/{id}`; log lines appear; final status badge says success |
| US2-AC3 | Apply log level filter to a `multi-step` execution; paginate segments | Filtered output shown; pagination state changes |
| US2-AC4 | Click "Sign out" | Redirect to `/login`; subsequent `/dashboard` GET redirects back to `/login` |
| US2-AC5 | Unauth GET `/dashboard` | Redirect to `/login` |
| US3-AC1 | Visual sweep at 1280×800 | Typography/spacing/button-style consistent (skill records computed-style fingerprints of `h1`, `h2`, `.button.is-primary`, `.input` on each page) |
| US3-AC2 | Resize to 375×812; revisit every page | No horizontal scroll on `main` (assert `scrollWidth <= clientWidth`) |
| US3-AC3 | Keyboard tab through one full page (e.g., dashboard) | Every focusable element shows a visible focus indicator (computed `outline` or `box-shadow` non-empty) |
| US3-AC4 | Trigger an error (wrong password, run `fail-task`, kill WS) | Each error shows a `.notification.is-danger` or `.notification.is-warning` with a clear action |
| US4-AC1 | (this very procedure) | Procedure runs end-to-end |
| US4-AC2 | Re-run after intentionally breaking a templ class | Skill flags the failed scenario with URL+screenshot |
| US4-AC3 | Inspect HAR for the full run | Zero off-host requests, zero 404s on `/static/` |

### 3.3 Phone-viewport pass

After the desktop pass, resize to `375×812` and repeat US1-AC2,
US2-AC1, US2-AC2, US2-AC3, US3-AC2.

### 3.4 Report generation

After all scenarios run, the assistant writes:

- `artifacts/report.json` per the schema in
  `contracts/validation-report.md`
- `artifacts/report.md` summarizing pass/fail per story
- `artifacts/screenshots/*.png` for each viewport pass; on failure
  also a `failure-<id>.png`

Both `report.json` and `report.md` are committed alongside the PR
for the final pre-merge run.

## 4. Go-side acceptance checks (run by `task test`)

```bash
task test
```

Adds these checks to the existing suite:

- `internal/middleware/middleware_test.go`:
  - `TestCSPNonceMiddlewareGeneratesFreshNonce`
  - `TestSecurityHeadersExcludesUnsafeInline`
  - `TestSecurityHeadersExcludesUnsafeEval`
- `internal/assets/embed_test.go`:
  - `TestEmbeddedAssetURLs` (each required URL serves a non-empty
    body with the expected content type)
  - `TestForbiddenLegacyURLsReturn404`
- A small `internal/server/static/_audit_test.go` (or equivalent)
  that fails if `bulma.min.css`/`alpine.csp.min.js`/`sprite.svg` are
  missing or if Tailwind artifacts are present.

CI runs `go test -race ./...` per Constitution III.

## 5. Pre-merge checklist

1. `task build-all` succeeds from a clean checkout, no Node anywhere
   on PATH.
2. `task test` passes with `-race`.
3. `rg -i tailwind` over the repo returns zero matches outside
   `specs/` and the changelog.
4. The validation procedure (§3) was run and its `report.json` shows
   `"verdict": "pass"`.
5. `report.md` and `report.json` are committed under
   `specs/001-replace-tailwind-bulma/artifacts/`.

If any item fails, do not merge.
