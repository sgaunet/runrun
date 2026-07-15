# Feature Specification: Replace Tailwind with Bulma and ship a fully standalone binary

**Feature Branch**: `001-replace-tailwind-bulma`
**Created**: 2026-05-19
**Status**: Draft
**Input**: User description: "remove tailwindcss, replace tailwindcss by bulma, embed the css and js to have a standalone binary. use playwright-cli to test the program, ensure UX is great, clean pro. test it deeply to validate the replacement of tailwindcss by bulma. you will find bulma documentation here: https://bulma.io/documentation/"

## Clarifications

### Session 2026-05-19

- Q: Bulma ships no JavaScript — what should drive interactive widgets
  (navbar burger, dropdowns, modals, tabs) and the existing log-stream
  WebSocket client? → A: Alpine.js (embedded)
- Q: What icon strategy should the embedded UI use? → A: Bulma plus a
  small hand-picked SVG sprite embedded in the binary (no icon CDN,
  no webfont)
- Q: Which end-to-end browser test framework should we pin? → A: The
  `playwright-cli` skill only. End-to-end validation is performed by
  the AI assistant driving a real browser through the `playwright-cli`
  skill; no Node test runner, no committed Playwright test scripts,
  no CI-pinned headless suite are required by this feature.
- Q: What CSP posture should the new bundle target? → A: Strict CSP,
  no `unsafe-inline` for either scripts or styles. All JavaScript
  (including Alpine.js and any templ-generated glue) ships as
  embedded `.js` files; all stylesheets ship as embedded `.css`
  files. Per-request nonces or hashes are used wherever the strict
  policy would otherwise block legitimate inline content.
- Q: What accessibility bar should the redesigned UI commit to? → A:
  Status quo — keyboard reachability and visible focus indicators
  (FR-007) only. No new commitment to WCAG AA conformance, contrast
  ratios, ARIA labels, or screen-reader testing in this feature; any
  such expansion is out of scope here.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Operator runs a fully self-contained RunRun binary (Priority: P1)

An operator downloads (or builds) a single RunRun binary and runs it on a
clean host with no network access, no Node.js toolchain, no Tailwind CLI, and
no separate static-asset directory. The web interface loads completely from
the binary itself — every page, every stylesheet, every script — and looks
clean, modern, and professional.

**Why this priority**: This is the core promise of the change. If the binary
is not truly standalone, the migration has not delivered its value. Every
other story depends on the binary being self-contained.

**Independent Test**: Copy the freshly built binary to a clean container
or VM with no internet egress, no `node`, no `tailwindcss`, and no source
tree. Start the server with a minimal config, open the web UI in a browser,
and confirm that every page renders fully styled with no missing fonts,
no missing icons, no broken layouts, and no 404s in the network panel.

**Acceptance Scenarios**:

1. **Given** a clean host with the RunRun binary and a config file only,
   **When** the operator starts the server and opens the dashboard,
   **Then** the dashboard renders with full Bulma styling, the network
   panel shows zero external (off-host) asset requests, and zero 404s
   for CSS, JS, fonts, or icons.
2. **Given** the binary is running with no network egress,
   **When** the operator navigates between login, dashboard, task detail,
   and live logs pages, **Then** all pages render fully styled and
   interactive without any failed asset loads.
3. **Given** the operator builds the binary from source,
   **When** they run the documented build command, **Then** the build
   succeeds without invoking the Tailwind CLI, downloading a Tailwind
   binary, or requiring Node.js / npm / npx.

---

### User Story 2 - Existing user journeys continue to work after the visual migration (Priority: P1)

A returning user who already uses RunRun can sign in, browse tasks, launch
a task, watch live logs, filter logs by level, paginate through log
segments, and sign out — exactly as before. Nothing they could do on the
Tailwind UI is missing or broken on the Bulma UI.

**Why this priority**: A redesign that drops or breaks features is a
regression, not an improvement. Parity is non-negotiable.

**Independent Test**: With the new Bulma-based binary running, execute the
full set of pre-existing user journeys end-to-end in a browser and confirm
each one completes successfully and looks intentional (not "raw HTML").

**Acceptance Scenarios**:

1. **Given** a valid user account, **When** the user submits the login
   form with correct credentials, **Then** they land on the dashboard and
   the dashboard renders the full task list with tags, statuses, and
   action buttons.
2. **Given** a user on the dashboard, **When** they click "Run" on a task
   and accept any required confirmation, **Then** the task is submitted,
   the live logs page opens, log lines stream in, and final status is
   displayed clearly.
3. **Given** a user on the live logs page, **When** they apply the level
   filter and use the segment pagination controls, **Then** filtered logs
   are shown and pagination works exactly as in the previous UI.
4. **Given** a user with a valid session, **When** they sign out, **Then**
   they are returned to the login page and protected routes redirect to
   login on subsequent attempts.
5. **Given** an unauthenticated visitor, **When** they request a protected
   page, **Then** they are redirected to login with a clear, styled error
   message if applicable.

---

### User Story 3 - Visual quality and accessibility meet a clean, professional bar (Priority: P2)

A first-time visitor opening the RunRun web UI on a current desktop browser
forms an immediate impression of a clean, modern, professional product:
consistent spacing, legible typography, a coherent color system, clear
primary actions, and visible focus states. The same UI is usable on a
tablet- and phone-sized viewport without horizontal scrolling on primary
pages.

**Why this priority**: The user explicitly asked for "great UX, clean,
pro". Parity (Story 2) is necessary but not sufficient — the new UI must
also look the part.

**Independent Test**: Open every page in the application at desktop
(≥1280 px), tablet (~768 px), and phone (~375 px) widths, capture
screenshots, and verify visual consistency against a small acceptance
checklist (see Success Criteria).

**Acceptance Scenarios**:

1. **Given** the application is open in a current desktop browser,
   **When** a reviewer inspects each primary page, **Then** typography,
   spacing, button styles, form controls, and status colors are visually
   consistent across pages.
2. **Given** the dashboard or task detail page is loaded at phone width,
   **When** a reviewer scrolls vertically, **Then** no horizontal scroll
   bar appears on the primary content area and tap targets remain at
   least the size of a comfortable touch target.
3. **Given** a user navigates the UI with a keyboard only, **When** they
   Tab through interactive elements, **Then** each focused element shows
   a clearly visible focus indicator and the tab order matches the
   visual order.
4. **Given** an error occurs (invalid login, failed task launch, lost
   websocket), **When** the user sees the error message, **Then** the
   message is styled consistently (color, icon if applicable, placement)
   and tells the user what to do next.

---

### User Story 4 - Assistant-driven browser validation proves the migration is correct (Priority: P2)

A maintainer (the AI assistant operating in this repository, or a human
following the same script) drives a real browser through every primary
user journey via the `playwright-cli` skill, captures screenshots, and
reports pass/fail per scenario. Validation runs against a freshly built
binary and uses the test configuration file checked into the repository.

**Why this priority**: The user explicitly asked for deep validation
using a browser-driving tool. Assistant-driven validation turns "we
think it works" into "we observed it working" before merge.

**Independent Test**: Build the binary, start it with the test
configuration, and run the documented validation script via the
`playwright-cli` skill; the run produces a report (pass/fail per
scenario) and screenshots of every primary page at desktop and phone
widths.

**Acceptance Scenarios**:

1. **Given** a freshly built binary running with the test
   configuration, **When** the maintainer runs the documented
   validation procedure via the `playwright-cli` skill, **Then** every
   user journey listed in this spec is exercised and the result of
   each is recorded as pass or fail with a screenshot artifact.
2. **Given** a regression that breaks a primary journey, **When** the
   validation procedure is run, **Then** the corresponding scenario is
   reported as failed with the URL, the step that failed, and a
   screenshot at the point of failure.
3. **Given** the validation completes successfully, **When** the
   maintainer inspects the network panel captures, **Then** zero
   requests to off-host origins and zero 404s for embedded assets are
   observed across the run.

---

### Edge Cases

- The host blocks all egress (no CDN access): the UI must still render
  fully styled.
- The binary is run from a directory other than the source tree: no
  asset is loaded from disk; everything is embedded.
- A user opens the app on a phone-sized viewport: primary pages must
  remain usable without horizontal scroll on the main content area.
- A user navigates exclusively by keyboard: focus indicators are
  visible at every step.
- A user opens the app with browser caching disabled: pages load and
  function correctly on a cold cache.
- The websocket disconnects mid-stream on the logs page: the user sees
  a clear, styled status message.
- The browser's preferred locale or font stack differs from the
  developer's: layout does not break.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The system MUST present every existing page (login,
  dashboard, task detail, live logs) styled exclusively with Bulma — no
  Tailwind utility classes, no Tailwind output stylesheet, and no
  Tailwind tooling at build time.
- **FR-002**: The system MUST serve all CSS, JavaScript, fonts, and
  icons required by the UI from the binary itself, with no runtime
  dependency on external networks, CDNs, or filesystem paths outside
  the embedded asset bundle.
- **FR-003**: The build process MUST produce the runnable binary without
  invoking Node.js, npm, npx, the Tailwind CLI, or downloading a
  Tailwind binary.
- **FR-004**: The system MUST preserve every existing user-facing
  capability: authentication (login/logout), CSRF-protected task
  submission, task browsing, tag filtering on the dashboard, real-time
  log streaming, log level filtering, and log segment pagination.
- **FR-005**: Every page MUST render correctly with full styling on the
  current stable releases of the major desktop browsers (Chromium,
  Firefox, WebKit) and on a phone-sized viewport (~375 px width) for the
  primary content area.
- **FR-006**: Every primary action MUST have a visually prominent
  primary button styled distinctly from secondary and destructive
  actions; destructive actions MUST be visually distinguished (e.g.
  color and/or confirmation) from non-destructive ones.
- **FR-007**: All interactive elements (links, buttons, form controls,
  pagination controls) MUST display a visible keyboard focus indicator,
  and the visual tab order MUST match the DOM order.
- **FR-008**: All user-visible error states (failed login, failed task
  launch, lost websocket, validation errors) MUST be displayed with a
  styled message that states what happened and the next step the user
  can take.
- **FR-009**: The repository MUST contain a documented validation
  procedure that exercises every user story listed above using the
  `playwright-cli` skill against a freshly built binary started with
  the test configuration. The procedure MUST be runnable by following
  a single checked-in document (no extra setup beyond building the
  binary and ensuring the skill is available).
- **FR-010**: The validation procedure MUST report a failure for any
  of the following observations during a run: a page returns 4xx/5xx
  unexpectedly, an embedded asset returns 404, an external (off-host)
  asset is requested, or an asserted UI element is missing or
  unstyled. Each failure MUST include the URL, the failing step, and
  a screenshot.
- **FR-011**: Documentation (README and/or `docs/`) MUST be updated to
  remove every reference to Tailwind tooling (`task build-css`,
  `task watch-css`, `tools/tailwindcss`, `tailwind.config.js`) and to
  describe the new build, run, and test workflow.
- **FR-012**: No Tailwind input file, generated Tailwind stylesheet,
  Tailwind binary, or Tailwind configuration file remains in the
  repository after the migration.
- **FR-013**: The application MUST emit a strict Content-Security-Policy
  that does NOT permit `unsafe-inline` for `script-src` or `style-src`.
  Every page MUST render and every interactive widget MUST function
  under that policy. All JavaScript and CSS MUST be served from the
  embedded asset bundle (or via per-request nonces/hashes where strict
  CSP would otherwise block legitimate use).

### Key Entities

- **Embedded Asset Bundle**: The set of CSS, JS, font, and icon files
  compiled into the binary at build time and served at stable URLs
  under the application's static path. Replaces the previous
  Tailwind-built `styles.css` and any external font/icon CDN
  references.
- **Page Inventory**: The complete set of user-visible pages whose
  styling and behavior must be preserved: login, dashboard, task
  detail, live logs. The dashboard, task detail, and live logs pages
  are reachable only by an authenticated user.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 100 % of pages in the Page Inventory render with full
  Bulma styling on a host with no network egress, with zero 404s for
  embedded assets and zero requests to off-host origins.
- **SC-002**: 100 % of the user journeys listed in Stories 1, 2, and
  3 are completable end-to-end by a first-time user without needing
  developer assistance.
- **SC-003**: A reviewer comparing the pre-migration UI and the
  post-migration UI rates the post-migration UI as "equal or better"
  on each of: visual consistency, clarity of primary actions, and
  professional feel.
- **SC-004**: On a phone-sized viewport (~375 px wide), every primary
  page is usable with no horizontal scroll on the main content area,
  measured by inspection of each page in the Page Inventory.
- **SC-005**: The documented build command, run from a clean checkout
  on a host with no Node.js / npm / npx, produces a runnable binary
  in under five minutes on typical developer hardware.
- **SC-006**: The documented validation procedure (run via the
  `playwright-cli` skill against a freshly built binary) covers every
  user story in this spec, produces screenshots for every primary
  page at desktop and phone widths, and reports zero failed scenarios
  on the final pre-merge run.
- **SC-007**: After the migration, the repository contains zero files
  whose name or content references Tailwind (excluding historical
  references in changelogs, release notes, or this spec).
- **SC-008**: A keyboard-only user can complete every primary user
  journey (sign in, launch a task, watch logs, filter logs, sign out)
  without resorting to the mouse.
- **SC-009**: Every page loads and every interactive widget functions
  under a strict Content-Security-Policy with no `unsafe-inline` for
  scripts or styles, verified by observing zero CSP violations in the
  browser console across the validation procedure run.

## Assumptions

- The migration targets the existing page set only (login, dashboard,
  task detail, live logs) and the existing components (header, footer,
  spinner, task card). No new pages or features are added by this
  change.
- "Bulma" refers to the current stable release of the Bulma CSS
  framework as documented at <https://bulma.io/documentation/>. Bulma
  is CSS-only by design; interactive behavior (navbar burger,
  dropdowns, modals, tab toggles) is driven by Alpine.js, embedded
  in the binary as part of the asset bundle. The existing WebSocket
  log-stream client remains plain JavaScript, also embedded, and may
  be glued to the UI via Alpine where it improves clarity.
- Icons are delivered as a small hand-picked SVG sprite embedded in
  the binary (covering status, action, and log-level glyphs). No icon
  CDN, no webfont, no external font load at runtime.
- "Standalone binary" means the binary plus its config file are the
  only runtime artifacts required. A log directory may still be
  written to disk per existing behavior.
- End-to-end browser validation is performed by the AI assistant
  driving a real browser via the `playwright-cli` skill, against a
  binary started locally with a dedicated test configuration file
  checked into the repository. No Node test runner, committed
  Playwright test scripts, or CI-pinned headless suite are required
  by this feature; adding such automation later is out of scope here.
- "Current desktop browsers" means the current stable releases of
  Chromium-based browsers, Firefox, and WebKit/Safari at the time of
  the migration.
- The visual change is the only user-facing scope. Functional
  behavior, URLs, API contracts, and authentication semantics are
  unchanged.
- The existing WebSocket-based live log streaming continues to work
  unchanged; only its presentation layer is restyled.
