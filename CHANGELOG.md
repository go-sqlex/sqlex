## v1.5.5

> Focus: fixes from the code review in [#7](https://github.com/go-sqlex/sqlex/issues/7).

### ✨ Features

- **`Tx.CloseWithErrContext(ctx, err)`**: like `CloseWithErr`, but passes `ctx` to the commit/rollback Hooks, so tracing spans and call-scoped Hooks (`WithCallHooks`) observe the end of the transaction. `CloseWithErr` uses `context.Background()`.

### ⚠️ Behaviour Changes

- **`Tx.CloseWithErr` now returns an `error`**: a failed Commit was previously only reported via Hook, so callers could not tell the transaction was lost. It now returns the Commit error when `err` is nil, otherwise `err` itself (joined with the Rollback error if the rollback fails). Assign it back in the defer: `defer func() { err = tx.CloseWithErr(err) }()`. Existing calls that ignore the result still compile; code that stores the method as a `func(error)` or declares an interface with `CloseWithErr(error)` must be updated.

### 🐛 Bug Fixes

- **Named binding panics on nil arguments**: `NamedExec`/`NamedQuery`/`Named`/`NamedStmt` and friends now return an error instead of panicking when given `nil`, a nil struct pointer, or a batch containing nil elements.
- **`Stmt` ignored rewritten args**: `Stmt.ExecContext`/`QueryContext` now execute with `QueryEvent.Args` as rewritten by `BeforeQuery`, consistent with `DB`/`Tx`/`Conn`. The SQL of a prepared statement remains fixed.
- **`types.BitBool.Scan` panics on empty input**: an empty `[]byte` now returns an error.
- **`types.BitBool.Scan` misreads PostgreSQL `BIT(1)`**: PostgreSQL returns the text `"1"`, which was silently scanned as false; it is now read as true.
- **`types.GzippedText.Scan` rejects SQL NULL**: scanning NULL now sets the value to nil instead of returning an error (behaviour change from jmoiron/sqlx).
- **Batch binding picks a nested VALUES**: `fixBound` now prefers a top-level `VALUES`, so a `VALUES` inside a CTE no longer shadows the main INSERT's. `UPDATE ... FROM (VALUES ...)` is still supported.
- **Flaky Hook duration tests on Windows** ([#8](https://github.com/go-sqlex/sqlex/issues/8)): a zero `QueryEvent.Duration` is accepted on low-resolution timers.

## v1.5.4

> Focus: per-call Hook injection — attach behaviour to a single call without touching method signatures.

### ✨ Features

- **`WithCallHooks(ctx, hooks...)`**: register Hooks on a `context.Context` so they apply only to operations receiving that context, complementing the global Hooks from `WithHooks`/`AddHook`. Enables dynamic schema selection, tenant routing and ad-hoc SQL rewriting on a shared `*DB` without mutating it and without any API change. All `*Context` methods (`SelectContext`, `GetContext`, `ExecContext`, `QueryxContext`, `QueryRowxContext`, the `Named*Context` family, `BeginTxx`, and the `Stmt` equivalents) observe them.
  - Global Hooks wrap call-scoped ones: `global Before → call Before → SQL → call After → global After`.
  - Successive calls replace rather than accumulate, matching `context.WithValue` shadowing; pass multiple Hooks in one call to compose them.
  - `Tx` from `BeginTxx` does not capture them — pass the same context to the `Tx` methods.
  - `Stmt` merges them at execution time, so a prepared statement is never permanently bound to a call's Hooks.
  - Zero extra allocation when no call-scoped Hooks are present.

## v1.5.3

> Focus: fix 5 critical bugs inherited from jmoiron/sqlx — data corruption, panics, and silent data loss.

### 🐛 Bug Fixes

- **`Select` + `sql.RawBytes` data corruption** (sqlx [#931](https://github.com/jmoiron/sqlx/issues/931)): `Select`/`ScanAll` now rejects `sql.RawBytes` destinations with a clear error, preventing silent data corruption caused by driver buffer reuse across rows.
- **`In` panic on nil pointers** (sqlx [#952](https://github.com/jmoiron/sqlx/issues/952)): nil `driver.Valuer` and nil pointer slices no longer panic. Valuer nil pointers treated as NULL (mirrors `database/sql.callValuerValue`); pointer slices treated as empty slice (rejected in `IN (?)` context, passed as-is elsewhere). Non-nil pointer slices now correctly dereferenced.
- **`fixBound` VALUES expansion drops rows** (sqlx [#898](https://github.com/jmoiron/sqlx/issues/898)/[#694](https://github.com/jmoiron/sqlx/issues/694)/[#772](https://github.com/jmoiron/sqlx/issues/772)): batch INSERT/UPDATE with `VALUES (...)` no longer silently drops rows when VALUES is not preceded by `)`. Now supports `INSERT INTO t VALUES (:a, :b)` (no column list) and PG `UPDATE ... FROM (VALUES (:a, :b))`. Uses lexer-based keyword search instead of regex, correctly skipping string literals and comments.
- **`Rows.NextResultSet` cache stale** (sqlx [#857](https://github.com/jmoiron/sqlx/issues/857)): `NextResultSet()` now resets the StructScan cache (`started`/`fields`/`values`), preventing stale field mappings from corrupting scans of subsequent result sets with different column structures.

### ♻️ Refactoring

- **Remove unused `reflectx` public APIs** (#614): deleted `FieldMap`/`FieldByName`/`FieldsByName` (sqlex internal never calls them). Fixed `FieldByIndexesReadOnly` to handle nil pointers without panicking. Tests rewritten to use `TraversalsByName` + `FieldByIndexesReadOnly`, preserving full coverage.

## v1.5.2

- **Hook unification**: `autoIn → Rebind → Hook` is now a unified pipeline. Begin/Commit/Rollback fire Hooks.
- **`NewDb` → `NewDB`** (rename).
- **`QueryEvent.OperationType` is now `OpType`** enum: `OpQuery`, `OpExec`, `OpBegin`, `OpCommit`, `OpRollback`.
- **`QueryEvent.RowsAffected`** and **`LastInsertID`** now populated on exec.
- **`Tx.CloseWithErr`** now fires Hooks on both success and failure (was only failure).

## v1.5.1

- **Slice auto-expansion scoped to IN only**: `ANY(?)`, `VALUES(?)`, `func(?)` no longer incorrectly expand slices — only `IN (?)` / `NOT IN (?)` trigger it.
