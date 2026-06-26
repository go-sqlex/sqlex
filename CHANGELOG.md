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
