---
name: sqlex
description: sqlex is a modern enhancement wrapper for Go database/sql based on jmoiron/sqlx,
  providing struct scanning, named parameter queries, Hook aspects, generic JSONValue[T]
  JSON column type, automatic IN expansion, cross-database unified `?` placeholder, and more.
  Use this skill whenever the user is involved in any of the following scenarios: sqlex library
  usage, development, debugging, migration, or bug fixes; Go database operations
  (Get/Select/Exec/Queryx), SQL query result mapping to structs (StructScan/MapScan/SliceScan),
  NamedGet/NamedSelect/NamedExec named parameter queries, IN clause expansion,
  database Hook/interceptor/middleware/SQL aspects, JSONValue JSON column type,
  transaction management CloseWithErr/ExecFunc, NamedExt/BindExt unified interfaces,
  Rebind placeholder conversion, migration from jmoiron/sqlx,
  PostgreSQL/MySQL/SQLite/SQL Server cross-database compatibility, Preparex prepared statements,
  PrepareNamed named prepared statements, reflectx reflection mapping.
  This skill should be consulted even when the user vaguely mentions concepts like
  "Go SQL wrapper", "sqlx enhancement", "database struct mapping", "database/sql extension",
  "database query wrapper", "SQL parameter binding", or "database middleware".
---

[**English**](SKILL.md) | [中文](SKILL_zh.md)

# sqlex — AI Assistant Quick Reference

> This document is a decision reference for AI programming assistants, focused on "what API to use, how to use it, and what pitfalls to avoid".
> For full documentation (installation, testing, migration guide, etc.), see [README.md](README.md).

## 1. Core Concepts

**Positioning**: Go `database/sql` enhancement wrapper (not an ORM), upgraded from jmoiron/sqlx.
**Module path**: `github.com/go-sqlex/sqlex`
**Go version**: 1.21+ | **Databases**: PostgreSQL, MySQL, SQLite, Oracle, SQL Server

**Key Design**:
- Unified `?` placeholder; framework auto-Rebinds to `$N`/`@pN`/`:argN`
- DB / Tx / Conn interfaces aligned (`BindExt` + `NamedExt`); function signatures accept any
- Hook onion model; zero overhead when not registered
- StrictMode defaults to lenient (consistent with sqlx `Unsafe()`); can enable strict checking

## 2. API Decision Tree

```
Need to query?
├─ Single row → db.Get(&dest, query, args...)       or db.NamedGet(&dest, query, arg)
├─ Multiple rows → db.Select(&dest, query, args...)  or db.NamedSelect(&dest, query, arg)
└─ Raw rows → db.Queryx / db.QueryRowx              or db.NamedQuery

Need to execute?
├─ Normal → db.Exec / db.MustExec                    or db.NamedExec
└─ In transaction → tx.Exec / tx.CloseWithErr(err)   or tx.NamedExec

Need prepared statements?
├─ Positional → db.Preparex / db.PreparexContext
└─ Named → db.PrepareNamed / db.PrepareNamedContext

Need single connection? → db.Connx(ctx) → conn (aligned with DB/Tx interface)

Need JSON column? → types.JSONValue[T] (generic, replaces JSONText)
```

## 3. Usage Essentials

### 3.1 Connection

```go
db, err := sqlex.Connect("postgres", dsn)   // with Ping
db, err := sqlex.Open("mysql", dsn)          // no Ping
db := sqlex.MustConnect("sqlite3", ":memory:") // panics on failure
```

### 3.2 Queries (unified `?` placeholder)

```go
// Positional args — framework auto-Rebinds; MySQL/PG/SQLite/SQL Server unified syntax
db.Get(&user, "SELECT * FROM users WHERE id = ?", 1)
db.Select(&users, "SELECT * FROM users WHERE age > ?", 18)

// Named args — supports struct or map[string]any
db.NamedGet(&user, `SELECT * FROM users WHERE name = :name`, map[string]any{"name": "Alice"})
db.NamedSelect(&users, `SELECT * FROM users WHERE age > :min_age`, map[string]any{"min_age": 18})
db.NamedExec(`INSERT INTO users (name, email) VALUES (:name, :email)`, User{Name: "Alice", Email: "a@b.c"})
```

### 3.3 IN Queries (auto-expansion)

```go
// Positional args: auto-detects slice + IN list context recognition
db.Select(&users, "SELECT * FROM users WHERE id IN (?)", []int{1, 2, 3})

// Named args: built-in IN expansion
db.NamedSelect(&users, `SELECT * FROM users WHERE id IN (:ids)`, map[string]any{"ids": []int{1, 2, 3}})
```

**IN list context recognition**: slice auto-expansion requires both ① strict `(?)` form (only `?` + optional whitespace between `(` and `)`) and ② the complete identifier before `(` is `IN` (case-insensitive, including `NOT IN`). Other `(?)` contexts are treated as single values — **no `AsValue` needed**.

| SQL pattern | Slice arg | Behavior |
|---|---|---|
| `IN (?)` / `NOT IN (?)` | slice | Expand |
| `IN (?, ?, ?)` | scalars | No expand |
| `WHERE x = ?` | slice | No expand (single value) |
| `ANY(?)` / `ALL(?)` / `VALUES (?)` / `func(?)` | slice | No expand (correct behavior) |
| `col_in (?)` / `t.in (?)` | slice | No expand (full token comparison) |

**Escape hatches**: `sqlex.AsValue(v)` force no expand (even in IN context) | `sqlex.AsList(slice)` force expand (even outside IN context, e.g. `ANY(?)`)

**Known edge case**: `IN /* comment */ (?)` prevents IN recognition; use `AsList` as fallback.

### 3.4 Transaction Management

```go
tx, err := db.Beginx()
if err != nil {
    return err
}
defer func() { tx.CloseWithErr(err) }()  // err==nil → Commit, err!=nil → Rollback

_, err = tx.NamedExec(`INSERT INTO users (name) VALUES (:name)`, User{Name: "Bob"})
if err != nil {
    return err  // auto-Rollback in defer
}
return nil      // auto-Commit in defer
```

### 3.5 Conn (single connection)

```go
conn, err := db.Connx(ctx)
defer conn.Close()
// Conn fully aligned with DB/Tx interface
conn.Get(&user, "SELECT * FROM users WHERE id = ?", 1)
conn.NamedGet(&user, `SELECT * FROM users WHERE name = :name`, map[string]any{"name": "Alice"})
```

### 3.6 JSONValue[T]

```go
import "github.com/go-sqlex/sqlex/types"

type Config struct {
    ID       int                       `db:"id"`
    Settings types.JSONValue[Settings] `db:"settings"`
}
cfg := Config{Settings: types.NewJSONValue(Settings{Theme: "dark", FontSize: 14})}
if cfg.Settings.Valid {
    theme := cfg.Settings.Val.Theme // "dark"
}
// Val is zero value when !Valid
```

### 3.7 Hook Aspects

```go
type MetricsHook struct{}
func (h *MetricsHook) BeforeQuery(ctx context.Context, event *sqlex.QueryEvent) context.Context {
    return ctx
}
func (h *MetricsHook) AfterQuery(ctx context.Context, event *sqlex.QueryEvent) {
    recordMetric(event.Query, event.Duration, event.Error)
}
db.AddHook(&MetricsHook{})
// Hooks also apply to Tx/Conn (auto-inherited)
// Multiple Hooks: BeforeQuery forward order, AfterQuery reverse order (onion model)
```

**Conditional filtering**: sqlex does not ship a built-in filter; use decorators:
```go
// Only fire on slow queries
db.AddHook(SlowOnly(&AlertHook{}, 500*time.Millisecond))
// SlowOnly / OnError etc. are trivial to implement yourself
```

## 4. Best Practices and Common Pitfalls

### ✅ Recommended

1. **Use unified `?` placeholder** — All query methods auto-Rebind
2. **Use `CloseWithErr` for transactions** — `defer func() { tx.CloseWithErr(err) }()`
3. **Use Context methods in production** — `GetContext`/`SelectContext` for timeout control
4. **NamedSelect + IN** — No need to manually call `In()`
5. **ANY(?)/VALUES(?) auto-safe** — No longer expand by default; no `AsValue` needed
6. **Register Hooks at initialization** — Tx/Conn auto-inherit DB's Hooks
7. **PostgreSQL JSONB `?` operator** — Use `??` escape
8. **StrictMode optional** — Default lenient; enable `db.SetStrict(true)` for development

### ⚠️ Common Pitfalls

1. **Preparex already auto-Rebinds** — No need to manually distinguish database types
2. **NameMapper only set in `init()`** — Use `DB.MapperFunc()` at runtime instead
3. **Tx is not concurrency-safe** — Use `Tx.ExecFunc()` for concurrency protection
4. **Hooks execute synchronously** — Heavy operations should be async inside Hooks
5. **Named parameter names limited to ASCII** — `[A-Za-z_][A-Za-z0-9_.]*`; digit-starting `:123` not recognized
6. **StrictMode defaults to lenient** — Consistent with sqlx `db.Unsafe()` behavior

## 5. Differences from jmoiron/sqlx

### New Capabilities

| Feature | Description |
|---------|-------------|
| Hook aspects | `AddHook` pluggable SQL interceptors (onion model) |
| JSONValue[T] | Generic JSON column type |
| NamedGet/NamedSelect | Convenient named parameter queries (built-in IN expansion) |
| CloseWithErr | Auto Commit/Rollback based on error |
| ExecFunc | Tx mutex-protected function execution |
| NamedExt/BindExt | DB/Tx unified programming interface |
| Select/Get auto-IN | Detects slice args + IN list context recognition (only `IN (?)` expands) |
| StrictMode | Default lenient, can enable strict checking |
| Auto-Rebind | All query methods auto-convert `?` |
| Conn enhancement | Fully aligned with DB/Tx interface |
| SQL Server bracket identifiers | `scanBracketIdentifier` supports `[col?name]` |

### Bug Fixes

- **Unified lexical scanner**: Rebind/In/compileNamedQuery reuse `scanSkipSegment`, eliminating drift
- **Rebind full coverage**: Skips `?` inside strings/comments/PG double quotes/MySQL backticks/SQL Server brackets/PG dollar quoting
- **Named query symmetric fixes**: Skips colons inside strings/comments/double quotes/backticks/brackets/dollar quoting
- **Parameter name rules tightened**: `[A-Za-z_][A-Za-z0-9_.]*`; digit-starting not misidentified
- **Missing params preserved as literals**: Fallback for named parser misjudgment (not business fault-tolerance)
- **ConnectContext leak**, **NamedStmt.Exec return value**, **Named query Rebind missing** all fixed

## 6. API Quick Reference

### Top-level Functions

| Function | Description |
|----------|-------------|
| `Connect/ConnectContext/MustConnect` | Connect to database (with Ping) |
| `Open/MustOpen` | Open connection (no Ping) |
| `Select/SelectContext` | Query multiple rows (accepts Queryer) |
| `Get/GetContext` | Query single row (accepts Queryer) |
| `In` | Expand IN slice args |
| `AsValue` | Force no expansion (even in IN(?) context) |
| `AsList` | Force expansion (even outside IN(?) context) |
| `Named` | Named parameter binding |
| `Rebind` | Convert bind variable format |

### DB-Only Methods

`Beginx/BeginTxx`, `Connx`, `AddHook`, `MapperFunc`, `Preparex/PreparexContext`, `PrepareNamed/PrepareNamedContext`, `SetStrict/IsStrict`

### Tx-Only Methods

`CloseWithErr(err)`, `ExecFunc(fn)`, `Stmtx/StmtxContext`, `TryStmtx/TryStmtxContext`, `SetStrict/IsStrict`

### DB and Tx Shared Methods

`Get/GetContext`, `Select/SelectContext`, `Queryx/QueryxContext`, `QueryRowx/QueryRowxContext`, `Exec/ExecContext`, `MustExec/MustExecContext`, `NamedGet/NamedGetContext`, `NamedSelect/NamedSelectContext`, `NamedExec/NamedExecContext`, `NamedQuery/NamedQueryContext`, `Rebind`, `BindNamed`

### Conn Methods (aligned with DB/Tx)

**Context**: `GetContext`, `SelectContext`, `QueryxContext`, `QueryRowxContext`, `ExecContext`, `MustExecContext`, `NamedGetContext`, `NamedSelectContext`, `NamedExecContext`, `NamedQueryContext`

**Non-Context** (delegate `context.Background()`): `Get`, `Select`, `Queryx`, `QueryRowx`, `Exec`, `MustExec`, `NamedGet`, `NamedSelect`, `NamedExec`, `NamedQuery`

**Utility**: `Rebind`, `BindNamed`, `DriverName`, `SetStrict/IsStrict`, `BeginTxx`, `PreparexContext`, `PrepareNamedContext`

### types Sub-package

| Type | Description |
|------|-------------|
| `JSONValue[T]` | Generic JSON column (Scan/Value + MarshalJSON/UnmarshalJSON; Val/Valid direct access) |
| `JSONText` | json.RawMessage wrapper, supports Scan/Value |
| `NullJSONText` | Nullable JSONText |
| `GzippedText` | Auto gzip compress/decompress []byte |
| `BitBool` | MySQL BIT(1) boolean type |

### Hook Related

| Type | Description |
|------|-------------|
| `Hook` interface | `BeforeQuery(ctx, *QueryEvent) ctx` + `AfterQuery(ctx, *QueryEvent)` |
| `QueryEvent` | Contains Query, Args, StartTime, Duration, Error, OperationType, RowsAffected, LastInsertID |
| `OpType` | Operation type enum: OpQuery/OpExec/OpBegin/OpCommit/OpRollback |
