package sqlex

import (
	"context"
	"time"
)

// OpType represents the type of SQL operation.
type OpType string

const (
	OpQuery    OpType = "query"
	OpExec     OpType = "exec"
	OpBegin    OpType = "begin"
	OpCommit   OpType = "commit"
	OpRollback OpType = "rollback"
)

// QueryEvent describes the context of a SQL execution event.
type QueryEvent struct {
	// Query is the SQL statement.
	Query string
	// Args are the execution parameters.
	Args []any
	// StartTime is the start time (includes Hook chain execution time).
	StartTime time.Time
	// Duration is the total elapsed time from the start of the BeforeQuery chain
	// to the end of the AfterQuery chain. Includes Hook chain execution overhead,
	// suitable for distributed tracing.
	Duration time.Duration
	// Error is the execution error (only set in the AfterQuery phase).
	Error error
	// OperationType is the operation type.
	OperationType OpType
	// RowsAffected is the number of rows affected, only set for exec operations.
	RowsAffected int64
	// LastInsertID is the auto-increment ID of the last inserted row, only set for exec operations.
	LastInsertID int64
}

// Hook is the SQL execution aspect interface.
// BeforeQuery is called before SQL execution and can modify the context.
// AfterQuery is called after SQL execution.
// Multiple Hooks execute BeforeQuery in registration order and AfterQuery in
// reverse order (onion model).
//
// Note: For QueryRowx/QueryRowxContext methods, the Hook fires immediately after
// QueryContext returns, before the row data is scanned. Therefore, QueryEvent.Error
// only reflects errors from the query dispatch phase and does not include subsequent
// row.Scan() errors (e.g., type mismatch, sql.ErrNoRows). For full lifecycle
// observability, use the QueryxContext + rows.Next() + StructScan pattern.
type Hook interface {
	BeforeQuery(ctx context.Context, event *QueryEvent) context.Context
	AfterQuery(ctx context.Context, event *QueryEvent)
}

// pipeline encapsulates the query preparation pipeline: autoIn expansion +
// Rebind cross-database conversion + Hook triggering.
// DB, Tx, and Conn share this behavior by embedding pipeline.
type pipeline struct {
	driverName string
	hooks      []Hook
}

// prepare runs the autoIn → Rebind → executeHooks pipeline on the query,
// returning the prepared context, afterFunc, and event.
func (p *pipeline) prepare(ctx context.Context, query string, args []any, op OpType) (context.Context, func(), *QueryEvent, error) {
	var err error
	if query, args, err = autoIn(query, args...); err != nil {
		return ctx, nil, nil, err
	}
	query = Rebind(BindType(p.driverName), query)
	event := &QueryEvent{Query: query, Args: args, OperationType: op}
	ctx, afterFunc := executeHooks(ctx, p.hooks, event)
	return ctx, afterFunc, event, nil
}

// executeHooks calls the Hook chain in the DB's core execution path.
// Returns the context modified by BeforeQuery and an afterFunc that should be
// called after SQL execution completes.
func executeHooks(ctx context.Context, hooks []Hook, event *QueryEvent) (context.Context, func()) {
	if len(hooks) == 0 {
		return ctx, func() {}
	}

	event.StartTime = time.Now()

	// Execute BeforeQuery in order
	for _, h := range hooks {
		ctx = h.BeforeQuery(ctx, event)
	}

	return ctx, func() {
		event.Duration = time.Since(event.StartTime)
		// Execute AfterQuery in reverse order (onion model)
		for i := len(hooks) - 1; i >= 0; i-- {
			hooks[i].AfterQuery(ctx, event)
		}
	}
}
