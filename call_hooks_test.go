package sqlex

import (
	"context"
	"testing"
)

// recordingHook records BeforeQuery/AfterQuery invocations into shared slices,
// and optionally rewrites the event query, so tests can assert both ordering and
// the SQL observed by outer Hooks.
type recordingHook struct {
	name      string
	before    *[]string
	after     *[]string
	seenQuery *[]string
	rewrite   func(string) string
}

func (h *recordingHook) BeforeQuery(ctx context.Context, event *QueryEvent) context.Context {
	if h.before != nil {
		*h.before = append(*h.before, h.name)
	}
	if h.seenQuery != nil {
		*h.seenQuery = append(*h.seenQuery, event.Query)
	}
	if h.rewrite != nil {
		event.Query = h.rewrite(event.Query)
	}
	return ctx
}

func (h *recordingHook) AfterQuery(ctx context.Context, event *QueryEvent) {
	if h.after != nil {
		*h.after = append(*h.after, h.name)
	}
}

// TestWithCallHooks_NoHooksReturnsSameContext verifies that WithCallHooks with no
// Hooks is a no-op and does not wrap the context.
func TestWithCallHooks_NoHooksReturnsSameContext(t *testing.T) {
	ctx := context.Background()
	if got := WithCallHooks(ctx); got != ctx {
		t.Error("WithCallHooks with no hooks should return the original context")
	}
	if got := getCallHooks(ctx); got != nil {
		t.Errorf("plain context should carry no call hooks, got %v", got)
	}
}

// TestGetCallHooks_NilContext verifies getCallHooks tolerates a nil context instead
// of panicking, so a Hook lookup can never be the cause of a crash.
func TestGetCallHooks_NilContext(t *testing.T) {
	var nilCtx context.Context
	if got := getCallHooks(nilCtx); got != nil {
		t.Errorf("nil context should yield nil call hooks, got %v", got)
	}
}

// TestWithCallHooks_Replaces verifies that successive WithCallHooks calls replace
// rather than accumulate, matching the shadowing behaviour of context.WithValue.
func TestWithCallHooks_Replaces(t *testing.T) {
	var before []string
	mk := func(name string) *recordingHook {
		return &recordingHook{name: name, before: &before}
	}

	ctx := WithCallHooks(context.Background(), mk("outer"))
	ctx = WithCallHooks(ctx, mk("inner1"), mk("inner2"))

	p := &pipeline{driverName: "mysql"}
	if _, _, _, err := p.prepare(ctx, "SELECT 1", nil, OpQuery); err != nil {
		t.Fatalf("prepare failed: %v", err)
	}

	want := []string{"inner1", "inner2"}
	if len(before) != len(want) {
		t.Fatalf("expected %d hooks to fire, got %d: %v", len(want), len(before), before)
	}
	for i, name := range want {
		if before[i] != name {
			t.Errorf("hook[%d]: got %q, want %q", i, before[i], name)
		}
	}
}

// TestWithCallHooks_ReplaceKeepsParentIntact verifies that replacing hooks in a
// derived context leaves the parent context's chain untouched, so sibling contexts
// derived from the same parent stay independent.
func TestWithCallHooks_ReplaceKeepsParentIntact(t *testing.T) {
	outer := &recordingHook{name: "outer"}
	inner := &recordingHook{name: "inner"}

	parent := WithCallHooks(context.Background(), outer)
	child := WithCallHooks(parent, inner)

	if got := getCallHooks(parent); len(got) != 1 || got[0] != outer {
		t.Errorf("parent chain must stay unchanged, got %v", got)
	}
	if got := getCallHooks(child); len(got) != 1 || got[0] != inner {
		t.Errorf("child chain should hold only the innermost hook, got %v", got)
	}
}

// TestWithCallHooks_DoesNotAliasCallerSlice verifies that the stored chain is a copy,
// so a caller mutating its slice afterwards cannot change the context's hooks.
func TestWithCallHooks_DoesNotAliasCallerSlice(t *testing.T) {
	first := &recordingHook{name: "first"}
	second := &recordingHook{name: "second"}

	callerHooks := []Hook{first}
	ctx := WithCallHooks(context.Background(), callerHooks...)

	// Mutating the caller's slice must not affect the stored chain.
	callerHooks[0] = second

	if got := getCallHooks(ctx); len(got) != 1 || got[0] != first {
		t.Errorf("context must retain a copy of the original hooks, got %v", got)
	}
}

// TestWithCallHooks_OnionOrderWithGlobalHooks verifies that global Hooks wrap
// call-scoped Hooks: BeforeQuery runs global first, AfterQuery runs global last.
func TestWithCallHooks_OnionOrderWithGlobalHooks(t *testing.T) {
	var before, after []string
	mk := func(name string) *recordingHook {
		return &recordingHook{name: name, before: &before, after: &after}
	}

	p := &pipeline{driverName: "mysql", hooks: []Hook{mk("global")}}
	ctx := WithCallHooks(context.Background(), mk("call"))

	_, afterFunc, _, err := p.prepare(ctx, "SELECT 1", nil, OpQuery)
	if err != nil {
		t.Fatalf("prepare failed: %v", err)
	}

	if len(before) != 2 || before[0] != "global" || before[1] != "call" {
		t.Errorf("BeforeQuery order: got %v, want [global call]", before)
	}

	afterFunc()

	if len(after) != 2 || after[0] != "call" || after[1] != "global" {
		t.Errorf("AfterQuery order: got %v, want [call global]", after)
	}
}

// TestWithCallHooks_GlobalHookSeesPreRewriteQuery documents the consequence of
// placing call-scoped Hooks on the inside: a global Hook's BeforeQuery runs before
// the call-scoped rewrite, while its AfterQuery observes the final SQL.
func TestWithCallHooks_GlobalHookSeesPreRewriteQuery(t *testing.T) {
	var globalSeen []string
	globalHook := &recordingHook{name: "global", seenQuery: &globalSeen}
	callHook := &recordingHook{
		name:    "call",
		rewrite: func(string) string { return "SELECT 2" },
	}

	p := &pipeline{driverName: "mysql", hooks: []Hook{globalHook}}
	ctx := WithCallHooks(context.Background(), callHook)

	_, afterFunc, event, err := p.prepare(ctx, "SELECT 1", nil, OpQuery)
	if err != nil {
		t.Fatalf("prepare failed: %v", err)
	}
	afterFunc()

	if len(globalSeen) != 1 || globalSeen[0] != "SELECT 1" {
		t.Errorf("global BeforeQuery should see the original SQL, got %v", globalSeen)
	}
	if event.Query != "SELECT 2" {
		t.Errorf("call-scoped rewrite should win, got %q", event.Query)
	}
}

// TestWithCallHooks_IsolatedPerContext verifies that call-scoped Hooks do not leak
// into other calls and never mutate the pipeline's global Hook chain.
func TestWithCallHooks_IsolatedPerContext(t *testing.T) {
	var globalFired, callFired []string

	p := &pipeline{
		driverName: "mysql",
		hooks:      []Hook{&recordingHook{name: "global", before: &globalFired}},
	}

	ctx := WithCallHooks(context.Background(), &recordingHook{name: "call", before: &callFired})
	if _, _, _, err := p.prepare(ctx, "SELECT 1", nil, OpQuery); err != nil {
		t.Fatalf("prepare with call hooks failed: %v", err)
	}
	if _, _, _, err := p.prepare(context.Background(), "SELECT 2", nil, OpQuery); err != nil {
		t.Fatalf("prepare without call hooks failed: %v", err)
	}

	if len(globalFired) != 2 {
		t.Errorf("global hook should fire on both calls, got %d", len(globalFired))
	}
	if len(callFired) != 1 {
		t.Errorf("call-scoped hook should fire only once, got %d", len(callFired))
	}
	if len(p.hooks) != 1 {
		t.Errorf("pipeline hooks must stay unchanged, got %d", len(p.hooks))
	}
}

// TestHookChain covers all four global/call-scoped input combinations.
func TestHookChain(t *testing.T) {
	globalHook := &recordingHook{name: "global"}
	callHook := &recordingHook{name: "call"}
	global := []Hook{globalHook}
	ctxWithCall := WithCallHooks(context.Background(), callHook)

	t.Run("no_call_hooks_returns_global_unchanged", func(t *testing.T) {
		got := hookChain(context.Background(), global)
		if len(got) != 1 || got[0] != globalHook {
			t.Errorf("got %v, want the global slice", got)
		}
	})

	t.Run("no_global_hooks_returns_call_hooks", func(t *testing.T) {
		got := hookChain(ctxWithCall, nil)
		if len(got) != 1 || got[0] != callHook {
			t.Errorf("got %v, want the call hook", got)
		}
	})

	t.Run("both_present_global_first", func(t *testing.T) {
		got := hookChain(ctxWithCall, global)
		if len(got) != 2 || got[0] != globalHook || got[1] != callHook {
			t.Errorf("got %v, want [global call]", got)
		}
	})

	t.Run("neither_present", func(t *testing.T) {
		if got := hookChain(context.Background(), nil); len(got) != 0 {
			t.Errorf("got %v, want empty", got)
		}
	})

	t.Run("global_slice_not_mutated", func(t *testing.T) {
		if len(global) != 1 || global[0] != globalHook {
			t.Errorf("global slice was mutated: %v", global)
		}
	})
}

// TestHookChain_StmtHooksNotMutated verifies that building a chain for a Stmt call
// leaves the Stmt's inherited Hooks untouched.
func TestHookChain_StmtHooksNotMutated(t *testing.T) {
	inherited := &recordingHook{name: "inherited"}
	stmt := &Stmt{hooks: []Hook{inherited}}

	ctx := WithCallHooks(context.Background(), &recordingHook{name: "call"})
	chain := hookChain(ctx, stmt.hooks)

	if len(chain) != 2 || chain[0] != inherited {
		t.Errorf("chain should start with the inherited hook, got %v", chain)
	}
	if len(stmt.hooks) != 1 || stmt.hooks[0] != inherited {
		t.Errorf("stmt hooks must stay unchanged, got %v", stmt.hooks)
	}
}
