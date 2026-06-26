## v1.5.3

- **Fix nil `driver.Valuer` panic in `In`** (sqlx #952): nil pointer Valuers (e.g. `(*T)(nil)` with value-receiver `Value()`) no longer panic; they are treated as NULL, mirroring `database/sql.callValuerValue`.
- **Fix nil pointer slice panic in `asSliceForIn`**: nil pointer to a slice type (e.g. `*[]int(nil)`) no longer panics; it is treated as "not a slice" (no expansion). Non-nil pointer slices are now correctly dereferenced.

## v1.5.2

- **Hook unification**: `autoIn → Rebind → Hook` is now a unified pipeline. Begin/Commit/Rollback fire Hooks.
- **`NewDb` → `NewDB`** (rename).
- **`QueryEvent.OperationType` is now `OpType`** enum: `OpQuery`, `OpExec`, `OpBegin`, `OpCommit`, `OpRollback`.
- **`QueryEvent.RowsAffected`** and **`LastInsertID`** now populated on exec.
- **`Tx.CloseWithErr`** now fires Hooks on both success and failure (was only failure).

## v1.5.1

- **Slice auto-expansion scoped to IN only**: `ANY(?)`, `VALUES(?)`, `func(?)` no longer incorrectly expand slices — only `IN (?)` / `NOT IN (?)` trigger it.
