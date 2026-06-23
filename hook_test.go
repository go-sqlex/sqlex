package sqlex

import (
	"context"
	"testing"
)

// onionTestHook is a Hook implementation for testing the onion model, recording the call order of BeforeQuery and AfterQuery.
type onionTestHook struct {
	name   string
	before *[]string
	after  *[]string
}

func (h *onionTestHook) BeforeQuery(ctx context.Context, event *QueryEvent) context.Context {
	*h.before = append(*h.before, h.name)
	return ctx
}

func (h *onionTestHook) AfterQuery(ctx context.Context, event *QueryEvent) {
	*h.after = append(*h.after, h.name)
}

// TestHookOnionModel verifies the onion model execution order of multiple Hooks:
// BeforeQuery executes in forward order, AfterQuery executes in reverse order.
func TestHookOnionModel(t *testing.T) {
	var beforeCalls []string
	var afterCalls []string

	hooks := []Hook{
		&onionTestHook{name: "A", before: &beforeCalls, after: &afterCalls},
		&onionTestHook{name: "B", before: &beforeCalls, after: &afterCalls},
		&onionTestHook{name: "C", before: &beforeCalls, after: &afterCalls},
	}

	event := &QueryEvent{Query: "SELECT 1", OperationType: OpQuery}
	ctx := context.Background()

	ctx, afterFunc := executeHooks(ctx, hooks, event)
	_ = ctx

	// Verify BeforeQuery executes in forward order
	if len(beforeCalls) != 3 {
		t.Fatalf("expected 3 before calls, got %d", len(beforeCalls))
	}
	expectedBefore := []string{"A", "B", "C"}
	for i, name := range expectedBefore {
		if beforeCalls[i] != name {
			t.Errorf("before[%d]: expected %s, got %s", i, name, beforeCalls[i])
		}
	}

	// Call afterFunc to trigger AfterQuery
	afterFunc()

	// Verify AfterQuery executes in reverse order
	if len(afterCalls) != 3 {
		t.Fatalf("expected 3 after calls, got %d", len(afterCalls))
	}
	expectedAfter := []string{"C", "B", "A"}
	for i, name := range expectedAfter {
		if afterCalls[i] != name {
			t.Errorf("after[%d]: expected %s, got %s", i, name, afterCalls[i])
		}
	}
}

// TestHookZeroHooks verifies that zero Hooks do not cause a panic.
func TestHookZeroHooks(t *testing.T) {
	event := &QueryEvent{Query: "SELECT 1", OperationType: OpQuery}
	ctx := context.Background()

	ctx, afterFunc := executeHooks(ctx, nil, event)
	_ = ctx
	afterFunc() // should not panic
}

// TestHookSingleHook verifies normal execution of a single Hook.
func TestHookSingleHook(t *testing.T) {
	var beforeCalls []string
	var afterCalls []string

	hooks := []Hook{
		&onionTestHook{name: "only", before: &beforeCalls, after: &afterCalls},
	}

	event := &QueryEvent{Query: "SELECT 1", OperationType: OpQuery}
	ctx := context.Background()
	ctx, afterFunc := executeHooks(ctx, hooks, event)
	_ = ctx
	afterFunc()

	if len(beforeCalls) != 1 || beforeCalls[0] != "only" {
		t.Errorf("expected [only], got %v", beforeCalls)
	}
	if len(afterCalls) != 1 || afterCalls[0] != "only" {
		t.Errorf("expected [only], got %v", afterCalls)
	}
}

// TestCompileNamedQuery_MissingTolerance tests the tolerance mode: named parameters
// in args that do not exist are preserved as :name literals during compileNamedQueryWith compilation;
// existing parameters are numbered normally. Also verifies that ?/$N/@pN inside
// string literals/comments/dollar-quote are not mistakenly recognized.
func TestCompileNamedQuery_MissingTolerance(t *testing.T) {
	// existsSet constructs a nameExists closure: names listed in present are considered existing
	existsSet := func(present ...string) func(string) bool {
		s := make(map[string]bool, len(present))
		for _, n := range present {
			s[n] = true
		}
		return func(name string) bool { return s[name] }
	}

	tests := []struct {
		name          string
		query         string
		bindType      int
		present       []string // parameter names considered existing (others considered missing)
		expectedQuery string
		expectedNames []string
	}{
		// --- QUESTION type ---
		{
			name:          "QUESTION_none_missing",
			query:         "SELECT * FROM t WHERE a = :a AND b = :b",
			bindType:      QUESTION,
			present:       []string{"a", "b"},
			expectedQuery: "SELECT * FROM t WHERE a = ? AND b = ?",
			expectedNames: []string{"a", "b"},
		},
		{
			name:          "QUESTION_partially_missing_middle",
			query:         "SELECT * FROM t WHERE a = :a AND b = :b AND c = :c",
			bindType:      QUESTION,
			present:       []string{"a", "c"},
			expectedQuery: "SELECT * FROM t WHERE a = ? AND b = :b AND c = ?",
			expectedNames: []string{"a", "c"},
		},
		{
			name:          "QUESTION_all_missing",
			query:         "SELECT * FROM t WHERE a = :a AND b = :b",
			bindType:      QUESTION,
			present:       nil,
			expectedQuery: "SELECT * FROM t WHERE a = :a AND b = :b",
			expectedNames: []string{},
		},

		// --- DOLLAR type: missing items do not participate in $N numbering ---
		{
			name:          "DOLLAR_none_missing",
			query:         "SELECT * FROM t WHERE a = :a AND b = :b",
			bindType:      DOLLAR,
			present:       []string{"a", "b"},
			expectedQuery: "SELECT * FROM t WHERE a = $1 AND b = $2",
			expectedNames: []string{"a", "b"},
		},
		{
			name:          "DOLLAR_first_missing_remaining_start_from_$1",
			query:         "SELECT * FROM t WHERE a = :a AND b = :b AND c = :c",
			bindType:      DOLLAR,
			present:       []string{"b", "c"},
			expectedQuery: "SELECT * FROM t WHERE a = :a AND b = $1 AND c = $2",
			expectedNames: []string{"b", "c"},
		},
		{
			name:          "DOLLAR_middle_missing_remaining_consecutive_numbering",
			query:         "SELECT * FROM t WHERE a = :a AND b = :b AND c = :c",
			bindType:      DOLLAR,
			present:       []string{"a", "c"},
			expectedQuery: "SELECT * FROM t WHERE a = $1 AND b = :b AND c = $2",
			expectedNames: []string{"a", "c"},
		},
		{
			name:          "DOLLAR_all_missing",
			query:         "SELECT * FROM t WHERE a = :a AND b = :b",
			bindType:      DOLLAR,
			present:       nil,
			expectedQuery: "SELECT * FROM t WHERE a = :a AND b = :b",
			expectedNames: []string{},
		},

		// --- AT type ---
		{
			name:          "AT_partially_missing",
			query:         "SELECT * FROM t WHERE a = :a AND b = :b AND c = :c",
			bindType:      AT,
			present:       []string{"a", "c"},
			expectedQuery: "SELECT * FROM t WHERE a = @p1 AND b = :b AND c = @p2",
			expectedNames: []string{"a", "c"},
		},

		// --- Lexical element skip (P0 regression): :name inside string literals / comments / dollar-quoting
		// is not mistakenly recognized — this is a natural property of compileNamedQueryWith
		// reusing scanSkipSegment. The tolerance path shares the same lexical logic as the normal path. ---
		{
			name:          "colon_fake_inside_string_literal_not_recognized_only_compile_trailing_missing",
			query:         "SELECT 'a:fake' AS note FROM t WHERE x = :missing",
			bindType:      QUESTION,
			present:       nil,
			expectedQuery: "SELECT 'a:fake' AS note FROM t WHERE x = :missing",
			expectedNames: []string{},
		},
		{
			name:          "colon_fake_inside_line_comment_not_recognized",
			query:         "SELECT * FROM t -- comment with :fake\nWHERE x = :real",
			bindType:      DOLLAR,
			present:       []string{"real"},
			expectedQuery: "SELECT * FROM t -- comment with :fake\nWHERE x = $1",
			expectedNames: []string{"real"},
		},
		{
			name:          "colon_fake_inside_dollar_quote_not_recognized",
			query:         "SELECT $tag$ body :fake $tag$, x = :real",
			bindType:      DOLLAR,
			present:       []string{"real"},
			expectedQuery: "SELECT $tag$ body :fake $tag$, x = $1",
			expectedNames: []string{"real"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotQuery, gotNames, err := compileNamedQueryWith(
				[]byte(tt.query), tt.bindType, existsSet(tt.present...))
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if gotQuery != tt.expectedQuery {
				t.Errorf("query:\n  got:  %q\n  want: %q", gotQuery, tt.expectedQuery)
			}
			if len(gotNames) != len(tt.expectedNames) {
				t.Fatalf("names length: got %d (%v), want %d (%v)",
					len(gotNames), gotNames, len(tt.expectedNames), tt.expectedNames)
			}
			for i, name := range tt.expectedNames {
				if gotNames[i] != name {
					t.Errorf("names[%d]: got %q, want %q", i, gotNames[i], name)
				}
			}
		})
	}
}
