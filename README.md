**English** | [中文](README_zh.md)

[![CI](https://github.com/go-sqlex/sqlex/actions/workflows/ci.yml/badge.svg)](https://github.com/go-sqlex/sqlex/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/go-sqlex/sqlex)](https://goreportcard.com/report/github.com/go-sqlex/sqlex)
[![GoDoc](https://pkg.go.dev/badge/github.com/go-sqlex/sqlex)](https://pkg.go.dev/github.com/go-sqlex/sqlex)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

# sqlex

> A **drop-in replacement** for [jmoiron/sqlx](https://github.com/jmoiron/sqlx) — 100% API-compatible, with added Hook aspects, generic JSON types, bug fixes, and more.

**sqlex is fully API-compatible with sqlx.** All sqlx methods (`Get`, `Select`, `Exec`, `NamedQuery`, `Preparex`, etc.) work identically. Migrating takes 30 seconds — just change the import path. New features are purely additive and optional.

```diff
- import "github.com/jmoiron/sqlx"
+ import "github.com/go-sqlex/sqlex"
```

What you get for free after migrating:

- 🚀 **Auto-Rebind** — write `?` everywhere, works on PostgreSQL (`$1`), MySQL (`?`), SQLite (`?`), SQL Server (`@p1`). No more manual `db.Rebind()`. Including `Preparex`.
- 🐛 **SQL parsing fixes** — colons in strings, `::` type casts, `?` in comments are correctly handled. Silent bugs from sqlx are gone.
- 🎯 **Unified interfaces** — `Ext` / `ExtContext` / `NamedExt` / `BindExt` / `Preparer` / `PreparerContext` with compile-time checks. Write `func f(ext NamedExt)` and pass DB, Tx, or Conn.
- 🔀 **Auto IN expansion** — slices in `IN (?)` detected and expanded automatically on all methods.
- 🪝 **Hook system** — pluggable SQL interceptors for logging, tracing, metrics (onion model).
- 📦 **JsonValue[T]** — generic JSON column type with auto serialize/deserialize.
- 🛡️ **StrictMode** — lenient by default (matching sqlx `Unsafe()`), optionally strict for debugging.

→ [Migration Guide](#migration-from-jmoironsqlx)

## Table of Contents

- [Installation](#installation)
- [Migration from jmoiron/sqlx](#migration-from-jmoironsqlx)
- [Quick Start](#quick-start)
- [New Features](#new-features)
- [Usage Examples](#usage-examples)
  - [Basic CRUD](#basic-crud)
  - [Named Parameter Queries](#named-parameter-queries)
  - [IN Queries](#in-queries)
  - [Prepared Statements](#prepared-statements)
  - [Transaction Management](#transaction-management)
  - [JsonValue[T]](#jsonvaluet)
  - [Hook Aspects](#hook-aspects)
  - [StrictMode](#strictmode)
  - [NamedExt / BindExt Unified Interfaces](#unified-interfaces)
- [Comparison with jmoiron/sqlx](#comparison-with-jmoironsqlx)
- [Bug Fixes & Improvements](#bug-fixes--improvements)
- [Performance](#performance)
- [License](#license)

## Installation

```bash
go get github.com/go-sqlex/sqlex
```

Requires Go 1.21 or later.

## Migration from jmoiron/sqlx

**30 seconds, 3 steps:**

**1. Change import path:**

```go
// old
import "github.com/jmoiron/sqlx"

// new
import "github.com/go-sqlex/sqlex"
```

**2. Change package references:**

```go
// old
db, err := sqlx.Connect("postgres", dsn)

// new
db, err := sqlex.Connect("postgres", dsn)
```

**3. Update go.mod:**

```bash
go get github.com/go-sqlex/sqlex
```

**Done.** All your existing sqlx code works without changes.

> **Note on StrictMode**: sqlex defaults to lenient mode (`strict=false`), matching sqlx's `db.Unsafe()` behavior (silently ignore extra columns). You kept `db.Unsafe()` in your codebase? No changes needed — sqlex inherits the same lenient default. To enable strict struct-field matching for debugging, call `db.SetStrict(true)`.

### Gradual adoption

New features are optional — adopt at your own pace:

| Step | Action | Time |
|------|--------|------|
| 1 | Replace import path | 30s |
| 2 | Switch transactions to `CloseWithErr` pattern | per-use |
| 3 | Use `NamedGet`/`NamedSelect` instead of `NamedQuery` + manual scan | per-use |
| 4 | Register custom Hooks (logging, tracing, metrics) | as needed |

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

## New Features

sqlex preserves all sqlx APIs and adds the following capabilities:

| Feature | Description |
|---------|-------------|
| **Hook aspects** | `AddHook` — pluggable SQL execution interceptors (onion model) |
| **JsonValue[T]** | `types.JsonValue[T]` — generic JSON column type |
| **NamedGet/NamedSelect** | Named parameter convenience methods on DB/Tx (built-in IN expansion) |
| **CloseWithErr** | Auto Commit/Rollback based on error |
| **Unified interfaces** | `Ext` / `ExtContext` / `NamedExt` / `BindExt` / `Preparer` / `PreparerContext` — DB, Tx, and Conn share identical method signatures with compile-time checks |
| **Auto IN expansion** | All methods auto-detect slice args and expand IN clauses |
| **Auto Rebind** | All query methods auto-convert `?` to target database placeholders |
| **StrictMode** | Optional strict struct-field matching for debugging (off by default) |
| **Cross-database out of the box** | Write SQL with `?` everywhere — works on PostgreSQL, MySQL, SQLite, SQL Server |

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

// NamedQuery — return *Rows for manual iteration
rows, err := db.NamedQuery(`SELECT * FROM users WHERE name = :name`, params)
defer rows.Close()
for rows.Next() {
    var u User
    rows.StructScan(&u)
}
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

#### Slice argument handling (IN list context recognition)

sqlex uses **IN list context recognition** to decide whether to auto-expand slices: slices are only expanded when `?` is in the `IN (?)` context. Two conditions must be met:

1. **Strict `(?)` form**: only one `?` and optional ASCII whitespace (space/Tab/newline/CR) between `(` and `)`
2. **The complete identifier immediately before `(` is the `IN` keyword** (case-insensitive); `NOT IN (?)` also matches

Other `(?)` contexts (`ANY(?)` / `ALL(?)` / `VALUES (?)` / `func(?)` / scalar subquery `= (?)` etc.) are treated as single values — **no need for `AsValue` escape hatch**.

**Detection rules**:

| SQL pattern | Argument | Behavior | Notes |
|---|---|---|---|
| `WHERE id IN (?)` | `[]int{1,2,3}` | Expand | Preceded by IN |
| `WHERE id NOT IN (?)` | `[]int{1,2,3}` | Expand | NOT IN still matches |
| `WHERE id IN (\n  ?\n)` | `[]int{1,2,3}` | Expand | Multi-line IN (?) |
| `WHERE x = ANY(?)` | `[]int{1,2,3}` | No expand | Preceded by ANY, not IN |
| `INSERT ... VALUES (?)` | `[]int{1,2,3}` | No expand | Preceded by VALUES, not IN |
| `SELECT func(?)` | `[]int{1,2,3}` | No expand | Preceded by function name |
| `WHERE x = (?)` | `[]int{1,2,3}` | No expand | Preceded by `=`, not IN |
| `WHERE col_in (?)` | `[]int{1,2,3}` | No expand | Full token is `col_in`, not IN |
| `IN (?, ?, ?)` | `1, 2, 3` scalars | No expand | Multiple `?` → user already expanded |
| `WHERE x = ?` | `[]int{1,2,3}` | No expand | `?` not in `(?)` form |

**Escape hatch APIs**:

```go
import "github.com/go-sqlex/sqlex"

// ① sqlex.AsValue(v) — force no expansion (even in IN (?) context)
db.Select(&rows, `SELECT * FROM t WHERE id IN (?)`,
    sqlex.AsValue([]int{1, 2, 3})) // entire slice as single value to driver

// ② sqlex.AsList(slice) — force expansion (even outside IN (?) context)
db.Select(&rows, `SELECT * FROM t WHERE id = ANY(?)`,
    sqlex.AsList([]int{1, 2, 3})) // force expand to ?, ?, ?

// ③ Other native approaches still work
db.Exec(`INSERT INTO users (tags) VALUES (?)`, pq.Array([]int{1, 2, 3})) // driver.Valuer
data, _ := json.Marshal([]int{1, 2, 3})
db.Exec(`INSERT INTO t (json_col) VALUES (?)`, data) // []byte is a standard driver type
```

> Note: `ANY(?)` / `VALUES (?)` etc. now default to **no expansion** — just pass the slice directly or wrap with `pq.Array`. No `AsValue` needed.

**Priority** (high to low):

1. `sqlex.AsValue(v)` / `sqlex.AsList(s)` — explicit declaration, highest priority
2. `driver.Valuer` interface (including `pq.Array`) — treated as single value
3. `[]byte` — standard driver type, treated as single value
4. `IN (?)` context match + slice — auto-expand
5. Other positions + slice — no expansion, passed as single value (driver will likely error)

**Known edge case**: A comment between `IN` and `(` (e.g. `IN /* c */ (?)`) prevents IN recognition and won't expand. This pattern is extremely rare; use `sqlex.AsList` as a fallback if needed.

**Empty slice handling** (context-sensitive):

| Scenario | Behavior |
|---|---|
| `IN (?)` context + `[]int{}` | Error `sqlex: empty slice cannot be expanded into IN ()` (IN () is invalid SQL) |
| Non-IN context (`WHERE x = ?` / `VALUES (?)`) + `[]int{}` | OK, entire slice passed to driver |
| `sqlex.AsValue([]int{})` | OK (already single-value semantics) |
| `sqlex.AsList([]int{})` | Error `sqlex.AsList: empty slice` (expanding to nothing is meaningless) |

#### Named parameter name rules & lexical context

Named parameter `:name` rule: `[A-Za-z_][A-Za-z0-9_.]*` (letter/underscore start, digits/underscore/dot allowed; dots for nested fields like `:user.name`).

| Pattern | Recognized? | Notes |
|---|---|---|
| `:name` / `:user_id` / `:arg1` | ✅ | Standard named parameter |
| `:user.name` | ✅ | Dot-nested field |
| `:123` / `:1` | ❌ preserved as literal | Digit-start rejected (avoids Oracle `:N` / SQLite `?NNN` conflicts) |
| `:名字` (Unicode) | ❌ preserved as literal | ASCII-only param names (matches `db` tag / map key convention) |
| `::int` (PG type cast) | ❌ preserved as literal | `::` recognized as type cast, not parameter |
| `:=` (assignment) | ❌ preserved as literal | Output as-is |

**Lexical scanning**: `:name` / `?` inside these regions are skipped (shared `lexer.go` scanner):
- Single-quoted strings `'...'` (with `''` escapes), double-quoted identifiers `"..."`, backtick identifiers `` `...` ``
- Dollar-quoted strings `$$...$$` / `$tag$...$tag$`
- Line comments `-- ...`, block comments `/* ... */`

> If edge cases trigger a misparse, `compileNamedQuery` preserves unmatched `:name` as literals (same behavior as GORM's `@name` handling), allowing the original SQL to still execute correctly.

### Prepared Statements

```go
// Preparex auto-Rebinds — use ? uniformly
stmt, err := db.Preparex("SELECT * FROM users WHERE name = ?")
var user User
err = stmt.Get(&user, "Alice")

// PreparexContext — context-aware version
ctx := context.Background()
stmt, err = db.PreparexContext(ctx, "SELECT * FROM users WHERE name = ?")

// PrepareNamed — named prepared statement
nstmt, err := db.PrepareNamed("SELECT * FROM users WHERE name = :name")
err = nstmt.Get(&user, map[string]any{"name": "Alice"})

// PreparerContext — write generic prepare functions accepting DB/Tx/Conn
func prepareQuery(p sqlex.PreparerContext) (*sqlex.Stmt, error) {
    return sqlex.PreparexContext(context.Background(), p, "SELECT * FROM users WHERE name = ?")
}
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
if a.Metadata.Valid {
    fmt.Println(a.Metadata.Val.Tags) // ["go", "sql"]
}
// ValueOrZero returns zero value if !Valid
meta := a.Metadata.ValueOrZero()
// Marshal/Unmarshal (implements json.Marshaler/Unmarshaler)
data, _ := json.Marshal(a.Metadata)
json.Unmarshal(data, &a.Metadata)
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

### Unified Interfaces

DB, Tx, and Conn implement a common set of interfaces (enforced by compile-time assertions):

| Interface | Methods | Purpose |
|-----------|---------|---------|
| `Ext` | `Exec`, `Queryx`, `QueryRowx` | Basic query/execution |
| `ExtContext` | `ExecContext`, `QueryxContext`, `QueryRowxContext` | Context-aware variants |
| `NamedExt` | `NamedExec`, `NamedQuery`, `NamedGet`, `NamedSelect` | Named parameter queries |
| `BindExt` | `BindNamed`, `Get`, `Select`, `Rebind`, `DriverName` | Positional parameter queries |
| `Preparer` | `Preparex`, `PrepareNamed` | Prepared statement creation |
| `PreparerContext` | `PreparexContext`, `PrepareNamedContext` | Context-aware preparation |

```go
// Accept DB, Tx, or Conn via NamedExt
func getUserByName(ext sqlex.NamedExt, name string) (*User, error) {
    var user User
    err := ext.NamedGet(&user, `SELECT * FROM users WHERE name = :name`,
        map[string]any{"name": name})
    return &user, err
}

user, err := getUserByName(db, "Alice")
tx, _ := db.Beginx()
user, err = getUserByName(tx, "Bob")
conn, _ := db.Connx(ctx)
user, err = getUserByName(conn, "Charlie")
```

## Comparison with jmoiron/sqlx

| Feature | jmoiron/sqlx | sqlex |
|---------|-------------|-------|
| Go version | 1.10+ | 1.21+ |
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
| Unified interfaces | ❌ DB/Tx methods overlap but no shared interface | ✅ `Ext` / `ExtContext` / `NamedExt` / `BindExt` / `Preparer` / `PreparerContext` — DB/Tx/Conn unified with compile-time checks |
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
