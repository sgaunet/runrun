# Contract — Page ↔ Component ↔ Bulma class map

**Feature**: `001-replace-tailwind-bulma`

This contract enumerates which Bulma primitives each templ component
uses and what DOM identifiers the validation procedure depends on. It
is the source of truth for both the templ rewrite and the
`playwright-cli` selectors.

## Stable DOM identifiers (selectors used by validation)

| Page | Selector | Purpose |
|---|---|---|
| Login | `form#login-form` | Submission target |
| Login | `input#username` | Username field |
| Login | `input#password` | Password field |
| Login | `button#submitBtn` | Primary submit |
| Login | `.notification.is-danger` (when present) | Failure message |
| Dashboard | `#stat-total`, `#stat-running`, `#stat-success`, `#stat-failed`, `#stat-idle`, `#stat-executions` | Stat tiles |
| Dashboard | `input#searchInput` | Search filter |
| Dashboard | `select#statusFilter` | Status filter |
| Dashboard | `select#tagFilter` | Tag filter |
| Dashboard | `button[data-action="clear-filters"]` | "Clear" button (replaces previous `onclick="clearFilters()"`) |
| Dashboard | `[data-task-card]` | Each task card |
| Dashboard | `#noResults` | Empty-state for "no match" |
| Task detail | `h2[data-task-name]` | Task title element with `data-task-name="<name>"` |
| Task detail | `button[data-action="run-task"]` | "Run Task" button (replaces previous `onclick="runTask(event)"`) |
| Task detail | `table[data-execution-history]` | History table |
| Logs | `#log-container` | Streaming log container |
| Logs | `select[data-action="log-level-filter"]` | Level filter |
| Logs | `nav.pagination[data-log-segments]` | Segment pager |
| Header | `nav.navbar` | Top nav |
| Header | `a.navbar-burger[data-action="toggle-burger"]` | Mobile burger |

> The validation procedure resolves these selectors with the
> `playwright-cli` skill. Renaming any of them is a breaking change
> for the procedure and MUST be reflected here.

## Bulma class allowlist per component

This is the **definition of "fully Bulma-styled"** for grep-based
audits. A templ file in this feature is conformant if every class
attribute is a subset of this allowlist or the `runrun-*` overrides
defined in `app.css`.

### Base layout (`layouts/base.templ`)
`container`, `section`, `is-fluid`.

### Header (`components/header.templ`)
`navbar`, `navbar-brand`, `navbar-item`, `navbar-menu`, `navbar-start`,
`navbar-end`, `navbar-burger`, `is-active`, `has-text-primary`.

### Footer (`components/footer.templ`)
`footer`, `content`, `has-text-centered`, `is-size-7`.

### Spinner (`components/spinner.templ`)
`button`, `is-loading`, `is-large`, `is-primary`, `is-light`.

### Task Card (`components/task_card.templ`)
`card`, `card-header`, `card-content`, `card-footer`, `card-footer-item`,
`media`, `media-content`, `media-right`, `tag`, `tags`, `is-primary`,
`is-light`, `is-info`, `is-success`, `is-danger`, `is-warning`,
`button`, `is-small`.

### Dashboard (`pages/dashboard.templ`)
`level`, `level-item`, `box`, `columns`, `column`, `is-multiline`,
`field`, `field-body`, `label`, `control`, `input`, `select`,
`is-fullwidth`, `tag`, `notification`, `is-light`, `has-text-grey`,
`is-narrow`.

### Login (`pages/login.templ`)
`hero`, `is-fullheight`, `hero-body`, `container`, `columns`,
`column`, `is-half-tablet`, `is-one-third-desktop`, `box`, `field`,
`label`, `control`, `input`, `is-primary`, `is-fullwidth`, `button`,
`notification`, `is-danger`.

### Task Detail (`pages/task_detail.templ`)
`section`, `box`, `level`, `tags`, `tag`, `button`, `is-primary`,
`is-light`, `is-small`, `table`, `is-fullwidth`, `is-striped`,
`is-hoverable`.

### Logs (`pages/logs.templ`)
`section`, `box`, `field`, `select`, `tabs`, `pagination`,
`pagination-list`, `pagination-link`, `pagination-previous`,
`pagination-next`, `is-current`, `tag`, `is-info`, `is-warning`,
`is-danger`, plus `runrun-log-line`, `runrun-log-container` from
`app.css`.

## RunRun overrides allowed in `app.css`

```text
.runrun-log-container   { /* monospace, dark background, scroll */ }
.runrun-log-line        { /* whitespace-pre-wrap, break-words */ }
.runrun-log-line--debug { /* color for DEBUG */ }
.runrun-log-line--info  { /* color for INFO */ }
.runrun-log-line--warn  { /* color for WARN */ }
.runrun-log-line--error { /* color for ERROR */ }
.runrun-status-dot      { /* tiny inline status indicator */ }
.runrun-focus-ring      { /* unified focus indicator, FR-007 */ }
.runrun-sr-only         { /* screen-reader-only label (used as needed) */ }
```

Anything outside this allowlist OR the Bulma per-component lists
constitutes a contract violation.

## Acceptance tests

- Grep audit: `rg "class=\"[^\"]*"` over `internal/templates/**/*.templ`
  MUST produce zero classes that are neither in Bulma nor in the
  `runrun-*` allowlist.
- DOM audit (via `playwright-cli`): for each row in the "Stable DOM
  identifiers" table, the selector MUST resolve on the named page
  after authentication and navigation.
