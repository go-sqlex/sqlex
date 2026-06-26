## v1.5.3

- **Fix `Select` + `sql.RawBytes` data corruption** (sqlx #931): `Select`/`ScanAll` now rejects `sql.RawBytes` destinations with a clear error, preventing silent data corruption caused by driver buffer reuse across rows.
- **Fix `Rows.NextResultSet` cache invalidation** (sqlx #857): `NextResultSet()` now resets the StructScan cache (`started`/`fields`/`values`), preventing stale field mappings from corrupting scans of subsequent result sets with different column structures.
- **Fix `fixBound` VALUES expansion** (sqlx #898/#694/#772): batch INSERT/UPDATE with `VALUES (...)` no longer silently drops rows when VALUES is not preceded by `)`. Now supports `INSERT INTO t VALUES (:a, :b)` (no column list) and PG `UPDATE ... FROM (VALUES (:a, :b))`. Uses lexer-based keyword search instead of regex, correctly skipping string literals and comments.
- **Fix nil `driver.Valuer` panic in `In`** (sqlx #952): nil pointer Valuers (e.g. `(*T)(nil)` with value-receiver `Value()`) no longer panic; they are treated as NULL, mirroring `database/sql.callValuerValue`.
- **Fix nil pointer slice panic in `asSliceForIn`**: nil pointer to a slice type (e.g. `*[]int(nil)`) no longer panics; it is treated as empty slice (rejected in IN(?) context, passed as-is elsewhere). Non-nil pointer slices are now correctly dereferenced.

## v1.5.2

- **Hook unification**: `autoIn → Rebind → Hook` is now a unified pipeline. Begin/Commit/Rollback fire Hooks.
- **`NewDb` → `NewDB`** (rename).
- **`QueryEvent.OperationType` is now `OpType`** enum: `OpQuery`, `OpExec`, `OpBegin`, `OpCommit`, `OpRollback`.
- **`QueryEvent.RowsAffected`** and **`LastInsertID`** now populated on exec.
- **`Tx.CloseWithErr`** now fires Hooks on both success and failure (was only failure).

## v1.5.1

- **Slice auto-expansion scoped to IN only**: `ANY(?)`, `VALUES(?)`, `func(?)` no longer incorrectly expand slices — only `IN (?)` / `NOT IN (?)` trigger it.
