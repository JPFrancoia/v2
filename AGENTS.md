# AGENTS.md — Miniflux Codebase Guide

## Project Overview

Miniflux is a minimalist RSS reader written in Go with a PostgreSQL backend.
Module: `miniflux.app/v2` | Go >= 1.24 | License: Apache-2.0

## Before Changing Code

- Apply the owner's coding and review rules in this guide to new code when they conflict with inherited Miniflux style.
- Preserve existing public interfaces and behavior. Do not reformat or rename unrelated legacy code.
- Check the current branch, worktree status, and intended base before editing.
- Read the relevant workspace `docs/reference/` documents, accepted decisions, and plan when the task crosses repository boundaries.
- Read nearby code before adding a helper, type, dependency, or new pattern.
- Use a dedicated Git worktree for substantial code changes. Give each concurrent writer a separate worktree.
- Make small, focused documentation or configuration edits in the current checkout when safe.
- If a worktree is blocked by existing changes or an unclear base, ask before stashing, resetting, or moving anything.
- Treat an approved plan slice as a hard boundary. Check `git diff --name-only` before review.
- If validation calls for an out-of-scope change, report it instead of extending the task.
- Do not commit, push, deploy, or merge without explicit approval in the current conversation.
- After Go code changes, run the `go-reviewer` subagent against the changed Go code before completion.
- Address actionable review findings or report them. Do not change unrelated code to satisfy a review convention.

## Build Commands

```bash
make miniflux          # Build binary for current platform (PIE mode)
make run               # Build + run locally with debug logging, migrations, admin creation
make build             # Cross-compile for all supported platforms
make linux-amd64       # Build for a specific platform (also: darwin-arm64, etc.)
make docker-image      # Build Alpine-based Docker image
make clean             # Remove build artifacts
```

## Test Commands

```bash
make test                    # Unit tests: go test -cover -race -count=1 ./...
go test -v -run TestName ./internal/pkg/...   # Run a single test by name
go test -v -run TestName -count=1 ./internal/reader/sanitizer/  # Single test, specific package
make integration-test        # Full API integration tests (needs PostgreSQL)
make clean-integration-test  # Cleanup after integration tests
```

## Lint Commands

```bash
make lint              # Runs: go vet ./..., gofmt check, golangci-lint run
```

Linter config in `.golangci.yml`: standard set with `errcheck` disabled. Enforces SPDX license header.
Enables: errname, gocritic, goheader, loggercheck, misspell, perfsprint, sqlclosecheck, staticcheck, whitespace.

## Commit Messages

Conventional commits enforced in CI: `type(scope): subject`
Valid types: `build`, `chore`, `ci`, `docs`, `feat`, `fix`, `perf`, `refactor`, `revert`, `style`, `test`

## Key Directories

- `main.go` — Entry point, calls `cli.Parse()`
- `internal/` — Core app: `api/`, `cli/`, `config/`, `database/`, `fever/`, `googlereader/`, `http/`, `integration/`, `locale/`, `model/`, `reader/`, `storage/`, `ui/`, `validator/`, `worker/`
- `client/` — Go API client library (reusable package)
- `packaging/` — Docker, Debian, RPM, systemd packaging

## Code Style

### Human Maintainability

Write for the next person who maintains the code. Prefer simple, explicit changes. Follow existing patterns unless they conflict with these rules.
Avoid speculative abstractions and unnecessary dependencies. Explain non-obvious choices. Add a focused check for behavior that can break.

### License Header (required on every .go file)

```go
// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0
```

### Package Declaration

Every package includes an import path comment:
```go
package storage // import "miniflux.app/v2/internal/storage"
```

### Imports

For new imports, use three groups separated by blank lines: standard library, internal packages, third-party packages.

```go
import (
    "fmt"
    "net/http"

    "miniflux.app/v2/internal/model"
    "miniflux.app/v2/internal/storage"

    "github.com/gorilla/mux"
)
```

Use aliases only when needed to resolve name collisions (e.g., `json_parser "encoding/json"`).

### Naming Conventions

- **Packages**: lowercase, single word or concatenated (`mediaproxy`, `urllib`, `googlereader`)
- **Exported functions**: `PascalCase`, verb+noun (`CreateFeed`, `FeedByID`, `ValidateFeedCreation`)
- **Unexported functions**: `camelCase` (`createFeed`, `getFeedsSorted`)
- **Boolean functions**: `Is`/`Has`/`Exists` prefix (`FeedExists`, `IsValidURL`)
- **Builder methods**: `With` prefix (`WithCategoryID`, `WithCounters`, `WithSorting`)
- **Constants**: `PascalCase` exported, `camelCase` unexported
- **Receivers**: short abbreviations (`s *Storage`, `f *Feed`, `h *handler`, `e *EntryQueryBuilder`)
- **HTTP params**: `w`/`r` for ResponseWriter/Request
- **Acronyms in new names**: use `Id` and `Url`, except when matching existing public names or wire formats
- **Interface values**: use `any` instead of `interface{}` in new code
- **Type aliases**: avoid aliases that do not clarify a boundary

### Constructors and Receivers

- Use the existing `NewXxx` pattern for constructors. Return pointers for stateful handlers and stores.
- Use pointer receivers when methods change state or the value owns resources that must not be copied.
- Prefer value receivers for small data types when copying is safe. Keep the receiver style consistent for each type.
- Do not add a one-implementation interface unless an existing boundary needs it.

### Error Handling

- **Early return guard pattern** — check errors immediately, return, no else branches
- **Error prefix convention**: `package: description` (e.g., `store: unable to create feed`)
- **Use `%w`** when adding context to returned errors in new code, including storage and client code
- **Sentinel errors** with `errors.New` for domain errors (`ErrFeedNotFound`, `ErrDuplicatedFeed`)
- **Check with `errors.Is`** for sentinel comparisons
- **Backtick strings** for error format strings: `` fmt.Errorf(`store: unable to fetch feed: %w`, err) ``
- Use the existing response helpers in HTTP handlers. Handle errors immediately and return.

### SQL / Database

- No ORM — all raw SQL with `database/sql` and `github.com/lib/pq`
- Write new SQL in lowercase. Put new fixed queries in `.sql` files and load them with `//go:embed`.
- Keep dynamic query builders when a fixed query file cannot express the query.
- PostgreSQL `$N` placeholders (`$1`, `$2`, ...) unless the existing query API supports named parameters
- `QueryRow` for single results, `Query` for multiple rows with `rows.Next()`/`rows.Scan()`/`defer rows.Close()`
- Builder pattern for complex queries (`EntryQueryBuilder`, `FeedQueryBuilder`)
- Manual transactions: `Begin()`/`Commit()`/`Rollback()`

### HTTP Handlers

- Handlers are **unexported methods** on an unexported `handler` struct
- Register API routes in `internal/api/api.go` with method patterns on `http.ServeMux`
- Request data via `request` package helpers: `request.UserID(r)`, `request.RouteInt64Param(r, "feedID")`
- Respond with the existing `internal/http/response` helpers, such as `response.JSON(w, r, value)` and `response.JSONBadRequest(w, r, err)`
- Guard clause pattern — check error/not-found, respond, `return` immediately

### Logging

Uses `log/slog` (structured logging) exclusively. No third-party logger.

```go
slog.Info("Description",
    slog.Int64("user_id", userID),
    slog.String("feed_url", feedURL),
)
```

- Component prefix in brackets for subsystem logging: `[API]`, `[Middleware]`
- Use `slog.Group` for grouping related attributes (e.g., request/response context)
- Use context-aware `slog` methods when request or task context is available. Do not log credentials.

### Types and Models

- **Modification requests** use pointer fields to distinguish "not set" from zero value:
  `FeedURL *string`, `Disabled *bool`
- **Type aliases for slices**: `type Feeds []*Feed`
- **JSON tags** on all exported struct fields: `json:"feed_url"`, `json:"-"` for internal-only
- Define structs at package level, including implementation-only types.

### Comments

- Write Go doc comments for every new or changed function, including private helpers. Explain what the function does and why it exists.
- Add a short doc comment to every new or changed test function. State what the test verifies.
- Use inline comments to explain non-obvious logic, not to repeat the code.
- Do not add decorative comment separators. Leave unrelated existing functions unchanged.

### Testing

- Use `github.com/stretchr/testify`: `assert` for non-fatal checks and `require` for fatal preconditions in new tests.
- Keep existing standard-library tests unchanged unless the task touches them.
- Most tests are individual `TestXxx` functions (not table-driven)
- Map-based scenarios used for simple input/output tests
- Struct-based table-driven tests with `t.Run` for complex cases
- Test helpers are **unexported functions** within the test file
- Use `httptest.NewRequest` and `httptest.NewRecorder` for isolated HTTP handler checks
- Use `t.Setenv()` when a test needs environment variables
- Integration tests in `internal/api/api_integration_test.go` require a running server + PostgreSQL
- For database behavior, prefer real PostgreSQL integration tests over mocks when practical
- Run focused tests during development and relevant full checks before completion.
- If PostgreSQL is configured, use `make integration-test` for the full API integration suite.
- Benchmarks (`BenchmarkXxx`) and fuzz tests (`FuzzXxx`) exist in sanitizer package

### Regression Tests

- Reproduce each reported failure before an edit.
- If reproduction is impossible, record the reason before an edit.
- Add an automated regression test for every confirmed bug fix.
- Make sure that the test fails with the faulty code and passes with the fix.
- Test the root cause or user-visible behavior. Do not test source text or incidental implementation details.
- Run the focused test and the relevant test suite before completion.
- Do not mark a bug fix complete if its regression test is absent or fails.
- If automation is impossible, stop and request explicit user approval for an exception.

## Philosophy

From CONTRIBUTING.md — Miniflux follows a **minimalist philosophy**:
- Improving existing features over adding new ones
- Quality over quantity; simple, maintainable code
- No unnecessary dependencies
- Keep functions small and focused
