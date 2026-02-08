# AGENTS.md — Miniflux Codebase Guide

## Project Overview

Miniflux is a minimalist RSS reader written in Go with a PostgreSQL backend.
Module: `miniflux.app/v2` | Go >= 1.24 | License: Apache-2.0

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
make lint              # Runs: go vet ./... && gofmt -d -e . && golangci-lint run
```

Linter config in `.golangci.yml`: standard set with `errcheck` disabled. Enforces SPDX license header.
Enables: errname, gocritic, misspell, perfsprint, prealloc, sqlclosecheck, staticcheck.

## Commit Messages

Conventional commits enforced in CI: `type(scope): subject`
Valid types: `build`, `chore`, `ci`, `docs`, `feat`, `fix`, `perf`, `refactor`, `revert`, `style`, `test`

## Key Directories

- `main.go` — Entry point, calls `cli.Parse()`
- `internal/` — Core app: `api/`, `cli/`, `config/`, `database/`, `fever/`, `googlereader/`, `http/`, `integration/`, `locale/`, `model/`, `reader/`, `storage/`, `ui/`, `validator/`, `worker/`
- `client/` — Go API client library (reusable package)
- `packaging/` — Docker, Debian, RPM, systemd packaging

## Code Style

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

Three groups separated by blank lines: stdlib, third-party, internal.

```go
import (
    "fmt"
    "net/http"

    "github.com/gorilla/mux"

    "miniflux.app/v2/internal/model"
    "miniflux.app/v2/internal/storage"
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

### Error Handling

- **Early return guard pattern** — check errors immediately, return, no else branches
- **Error prefix convention**: `package: description` (e.g., `store: unable to create feed`)
- **Use `%v` (not `%w`)** in `storage` layer errors (intentionally non-wrappable)
- **Use `%w`** in the `client` package for wrappable errors
- **Sentinel errors** with `errors.New` for domain errors (`ErrFeedNotFound`, `ErrDuplicatedFeed`)
- **Check with `errors.Is`** for sentinel comparisons
- **Backtick strings** for error format strings: `` fmt.Errorf(`store: unable to fetch feed: %v`, err) ``

### SQL / Database

- No ORM — all raw SQL with `database/sql` and `github.com/lib/pq`
- SQL in **backtick-delimited raw strings**, multi-line with tab indentation
- PostgreSQL `$N` placeholders (`$1`, `$2`, ...)
- `QueryRow` for single results, `Query` for multiple rows with `rows.Next()`/`rows.Scan()`/`defer rows.Close()`
- Builder pattern for complex queries (`EntryQueryBuilder`, `FeedQueryBuilder`)
- Manual transactions: `Begin()`/`Commit()`/`Rollback()`

### HTTP Handlers

- Handlers are **unexported methods** on an unexported `handler` struct
- Routes registered centrally in a `Serve()` function via gorilla/mux
- Request data via `request` package helpers: `request.UserID(r)`, `request.RouteInt64Param(r, "feedID")`
- Responses via semantic helpers: `json.OK()`, `json.Created()`, `json.NotFound()`, `json.BadRequest()`, `json.ServerError()`
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

### Types and Models

- **Modification requests** use pointer fields to distinguish "not set" from zero value:
  `FeedURL *string`, `Disabled *bool`
- **Type aliases for slices**: `type Feeds []*Feed`
- **JSON tags** on all exported struct fields: `json:"feed_url"`, `json:"-"` for internal-only

### Testing

- Standard `testing` package only — no assertion libraries
- Assertions via `t.Errorf`, `t.Fatalf` with backtick format strings
- Most tests are individual `TestXxx` functions (not table-driven)
- Map-based scenarios used for simple input/output tests
- Struct-based table-driven tests with `t.Run` for complex cases
- Test helpers are **unexported functions** within the test file
- Integration tests in `internal/api/api_integration_test.go` require a running server + PostgreSQL
- Benchmarks (`BenchmarkXxx`) and fuzz tests (`FuzzXxx`) exist in sanitizer package

## Philosophy

From CONTRIBUTING.md — Miniflux follows a **minimalist philosophy**:
- Improving existing features over adding new ones
- Quality over quantity; simple, maintainable code
- No unnecessary dependencies
- Keep functions small and focused
