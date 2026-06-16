**English** | [中文](README_zh.md)

[![CI](https://github.com/go-sqlex/sqlex/actions/workflows/ci.yml/badge.svg)](https://github.com/go-sqlex/sqlex/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/go-sqlex/sqlex)](https://goreportcard.com/report/github.com/go-sqlex/sqlex)
[![GoDoc](https://pkg.go.dev/badge/github.com/go-sqlex/sqlex)](https://pkg.go.dev/github.com/go-sqlex/sqlex)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

# sqlex

> A modern enhancement of Go's `database/sql` — inheriting all capabilities of [jmoiron/sqlx](https://github.com/jmoiron/sqlx) with added Hook aspects, generic JSON types, and more.

## Table of Contents

- [Highlights](#highlights)
- [Installation](#installation)
- [Quick Start](#quick-start)
- [Features](#features)
- [Usage Examples](#usage-examples)
  - [Basic CRUD](#basic-crud)
  - [Named Parameter Queries](#named-parameter-queries)
  - [IN Queries](#in-queries)
  - [Prepared Statements](#prepared-statements)
  - [Transaction Management](#transaction-management)
  - [JsonValue[T]](#jsonvaluet)
  - [Hook Aspects](#hook-aspects)
  - [StrictMode](#strictmode)
  - [NamedExt / BindExt Unified Interfaces](#namedext--bindext-unified-interfaces)
- [Comparison with jmoiron/sqlx](#comparison-with-jmoironsqlx)
- [Bug Fixes & Improvements](#bug-fixes--improvements)
- [Migration Guide](#migration-guide)
- [Performance](#performance)
- [License](#license)

## Highlights

- 🔄 **Fully compatible with database/sql** — all standard methods preserved, incremental enhancements
- 🏗️ **Struct scanning** — `Get/Select` maps query results directly to Go structs
- 📝 **Named parameters** — `:name` style named queries, supporting structs and maps
- 🪝 **Hook aspects** — pluggable SQL execution interceptors (logging, tracing, metrics)
- 📦 **JsonValue[T]** — generic JSON column type, mapping database JSON fields to strong types
- 🔀 **Transparent IN expansion** — auto-detects slice args and expands IN clauses
- 🚀 **Cross-database out of the box** — write SQL with `?` placeholders universally; the framework auto-converts to target database format (`$N`, `:argN`, `@pN`)
- 🔐 **Enhanced transaction management** — `CloseWithErr` auto-commits/rolls back
- 🎯 **NamedExt/BindExt unified interfaces** — DB, Tx, and Conn share the same extended method signatures
- 🛠️ **Multi-driver support** — PostgreSQL, MySQL, SQLite, Oracle, SQL Server
- 🛡️ **StrictMode** — off by default (lenient mode); enable with `SetStrict(true)` to catch field mismatches early in development
- ⚡ **Go 1.24 modernization** — `any` type, modular file structure, enhanced error messages

## Installation

```bash
go get github.com/go-sqlex/sqlex
```

Requires Go 1.24 or later.

### Version Specification

You can also pin to a specific version using semantic versioning:

```bash
# Latest version
go get github.com/go-sqlex/sqlex@latest

# Specific version
go get github.com/go-sqlex/sqlex@v1.5.0

# Or in go.mod
require github.com/go-sqlex/sqlex v1.5.0
```

For the list of available versions and releases, see [GitHub Releases](https://github.com/go-sqlex/sqlex/releases).

## Quick Start

```go
package main

import (
    "fmt"
    "log"

    "github.com/go-sqlex/sqlex"
    _ "github.com/mattn/go-sqlite3"
)

type User struct {
    ID    int    `db:"id"`
    Name  string `db:"name"`
    Email string `db:"email"`
}

func main() {
    // Connect to database
    db, err := sqlex.Connect("sqlite3", ":memory:")
    if err != nil {
        log.Fatal(err)
    }
    defer db.Close()

    // Create table
    db.MustExec(`CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT, email TEXT)`)
    db.MustExec(`INSERT INTO users (name, email) VALUES ('Alice', 'alice@example.com')`)
    db.MustExec(`INSERT INTO users (name, email) VALUES ('Bob', 'bob@example.com')`)

    // Query single row
    var user User
    err = db.Get(&user, "SELECT * FROM users WHERE id = ?", 1)
    fmt.Printf("User: %+v\n", user)

    // Query multiple rows
    var users []User
    err = db.Select(&users, "SELECT * FROM users")
    fmt.Printf("Users: %+v\n", users)
}
```

## Features

### Core capabilities inherited from sqlx

- **Struct scanning**: `Get`, `Select`, `StructScan` map row results to Go structs
- **Named queries**: `:name` style parameters via `NamedQuery`, `NamedExec`, `NamedStmt`
- **Multi-driver binding**: `Rebind` auto-converts `?` to target database bindvars (`$1`, `:arg1`, `@p1`)
- **IN clause expansion**: `In()` function expands slice args into multiple placeholders
- **Multiple scan modes**: `StructScan`, `SliceScan`, `MapScan`
- **Prepared statements**: `Preparex`, `PrepareNamed` with enhanced preparation
- **Connection management**: `Connect` (with Ping), `Open`, `MustConnect`

### sqlex new capabilities

| Feature | Description |
|---------|-------------|
| **Hook aspects** | `AddHook` — pluggable SQL execution interceptors (onion model) |
| **JsonValue[T]** | `types.JsonValue[T]` — generic JSON column type |
| **NamedGet/NamedSelect** | Named parameter convenience methods on DB/Tx (built-in IN expansion) |
| **NamedExec/NamedExecContext** | Named parameter execution (built-in IN expansion) |
| **CloseWithErr** | Auto Commit/Rollback based on error |
| **NamedExt interface** | Unified named parameter programming interface for DB/Tx/Conn |
| **BindExt interface** | Unified basic query programming interface for DB/Tx/Conn |
| **Positional auto-IN** | `Select`/`Get`/`Exec`/`Queryx`/`QueryRowx`/`MustExec` (with Context versions) auto-detect slice args and expand IN clauses |
| **Auto Rebind** | All query methods auto-convert `?` to target database placeholders, zero code change for cross-database support |
| **StrictMode** | Default lenient mode (`strict=false`); enable with `SetStrict(true)` for detailed errors on column-field mismatch |

## Usage Examples

### Basic CRUD

```go
// Use ? placeholders universally — the framework auto-converts
// to target database bindvar format ($N, :argN, @pN)

// Insert
result, err := db.Exec("INSERT INTO users (name, email) VALUES (?, ?)", "Alice", "alice@example.com")

// Query single row → struct
var user User
err = db.Get(&user, "SELECT * FROM users WHERE id = ?", 1)

// Query multiple rows → slice
var users []User
err = db.Select(&users, "SELECT * FROM users WHERE age > ?", 18)

// Update
_, err = db.Exec("UPDATE users SET name = ? WHERE id = ?", "Alice Updated", 1)

// Delete
_, err = db.Exec("DELETE FROM users WHERE id = ?", 1)
```

### Named Parameter Queries

```go
// Using struct as parameter
user := User{Name: "Alice", Email: "alice@example.com"}
_, err = db.NamedExec(`INSERT INTO users (name, email) VALUES (:name, :email)`, user)

// Using map as parameter
params := map[string]any{"name": "Alice"}

// NamedGet — query single row
var result User
err = db.NamedGet(&result, `SELECT * FROM users WHERE name = :name`, params)

// NamedSelect — query multiple rows
var results []User
err = db.NamedSelect(&results, `SELECT * FROM users WHERE name = :name`, params)
```

### IN Queries

```go
ids := []int{1, 2, 3, 4, 5}

// Positional: auto-detects slice and expands IN
var users []User
err = db.Select(&users, "SELECT * FROM users WHERE id IN (?)", ids)

// Named: built-in IN expansion
err = db.NamedSelect(&users,
    `SELECT * FROM users WHERE id IN (:ids) AND status = :status`,
    map[string]any{"ids": ids, "status": "active"})
```

> Note: `sqlex.In()` / `sqlex.Named()` are legacy top-level functions; the framework calls them automatically. Use the high-level methods above which include Rebind/Hook/StrictMode.

#### Escape APIs for edge cases

```go
import "github.com/go-sqlex/sqlex"

// sqlex.AsValue(v) — force no expansion (even in (?) context)
db.Select(&rows, `SELECT * FROM t WHERE id = ANY(?)`,
    sqlex.AsValue(pq.Array([]int{1, 2, 3})))

// sqlex.AsList(slice) — force expansion (even outside (?) context)
db.Exec(`SELECT some_func(?, ?)`, 100,
    sqlex.AsList([]int{1, 2, 3}))
```

### Prepared Statements

```go
// Preparex auto-Rebinds — use ? uniformly
stmt, err := db.Preparex("SELECT * FROM users WHERE name = ?")
var user User
err = stmt.Get(&user, "Alice")

// PrepareNamed — named prepared statement
nstmt, err := db.PrepareNamed("SELECT * FROM users WHERE name = :name")
err = nstmt.Get(&user, map[string]any{"name": "Alice"})
```

### Transaction Management

```go
// Recommended pattern: CloseWithErr auto-management
func createUserWithProfile(db *sqlex.DB, user User, profile Profile) (err error) {
    tx, err := db.Beginx()
    if err != nil {
        return err
    }
    defer func() { tx.CloseWithErr(err) }() // auto Commit or Rollback

    _, err = tx.NamedExec(`INSERT INTO users (name) VALUES (:name)`, user)
    if err != nil {
        return err // CloseWithErr detects err != nil, auto Rollback
    }

    _, err = tx.NamedExec(`INSERT INTO profiles (user_name, bio) VALUES (:user_name, :bio)`, profile)
    return nil // CloseWithErr detects err == nil, auto Commit
}
```

### JsonValue[T]

```go
import "github.com/go-sqlex/sqlex/types"

type Article struct {
    ID       int                            `db:"id"`
    Title    string                         `db:"title"`
    Metadata types.JsonValue[ArticleMeta]   `db:"metadata"`
}

type ArticleMeta struct {
    Tags      []string `json:"tags"`
    ViewCount int      `json:"view_count"`
}

// Write — auto-serializes to JSON
article := Article{
    Title:    "Hello World",
    Metadata: types.NewJsonValue(ArticleMeta{
        Tags:      []string{"go", "sql"},
        ViewCount: 0,
    }),
}
db.NamedExec(`INSERT INTO articles (title, metadata) VALUES (:title, :metadata)`, article)

// Read — auto-deserializes
var a Article
db.Get(&a, "SELECT * FROM articles WHERE id = ?", 1)
```

### Hook Aspects

```go
// Custom Hook — e.g., OpenTelemetry tracing
type TracingHook struct{}

func (h *TracingHook) BeforeQuery(ctx context.Context, event *sqlex.QueryEvent) context.Context {
    ctx, span := tracer.Start(ctx, "sql."+event.OperationType)
    span.SetAttributes(attribute.String("db.statement", event.Query))
    return ctx
}

func (h *TracingHook) AfterQuery(ctx context.Context, event *sqlex.QueryEvent) {
    span := trace.SpanFromContext(ctx)
    if event.Error != nil {
        span.RecordError(event.Error)
    }
    span.End()
}

db.AddHook(&TracingHook{})
```

### StrictMode

```go
// Default: lenient mode (strict=false), silently ignores extra columns
db, _ := sqlex.Connect("postgres", dsn)
fmt.Println(db.IsStrict()) // false

// Enable strict mode: returns detailed error on field mismatch
db.SetStrict(true)
err = db.Select(&users, "SELECT * FROM users")
// err: missing destination name email (index 2), age (index 3) in UserPartial

// strict auto-propagates to Tx/Conn
tx, _ := db.Beginx()    // inherits DB's strict setting
conn, _ := db.Connx(ctx) // inherits DB's strict setting
```

### NamedExt / BindExt Unified Interfaces

```go
// NamedExt: write context-agnostic functions (named parameters)
func getUserByName(ext sqlex.NamedExt, name string) (*User, error) {
    var user User
    err := ext.NamedGet(&user, `SELECT * FROM users WHERE name = :name`,
        map[string]any{"name": name})
    return &user, err
}

// Works with DB, Tx, or Conn
user, err := getUserByName(db, "Alice")
tx, _ := db.Beginx()
user, err = getUserByName(tx, "Bob")

// BindExt: write context-agnostic functions (positional parameters)
func listUsers(ext sqlex.BindExt, minAge int) ([]User, error) {
    var users []User
    err := ext.Select(&users, "SELECT * FROM users WHERE age > ?", minAge)
    return users, err
}
```

## Comparison with jmoiron/sqlx

| Feature | jmoiron/sqlx | sqlex |
|---------|-------------|-------|
| Go version | 1.10+ | 1.24+ |
| Struct scanning | ✅ | ✅ |
| Named queries | ✅ | ✅ |
| Bindvar conversion | ✅ | ✅ (enhanced: supports `\?` and `??` escape, skips string literals, identifiers, comments, PG dollar quoting) |
| IN clause expansion | ✅ `In()` | ✅ Auto-IN across all DB/Tx/Conn × Exec/Query/Select/Get/Named* paths |
| Cross-database placeholders | ❌ Manual Rebind | ✅ All methods auto-Rebind, use `?` uniformly (including `Preparex`) |
| Field matching | `unsafe` (default strict) | `StrictMode` (default lenient, more intuitive) |
| Hook aspects | ❌ | ✅ `AddHook` pluggable SQL interceptors |
| JsonValue[T] | ❌ | ✅ `types.JsonValue[T]` |
| NamedGet/NamedSelect | ❌ | ✅ DB/Tx convenience methods |
| CloseWithErr | ❌ | ✅ Auto transaction management |
| NamedExt/BindExt interfaces | ❌ | ✅ DB/Tx/Conn unified interfaces |
| Unicode named parameters | ⚠️ Unreliable | ❌ Not supported (ASCII only; Unicode elsewhere is safe) |
| PostgreSQL `::` | ❌ Misidentified | ✅ Correctly handled |
| Named query string literals | ❌ Colons misidentified | ✅ Skips colons in strings/comments |
| Named parameter fallback | ❌ Errors on misidentification | ✅ Missing params preserved as `:name` literals |

## Bug Fixes & Improvements

sqlex fixes the following known issues from jmoiron/sqlx:

- **SQL lexical element handling**: Original assumes all `?`/`:name` are placeholders, ignoring string literals, comments, identifiers, and PG dollar quoting. sqlex correctly handles all SQL lexical elements.
- **ConnectContext connection leak**: `ConnectContext` didn't close the connection on Ping failure. sqlex calls `db.Close()`.
- **Rebind escaped question marks**: Original doesn't support `\?` or `??` escapes. sqlex supports both.
- **Rebind string literals**: Original replaces `?` inside string literals. sqlex correctly skips them.
- **Named query string literal colons**: Original misidentifies colons in strings (e.g., IPv6 addresses, time formats) as named parameters ([#872](https://github.com/jmoiron/sqlx/issues/872)).
- **Named parameter fallback**: Original errors on missing params; sqlex preserves them as `:name` literals ([#892](https://github.com/jmoiron/sqlx/issues/892)).
- **Unified lexer**: Original has duplicated skip logic in `Rebind`/`In`/`compileNamedQuery`. sqlex uses a shared `scanSkipSegment` in `lexer.go`.
- **Named parameter name rules**: Original allows digits at start (`:123`). sqlex requires `[A-Za-z_][A-Za-z0-9_.]*`.
- **`::` handling**: Original misidentifies PG type cast `::` as named parameter. sqlex correctly skips it.
- **Positional query cross-database failure**: Original `Select`/`Get`/`Exec` don't auto-Rebind, failing on PostgreSQL. sqlex auto-Rebinds all methods.

## Migration Guide

### From jmoiron/sqlx

**1. Change import path:**

```go
// Old
import "github.com/jmoiron/sqlx"

// New
import "github.com/go-sqlex/sqlex"
```

**2. Change package references:**

```go
// Old
db, err := sqlx.Connect("postgres", dsn)

// New
db, err := sqlex.Connect("postgres", dsn)
```

**3. StrictMode default change:**

sqlex defaults to lenient mode (`strict=false`), matching jmoiron/sqlx's `unsafe=true`. To catch mismatches early:

```go
db.SetStrict(true)
```

**4. Gradual adoption:**

All new features are optional:
- Step 1: Replace import path and package name (zero code changes needed)
- Step 2: Switch transactions to `CloseWithErr` pattern
- Step 3: Use `NamedGet/NamedSelect` instead of `NamedQuery` + manual scanning
- Step 4: Register custom Hooks as needed (logging, tracing, metrics)

## Testing

```bash
# Main package unit tests (no DB dependency)
go test -count=1 -timeout=120s .

# Cross-DB tests (SQLite only, no external dependencies)
SQLX_MYSQL_DSN=skip SQLX_POSTGRES_DSN=skip \
  go test -count=1 -timeout=120s ./tests/cross_db/

# All tests
go test -count=1 -timeout=300s ./...

# With race detector
go test -v -race -count=1 ./...
```

**DSN configuration**: Write complete DSNs directly in `.env.test` using the `SQLX_*_DSN` namespace. Set to `skip` to skip tests for that driver. SQLite defaults to `:memory:`.

## Performance

- **Zero-overhead principle**: No Hook overhead when unregistered; auto-Rebind is a no-op for `QUESTION`-type drivers (MySQL/SQLite)
- **Slice arg detection**: `needsInRewrite` uses reflection type checks (nanosecond-level for non-slice args)
- **Mapper caching**: Field mapping results cached after first use
- **Hook execution**: Hooks run synchronously; use lightweight operations or async for heavy ones

## License

[MIT License](LICENSE)

Based on [jmoiron/sqlx](https://github.com/jmoiron/sqlx) — thanks to Jason Moiron for the excellent work.
