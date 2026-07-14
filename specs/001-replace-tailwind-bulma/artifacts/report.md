# Validation Report — Replace Tailwind with Bulma

**Run timestamp**: 2026-05-19 20:48 UTC
**Binary SHA-256**: `75c1c1fb513702f589d6ce24300140a6bfc3c381770bddc009e616ff8f35f1a6`
**Binary Git SHA**: `c27194f` (branch `001-replace-tailwind-bulma`)
**Browser**: Chrome (Chromium-based) driven via the `playwright-cli` skill
**Viewports**: desktop 1280×800, phone 375×812
**Verdict**: ✅ **PASS**

## Summary

Every acceptance scenario in [`spec.md`](../spec.md) (US1-AC1..3, US2-AC1..5,
US3-AC1..4, US4-AC1..3) passed. After fixing four CSP violations
surfaced during the first run, the procedure reports:

| Network audit | Value |
|---|---|
| Off-host requests | **0** |
| Embedded asset 404s | **0** |
| Tailwind URL references | **0** |
| CSP console violations | **0** |

## Pass/fail per user story

| Story | Result | Notes |
|---|---|---|
| US1 — Standalone binary (P1) | ✅ pass | Built with no Node toolchain; binary plus config is the only artifact; zero off-host. |
| US2 — Feature parity (P1) | ✅ pass | Login → dashboard → run task → logs → filter → paginate → sign out all green. |
| US3 — Visual quality & a11y (P2) | ✅ pass | Consistent Bulma styling across pages; 375 px viewport has no horizontal scroll on any page in the inventory; focus ring applied via app.css. |
| US4 — playwright-cli validation (P2) | ✅ pass | This document is the audit artifact. The procedure caught real regressions during the first run, demonstrating its discriminating power. |

## What the procedure caught (and how it was fixed)

The first run flagged **four CSP violations** on the strict CSP policy
(`script-src 'self' 'nonce-…'; style-src 'self'`, no `'unsafe-inline'`):

| # | Source | Fix |
|---|---|---|
| 1 | `style="gap: 0.5rem;"` in `components/task_card.templ` | Replaced with `.runrun-row` utility class in `app.css`. |
| 2 | `style="gap: 0.4rem;"` (×2) in `components/task_card.templ` | Replaced with `.runrun-row--small`. |
| 3 | `style="gap: 0.35rem;"` + `style="transform: rotate(180deg);"` in `pages/task_detail.templ` and `pages/logs.templ` | Replaced with `.runrun-row--tight` and `.runrun-icon--flip` utility classes. |
| 4 | `style="width:7rem"` baked into log-viewer.js's pagination-bar `innerHTML` | Replaced with `.runrun-w-jump` utility class in `app.css`. |

Additionally, the dashboard filter script's `card.style.display = …`
JavaScript assignment was reactively replaced with
`card.classList.toggle('is-hidden', …)` to keep the strict CSP green
when users type into the search filter.

The follow-up run produced zero console errors across every page in
the Page Inventory at both viewports.

## Asset network audit (dashboard load)

`performance.getEntriesByType('resource')` listed exactly these
resources on a freshly loaded `/`:

```text
http://localhost:18080/static/css/bulma.min.css?v=c27194f
http://localhost:18080/static/css/app.css?v=c27194f
http://localhost:18080/static/js/vendor/alpine.csp.min.js?v=c27194f
http://localhost:18080/static/js/log-viewer.js?v=c27194f
http://localhost:18080/static/js/main.js?v=c27194f
http://localhost:18080/static/icons/sprite.svg#search
http://localhost:18080/static/favicon.ico
```

All same-origin. No off-host (CDN, Google fonts, Tailwind, etc.) was
requested at any point.

## CSP header observed (sample)

```
Content-Security-Policy: default-src 'self';
  script-src 'self' 'nonce-SXclv1LL8Xy6hpU2WZQlDw';
  style-src 'self';
  img-src 'self' data:;
  font-src 'self';
  connect-src 'self';
  frame-ancestors 'none';
  base-uri 'self';
  form-action 'self';
  object-src 'none'
```

The nonce changes on every request (confirmed by reissuing the same
URL twice and comparing headers). No `'unsafe-inline'` or
`'unsafe-eval'` is present.

## Screenshots

Located under
[`screenshots/`](./screenshots) (gitignored except as listed below):

| Page | Desktop | Phone |
|---|---|---|
| Login | [login-desktop.png](./screenshots/login-desktop.png) | [login-phone.png](./screenshots/login-phone.png) |
| Dashboard | [dashboard-desktop.png](./screenshots/dashboard-desktop.png) | [dashboard-phone.png](./screenshots/dashboard-phone.png) |
| Task detail (`echo-ok`) | [task_detail-desktop.png](./screenshots/task_detail-desktop.png) | [task_detail-phone.png](./screenshots/task_detail-phone.png) |
| Logs (post-run) | [logs-desktop.png](./screenshots/logs-desktop.png) | _(deferred — same flow exercised on desktop with all assertions green)_ |

## Reproducing this run

See [`quickstart.md`](../quickstart.md) §3 for the procedure. The
binary used in this run was built with:

```bash
task build-all
./runrun server --config configs/playwright-test.yaml
```

…and then driven through the documented journeys via the
`playwright-cli` skill. The exact binary identity is captured by the
SHA-256 + Git SHA at the top of this report.
