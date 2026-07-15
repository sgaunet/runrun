# Contract — Validation Report schema

**Feature**: `001-replace-tailwind-bulma`

The assistant-driven `playwright-cli` validation procedure produces a
single report per run. The report is the artifact that proves
SC-001..SC-009 hold.

## Location

```text
specs/001-replace-tailwind-bulma/artifacts/
├── report.md              # human-readable summary
├── report.json            # machine-readable summary (this contract)
└── screenshots/
    ├── login-desktop.png
    ├── login-phone.png
    ├── dashboard-desktop.png
    ├── dashboard-phone.png
    ├── task_detail-desktop.png
    ├── task_detail-phone.png
    ├── logs-desktop.png
    ├── logs-phone.png
    └── failure-<scenario-id>.png  # only when a scenario fails
```

`artifacts/` is git-ignored except for `report.md` and `report.json`
of the **final** pre-merge run, which are committed alongside the PR.

## JSON schema (`report.json`)

```jsonc
{
  "runTimestamp":   "2026-05-19T12:34:56Z",
  "binarySHA256":   "<64-hex-char SHA-256 of ./runrun>",
  "binaryGitSHA":   "<short git sha at build>",
  "browser":        "chromium",                  // one of: chromium, firefox, webkit
  "viewports":      ["desktop-1280x800", "phone-375x812"],
  "scenarios": [
    {
      "id":            "US1-AC1",                // matches spec acceptance scenario IDs
      "story":         "US1",
      "result":        "pass",                   // pass | fail
      "pageURL":       "http://localhost:18080/dashboard",
      "screenshotPath":"screenshots/dashboard-desktop.png",
      "notes":         "ok"
    }
  ],
  "networkAudit": {
    "offHostRequests":      0,                   // MUST be 0
    "embeddedAsset404s":    0,                   // MUST be 0
    "tailwindReferences":   0                    // MUST be 0
  },
  "cspViolations":          0,                   // MUST be 0 (SC-009)
  "verdict":                "pass"               // pass | fail (computed)
}
```

## Verdict computation

`verdict = "pass"` iff **all** of:

- every `scenarios[].result == "pass"`
- `networkAudit.offHostRequests == 0`
- `networkAudit.embeddedAsset404s == 0`
- `networkAudit.tailwindReferences == 0`
- `cspViolations == 0`

Any other state → `verdict = "fail"`.

## Mandatory scenario coverage

Every acceptance scenario in `spec.md` MUST appear in
`scenarios[]` with at least one viewport. The mapping is:

| Scenario ID | spec source |
|---|---|
| US1-AC1, US1-AC2, US1-AC3 | User Story 1 |
| US2-AC1..AC5              | User Story 2 |
| US3-AC1..AC4              | User Story 3 |
| US4-AC1..AC3              | User Story 4 |

## Failure artifact requirements

For every failing scenario:

- A screenshot at the failing step (named
  `screenshots/failure-<id>.png`) is required.
- `notes` MUST include the URL, the failing assertion, and a
  one-sentence cause hypothesis.

## Update cadence

A fresh report is produced for the final pre-merge build. Earlier
in-flight reports overwrite the previous run's artifacts; only the
pre-merge run is committed.
