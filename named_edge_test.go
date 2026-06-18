package sqlex

import (
	"reflect"
	"testing"
)

// TestCompileNamedQuery_EdgeCases verifies compileNamedQuery behavior across SQL lexical elements.
// Focus: :name inside string literals, double-quoted identifiers, backtick identifiers,
// dollar-quoted strings, line comments, and block comments must NOT be parsed as named parameters.
func TestCompileNamedQuery_EdgeCases(t *testing.T) {
	cases := []struct {
		name         string
		query        string
		bindType     int
		wantBound    string
		wantNames    []string
		wantErrMsg   string // 非空表示期望错误（substring 匹配）
		skipBoundCmp bool   // 仅校验 names，不校验 bound（极端动态场景）
	}{
		// ===== A. 字符串字面量 =====
		{
			name:  "A1_单引号字符串内的:name不被识别",
			query: `SELECT 'hello :world' AS x WHERE id = :id`, bindType: DOLLAR,
			wantBound: `SELECT 'hello :world' AS x WHERE id = $1`,
			wantNames: []string{"id"},
		},
		{
			name: "A2_SQL转义引号", query: `SELECT 'O''Reilly :fake' WHERE id = :id`, bindType: DOLLAR,
			wantBound: `SELECT 'O''Reilly :fake' WHERE id = $1`,
			wantNames: []string{"id"},
		},
		{
			name:  "A3_PG双引号标识符内含冒号",
			query: `SELECT "col:name" FROM t WHERE id = :id`, bindType: DOLLAR,
			wantBound: `SELECT "col:name" FROM t WHERE id = $1`,
			wantNames: []string{"id"},
		},
		{
			name:  "A4_PG双引号转义",
			query: `SELECT "a""b:fake" FROM t WHERE id = :id`, bindType: DOLLAR,
			wantBound: `SELECT "a""b:fake" FROM t WHERE id = $1`,
			wantNames: []string{"id"},
		},
		{
			name:      "A5_行注释内:name",
			query:     "SELECT * FROM t -- :fake\nWHERE id = :id",
			bindType:  DOLLAR,
			wantBound: "SELECT * FROM t -- :fake\nWHERE id = $1",
			wantNames: []string{"id"},
		},
		{
			name:  "A6_块注释内:name",
			query: `SELECT * FROM t /* :fake */ WHERE id = :id`, bindType: DOLLAR,
			wantBound: `SELECT * FROM t /* :fake */ WHERE id = $1`,
			wantNames: []string{"id"},
		},
		{
			name:      "A7_块注释跨多行",
			query:     "SELECT * FROM t /* line1\n:fake\nline2 */ WHERE id = :id",
			bindType:  DOLLAR,
			wantBound: "SELECT * FROM t /* line1\n:fake\nline2 */ WHERE id = $1",
			wantNames: []string{"id"},
		},

		// ===== B. PG 类型转换 :: 与赋值 := =====
		{
			name: "B1_单层类型转换", query: `SELECT :id::int`, bindType: DOLLAR,
			wantBound: `SELECT $1::int`, wantNames: []string{"id"},
		},
		{
			name: "B2_双层类型转换", query: `SELECT :id::text::varchar`, bindType: DOLLAR,
			wantBound: `SELECT $1::text::varchar`, wantNames: []string{"id"},
		},
		{
			// := 赋值操作符原样保留，不识别为命名参数；冒号后紧跟参数才识别（:total）
			name:  "B3_赋值操作符:=原样保留",
			query: `SELECT @x := :total`, bindType: DOLLAR,
			wantBound: `SELECT @x := $1`, wantNames: []string{"total"},
		},
		{
			name:  "B4_:name::type在末尾",
			query: `SELECT * FROM t WHERE x = :id::int`, bindType: DOLLAR,
			wantBound: `SELECT * FROM t WHERE x = $1::int`,
			wantNames: []string{"id"},
		},
		{
			name:  "B5_两个:name连续",
			query: `SELECT * FROM t WHERE a = :a OR b = :b`, bindType: DOLLAR,
			wantBound: `SELECT * FROM t WHERE a = $1 OR b = $2`,
			wantNames: []string{"a", "b"},
		},

		// ===== C. 命名参数字符规则 =====
		{
			// sqlex 不支持 Unicode 命名参数名（行为变更，见 compileNamedQuery 注释）：
			// :名字 的首字符是中文，非 ASCII bindStart，整个 :名字 原样保留不识别为参数。
			name:  "C5_Unicode命名参数_不识别_原样保留",
			query: `SELECT * WHERE name = :名字`, bindType: DOLLAR,
			wantBound: `SELECT * WHERE name = :名字`, wantNames: []string{},
		},
		{
			name: "C6_数字结尾命名参数", query: `SELECT * WHERE x = :arg1`, bindType: DOLLAR,
			wantBound: `SELECT * WHERE x = $1`, wantNames: []string{"arg1"},
		},
		{
			name: "C7_下划线命名参数", query: `SELECT * WHERE x = :user_id`, bindType: DOLLAR,
			wantBound: `SELECT * WHERE x = $1`, wantNames: []string{"user_id"},
		},
		{
			name: "C8_点号嵌套字段", query: `SELECT * WHERE x = :user.id`, bindType: DOLLAR,
			wantBound: `SELECT * WHERE x = $1`, wantNames: []string{"user.id"},
		},
		{
			// 修复：数字开头不再被识别为参数名（旧实现 :123 → names=[123]）。
			// 参数名首字符必须是字母或下划线，:123 原样保留。
			name:  "C8b_数字开头_不识别为参数",
			query: `SELECT :123 FROM t`, bindType: DOLLAR,
			wantBound: `SELECT :123 FROM t`, wantNames: []string{},
		},
		{
			name:  "C9_:后跟空格_保留原样",
			query: `SELECT : FROM t`, bindType: DOLLAR,
			wantBound: `SELECT : FROM t`, wantNames: []string{},
		},
		{
			name:  "C10_query末尾单冒号_在字符串内",
			query: `SELECT * FROM t WHERE x = 'a:'`, bindType: DOLLAR,
			wantBound: `SELECT * FROM t WHERE x = 'a:'`, wantNames: []string{},
		},
		{
			// 行为变更：旧实现 :a:b 报错 "unexpected :"，新实现把它解析为
			// 两个独立命名参数 :a 和 :b（读完 :a 遇到 : 结束，: 后是 b 即新参数）。
			// 这更宽容，且与"参数名按 ASCII 规则贪婪读取"的简单模型一致。
			name:  "C12_两个连续命名参数_:a:b_解析为两个参数",
			query: `SELECT :a:b`, bindType: DOLLAR,
			wantBound: `SELECT $1$2`, wantNames: []string{"a", "b"},
		},

		// ===== D. 字符串内伪注释 =====
		{
			name:  "D1_字符串内含dash_dash_不当作行注释",
			query: `SELECT '-- not comment :fake' AS x WHERE id = :id`, bindType: DOLLAR,
			wantBound: `SELECT '-- not comment :fake' AS x WHERE id = $1`,
			wantNames: []string{"id"},
		},
		{
			name:  "D2_字符串内含slash_star_不当作块注释",
			query: `SELECT '/* not comment :fake */' AS x WHERE id = :id`, bindType: DOLLAR,
			wantBound: `SELECT '/* not comment :fake */' AS x WHERE id = $1`,
			wantNames: []string{"id"},
		},

		// ===== E. 多语句 =====
		{
			name:  "E1_多条语句",
			query: `INSERT INTO t (a) VALUES (:a); INSERT INTO t (b) VALUES (:b)`, bindType: DOLLAR,
			wantBound: `INSERT INTO t (a) VALUES ($1); INSERT INTO t (b) VALUES ($2)`,
			wantNames: []string{"a", "b"},
		},

		// ===== F. 同名复用 =====
		{
			name:  "F1_同名:id出现两次_应分别绑定",
			query: `SELECT * WHERE a = :id OR b = :id`, bindType: DOLLAR,
			wantBound: `SELECT * WHERE a = $1 OR b = $2`,
			wantNames: []string{"id", "id"},
		},
		{
			name: "F2_同名:id出现三次", query: `SELECT :id, :id, :id`, bindType: DOLLAR,
			wantBound: `SELECT $1, $2, $3`,
			wantNames: []string{"id", "id", "id"},
		},

		// ===== G. PostgreSQL Dollar Quoting（本次修复点）=====
		{
			name:  "G1_dollar_quote_内的:fake不被识别",
			query: `SELECT $$hello :fake$$ WHERE id = :id`, bindType: DOLLAR,
			wantBound: `SELECT $$hello :fake$$ WHERE id = $1`,
			wantNames: []string{"id"},
		},
		{
			name:  "G2_tagged_dollar_quote",
			query: `SELECT $tag$:fake$tag$ WHERE id = :id`, bindType: DOLLAR,
			wantBound: `SELECT $tag$:fake$tag$ WHERE id = $1`,
			wantNames: []string{"id"},
		},
		{
			name:  "G3_dollar_quote_内含跨行内容",
			query: "SELECT $$line1\n:fake\nline2$$ WHERE id = :id", bindType: DOLLAR,
			wantBound: "SELECT $$line1\n:fake\nline2$$ WHERE id = $1",
			wantNames: []string{"id"},
		},
		{
			name:  "G4_dollar_quote_内含特殊字符_引号",
			query: `SELECT $$it's a "test" :fake$$ WHERE id = :id`, bindType: DOLLAR,
			wantBound: `SELECT $$it's a "test" :fake$$ WHERE id = $1`,
			wantNames: []string{"id"},
		},
		{
			name:  "G5_单独的$不是dollar_quoting",
			query: `SELECT 100$ FROM t WHERE id = :id`, bindType: DOLLAR,
			wantBound: `SELECT 100$ FROM t WHERE id = $1`,
			wantNames: []string{"id"},
		},

		// ===== H. MySQL 反引号标识符（本次修复点）=====
		{
			name:      "H1_反引号标识符内含冒号_不被识别",
			query:     "SELECT `col:fake` FROM t WHERE id = :id",
			bindType:  QUESTION,
			wantBound: "SELECT `col:fake` FROM t WHERE id = ?",
			wantNames: []string{"id"},
		},
		{
			name:      "H2_反引号转义",
			query:     "SELECT `a``b:fake` FROM t WHERE id = :id",
			bindType:  QUESTION,
			wantBound: "SELECT `a``b:fake` FROM t WHERE id = ?",
			wantNames: []string{"id"},
		},
		{
			name:      "H3_中文表/列名",
			query:     `SELECT * FROM 用户表 WHERE 名字 = :name`,
			bindType:  DOLLAR,
			wantBound: `SELECT * FROM 用户表 WHERE 名字 = $1`,
			wantNames: []string{"name"},
		},
		{
			name:      "H4_:在最末尾_保留原样",
			query:     `SELECT * FROM t WHERE x = :`,
			bindType:  DOLLAR,
			wantBound: `SELECT * FROM t WHERE x = :`,
			wantNames: []string{},
		},

		// ===== I. NAMED 类型 =====
		{
			name:      "I1_NAMED类型保留:name",
			query:     `SELECT * WHERE id = :id AND name = :name`,
			bindType:  NAMED,
			wantBound: `SELECT * WHERE id = :id AND name = :name`,
			wantNames: []string{"id", "name"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotBound, gotNames, err := compileNamedQuery([]byte(c.query), c.bindType)

			if c.wantErrMsg != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", c.wantErrMsg)
				}
				if !contains(err.Error(), c.wantErrMsg) {
					t.Errorf("error message = %q, want substring %q", err.Error(), c.wantErrMsg)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !c.skipBoundCmp && gotBound != c.wantBound {
				t.Errorf("bound mismatch:\n  query=%q\n  got =%q\n  want=%q",
					c.query, gotBound, c.wantBound)
			}
			// names 比较：[]string{} 与 nil 视为相等
			if !equalStrSlice(gotNames, c.wantNames) {
				t.Errorf("names mismatch:\n  query=%q\n  got =%v\n  want=%v",
					c.query, gotNames, c.wantNames)
			}
		})
	}
}

// TestMissingNames_EdgeCases 验证 bindStruct / bindMap 在容错场景的正确性：
// 缺失参数原样保留为 :name 字面量、剩余参数正确编号、arglist 与 bound 一致。
func TestMissingNames_EdgeCases(t *testing.T) {
	type args struct {
		Foo int `db:"foo"`
		Bar int `db:"bar"`
	}

	t.Run("Struct_部分缺失_:baz保持字面量", func(t *testing.T) {
		query := `SELECT a = :foo, b = :bar, c = :baz`
		bound, gotArgs, err := bindStruct(QUESTION, query, args{Foo: 1, Bar: 2}, mapper())
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		want := `SELECT a = ?, b = ?, c = :baz`
		if bound != want {
			t.Errorf("bound = %q, want %q", bound, want)
		}
		if !reflect.DeepEqual(gotArgs, []any{1, 2}) {
			t.Errorf("args = %v, want [1 2]", gotArgs)
		}
	})

	t.Run("Struct_全部缺失_全部保持字面量", func(t *testing.T) {
		query := `SELECT :x, :y, :z`
		bound, gotArgs, err := bindStruct(QUESTION, query, args{}, mapper())
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if bound != query {
			t.Errorf("bound = %q, want unchanged %q", bound, query)
		}
		if len(gotArgs) != 0 {
			t.Errorf("args = %v, want []", gotArgs)
		}
	})

	t.Run("Map_部分缺失_DOLLAR编号紧凑", func(t *testing.T) {
		query := `SELECT a = :foo, b = :bar, c = :baz`
		bound, gotArgs, err := bindMap(DOLLAR, query, map[string]any{"foo": 1})
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		// :foo 编号为 $1，:bar/:baz 缺失保持字面量
		want := `SELECT a = $1, b = :bar, c = :baz`
		if bound != want {
			t.Errorf("bound = %q, want %q", bound, want)
		}
		if !reflect.DeepEqual(gotArgs, []any{1}) {
			t.Errorf("args = %v, want [1]", gotArgs)
		}
	})

	t.Run("Map_空_全部缺失", func(t *testing.T) {
		query := `SELECT :a, :b`
		bound, gotArgs, err := bindMap(QUESTION, query, map[string]any{})
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if bound != query {
			t.Errorf("bound = %q, want unchanged %q", bound, query)
		}
		if len(gotArgs) != 0 {
			t.Errorf("args = %v, want []", gotArgs)
		}
	})

	t.Run("BindStruct_重复名字+部分缺失", func(t *testing.T) {
		// :foo 出现两次（存在），:bar 出现一次（缺失）
		query := `SELECT * WHERE a = :foo OR b = :foo OR c = :bar`
		bound, gotArgs, err := bindStruct(DOLLAR, query, struct {
			Foo int `db:"foo"`
		}{Foo: 42}, mapper())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		wantBound := `SELECT * WHERE a = $1 OR b = $2 OR c = :bar`
		if bound != wantBound {
			t.Errorf("bound = %q, want %q", bound, wantBound)
		}
		if !reflect.DeepEqual(gotArgs, []any{42, 42}) {
			t.Errorf("args = %v, want [42 42]", gotArgs)
		}
	})

	t.Run("BindMap_重复名字+部分缺失", func(t *testing.T) {
		query := `SELECT * WHERE a = :foo OR b = :foo OR c = :bar`
		bound, gotArgs, err := bindMap(DOLLAR, query, map[string]any{"foo": 42})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		wantBound := `SELECT * WHERE a = $1 OR b = $2 OR c = :bar`
		if bound != wantBound {
			t.Errorf("bound = %q, want %q", bound, wantBound)
		}
		if !reflect.DeepEqual(gotArgs, []any{42, 42}) {
			t.Errorf("args = %v, want [42 42]", gotArgs)
		}
	})

	t.Run("BindMap_全部缺失_保留全部:name", func(t *testing.T) {
		query := `SELECT :a, :b, :c`
		bound, gotArgs, err := bindMap(DOLLAR, query, map[string]any{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if bound != query {
			t.Errorf("bound = %q, want %q (unchanged)", bound, query)
		}
		if len(gotArgs) != 0 {
			t.Errorf("args = %v, want []", gotArgs)
		}
	})
}

// TestBindArray_EdgeCases — bindArray 切片 INSERT 场景
func TestBindArray_EdgeCases(t *testing.T) {
	type Row struct {
		Foo int `db:"foo"`
	}

	t.Run("Slice_of_struct", func(t *testing.T) {
		query := `INSERT INTO t (foo) VALUES (:foo)`
		bound, gotArgs, err := bindArray(QUESTION, query, []Row{{1}, {2}, {3}}, mapper())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := `INSERT INTO t (foo) VALUES (?),(?),(?)`
		if bound != want {
			t.Errorf("bound = %q, want %q", bound, want)
		}
		if !reflect.DeepEqual(gotArgs, []any{1, 2, 3}) {
			t.Errorf("args = %v, want [1 2 3]", gotArgs)
		}
	})

	t.Run("Empty_slice_应报错", func(t *testing.T) {
		query := `INSERT INTO t (foo) VALUES (:foo)`
		_, _, err := bindArray(QUESTION, query, []Row{}, mapper())
		if err == nil {
			t.Fatal("expected error for empty slice, got nil")
		}
		if !contains(err.Error(), "length of array is 0") {
			t.Errorf("err = %v, want contain 'length of array is 0'", err)
		}
	})
}

// 抑制 unused 警告
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		(len(s) > len(substr) && stringContains(s, substr)))
}

func stringContains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func equalStrSlice(a, b []string) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	return reflect.DeepEqual(a, b)
}
