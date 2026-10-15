package sqlex

import (
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/go-sqlex/sqlex/types"
)

type guardListNode struct {
	ID   int            `db:"id"`
	Next *guardListNode `db:"-"`
}

type guardTreeNode struct {
	ID       int             `db:"id"`
	Children []guardTreeNode `db:"-"`
}

type guardCategory struct {
	Name     string          `json:"name"`
	Children []guardCategory `json:"children"`
}

type guardTreeRow struct {
	ID   int                              `db:"id"`
	Tree types.JSONValue[[]guardCategory] `db:"tree"`
}

// TestSelect_SelfReferentialDest verifies Select into self-referential types no
// longer overflows the stack in the RawBytes guard. A stack overflow is a fatal
// error that recover cannot catch, so a regression aborts the whole test binary.
func TestSelect_SelfReferentialDest(t *testing.T) {
	if !TestSqlite {
		t.Skip("SQLite not available")
	}

	t.Run("linked list", func(t *testing.T) {
		var out []guardListNode
		if err := sldb.Select(&out, `SELECT 1 AS id`); err != nil {
			t.Fatalf("Select: %v", err)
		}
		if len(out) != 1 || out[0].ID != 1 {
			t.Errorf("unexpected result: %+v", out)
		}
	})

	t.Run("tree", func(t *testing.T) {
		var out []guardTreeNode
		if err := sldb.Select(&out, `SELECT 1 AS id`); err != nil {
			t.Fatalf("Select: %v", err)
		}
		if len(out) != 1 || out[0].ID != 1 {
			t.Errorf("unexpected result: %+v", out)
		}
	})

	t.Run("JSONValue of a recursive type", func(t *testing.T) {
		var out []guardTreeRow
		q := `SELECT 1 AS id, '[{"name":"a","children":[{"name":"b","children":[]}]}]' AS tree`
		if err := sldb.Select(&out, q); err != nil {
			t.Fatalf("Select: %v", err)
		}
		if len(out) != 1 || !out[0].Tree.Valid || len(out[0].Tree.Val) != 1 {
			t.Fatalf("unexpected result: %+v", out)
		}
		if got := out[0].Tree.Val[0]; got.Name != "a" || len(got.Children) != 1 || got.Children[0].Name != "b" {
			t.Errorf("unexpected tree: %+v", got)
		}
	})
}

// TestRowsStructScan_NilPointer verifies StructScan returns an error on a typed
// nil pointer destination instead of panicking (#10 F-0001).
func TestRowsStructScan_NilPointer(t *testing.T) {
	r := Rows{}
	err := r.StructScan((*struct{ X int })(nil))
	if err == nil {
		t.Fatal("expected error for nil pointer destination, got nil")
	}
}

// TestRowColumns_NilRows verifies Columns/ColumnTypes return an error instead of
// panicking when the Row holds no rows and no deferred error (#10 F-0008).
func TestRowColumns_NilRows(t *testing.T) {
	var r Row
	if _, err := r.Columns(); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("Columns: expected sql.ErrNoRows, got %v", err)
	}
	if _, err := r.ColumnTypes(); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("ColumnTypes: expected sql.ErrNoRows, got %v", err)
	}
}

// TestRowsStructScan_SwitchDestType verifies the traversal cache is rebuilt when
// the destination struct type changes, instead of writing through field indexes
// computed for the previous type (#10 F-0018).
func TestRowsStructScan_SwitchDestType(t *testing.T) {
	if !TestSqlite {
		t.Skip("SQLite not available")
	}
	type ab struct {
		A int `db:"a"`
		B int `db:"b"`
	}
	type ba struct {
		B int `db:"b"`
		A int `db:"a"`
	}

	rows, err := sldb.Queryx(`SELECT 1 AS a, 2 AS b UNION ALL SELECT 1, 2`)
	if err != nil {
		t.Fatalf("Queryx: %v", err)
	}
	defer rows.Close()

	if !rows.Next() {
		t.Fatalf("expected first row, err=%v", rows.Err())
	}
	var first ab
	if err = rows.StructScan(&first); err != nil {
		t.Fatalf("StructScan(ab): %v", err)
	}
	if first != (ab{A: 1, B: 2}) {
		t.Errorf("first row: got %+v, want {A:1 B:2}", first)
	}

	if !rows.Next() {
		t.Fatalf("expected second row, err=%v", rows.Err())
	}
	var second ba
	if err = rows.StructScan(&second); err != nil {
		t.Fatalf("StructScan(ba): %v", err)
	}
	if second != (ba{B: 2, A: 1}) {
		t.Errorf("second row with a different type: got %+v, want {B:2 A:1}", second)
	}
}

type namedBytes []byte

// TestIn_ByteSliceKindsNotExpanded verifies that every byte-slice type is bound
// as a single value, not expanded into an IN list (#10 F-0012).
func TestIn_ByteSliceKindsNotExpanded(t *testing.T) {
	cases := []struct {
		name string
		arg  any
	}{
		{"[]byte", []byte{1, 2, 3}},
		{"named []byte", namedBytes{1, 2, 3}},
		{"json.RawMessage", json.RawMessage(`{"a":1}`)},
		{"*named []byte", &namedBytes{1, 2, 3}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			q, args, err := In(`SELECT * FROM t WHERE h IN (?)`, c.arg)
			if err != nil {
				t.Fatalf("In: %v", err)
			}
			if q != `SELECT * FROM t WHERE h IN (?)` || len(args) != 1 {
				t.Errorf("byte slice expanded: q=%q args=%d", q, len(args))
			}
		})
	}

	// A slice of a byte-slice type is still a list of values.
	q, args, err := In(`SELECT * FROM t WHERE h IN (?)`, []namedBytes{{1}, {2}})
	if err != nil {
		t.Fatalf("In: %v", err)
	}
	if q != `SELECT * FROM t WHERE h IN (?, ?)` || len(args) != 2 {
		t.Errorf("[]namedBytes should expand: q=%q args=%d", q, len(args))
	}
}

// TestIn_DigitDollarNotDollarQuote verifies that $1$ is not treated as a
// dollar-quote opener that would swallow the following placeholders (#10 F-0015).
func TestIn_DigitDollarNotDollarQuote(t *testing.T) {
	q, args, err := In(`SELECT $1$ WHERE a IN (?) $1$, ?`, []int{1, 2}, 3)
	if err != nil {
		t.Fatalf("In: %v", err)
	}
	if q != `SELECT $1$ WHERE a IN (?, ?) $1$, ?` || len(args) != 3 {
		t.Errorf("unexpected expansion: q=%q args=%d", q, len(args))
	}
}

// TestBindStruct_NilArg verifies the name-existence closure tolerates a nil
// argument and the error is reported instead of panicking (#10 F-0013).
func TestBindStruct_NilArg(t *testing.T) {
	type T struct {
		A int `db:"a"`
	}
	if exists := nameExistsInStruct((*T)(nil), mapper()); exists("a") {
		t.Error("nil arg should report no names")
	}
	if _, _, err := bindStruct(QUESTION, `SELECT :a`, (*T)(nil), mapper()); err == nil {
		t.Error("expected error binding a nil struct pointer")
	}
}

// TestMapper_RebuildOnNameMapperChange verifies the lock-free fast path still
// picks up a replaced NameMapper (#10 F-0005).
func TestMapper_RebuildOnNameMapperChange(t *testing.T) {
	orig := NameMapper
	defer func() { NameMapper = orig }()

	type T struct{ FooBar int }
	before := mapper()
	if mapper() != before {
		t.Fatal("mapper() should be stable while NameMapper is unchanged")
	}

	NameMapper = strings.ToUpper
	after := mapper()
	if after == before {
		t.Fatal("mapper() should be rebuilt after NameMapper changes")
	}
	if fi := after.TypeMap(reflect.TypeOf(T{})).GetByPath("FOOBAR"); fi == nil {
		t.Error("rebuilt mapper should use the new NameMapper")
	}

	NameMapper = orig
	if fi := mapper().TypeMap(reflect.TypeOf(T{})).GetByPath("foobar"); fi == nil {
		t.Error("mapper should follow NameMapper back to the original")
	}
}

// TestMapper_Concurrent exercises the fast path from many goroutines; run with -race.
func TestMapper_Concurrent(t *testing.T) {
	const goroutines = 16
	const calls = 1000

	want := mapper()
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < calls; j++ {
				if mapper() != want {
					t.Error("mapper() changed without a NameMapper change")
					return
				}
			}
		}()
	}
	wg.Wait()
}

// BenchmarkMapperParallel measures contention on the default mapper (#10 F-0005).
func BenchmarkMapperParallel(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = mapper()
		}
	})
}
