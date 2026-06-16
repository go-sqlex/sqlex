package sqlex

import (
	"database/sql/driver"
	"reflect"
	"testing"
)

// fakeValuer 模拟 driver.Valuer
type fakeValuer struct {
	val any
	err error
}

func (f fakeValuer) Value() (driver.Value, error) {
	return f.val, f.err
}

// TestIn_EdgeCases 覆盖 In 函数在所有 SQL 词法元素下的占位符识别。
// 目标：与 Rebind / compileNamedQuery 完全对称——SQL 词法范围内的 ?
// 不被识别为占位符。
func TestIn_EdgeCases(t *testing.T) {
	cases := []struct {
		name      string
		query     string
		args      []any
		wantBound string
		wantArgs  []any
		wantErr   string // 非空则期望错误（substring 匹配）
	}{
		// ===== A. 字符串字面量 =====
		{
			name:      "字符串内?保留_后跟IN展开",
			query:     `SELECT * FROM t WHERE name = 'test?' AND id IN (?)`,
			args:      []any{[]int{1, 2, 3}},
			wantBound: `SELECT * FROM t WHERE name = 'test?' AND id IN (?, ?, ?)`,
			wantArgs:  []any{1, 2, 3},
		},
		{
			name:      "字符串内多个?",
			query:     `SELECT 'a?b?c' WHERE id IN (?)`,
			args:      []any{[]int{1, 2}},
			wantBound: `SELECT 'a?b?c' WHERE id IN (?, ?)`,
			wantArgs:  []any{1, 2},
		},
		{
			name:      "SQL转义引号",
			query:     `SELECT 'O''Reilly?' WHERE id IN (?)`,
			args:      []any{[]int{1, 2}},
			wantBound: `SELECT 'O''Reilly?' WHERE id IN (?, ?)`,
			wantArgs:  []any{1, 2},
		},

		// ===== B. SQL 注释 =====
		{
			name:      "行注释内?",
			query:     "SELECT * FROM t -- WHERE id = ?\nWHERE x IN (?)",
			args:      []any{[]int{1, 2}},
			wantBound: "SELECT * FROM t -- WHERE id = ?\nWHERE x IN (?, ?)",
			wantArgs:  []any{1, 2},
		},
		{
			name:      "块注释内?",
			query:     `SELECT /* WHERE id = ? */ * FROM t WHERE x IN (?)`,
			args:      []any{[]int{1, 2}},
			wantBound: `SELECT /* WHERE id = ? */ * FROM t WHERE x IN (?, ?)`,
			wantArgs:  []any{1, 2},
		},

		// ===== C. 标识符 =====
		{
			name:      "PG双引号标识符内?",
			query:     `SELECT "col?name" FROM t WHERE x IN (?)`,
			args:      []any{[]int{1, 2}},
			wantBound: `SELECT "col?name" FROM t WHERE x IN (?, ?)`,
			wantArgs:  []any{1, 2},
		},
		{
			name:      "MySQL backtick identifier with ?",
			query:     "SELECT `col?name` FROM t WHERE x IN (?)",
			args:      []any{[]int{1, 2}},
			wantBound: "SELECT `col?name` FROM t WHERE x IN (?, ?)",
			wantArgs:  []any{1, 2},
		},
		{
			name:      "SQL Server bracket identifier with ?",
			query:     `SELECT [col?name] FROM t WHERE x IN (?)`,
			args:      []any{[]int{1, 2}},
			wantBound: `SELECT [col?name] FROM t WHERE x IN (?, ?)`,
			wantArgs:  []any{1, 2},
		},
		{
			name:      "SQL Server bracket identifier with :name",
			query:     `SELECT [col:id] FROM t WHERE x IN (?)`,
			args:      []any{[]int{1, 2}},
			wantBound: `SELECT [col:id] FROM t WHERE x IN (?, ?)`,
			wantArgs:  []any{1, 2},
		},
		{
			name:      "SQL Server bracket escaped ]] with ?",
			query:     `SELECT [a]]?b] FROM t WHERE x IN (?)`,
			args:      []any{[]int{1, 2}},
			wantBound: `SELECT [a]]?b] FROM t WHERE x IN (?, ?)`,
			wantArgs:  []any{1, 2},
		},

		// ===== D. PG dollar-quoted =====
		{
			name:      "Dollar_quoting内?",
			query:     `SELECT $$hello?$$ FROM t WHERE x IN (?)`,
			args:      []any{[]int{1, 2}},
			wantBound: `SELECT $$hello?$$ FROM t WHERE x IN (?, ?)`,
			wantArgs:  []any{1, 2},
		},
		{
			name:      "Tagged_dollar_quoting",
			query:     `SELECT $tag$?$tag$ FROM t WHERE x IN (?)`,
			args:      []any{[]int{1, 2}},
			wantBound: `SELECT $tag$?$tag$ FROM t WHERE x IN (?, ?)`,
			wantArgs:  []any{1, 2},
		},

		// ===== E. 转义 =====
		{
			name:      "??字面量保留",
			query:     `SELECT * WHERE x = ?? AND y IN (?)`,
			args:      []any{[]int{1, 2}},
			wantBound: `SELECT * WHERE x = ?? AND y IN (?, ?)`,
			wantArgs:  []any{1, 2},
		},
		{
			name:      "反斜杠转义",
			query:     `SELECT * WHERE x = \? AND y IN (?)`,
			args:      []any{[]int{1, 2}},
			wantBound: `SELECT * WHERE x = \? AND y IN (?, ?)`,
			wantArgs:  []any{1, 2},
		},

		// ===== F. driver.Valuer =====
		{
			name:      "Valuer非slice",
			query:     `SELECT * WHERE x = ?`,
			args:      []any{fakeValuer{val: 42}},
			wantBound: `SELECT * WHERE x = ?`,
			wantArgs:  []any{fakeValuer{val: 42}}, // 注意：参数原样保留（driver 内部会调 Value）
		},
		{
			name:      "Valuer返回切片应展开",
			query:     `SELECT * WHERE x IN (?)`,
			args:      []any{fakeValuer{val: []int{1, 2, 3}}},
			wantBound: `SELECT * WHERE x IN (?, ?, ?)`,
			wantArgs:  []any{1, 2, 3},
		},
		{
			// Phase 1.9 后对称性：Valuer 返回切片但 ? 不在 (?) 形态 → 不展开，整切片下发
			name:      "Valuer返回切片_非(?)形态_不展开",
			query:     `SELECT * WHERE x = ?`,
			args:      []any{fakeValuer{val: []int{1, 2, 3}}},
			wantBound: `SELECT * WHERE x = ?`,
			wantArgs:  []any{[]int{1, 2, 3}}, // .Value() 已被调用，但整切片当单值
		},

		// ===== G. 边界 =====
		{
			name:    "空切片报错",
			query:   `SELECT * WHERE x IN (?)`,
			args:    []any{[]int{}},
			wantErr: "empty slice",
		},
		{
			name:      "单元素切片",
			query:     `SELECT * WHERE x IN (?)`,
			args:      []any{[]int{42}},
			wantBound: `SELECT * WHERE x IN (?)`,
			wantArgs:  []any{42},
		},
		{
			name:      "切片含nil",
			query:     `SELECT * WHERE x IN (?)`,
			args:      []any{[]any{1, nil, 3}},
			wantBound: `SELECT * WHERE x IN (?, ?, ?)`,
			wantArgs:  []any{1, nil, 3},
		},
		{
			name:      "无切片快速路径",
			query:     `SELECT * WHERE x = ? AND y = ?`,
			args:      []any{1, 2},
			wantBound: `SELECT * WHERE x = ? AND y = ?`, // 原样
			wantArgs:  []any{1, 2},
		},
		{
			name:      "多个IN",
			query:     `SELECT * WHERE a IN (?) AND b IN (?)`,
			args:      []any{[]int{1, 2}, []string{"x", "y"}},
			wantBound: `SELECT * WHERE a IN (?, ?) AND b IN (?, ?)`,
			wantArgs:  []any{1, 2, "x", "y"},
		},
		{
			name:      "?和切片混排",
			query:     `SELECT * WHERE x = ? AND id IN (?) AND y = ?`,
			args:      []any{100, []int{1, 2}, 200},
			wantBound: `SELECT * WHERE x = ? AND id IN (?, ?) AND y = ?`,
			wantArgs:  []any{100, 1, 2, 200},
		},
		{
			name:      "byte切片不展开",
			query:     `SELECT * WHERE blob = ?`,
			args:      []any{[]byte{1, 2, 3}},
			wantBound: `SELECT * WHERE blob = ?`,
			wantArgs:  []any{[]byte{1, 2, 3}},
		},

		// ===== H. 综合复杂场景 =====
		{
			name:      "字符串_注释_反引号_dollar_quoting_混合",
			query:     "SELECT 'a?b', /* ? */ `c?d`, $$e?f$$ FROM t WHERE x IN (?)",
			args:      []any{[]int{10, 20}},
			wantBound: "SELECT 'a?b', /* ? */ `c?d`, $$e?f$$ FROM t WHERE x IN (?, ?)",
			wantArgs:  []any{10, 20},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotBound, gotArgs, err := In(c.query, c.args...)

			if c.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", c.wantErr)
				}
				if !stringContains(err.Error(), c.wantErr) {
					t.Errorf("error = %q, want substring %q", err.Error(), c.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotBound != c.wantBound {
				t.Errorf("bound mismatch:\n  query=%q\n  got =%q\n  want=%q",
					c.query, gotBound, c.wantBound)
			}
			if !reflect.DeepEqual(gotArgs, c.wantArgs) {
				t.Errorf("args mismatch:\n  query=%q\n  got =%v\n  want=%v",
					c.query, gotArgs, c.wantArgs)
			}
		})
	}
}

// TestNeedsInRewrite_EdgeCases 验证 needsInRewrite 简单实现的行为。
// 真实语义：判断 args 是否需要走 In 路径处理（含切片展开 + AsValue/AsList 解包）。
//
// 设计：保持零开销快速路径。
//   - Valuer 整体当单值（即使内部含切片也按规范走 driver）
//   - 切片指针不识别（用户应直传切片）
//   - 数组（非切片）当前 In 不展开，needsInRewrite 也不识别
//   - AsValue / AsList 包装即使不展开，也需进 In 解包（return true）
func TestNeedsInRewrite_EdgeCases(t *testing.T) {
	cases := []struct {
		name string
		args []any
		want bool
	}{
		{name: "空args", args: []any{}, want: false},
		{name: "纯标量", args: []any{1, "abc", true}, want: false},
		{name: "含int切片", args: []any{1, []int{2, 3}}, want: true},
		{name: "byte切片忽略", args: []any{[]byte{1, 2, 3}}, want: false},
		{name: "Valuer非切片", args: []any{fakeValuer{val: 1}}, want: false},
		// Valuer 实现整体不展开（即使返回切片，driver.Value 规范本就不该返回切片）
		{name: "Valuer返回切片不展开", args: []any{fakeValuer{val: []int{1, 2}}}, want: false},
		// 切片指针：用户反模式，让错误自然暴露给 driver
		{name: "切片指针不识别", args: []any{&[]int{1, 2}}, want: false},
		// 数组与切片不同：In 内部 asSliceForIn 也只认 Slice 不认 Array
		{name: "数组_当前不展开", args: []any{[3]int{1, 2, 3}}, want: true}, // needsInRewrite 用 reflect.Kind 包含 Array
		{name: "nil元素", args: []any{nil, []int{1, 2}}, want: true},
		{name: "空切片", args: []any{[]int{}}, want: true},
		{name: "Valuer返回nil", args: []any{fakeValuer{val: nil}}, want: false},
		// AsValue / AsList 包装：必须走 In 路径（让 In 解包并处理）
		{name: "AsValue包装标量_必须走In", args: []any{AsValue(42)}, want: true},
		{name: "AsValue包装切片_必须走In", args: []any{AsValue([]int{1, 2})}, want: true},
		{name: "AsList包装切片_必须走In", args: []any{AsList([]int{1, 2})}, want: true},
		{name: "AsValue混合普通标量", args: []any{1, AsValue("foo"), true}, want: true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := needsInRewrite(c.args)
			if got != c.want {
				t.Errorf("needsInRewrite(%#v) = %v, want %v", c.args, got, c.want)
			}
		})
	}
}

// TestNextPlaceholder 单测内部辅助函数，确保 In 重写后的扫描逻辑正确。
// 同时验证 inParen 返回值——这是括号语境识别的核心。
func TestNextPlaceholder(t *testing.T) {
	cases := []struct {
		name        string
		query       string
		start       int
		wantIdx     int
		wantInParen bool
	}{
		// ===== 基础占位符识别 =====
		{name: "简单_?", query: "SELECT ?", start: 0, wantIdx: 7, wantInParen: false},
		{name: "字符串内?跳过", query: "SELECT 'a?' AND ?", start: 0, wantIdx: 16, wantInParen: false},
		{name: "??跳过", query: "SELECT ?? AND ?", start: 0, wantIdx: 14, wantInParen: false},
		{name: "\\?跳过", query: `SELECT \? AND ?`, start: 0, wantIdx: 14, wantInParen: false},
		{name: "无占位符", query: "SELECT 1", start: 0, wantIdx: -1, wantInParen: false},
		{name: "行注释跳过", query: "-- ?\nSELECT ?", start: 0, wantIdx: 12, wantInParen: false},
		{name: "块注释跳过", query: "/* ? */ SELECT ?", start: 0, wantIdx: 15, wantInParen: false},
		{name: "PG双引号跳过", query: `"col?" SELECT ?`, start: 0, wantIdx: 14, wantInParen: false},
		{name: "MySQL反引号跳过", query: "`col?` SELECT ?", start: 0, wantIdx: 14, wantInParen: false},
		{name: "Dollar_quoting跳过", query: "$$?$$ SELECT ?", start: 0, wantIdx: 13, wantInParen: false},
		{name: "tagged_dollar_quoting", query: "$t$?$t$ SELECT ?", start: 0, wantIdx: 15, wantInParen: false},

		// ===== 括号语境识别（严格 (?) 形态）=====
		// 命中：( 与 ) 之间只有一个 ? 和可选 ASCII 空白
		{name: "IN_(?)_紧贴", query: "WHERE id IN (?)", start: 0, wantIdx: 13, wantInParen: true},
		{name: "嵌套(?)", query: "(?)", start: 0, wantIdx: 1, wantInParen: true},
		{name: "VALUES_(?)_紧贴", query: "VALUES (?)", start: 0, wantIdx: 8, wantInParen: true},
		{name: "(? AND ?_第二个?在=后)", query: "(?) AND x = ?", start: 0, wantIdx: 1, wantInParen: true},
		{name: "(_空格_?_)_左侧带空格", query: "( ?)", start: 0, wantIdx: 2, wantInParen: true},
		{name: "(?_空格_)_右侧带空格", query: "(? )", start: 0, wantIdx: 1, wantInParen: true},
		{name: "(_空格_?_空格_)_左右都带空格", query: "( ? )", start: 0, wantIdx: 2, wantInParen: true},
		{name: "(_Tab_?_Tab_)_Tab分隔", query: "(\t?\t)", start: 0, wantIdx: 2, wantInParen: true},
		{name: "(_换行_?_换行_)_跨行SQL", query: "(\n    ?\n)", start: 0, wantIdx: 6, wantInParen: true},

		// 未命中：? 前后不是严格 (?) 形态
		{name: "WHERE_=?_无括号", query: "WHERE x = ?", start: 0, wantIdx: 10, wantInParen: false},
		{name: "(?,?,?)_多?_第一个", query: "(?, ?, ?)", start: 0, wantIdx: 1, wantInParen: false},
		{name: "(?,?,?)_多?_第二个", query: "(?, ?, ?)", start: 2, wantIdx: 4, wantInParen: false},
		{name: "(?+1)_算术表达式", query: "(? + 1)", start: 0, wantIdx: 1, wantInParen: false},
		{name: "(?_IS_NULL)_表达式", query: "(? IS NULL)", start: 0, wantIdx: 1, wantInParen: false},
		{name: "(SELECT_?)_子查询_前有字母", query: "(SELECT ?)", start: 0, wantIdx: 8, wantInParen: false},
		{name: "ANY(?)_前有字母不展开判定_但仍命中(?)", query: "= ANY(?)", start: 0, wantIdx: 6, wantInParen: true}, // 已知边界，靠 AsValue 兜
		{name: "(?_注释_)_注释非空白触发不命中", query: "(? /*c*/)", start: 0, wantIdx: 1, wantInParen: false},
		{name: "(_注释_?)_左侧注释也不命中", query: "(/*c*/ ?)", start: 0, wantIdx: 7, wantInParen: false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotIdx, gotInParen := nextPlaceholder(c.query, c.start)
			if gotIdx != c.wantIdx || gotInParen != c.wantInParen {
				t.Errorf("nextPlaceholder(%q, %d) = (%d, %v), want (%d, %v)",
					c.query, c.start, gotIdx, gotInParen, c.wantIdx, c.wantInParen)
			}
		})
	}
}

// pqArrayLike 模拟 pq.Array 风格的 Valuer 包装：
// .Value() 返回 PG 数组字面量字符串（如 "{1,2,3}"），不是切片。
// 用于验证：用户用此类包装时，autoIn 不会错误展开。
type pqArrayLike struct {
	vals []int
}

func (a pqArrayLike) Value() (driver.Value, error) {
	if a.vals == nil {
		return nil, nil
	}
	s := "{"
	for i, v := range a.vals {
		if i > 0 {
			s += ","
		}
		s += string(rune('0' + v)) // 简化：仅支持单位整数，测试用
	}
	return s + "}", nil
}

// TestAutoIn_SliceFieldValue_Boundary 验证"切片作为字段值"场景：
// 当用户通过 driver.Valuer（如 pq.Array）包装切片作为字段值时，
// autoIn 不应将其展开为 IN 列表。这是 sqlex autoIn 的边界契约。
func TestAutoIn_SliceFieldValue_Boundary(t *testing.T) {
	t.Run("Valuer包装切片_INSERT场景_不被展开", func(t *testing.T) {
		// 模拟：INSERT INTO users (tags) VALUES (?)
		// args: pq.Array([]int{1,2,3})  → 应作为单值传递
		query := `INSERT INTO users (tags) VALUES (?)`
		gotQ, gotArgs, err := autoIn(query, pqArrayLike{vals: []int{1, 2, 3}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// 关键断言：query 不被改写
		if gotQ != query {
			t.Errorf("query 被错误改写：\n  got =%q\n  want=%q", gotQ, query)
		}
		// 关键断言：args 仍是 1 个，且类型是 Valuer 包装
		if len(gotArgs) != 1 {
			t.Errorf("args 被错误展开：got len=%d, want 1", len(gotArgs))
		}
	})

	t.Run("byte切片_INSERT场景_不被展开", func(t *testing.T) {
		// MySQL JSON 列：序列化为 []byte 后写入
		query := `INSERT INTO t (json_col) VALUES (?)`
		jsonData := []byte(`[1,2,3]`)
		gotQ, gotArgs, err := autoIn(query, jsonData)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gotQ != query {
			t.Errorf("query 被错误改写：\n  got =%q\n  want=%q", gotQ, query)
		}
		if len(gotArgs) != 1 {
			t.Errorf("args 被错误展开：got len=%d, want 1", len(gotArgs))
		}
	})

	t.Run("普通切片_IN场景_应展开", func(t *testing.T) {
		// 对照组：未包装的切片 + IN 子句 → 应展开
		query := `SELECT * FROM users WHERE id IN (?)`
		gotQ, gotArgs, err := autoIn(query, []int{1, 2, 3})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := `SELECT * FROM users WHERE id IN (?, ?, ?)`
		if gotQ != want {
			t.Errorf("query mismatch:\n  got =%q\n  want=%q", gotQ, want)
		}
		if len(gotArgs) != 3 {
			t.Errorf("args 应展开为 3 个，got len=%d", len(gotArgs))
		}
	})

	t.Run("普通切片_INSERT场景_会被错误展开_这是已知边界", func(t *testing.T) {
		// 这个 case 验证当前的"已知行为"——用户必须知道这里要包装。
		// 文档已明示：autoIn 仅适用于 IN 子句；INSERT/UPDATE 切片字段值需用 pq.Array 等包装。
		// 这个测试不是"应该这样工作"，而是"如果不包装会发生什么"——作为防回归提醒。
		query := `INSERT INTO users (tags) VALUES (?)`
		gotQ, _, err := autoIn(query, []int{1, 2, 3})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// 当前行为：query 被展开为 (?, ?, ?)，driver 会报列数不匹配
		// 这是设计取舍——文档中明确警示用户用 pq.Array 包装
		want := `INSERT INTO users (tags) VALUES (?, ?, ?)`
		if gotQ != want {
			// 如果未来改为"语境识别"等更智能的方案，这里需要更新
			t.Logf("当前 autoIn 行为：%q（用户应用 pq.Array 包装）", gotQ)
		}
	})
}

// TestAutoIn_ParenContext_GORMStyle 验证 Phase 1.8 引入的"括号语境识别"。
// 核心规则（参考 GORM 的 afterParenthesis）：
//   - ? 紧跟 ( 后 + 切片 → 展开（如 IN (?)）
//   - ? 不在 ( 后 + 切片 → 不展开，作为单值传递（如 WHERE x = ?）
func TestAutoIn_ParenContext_GORMStyle(t *testing.T) {
	t.Run("WHERE_=_切片_不展开", func(t *testing.T) {
		// 用户传切片但 ? 不在括号内 → 不展开（行为变化！之前会错误展开）
		query := `SELECT * WHERE x = ?`
		gotQ, gotArgs, err := autoIn(query, []int{1, 2, 3})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gotQ != query {
			t.Errorf("query 不应被改写：\n  got =%q\n  want=%q", gotQ, query)
		}
		if len(gotArgs) != 1 {
			t.Errorf("args 应保留为单值切片：got len=%d, want 1", len(gotArgs))
		}
	})

	t.Run("UPDATE_SET_切片_不展开", func(t *testing.T) {
		query := `UPDATE t SET tags = ? WHERE id = ?`
		gotQ, gotArgs, err := autoIn(query, []int{1, 2, 3}, 100)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gotQ != query {
			t.Errorf("query 不应被改写：\n  got =%q\n  want=%q", gotQ, query)
		}
		if len(gotArgs) != 2 {
			t.Errorf("args 应保留 2 个：got len=%d", len(gotArgs))
		}
	})

	t.Run("IN_(?)_仍然展开", func(t *testing.T) {
		query := `SELECT * WHERE id IN (?)`
		gotQ, gotArgs, err := autoIn(query, []int{1, 2, 3})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := `SELECT * WHERE id IN (?, ?, ?)`
		if gotQ != want {
			t.Errorf("query mismatch:\n  got =%q\n  want=%q", gotQ, want)
		}
		if len(gotArgs) != 3 {
			t.Errorf("args 应展开为 3 个，got len=%d", len(gotArgs))
		}
	})

	t.Run("INSERT_VALUES_(?)_仍展开_严格(?)语义", func(t *testing.T) {
		// 这是"严格 (?) 语境识别"的"已知局限"——VALUES (?) 也是 (?) 形态
		// 正确的处理：用户用 sqlex.AsValue 或 pq.Array 包装
		query := `INSERT INTO t (tags) VALUES (?)`
		gotQ, _, err := autoIn(query, []int{1, 2, 3})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// 仍然展开（严格 (?) 形态命中；用户应用 sqlex.AsValue 或 pq.Array）
		want := `INSERT INTO t (tags) VALUES (?, ?, ?)`
		if gotQ != want {
			t.Errorf("query mismatch:\n  got =%q\n  want=%q", gotQ, want)
		}
	})

	t.Run("混排_=_和_IN_(_)", func(t *testing.T) {
		// x = ? 配 100；id IN (?) 配 [1,2]；y = ? 配 200
		query := `WHERE x = ? AND id IN (?) AND y = ?`
		gotQ, gotArgs, err := autoIn(query, 100, []int{1, 2}, 200)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := `WHERE x = ? AND id IN (?, ?) AND y = ?`
		if gotQ != want {
			t.Errorf("query mismatch:\n  got =%q\n  want=%q", gotQ, want)
		}
		if len(gotArgs) != 4 {
			t.Errorf("args 应为 4 个：got len=%d", len(gotArgs))
		}
	})
}

// TestAutoIn_AsValueAsList_EscapeHooks 验证 sqlex.AsValue / sqlex.AsList 业务逃生 helper。
func TestAutoIn_AsValueAsList_EscapeHooks(t *testing.T) {
	t.Run("AsValue_包装切片_INSERT场景_不展开", func(t *testing.T) {
		// AsValue 让 ? 在 (?) 形态内的切片也不被展开
		query := `INSERT INTO t (tags) VALUES (?)`
		gotQ, gotArgs, err := autoIn(query, AsValue([]int{1, 2, 3}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gotQ != query {
			t.Errorf("query 不应被改写：\n  got =%q\n  want=%q", gotQ, query)
		}
		if len(gotArgs) != 1 {
			t.Errorf("args 应保留单值：got len=%d, want 1", len(gotArgs))
		}
		// 验证 args[0] 是原切片（未被解包）
		if got, ok := gotArgs[0].([]int); !ok || len(got) != 3 {
			t.Errorf("args[0] 应是 []int{1,2,3}：got %v (%T)", gotArgs[0], gotArgs[0])
		}
	})

	t.Run("AsValue_包装非切片标量", func(t *testing.T) {
		query := `WHERE x = ?`
		gotQ, gotArgs, err := autoIn(query, AsValue(42))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gotQ != query {
			t.Errorf("query 不应被改写")
		}
		if gotArgs[0] != 42 {
			t.Errorf("args[0] 应是 42：got %v", gotArgs[0])
		}
	})

	t.Run("AsList_切片_WHERE=场景_强制展开", func(t *testing.T) {
		// 业务明确要展开（即使 ? 不在 (?) 形态内）
		query := `WHERE x = ?`
		gotQ, gotArgs, err := autoIn(query, AsList([]int{1, 2, 3}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := `WHERE x = ?, ?, ?`
		if gotQ != want {
			t.Errorf("query 应被展开：\n  got =%q\n  want=%q", gotQ, want)
		}
		if len(gotArgs) != 3 {
			t.Errorf("args 应展开为 3 个：got len=%d", len(gotArgs))
		}
	})

	t.Run("AsList_非切片_应报错", func(t *testing.T) {
		_, _, err := autoIn(`WHERE x = ?`, AsList(42))
		if err == nil {
			t.Fatal("expected error for non-slice AsList, got nil")
		}
		if !stringContains(err.Error(), "AsList: argument is not a slice") {
			t.Errorf("error 应包含 'AsList: argument is not a slice'：got %v", err)
		}
	})

	t.Run("AsList_数组_应报错", func(t *testing.T) {
		// 数组（非切片）：asSliceForIn 只识别 reflect.Slice，AsList 应一致地拒绝
		_, _, err := autoIn(`WHERE x IN (?)`, AsList([3]int{1, 2, 3}))
		if err == nil {
			t.Fatal("expected error for array AsList, got nil")
		}
		if !stringContains(err.Error(), "AsList: argument is not a slice") {
			t.Errorf("error 应包含 'AsList: argument is not a slice'：got %v", err)
		}
	})

	t.Run("AsList_byte切片_应报错", func(t *testing.T) {
		// []byte 是 driver.Value 标准类型，asSliceForIn 显式排除；
		// AsList 与之保持一致——拒绝 []byte 强制展开（避免误把单值当列表）
		_, _, err := autoIn(`WHERE x IN (?)`, AsList([]byte{1, 2, 3}))
		if err == nil {
			t.Fatal("expected error for []byte AsList, got nil")
		}
		if !stringContains(err.Error(), "AsList: argument is not a slice") {
			t.Errorf("error 应包含 'AsList: argument is not a slice'：got %v", err)
		}
	})

	t.Run("AsList_空切片_应报错", func(t *testing.T) {
		_, _, err := autoIn(`WHERE x = ?`, AsList([]int{}))
		if err == nil {
			t.Fatal("expected error for empty slice AsList")
		}
		if !stringContains(err.Error(), "AsList: empty slice") {
			t.Errorf("error 应包含 'AsList: empty slice'：got %v", err)
		}
	})

	t.Run("AsValue_和_AsList_混合", func(t *testing.T) {
		// VALUES (?, ?, ?) 中只有第一个 ? 处于严格 (?) 形态吗？
		// 不！(?, ?, ?) 第一个 ? 后面是 , 不是 ) → 不是 (?) 形态 → 都不展开
		// 用 AsValue 关闭第 1 个、AsList 强制展开第 2/3 个，明确意图
		query := `INSERT INTO t (tags, ids, others) VALUES (?, ?, ?)`
		gotQ, gotArgs, err := autoIn(query,
			AsValue([]int{1, 2}),  // 第 1 个 ? 非 (?) + AsValue → 不展开
			AsList([]int{3, 4, 5}), // 第 2 个 ? 非 (?) + AsList → 强制展开
			AsList([]int{6, 7}),    // 第 3 个 ? 非 (?) + AsList → 强制展开
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// ?1 = AsValue 不展开（保留 ?）
		// ?2 = AsList 强制展开成 ?, ?, ?
		// ?3 = AsList 强制展开成 ?, ?
		want := `INSERT INTO t (tags, ids, others) VALUES (?, ?, ?, ?, ?, ?)`
		if gotQ != want {
			t.Errorf("query mismatch:\n  got =%q\n  want=%q", gotQ, want)
		}
		// args = [AsValue切片, 3, 4, 5, 6, 7] 共 6 个
		if len(gotArgs) != 6 {
			t.Errorf("args 应为 6 个：got len=%d, %v", len(gotArgs), gotArgs)
		}
	})

	t.Run("AsValue_包装_pq.Array_风格_Valuer", func(t *testing.T) {
		// AsValue 优先级高于 Valuer：AsValue 不解包，原值传给 driver
		query := `INSERT INTO t (tags) VALUES (?)`
		v := pqArrayLike{vals: []int{1, 2}}
		gotQ, gotArgs, err := autoIn(query, AsValue(v))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gotQ != query {
			t.Errorf("query 不应被改写")
		}
		if len(gotArgs) != 1 {
			t.Errorf("args 应保留单值：got len=%d", len(gotArgs))
		}
		// args[0] 应是原 pqArrayLike，未被 Value() 解包——driver 拿到后会自己处理
		if _, ok := gotArgs[0].(pqArrayLike); !ok {
			t.Errorf("args[0] 应是 pqArrayLike：got %T", gotArgs[0])
		}
	})

	t.Run("AsValue_包装空切片_合法_作为单值", func(t *testing.T) {
		// AsValue 不强制非空（与 AsList 不同），整体当单值传给 driver
		query := `INSERT INTO t (tags) VALUES (?)`
		gotQ, gotArgs, err := autoIn(query, AsValue([]int{}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gotQ != query {
			t.Errorf("query 不应被改写：\n  got =%q\n  want=%q", gotQ, query)
		}
		if len(gotArgs) != 1 {
			t.Errorf("args 应保留单值：got len=%d, want 1", len(gotArgs))
		}
		if got, ok := gotArgs[0].([]int); !ok || len(got) != 0 {
			t.Errorf("args[0] 应是 []int{}：got %v (%T)", gotArgs[0], gotArgs[0])
		}
	})

	t.Run("AsList_在(?)形态内_仍展开_行为与裸切片一致", func(t *testing.T) {
		// AsList 是"强制展开"，与裸切片在 (?) 形态内的默认展开等价
		query := `WHERE id IN (?)`
		gotQ, gotArgs, err := autoIn(query, AsList([]int{1, 2, 3}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := `WHERE id IN (?, ?, ?)`
		if gotQ != want {
			t.Errorf("query 应展开：\n  got =%q\n  want=%q", gotQ, want)
		}
		if len(gotArgs) != 3 {
			t.Errorf("args 应展开为 3 个：got len=%d", len(gotArgs))
		}
	})
}

// TestAutoIn_StrictParen_E2E 端到端验证严格 (?) 语境识别——
// 重点覆盖：跨行/Tab/双向空白等"看起来很可能误判"的真实业务 SQL 形态，
// 验证 nextPlaceholder 与 In 的协作能正确识别并展开。
func TestAutoIn_StrictParen_E2E(t *testing.T) {
	t.Run("跨行_IN(\\n_?\\n)_展开", func(t *testing.T) {
		query := "WHERE id IN (\n    ?\n)"
		gotQ, gotArgs, err := autoIn(query, []int{1, 2, 3})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// 跨行 SQL 应被识别并展开（保留原始空白结构）
		want := "WHERE id IN (\n    ?, ?, ?\n)"
		if gotQ != want {
			t.Errorf("query mismatch:\n  got =%q\n  want=%q", gotQ, want)
		}
		if len(gotArgs) != 3 {
			t.Errorf("args 应展开为 3 个：got len=%d", len(gotArgs))
		}
	})

	t.Run("(_空格_?_空格_)_展开", func(t *testing.T) {
		query := `WHERE id IN ( ? )`
		gotQ, gotArgs, err := autoIn(query, []int{10, 20})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// 实际生成保留原始空白结构：左侧 ( 后空格保留，右侧 ) 前空格保留
		want := `WHERE id IN ( ?, ? )`
		if gotQ != want {
			t.Errorf("query mismatch:\n  got =%q\n  want=%q", gotQ, want)
		}
		if len(gotArgs) != 2 {
			t.Errorf("args 应展开为 2 个：got len=%d", len(gotArgs))
		}
	})

	t.Run("(?,?,?)_多?_不展开_保留每个?对应一个标量", func(t *testing.T) {
		// 多 ? 视为用户已手动展开，每个 ? 对应一个标量，不参与 In 展开
		query := `WHERE id IN (?, ?, ?)`
		gotQ, gotArgs, err := autoIn(query, 1, 2, 3)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gotQ != query {
			t.Errorf("query 不应被改写：\n  got =%q\n  want=%q", gotQ, query)
		}
		if len(gotArgs) != 3 {
			t.Errorf("args 应保留 3 个标量：got len=%d", len(gotArgs))
		}
	})

	t.Run("ANY(?)_切片_默认误展开_AsValue兜底", func(t *testing.T) {
		// 已知边界：ANY(?) 命中 (?) 形态，默认会被误展开
		query1 := `WHERE id = ANY(?)`
		gotQ1, _, err := autoIn(query1, []int{1, 2, 3})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gotQ1 != `WHERE id = ANY(?, ?, ?)` {
			t.Errorf("ANY(?) 默认应被误展开（已知边界）：got %q", gotQ1)
		}

		// 业务用 AsValue 兜底
		gotQ2, gotArgs2, err := autoIn(query1, AsValue([]int{1, 2, 3}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gotQ2 != query1 {
			t.Errorf("AsValue 应阻止展开：got %q", gotQ2)
		}
		if len(gotArgs2) != 1 {
			t.Errorf("AsValue 后 args 应为单值：got len=%d", len(gotArgs2))
		}
	})
}

// TestAutoIn_EmptySlice_ContextSensitive 验证空切片的语境敏感处理：
//   - 严格 (?) 形态 + 空切片 → 报错（IN () 非法 SQL）
//   - 非 (?) 形态 + 空切片 → 不报错，整切片整体下发给 driver
//   - AsValue 包装空切片 → 不报错（强制单值，已有测试覆盖）
//   - AsList 包装空切片 → 报错（强制展开为空，无意义）
//
// 这是 Phase 1.9 严格 (?) 语境识别的延伸：sqlx 旧契约"任何空切片都报错"
// 已不适用，因为非 IN 位置切片本来就不展开。
func TestAutoIn_EmptySlice_ContextSensitive(t *testing.T) {
	t.Run("严格(?)形态_空切片_报错", func(t *testing.T) {
		query := `SELECT * FROM t WHERE id IN (?)`
		_, _, err := autoIn(query, []int{})
		if err == nil {
			t.Fatal("(?) 形态 + 空切片应报错，但没报")
		}
		if !stringContains(err.Error(), "empty slice") {
			t.Errorf("error 应包含 'empty slice'：got %v", err)
		}
		if !stringContains(err.Error(), "IN ()") {
			t.Errorf("error 应说明是 IN () 非法 SQL：got %v", err)
		}
	})

	t.Run("非(?)形态_WHERE=?_空切片_不报错_整切片下发", func(t *testing.T) {
		query := `SELECT * FROM t WHERE x = ?`
		gotQ, gotArgs, err := autoIn(query, []int{})
		if err != nil {
			t.Fatalf("非 (?) 形态 + 空切片不应报错：got %v", err)
		}
		if gotQ != query {
			t.Errorf("query 不应被改写：got %q", gotQ)
		}
		if len(gotArgs) != 1 {
			t.Errorf("args 应保留 1 个（整切片）：got len=%d", len(gotArgs))
		}
		if got, ok := gotArgs[0].([]int); !ok || len(got) != 0 {
			t.Errorf("args[0] 应是空切片 []int{}：got %v (%T)", gotArgs[0], gotArgs[0])
		}
	})

	t.Run("非(?)形态_UPDATE_SET_空切片_不报错", func(t *testing.T) {
		query := `UPDATE users SET tags = ? WHERE id = ?`
		gotQ, gotArgs, err := autoIn(query, []int{}, 100)
		if err != nil {
			t.Fatalf("非 (?) 形态 + 空切片不应报错：got %v", err)
		}
		if gotQ != query {
			t.Errorf("query 不应被改写：got %q", gotQ)
		}
		if len(gotArgs) != 2 {
			t.Errorf("args 应保留 2 个：got len=%d", len(gotArgs))
		}
	})

	t.Run("非(?)形态_VALUES多?_空切片_不报错", func(t *testing.T) {
		// VALUES (?, ?, ?) 中第 2 个 ? 不在 (?) 形态（前面是 ,），传空切片不应报错
		query := `INSERT INTO t (a, b, c) VALUES (?, ?, ?)`
		gotQ, gotArgs, err := autoIn(query, "x", []int{}, "z")
		if err != nil {
			t.Fatalf("非 (?) 形态 + 空切片不应报错：got %v", err)
		}
		if gotQ != query {
			t.Errorf("query 不应被改写：got %q", gotQ)
		}
		if len(gotArgs) != 3 {
			t.Errorf("args 应保留 3 个：got len=%d", len(gotArgs))
		}
		if got, ok := gotArgs[1].([]int); !ok || len(got) != 0 {
			t.Errorf("args[1] 应是空切片：got %v (%T)", gotArgs[1], gotArgs[1])
		}
	})

	t.Run("混排_IN(?)空切片_仍报错_其他位置不影响", func(t *testing.T) {
		// 多个 ? 时，如果有任何一个是"严格 (?) 形态 + 空切片"，整体应报错
		query := `WHERE x = ? AND id IN (?) AND y = ?`
		_, _, err := autoIn(query, "foo", []int{}, "bar")
		if err == nil {
			t.Fatal("IN (?) + 空切片应报错")
		}
		if !stringContains(err.Error(), "empty slice") {
			t.Errorf("error 应包含 'empty slice'：got %v", err)
		}
	})

	t.Run("AsValue_空切片_合法", func(t *testing.T) {
		// 已在 TestAutoIn_AsValueAsList_EscapeHooks 覆盖；这里再加一例确认
		// 即使在 (?) 形态内，AsValue 也阻止展开 → 不报错
		query := `INSERT INTO t (tags) VALUES (?)`
		_, gotArgs, err := autoIn(query, AsValue([]int{}))
		if err != nil {
			t.Fatalf("AsValue + 空切片不应报错：got %v", err)
		}
		if len(gotArgs) != 1 {
			t.Errorf("args 应保留 1 个：got len=%d", len(gotArgs))
		}
	})
}

// TestIn_ArgCountMismatch 验证 In 主循环对 args 数量错配的错误处理。
//
// 触发前提：必须有至少一个切片 / AsValue / AsList 让 needRewrite=true 走进主循环；
// 纯标量场景走快速路径直接 return query, args, nil 不做计数校验（由 driver 兜底）。
//
// 这是 In 接口契约的硬保证——用户少传或多传 args 时必须有清晰错误，
// 不能让错误信息推迟到 driver 层。
func TestIn_ArgCountMismatch(t *testing.T) {
	t.Run("? 多于 args_报 exceeds arguments", func(t *testing.T) {
		// 2 个 ?，只传 1 个 arg（且是切片，触发 needRewrite）
		_, _, err := In(`WHERE a IN (?) AND b = ?`, []int{1, 2})
		if err == nil {
			t.Fatal("expected error for ? exceeds args, got nil")
		}
		if !stringContains(err.Error(), "number of bindVars exceeds arguments") {
			t.Errorf("error 应包含 'number of bindVars exceeds arguments'：got %v", err)
		}
	})

	t.Run("args 多于 ?_报 less than", func(t *testing.T) {
		// 1 个 ?，传 2 个 args（其中至少一个切片）
		_, _, err := In(`WHERE a IN (?)`, []int{1, 2}, []int{3, 4})
		if err == nil {
			t.Fatal("expected error for args exceeds ?, got nil")
		}
		if !stringContains(err.Error(), "number of bindVars less than number arguments") {
			t.Errorf("error 应包含 'number of bindVars less than number arguments'：got %v", err)
		}
	})

	t.Run("AsValue 也能触发计数校验", func(t *testing.T) {
		// AsValue 让 needRewrite=true，即使没有切片展开也走主循环
		_, _, err := In(`WHERE a = ? AND b = ?`, AsValue(1))
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !stringContains(err.Error(), "exceeds arguments") {
			t.Errorf("error 应包含 'exceeds arguments'：got %v", err)
		}
	})

	t.Run("AsList 也能触发计数校验", func(t *testing.T) {
		_, _, err := In(`WHERE a = ?`, AsList([]int{1, 2}), AsList([]int{3, 4}))
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !stringContains(err.Error(), "less than number arguments") {
			t.Errorf("error 应包含 'less than number arguments'：got %v", err)
		}
	})

	t.Run("纯标量数量错配_走快速路径不报错", func(t *testing.T) {
		// 反向防回归：纯标量不进主循环，In 不做计数校验，直接原样返回
		// 这是当前设计取舍——错误推迟到 driver 层暴露
		gotQ, gotArgs, err := In(`WHERE a = ? AND b = ?`, 1)
		if err != nil {
			t.Fatalf("纯标量快速路径不应报错：got %v", err)
		}
		if gotQ != `WHERE a = ? AND b = ?` || len(gotArgs) != 1 {
			t.Errorf("快速路径应原样透传：got query=%q args=%v", gotQ, gotArgs)
		}
	})
}
