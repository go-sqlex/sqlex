## v1.5.2

- **Hook unification**: `autoIn → Rebind → Hook` is now a unified pipeline. Begin/Commit/Rollback fire Hooks.
- **`NewDb` → `NewDB`** (rename).
- **`QueryEvent.OperationType` is now `OpType`** enum: `OpQuery`, `OpExec`, `OpBegin`, `OpCommit`, `OpRollback`.
- **`QueryEvent.RowsAffected`** and **`LastInsertID`** now populated on exec.
- **`Tx.CloseWithErr`** now fires Hooks on both success and failure (was only failure).

## v1.5.1

- **Slice auto-expansion scoped to IN only**: `ANY(?)`, `VALUES(?)`, `func(?)` no longer incorrectly expand slices — only `IN (?)` / `NOT IN (?)` trigger it.
