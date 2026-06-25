[English](README.md) | **中文**

[![CI](https://github.com/go-sqlex/sqlex/actions/workflows/ci.yml/badge.svg)](https://github.com/go-sqlex/sqlex/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/go-sqlex/sqlex)](https://goreportcard.com/report/github.com/go-sqlex/sqlex)
[![GoDoc](https://pkg.go.dev/badge/github.com/go-sqlex/sqlex)](https://pkg.go.dev/github.com/go-sqlex/sqlex)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Mentioned in Awesome Go](https://awesome.re/mentioned-badge.svg)](https://github.com/avelino/awesome-go)

# sqlex

> [jmoiron/sqlx](https://github.com/jmoiron/sqlx) 的**即插即用替代品** — 100% API 兼容，新增 Hook 切面、JSON 泛型类型、Bug 修复等实用功能。

**与 sqlx 完全 API 兼容。** 所有 sqlx 方法（`Get`、`Select`、`Exec`、`NamedQuery`、`Preparex` 等）行为完全一致。迁移只需 30 秒 — 换个 import 路径即可。新功能纯增量，完全可选。

```diff
- import "github.com/jmoiron/sqlx"
+ import "github.com/go-sqlex/sqlex"
```

迁移后即可免费获得：

- 🚀 **自动 Rebind** — 统一 `?` 写 SQL，PostgreSQL 自动转 `$1`、MySQL/SQLite 保持 `?`、SQL Server 转 `@p1`。无需手动 `db.Rebind()`，包括 `Preparex`。
- 🐛 **SQL 解析修复** — 字符串中的冒号、`::` 类型转换、注释中的 `?` 正确跳过。sqlx 的隐蔽解析 bug 全部修复。
- 🎯 **统一接口** — `Ext` / `ExtContext` / `NamedExt` / `BindExt` / `Preparer` / `PreparerContext`，编译期校验。写 `func f(ext NamedExt)` 即可接受 DB、Tx 或 Conn。
- 🔀 **IN 子句自动展开** — 切片参数在 `IN (?)` 中自动检测并展开，覆盖所有方法。
- 🪝 **Hook 系统** — 可插拔 SQL 拦截器，用于日志、追踪、指标（洋葱模型）。
- 📦 **JSONValue[T]** — 泛型 JSON 列类型，自动序列化/反序列化。
- 🛡️ **StrictMode** — 默认宽松（与 sqlx `Unsafe()` 一致），可选开启严格模式辅助调试。

→ [迁移指南](#从-jmoironsqlx-迁移)

## 安装

```bash
go get github.com/go-sqlex/sqlex
```

要求 Go 1.21 或更高版本。

## 从 jmoiron/sqlx 迁移

**30 秒，3 步：**

**1. 变更导入路径：**

```go
// 旧
import "github.com/jmoiron/sqlx"

// 新
import "github.com/go-sqlex/sqlex"
```

**2. 变更包名引用：**

```go
// 旧
db, err := sqlx.Connect("postgres", dsn)

// 新
db, err := sqlex.Connect("postgres", dsn)
```

**3. 更新 go.mod：**

```bash
go get github.com/go-sqlex/sqlex
```

**搞定。** 你现有的所有 sqlx 代码无需任何改动即可运行。

> **关于 StrictMode**：sqlex 默认宽松模式（`strict=false`），与 sqlx 的 `db.Unsafe()` 行为一致（静默忽略多余列）。你的代码中用了 `db.Unsafe()`？无需改动 — sqlex 继承了相同的宽松默认值。如需在调试时启用严格结构体字段匹配，调用 `db.SetStrict(true)` 即可。

### 渐进式采用

新功能完全可选，可按自己的节奏逐步采用：

| 步骤 | 操作 | 耗时 |
|------|------|------|
| 1 | 替换 import 路径 | 30s |
| 2 | 将事务代码改为 `CloseWithErr` 模式 | 按需 |
| 3 | 使用 `NamedGet`/`NamedSelect` 替代 `NamedQuery` + 手动扫描 | 按需 |
| 4 | 按需注册自定义 Hook（日志、追踪、指标等） | 按需 |

## 快速开始

```go
package main

import (
    "fmt"
    "log"

    "github.com/go-sqlex/sqlex"
    _ "github.com/mattn/go-sqlite3"
)

type User struct {
    ID    int    `db:"id"`
    Name  string `db:"name"`
    Email string `db:"email"`
}

func main() {
    // 连接数据库
    db, err := sqlex.Connect("sqlite3", ":memory:")
    if err != nil {
        log.Fatal(err)
    }
    defer db.Close()

    // 建表
    db.MustExec(`CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT, email TEXT)`)
    db.MustExec(`INSERT INTO users (name, email) VALUES ('Alice', 'alice@example.com')`)
    db.MustExec(`INSERT INTO users (name, email) VALUES ('Bob', 'bob@example.com')`)

    // 查询单行
    var user User
    err = db.Get(&user, "SELECT * FROM users WHERE id = ?", 1)
    fmt.Printf("User: %+v\n", user)

    // 查询多行
    var users []User
    err = db.Select(&users, "SELECT * FROM users")
    fmt.Printf("Users: %+v\n", users)

}
```

## 新增功能

sqlex 保留所有 sqlx API，并新增以下能力：

| 功能 | 说明 |
|------|------|
| **Hook 切面** | `AddHook` — 可插拔 SQL 执行拦截器（洋葱模型） |
| **JSONValue[T]** | `types.JSONValue[T]` — 泛型 JSON 列类型 |
| **NamedGet/NamedSelect** | DB/Tx 上的命名参数便捷查询方法（内置 IN 展开） |
| **CloseWithErr** | 根据 error 自动 Commit/Rollback |
| **统一接口** | `Ext` / `ExtContext` / `NamedExt` / `BindExt` / `Preparer` / `PreparerContext` — DB、Tx、Conn 共享完全一致的方法签名，编译期校验 |
| **自动 IN 展开** | 所有方法自动检测切片参数并展开 IN 子句 |
| **自动 Rebind** | 所有查询方法自动将 `?` 转换为目标数据库占位符 |
| **StrictMode 严格模式** | 可选的严格结构体字段匹配（默认关闭） |
| **跨数据库开箱即用** | 统一用 `?` 写 SQL — PostgreSQL、MySQL、SQLite、SQL Server 通吃 |

## 使用示例

### 基础 CRUD

```go
// 统一使用 ? 占位符，无论底层是 MySQL、PostgreSQL 还是其他数据库
// 框架自动将 ? 转换为目标数据库的绑定变量格式（$N、:argN、@pN）

// 插入
result, err := db.Exec("INSERT INTO users (name, email) VALUES (?, ?)", "Alice", "alice@example.com")

// 查询单行 → 结构体
var user User
err = db.Get(&user, "SELECT * FROM users WHERE id = ?", 1)

// 查询多行 → 切片
var users []User
err = db.Select(&users, "SELECT * FROM users WHERE age > ?", 18)

// 更新
_, err = db.Exec("UPDATE users SET name = ? WHERE id = ?", "Alice Updated", 1)

// 删除
_, err = db.Exec("DELETE FROM users WHERE id = ?", 1)
```

### 命名参数查询

```go
// 使用结构体作为参数
user := User{Name: "Alice", Email: "alice@example.com"}
_, err = db.NamedExec(`INSERT INTO users (name, email) VALUES (:name, :email)`, user)

// 使用 map 作为参数
params := map[string]any{"name": "Alice"}

// NamedGet — 查询单行
var result User
err = db.NamedGet(&result, `SELECT * FROM users WHERE name = :name`, params)

// NamedSelect — 查询多行
var results []User
err = db.NamedSelect(&results, `SELECT * FROM users WHERE name = :name`, params)

// NamedQuery — 返回 *Rows 供手动遍历
rows, err := db.NamedQuery(`SELECT * FROM users WHERE name = :name`, params)
defer rows.Close()
for rows.Next() {
    var u User
    rows.StructScan(&u)
}
```

### IN 查询

```go
ids := []int{1, 2, 3, 4, 5}

// 位置参数：Select / Get / Exec / Queryx / QueryRowx / MustExec 全路径自动检测切片并展开
var users []User
err = db.Select(&users, "SELECT * FROM users WHERE id IN (?)", ids)

// 命名参数：NamedSelect / NamedExec / NamedQuery / NamedGet 全路径内置 IN 展开
err = db.NamedSelect(&users,
    `SELECT * FROM users WHERE id IN (:ids) AND status = :status`,
    map[string]any{"ids": ids, "status": "active"})

_, err = db.NamedExecContext(ctx,
    `DELETE FROM users WHERE id IN (:ids)`,
    map[string]any{"ids": ids})
```

> 注：`sqlex.In()` / `sqlex.Named()` 是历史顶层函数，框架内部已自动调用，业务代码**无需手动调用**——直接用上面的高阶方法即可，自动包含 Rebind/Hook/StrictMode 等切面。

#### ⚠️ 切片参数处理规则（严格 `(?)` 语境识别）

sqlex 采用**严格 `(?)` 语境识别**判断是否自动展开切片，须同时满足两个条件：

1. **严格 `(?)` 形态**：`(` 与 `)` 之间只有一个 `?` 和可选 ASCII 空白（空格/Tab/换行/回车）
2. **`(` 前紧邻的完整标识符是 `IN`**（大小写不敏感）；`NOT IN (?)` 同样生效

其他 `(?)` 语境（`ANY(?)` / `VALUES (?)` / `func(?)` / 标量子查询 `= (?)` 等）均视为单值——**无需 `AsValue` 兜底**。

**判定规则**：

| SQL 形态 | 参数 | 行为 | 说明 |
|---|---|---|---|
| `WHERE id IN (?)` | `[]int{1,2,3}` | ✅ 展开 | 严格 `(?)` 形态命中 |
| `WHERE id IN (\n  ?\n)` | `[]int{1,2,3}` | ✅ 展开 | 跨行也是 `(?)` 形态 |
| `IN (?, ?, ?)` | `1, 2, 3` 标量 | 不展开 | 多 `?` → 视为用户已手动展开 |
| `WHERE x = ?` | `[]int{1,2,3}` | 不展开（按单值） | `?` 不在 `(?)` 形态内 |
| `(? + 1)` | 标量 | 不展开 | 算术表达式，不是 IN 列表 |
| `(SELECT ?)` | 标量 | 不展开 | `?` 前有字母，非 `(?)` 形态 |

**Escape hatch APIs**（一般不需要，仅边界场景使用）：

```go
import "github.com/go-sqlex/sqlex"

// ① sqlex.AsValue(v) — 强制不展开
db.Select(&rows, `SELECT * FROM t WHERE id IN (?)`,
    sqlex.AsValue([]int{1, 2, 3})) // 整个切片当单值传给 driver

// ② sqlex.AsList(slice) — 强制展开
db.Select(&rows, `SELECT * FROM t WHERE id = ANY(?)`,
    sqlex.AsList([]int{1, 2, 3})) // 强制展开为 ?, ?, ?

// ③ 其他原生方式仍然有效
db.Exec(`INSERT INTO users (tags) VALUES (?)`, pq.Array([]int{1, 2, 3})) // driver.Valuer 接口
data, _ := json.Marshal([]int{1, 2, 3})
db.Exec(`INSERT INTO t (json_col) VALUES (?)`, data) // []byte 是 driver 标准类型
```

> 注：`ANY(?)` / `VALUES (?)` / `func(?)` 等默认**不展开**，直接传切片或用 `pq.Array` 即可，无需 `AsValue`。

**优先级**（从高到低）：

1. `sqlex.AsValue(v)` / `sqlex.AsList(s)` — 显式声明，最高优先级
2. `driver.Valuer` 接口（包括 `pq.Array`）— 整体当单值
3. `[]byte` — driver 标准类型，整体当单值
4. 严格 `(?)` 形态命中 + 切片 — 自动展开
5. 其他位置 + 切片 — 不展开，作为单值传给 driver（多数情况下 driver 会报类型错）

**空切片的处理**（语境敏感）：

| 场景 | 行为 |
|---|---|
| `IN (?)` 严格 (?) 形态 + `[]int{}` | ❌ 报错 `sqlex: empty slice cannot be expanded into IN ()`（IN () 非法 SQL） |
| `WHERE x = ?` / `SET col = ?` 等非 (?) 形态 + `[]int{}` | ✅ 不报错，整切片整体下发给 driver（driver 决定兼容性） |
| `sqlex.AsValue([]int{})` 强制单值 | ✅ 不报错（已是单值语义） |
| `sqlex.AsList([]int{})` 强制展开 | ❌ 报错 `sqlex.AsList: empty slice`（强制展开为空无意义） |

#### 命名参数名规则与词法边界

命名参数 `:name` 的 `name` 规则：`[A-Za-z_][A-Za-z0-9_.]*`（首字符字母/下划线，后续可含数字、下划线、点号；点号用于嵌套字段 `:user.name`）。

| 形态 | 是否识别为参数 | 说明 |
|---|---|---|
| `:name` / `:user_id` / `:arg1` | ✅ | 标准命名参数 |
| `:user.name` | ✅ | 点号嵌套字段 |
| `:123` / `:1` | ❌ 原样保留 | 数字开头不识别——符合标识符规范，避免与 Oracle `:N` / SQLite `?NNN` 位置占位符冲突 |
| `:名字`（Unicode） | ❌ 原样保留 | sqlex 不支持 Unicode 参数名（参数名对应 db tag/map key，几乎全是 ASCII） |
| `::int`（PG 类型转换） | ❌ 原样保留 | `::` 被识别为类型转换，不当参数 |
| `:=`（赋值操作符） | ❌ 原样保留 | 原样输出 |

**词法跳过**：以下区域内的 `:name` / `?` 不被识别为占位符（`Rebind` / `In` / `compileNamedQuery` 共用 `lexer.go` 统一扫描器）：

- 单引号字符串 `'...'`（含 `''` 转义）、双引号标识符 `"..."`、反引号标识符 `` `...` ``
- dollar-quoted string `$$...$$` / `$tag$...$tag$`
- 行注释 `-- ...`、块注释 `/* ... */`

**已知词法边界**（罕见场景，与 PostgreSQL 标准的差异）：

- 块注释**不支持嵌套**（PG 支持 `/* /* */ */`，遇首个 `*/` 即结束）
- 单引号字符串只识别 SQL 标准 `''` 转义，不识别 MySQL 反斜杠转义 `\'`（需 `standard_conforming_strings=off` 才生效，PG 9.1+ 默认 on）

> 这些边界场景若触发误判，`compileNamedQuery` 容错路径会把 args 中不存在的 `:name` 原样保留为字面量（编译期一次成型），让原始 SQL 仍可能被 driver/server 正确执行。行为与 GORM 的 `@name` 处理一致。

### 预编译语句

```go
// Preparex 预编译语句 — 统一使用 ? 占位符
// Preparex 自动将 ? 转换为目标数据库的绑定变量格式（PostgreSQL 的 $N、Oracle 的 :argN 等），
// 与其他查询方法保持一致，无需关心底层数据库差异。

// MySQL、PostgreSQL、SQLite — 统一使用 ?
stmt, err := db.Preparex("SELECT * FROM users WHERE name = ?")
defer stmt.Close() // 预编译语句用完必须 Close，避免资源泄漏
var user User
err = stmt.Get(&user, "Alice")

// 事务中同样使用统一占位符
tx, _ := db.Beginx()
stmt, err = tx.Preparex("SELECT * FROM users WHERE age > ?")
defer stmt.Close()
var users []User
err = stmt.Select(&users, 18)

// PreparexContext — 带 Context 版本
ctx := context.Background()
stmt, err = db.PreparexContext(ctx, "SELECT * FROM users WHERE name = ?")
defer stmt.Close()
var user User
err = stmt.Get(&user, "Alice")

// PrepareNamed — 命名预编译语句（统一使用 :name 风格，框架内部处理绑定转换）
nstmt, err := db.PrepareNamed("SELECT * FROM users WHERE name = :name")
defer nstmt.Close() // 预编译语句用完必须 Close，避免资源泄漏
var user User
err = nstmt.Get(&user, map[string]any{"name": "Alice"})
```

> **统一体验**：`Preparex`/`PreparexContext` 与其他所有查询方法一样，自动 Rebind，统一使用 `?` 占位符。
> `PrepareNamed`/`PrepareNamedContext` 使用命名参数 `:name`，框架内部会正确处理绑定转换。
>
> **注意**：预编译语句（`Stmt`/`NamedStmt`）**不支持 IN 切片展开**。因为占位符数量在 `Prepare` 时已固定，无法在执行时动态展开。如需 IN 查询，请使用 `db.Select`/`db.NamedSelect` 等非预编译方法。
>
> **资源管理**：`Stmt`/`NamedStmt` 底层持有 `sql.Stmt`，必须在使用后调用 `Close()` 释放。忘记 `Close()` 会导致连接池中的预处理语句资源泄漏，直到 `DB.Close()` 才回收。推荐模式：`defer stmt.Close()`。
>
> **Hook 覆盖**：`Stmt`/`NamedStmt` 的 `Exec`/`Query` 方法同样触发 Hook，Hook 从所属的 DB/Tx/Conn 自动传播到 Stmt。

#### PreparerContext 统一接口

`PreparerContext` 接口组合了 `PrepareContext` 和绑定能力，`DB`/`Tx`/`Conn` 都实现了该接口，
可以编写不关心执行上下文的通用预编译函数：

```go
// 接受 DB、Tx 或 Conn
func prepareUserQuery(p sqlex.PreparerContext) (*sqlex.Stmt, error) {
    return sqlex.PreparexContext(context.Background(), p, "SELECT * FROM users WHERE name = ?")
}

stmt, err := prepareUserQuery(db)   // 通过 DB
stmt, err = prepareUserQuery(tx)    // 通过 Tx
stmt, err = prepareUserQuery(conn)  // 通过 Conn
```

### 事务管理

```go
// 推荐模式：CloseWithErr 自动管理
func createUserWithProfile(db *sqlex.DB, user User, profile Profile) (err error) {
    tx, err := db.Beginx()
    if err != nil {
        return err
    }
    defer func() { tx.CloseWithErr(err) }()  // 自动 Commit 或 Rollback

    _, err = tx.NamedExec(`INSERT INTO users (name) VALUES (:name)`, user)
    if err != nil {
        return err  // CloseWithErr 检测到 err != nil，自动 Rollback
    }

    _, err = tx.NamedExec(`INSERT INTO profiles (user_name, bio) VALUES (:user_name, :bio)`, profile)
    if err != nil {
        return err
    }

    return nil  // CloseWithErr 检测到 err == nil，自动 Commit
}

// 带 Context 和选项的事务
tx, err := db.BeginTxx(ctx, &sql.TxOptions{
    Isolation: sql.LevelSerializable,
    ReadOnly:  false,
})
```

### JSONValue[T]

```go
import "github.com/go-sqlex/sqlex/types"

// 定义包含 JSON 列的模型
type Article struct {
    ID       int                            `db:"id"`
    Title    string                         `db:"title"`
    Metadata types.JSONValue[ArticleMeta]   `db:"metadata"`
}

type ArticleMeta struct {
    Tags       []string `json:"tags"`
    ViewCount  int      `json:"view_count"`
}

// 写入 — 自动序列化为 JSON
article := Article{
    Title:    "Hello World",
    Metadata: types.NewJSONValue(ArticleMeta{
        Tags:      []string{"go", "sql"},
        ViewCount: 0,
    }),
}
db.NamedExec(`INSERT INTO articles (title, metadata) VALUES (:title, :metadata)`, article)

// 读取 — 自动反序列化
var a Article
db.Get(&a, "SELECT * FROM articles WHERE id = ?", 1)
if a.Metadata.Valid {
    fmt.Println(a.Metadata.Val.Tags) // ["go", "sql"]
}
// !Valid 时 Val 为零值（Scan / 零值初始化保证）
// JSON 序列化/反序列化（实现了 json.Marshaler/Unmarshaler）
data, _ := json.Marshal(a.Metadata)
json.Unmarshal(data, &a.Metadata)
```

### Hook 切面

```go
// 自定义 Hook — 例如 OpenTelemetry 追踪
type TracingHook struct{}

func (h *TracingHook) BeforeQuery(ctx context.Context, event *sqlex.QueryEvent) context.Context {
    ctx, span := tracer.Start(ctx, "sql."+event.OperationType)
    span.SetAttributes(attribute.String("db.statement", event.Query))
    return ctx
}

func (h *TracingHook) AfterQuery(ctx context.Context, event *sqlex.QueryEvent) {
    span := trace.SpanFromContext(ctx)
    if event.Error != nil {
        span.RecordError(event.Error)
    }
    span.SetAttributes(attribute.String("db.duration", event.Duration.String()))
    span.End()
}

db.AddHook(&TracingHook{})

// Hook 覆盖完整生命周期：query/exec/begin/commit/rollback
// 事务操作（Begin/Commit/Rollback）也会触发 Hook，无论成功或失败
tx, _ := db.Beginx()       // → Hook(OpBegin)
// tx 的查询也会触发注册的 Hook
tx.CloseWithErr(nil)       // → Hook(OpCommit) 或 Hook(OpRollback)
```

#### QueryEvent 字段

```go
type QueryEvent struct {
    Query         string        // SQL 语句
    Args          []any         // 执行参数
    Duration      time.Duration // 总耗时（含 Hook 链开销）
    Error         error         // 执行错误（AfterQuery 阶段有值）
    OperationType OpType        // 操作类型：OpQuery/OpExec/OpBegin/OpCommit/OpRollback
    RowsAffected  int64         // 受影响行数（仅 exec 操作）
    LastInsertID  int64         // 最后插入的自增 ID（仅 exec 操作）
}
```

#### 按条件过滤 Hook

sqlex 不内置过滤器，推荐用装饰器模式自行组合：

```go
// 仅在慢查询时触发
func SlowOnly(h sqlex.Hook, threshold time.Duration) sqlex.Hook {
    return &slowHook{hook: h, threshold: threshold}
}
type slowHook struct {
    hook      sqlex.Hook
    threshold time.Duration
}
func (h *slowHook) BeforeQuery(ctx context.Context, e *sqlex.QueryEvent) context.Context {
    return h.hook.BeforeQuery(ctx, e)
}
func (h *slowHook) AfterQuery(ctx context.Context, e *sqlex.QueryEvent) {
    if e.Duration >= h.threshold {
        h.hook.AfterQuery(ctx, e)
    }
}

// 仅在出错时触发
func OnError(h sqlex.Hook) sqlex.Hook { /* BeforeQuery 透传，AfterQuery 判 e.Error != nil */ }

db.AddHook(SlowOnly(&AlertHook{}, 500*time.Millisecond))
```

### StrictMode 严格模式

```go
// 默认宽松模式（strict=false），静默忽略多余列
db, _ := sqlex.Connect("postgres", dsn)
fmt.Println(db.IsStrict()) // false

type UserPartial struct {
    ID   int    `db:"id"`
    Name string `db:"name"`
}

// 默认宽松模式：SELECT * 返回的 email/age 列在 UserPartial 中没有对应字段，静默忽略
var users []UserPartial
err := db.Select(&users, "SELECT * FROM users") // 成功，忽略 email/age

// 开启严格模式：字段不匹配时报错
db.SetStrict(true)
err = db.Select(&users, "SELECT * FROM users")
// err: missing destination name email (index 2), age (index 3) in UserPartial

// strict 自动传递到 Tx/Conn
tx, _ := db.Beginx()           // 继承 DB 的 strict
conn, _ := db.Connx(ctx)       // 继承 DB 的 strict

// 也可以单独覆盖
tx.SetStrict(false)  // 仅影响该 Tx
```

### 统一接口

DB、Tx、Conn 实现了一套公共接口（编译期断言保证）：

| 接口 | 方法 | 用途 |
|-----------|---------|---------|
| `Ext` | `Exec`, `Queryx`, `QueryRowx` | 基础查询/执行 |
| `ExtContext` | `ExecContext`, `QueryxContext`, `QueryRowxContext` | Context 感知变体 |
| `NamedExt` | `NamedExec`, `NamedQuery`, `NamedGet`, `NamedSelect` | 命名参数查询 |
| `BindExt` | `BindNamed`, `Get`, `Select`, `Rebind`, `DriverName` | 位置参数查询 |
| `Preparer` | `Preparex`, `PrepareNamed` | 预编译语句创建 |
| `PreparerContext` | `PreparexContext`, `PrepareNamedContext` | Context 感知预编译 |

```go
// 通过 NamedExt 接受 DB、Tx 或 Conn
func getUserByName(ext sqlex.NamedExt, name string) (*User, error) {
    var user User
    err := ext.NamedGet(&user, `SELECT * FROM users WHERE name = :name`,
        map[string]any{"name": name})
    return &user, err
}

user, err := getUserByName(db, "Alice")
tx, _ := db.Beginx()
user, err = getUserByName(tx, "Bob")
conn, _ := db.Connx(ctx)
user, err = getUserByName(conn, "Charlie")
```

## 与 jmoiron/sqlx 对比

| 功能 | jmoiron/sqlx | sqlex |
|------|-------------|-------|
| Go 版本要求 | 1.10+ | 1.21+ |
| 结构体扫描 | ✅ | ✅ |
| 命名查询 | ✅ | ✅ |
| 绑定变量 | ✅ | ✅（增强：支持转义 `\?` 和 `??`，跳过字符串字面量、PG/MySQL 标识符、注释、PG dollar quoting 中的 `?`） |
| IN 子句展开 | ✅ `In()` | ✅ `In()` 内置完整 SQL 词法跳过 + 自动检测 Valuer/切片指针；DB/Tx/Conn × Exec/Queryx/QueryRowx/MustExec/Select/Get/Named* 全路径自动 IN |
| 跨数据库占位符 | ❌ 需手动 Rebind | ✅ **所有方法**自动 Rebind，统一用 `?`（包括 `Preparex`） |
| 字段匹配模式 | `unsafe` 字段（默认严格） | `StrictMode`（默认宽松，语义更直观） |
| Hook 切面 | ❌ | ✅ `AddHook` 可插拔 SQL 执行拦截器 |
| JSONValue[T] | ❌ | ✅ `types.JSONValue[T]` |
| NamedGet/NamedSelect | ❌ | ✅ DB/Tx 便捷方法（内置 IN 展开） |
| CloseWithErr | ❌ | ✅ 自动事务管理 |
| 统一接口 | ❌ DB/Tx 方法重叠但无共享接口 | ✅ `Ext` / `ExtContext` / `NamedExt` / `BindExt` / `Preparer` / `PreparerContext` — DB/Tx/Conn 统一，编译期校验 |
| Unicode 命名参数名 | ⚠️ 尝试支持但不可靠 | ❌ 不支持（参数名限 ASCII；SQL 其他位置 Unicode 安全） |
| Unicode 表名/列名/值 | ✅ | ✅ |
| PostgreSQL `::` | ❌ 会误判 | ✅ 正确处理 |
| 命名查询字符串字面量 | ❌ 冒号被误判 | ✅ 跳过字符串/注释中的冒号（[#872](https://github.com/jmoiron/sqlx/issues/872)） |
| 命名查询双引号标识符 | ❌ 冒号被误判 | ✅ 跳过双引号标识符中的冒号 |
| 命名参数解析误判兜底 | ❌ 误判后直接报错 | ✅ 缺失参数恢复为 `:name` 原样，让原始 SQL 仍可能正常执行（[#892](https://github.com/jmoiron/sqlx/issues/892)） |
| 类型系统 | `interface{}` | `any` |
| 文件结构 | 巨型单文件 | 模块化拆分 |
| Named 查询跨驱动兼容 | ❌ Named 查询在 PG 上失败 | ✅ 所有 Named 方法正确 Rebind |

## Bug 修复与优化

sqlex 在 jmoiron/sqlx 基础上修复了以下已知问题：

| 问题 | 说明 |
|------|------|
| **原版不识别 SQL 词法元素** | 原版假设 SQL 中所有 `?`/`:name` 都是占位符，不区分字符串字面量、注释、双引号/反引号标识符、PG dollar quoting 等词法元素。这与 driver/数据库是否支持这些语法元素无关——driver 把 SQL 原样透传给 server 解析；sqlex 必须在 SQL **离开自己之前**正确识别词法，否则 `compileNamedQuery`/`In`/`Rebind` 会把字符串/注释里的 `?`/`:name` 当成真占位符，导致计数错乱、绑定到错误位置。实证见 `tests/cross_db/named_test.go` |
| **ConnectContext 连接泄漏** | `ConnectContext` 在 Ping 失败时未关闭连接，导致泄漏。sqlex 在 Ping 失败时会 `db.Close()` |
| **Rebind 转义问号** | 原版不支持 `\?` 或 `??` 转义，导致字面量 `?` 被错误替换。sqlex 支持 `\?` → `?` 和 `??` → `?` |
| **Rebind 字符串字面量** | 原版 `Rebind` 将单引号字符串字面量中的 `?`（如 `'What?'`）也替换为绑定变量。sqlex 正确跳过字符串字面量，包括 SQL 标准转义引号 `''` |
| **Rebind 词法元素全覆盖** | 原版仅识别单引号字符串，导致 SQL 注释（`-- ?`、`/* ? */`）、PG 双引号标识符（`"col?"`）、MySQL 反引号标识符（`` `col?` ``）、PG dollar-quoted string（`$$?$$`、`$tag$?$tag$`）内的 `?` 都被错误替换。sqlex 在 Rebind 中识别全部 SQL 词法元素，与 compileNamedQuery 对称，不会再出现"占位符数量不匹配"的隐蔽 bug |
| **In 函数词法元素全覆盖** | 原版 `In()` 用简单 `IndexByte('?')` 扫描，遇到字符串字面量/注释/双引号/反引号/dollar quoting 内的 `?` 直接报 `number of bindVars exceeds arguments`。sqlex 重写 `In()` 使其与 Rebind 词法对称，正确处理 `??`/`\?` 转义，让 `db.Select(&u, "WHERE name LIKE 'test?%' AND id IN (?)", ids)` 这类查询正常工作 |
| **needsInRewrite 与 In 行为对齐** | 原版 autoIn 快速路径判定 `hasSliceArgs` 不解包 Valuer 也不识别切片指针，导致 `Valuer{val:[]int{...}}` 和 `&[]int{...}` 这类参数被错过 IN 展开。sqlex 改用与 In 内部一致的判断逻辑；并将函数改名为 `needsInRewrite`——更准确表达"是否需要 In 路径处理"（含 AsValue/AsList 解包），消除 `hasSliceArgs` 字面意思与 valueArg 包装语义之间的认知摩擦 |
| **命名查询字符串字面量冒号** | 原版将字符串字面量中的冒号（如 IPv6 地址 `'a:6e:...'`、时间格式 `'HH:mm:ss'`）误识别为命名参数（[#872](https://github.com/jmoiron/sqlx/issues/872)）。sqlex 正确跳过单引号字符串内的冒号，包括 SQL 标准转义引号 `''` |
| **命名查询双引号标识符** | 原版将双引号标识符中的冒号（如 PostgreSQL `"col:name"`）误识别为命名参数。sqlex 正确跳过双引号标识符内的冒号，包括转义双引号 `""` |
| **命名查询反引号 + dollar quoting** | sqlex 在 `compileNamedQuery` 中跳过 MySQL 反引号标识符（`` `col:name` ``）和 PG dollar-quoted string（`$$:fake$$`、`$tag$:fake$tag$`）内的冒号，与 Rebind 完全对称 |
| **命名参数解析误判兜底** | 原版若 `compileNamedQuery` 把字符串字面量/罕见词法中的 `:xxx` 误判为命名参数，由于 args 没有 `xxx`，会直接报 `could not find name xxx`，用户难以定位是 sqlx 的解析 bug。sqlex 在 `compileNamedQueryWith` 编译期一次成型：args 中存在的 `:name` 转为占位符并编号，不存在的原样保留为 `:name` 字面量（不计入编号），让原始 SQL 仍可能被 driver/server 正确执行。行为与 GORM 的 `@name` 处理一致（[#892](https://github.com/jmoiron/sqlx/issues/892) 相关延伸）。**注意**：这是 named 解析器边界 case 的安全网，不建议依赖它做业务级动态参数过滤；参数名应严格对齐 args |
| **SQL 注释中冒号跳过** | 原版不处理注释。sqlex 跳过 `--` 行注释和 `/* */` 块注释中的冒号，不将其解析为命名参数 |
| **统一词法扫描器** | 原版 `Rebind` / `In` / `compileNamedQuery` 三处各自实现"跳过字符串/标识符/注释"的逻辑，易出现"改一处漏两处"的漂移。sqlex 抽取统一词法扫描器（`lexer.go` 的 `scanSkipSegment`），三处复用同一份实现，保证对"哪些区域跳过"判定完全一致 |
| **命名参数名规则收紧 + `:123` 误判修复** | 原版参数名允许数字开头，`:123` 被误识别为名为 `123` 的参数。sqlex 收紧规则为 `[A-Za-z_][A-Za-z0-9_.]*`（首字符须字母/下划线），`:123` 原样保留——既符合通用标识符规范，又避免与 Oracle `:N` / SQLite `?NNN` 位置占位符语义冲突 |
| **Unicode 命名参数名（行为变更）** | 原版尝试支持 Unicode 参数名（按字节处理，实际不可靠）。sqlex **不支持** Unicode 命名参数名（如 `:名字`）——实践中参数名对应 struct 的 `db` tag 或 map key，几乎全是 ASCII。放弃此能力让词法统一按 byte 扫描，更简单更快。**注意**：SQL 其他位置的 Unicode（表名/列名/字符串值/参数值）完全不受影响，byte 扫描天然安全 |
| **命名查询 `::` 处理** | 原版将 PostgreSQL 的类型转换语法 `::` 误判为命名参数。sqlex 正确跳过 `::` |
| **错误信息增强** | 增强了多处错误信息的上下文，便于调试 |
| **NamedStmt.Exec 返回值** | 原版 `NamedStmt.Exec` 未正确返回 `sql.Result`，sqlex 已修正 |
| **Named 查询 Rebind 缺失** | `NamedGet`/`NamedSelect`/`NamedExec` 内部使用 `QUESTION` 绑定后，在无切片参数时缺少 Rebind 步骤，导致 PostgreSQL 等 `$N` 绑定类型的数据库上 Named 查询失败。sqlex 修复了 `autoIn` 函数，无论是否有切片参数均正确执行 Rebind |
| **位置参数查询跨数据库失败** | 原版 `Select`/`Get`/`Exec` 等位置参数方法不做自动 Rebind，在 PostgreSQL 上使用 `?` 占位符会失败。sqlex 所有查询方法均自动 Rebind，实现跨数据库统一 `?` 风格 |
| **位置参数 IN 切片展开覆盖不全** | 早期实现仅 `Select`/`Get` 接入 autoIn，`Exec`/`Queryx`/`QueryRowx`/`MustExec`/`NamedQuery` 漏掉，导致用户调用 `db.Exec("DELETE FROM t WHERE id IN (?)", ids)` 直接报参数不匹配。sqlex 已在 `BindExt`/`NamedExt` 全路径补齐，并写入接口契约保证防回归 |

## 测试

```bash
# 1) 主包单元测试（无 DB 依赖，最快）
go test -count=1 -timeout=120s .

# 2) cross_db 仅 MySQL（隔离 PG/SQLite，方便定位）
SQLX_POSTGRES_DSN=skip SQLX_SQLITE_DSN=skip \
  go test -count=1 -timeout=300s ./tests/cross_db/

# 3) cross_db 仅 PostgreSQL
SQLX_MYSQL_DSN=skip SQLX_SQLITE_DSN=skip \
  go test -count=1 -timeout=300s ./tests/cross_db/

# 4) cross_db 仅 SQLite（无外部依赖，最快）
SQLX_MYSQL_DSN=skip SQLX_POSTGRES_DSN=skip \
  go test -count=1 -timeout=120s ./tests/cross_db/

# 5) cross_db 三驱动一起（CI 推荐）
go test -count=1 -timeout=300s ./tests/cross_db/

# 6) integration 业务集成
go test -count=1 -timeout=120s ./tests/integration/

# 7) pg 专属测试（PostgreSQL 独有特性）
go test -count=1 -timeout=120s ./tests/pg/

# 8) types / reflectx 子包
go test -count=1 -timeout=60s ./types/ ./reflectx/
```

**为什么必须按驱动分别跑**：一把 `go test ./...` 会把 MySQL/PG/SQLite 一起跑，无法定位"某改动只在 PG 下挂"这类问题。分驱动跑能快速二分定位 bug 所在的驱动。

**DSN 配置**：在项目根 `.env.test` 中直接写入完整 DSN，使用 `SQLX_*_DSN` 命名空间。也可以命令行直接覆盖。设为 `skip` 即跳过该驱动相关测试。SQLite 默认用 `:memory:`。

| 环境变量 | 设置 | 行为 |
|---------|------|------|
| `SQLX_MYSQL_DSN` | 完整 DSN | 用此 DSN |
| `SQLX_MYSQL_DSN` | `skip` 或空 | 跳过 MySQL 测试 |
| `SQLX_POSTGRES_DSN` | 同上 | |
| `SQLX_SQLITE_DSN` | 同上（默认 `:memory:`） | |

### 单点调试与定位

```bash
# 跑单个测试函数
go test -count=1 -timeout=60s -run "TestNextPlaceholder" -v .

# 跑单个 sub-test
go test -count=1 -timeout=60s -run "TestNextPlaceholder/multiline_IN" -v .

# Race 检测
go test -count=1 -race -timeout=180s .

# 覆盖率
go test -count=1 -cover -coverprofile=cover.out -timeout=120s .
go tool cover -html=cover.out

# Bench（仅跑 bench，不跑普通测试）
go test -bench=. -benchmem -run=NoSuch -benchtime=2s .
```

## 性能说明

- **预编译语句**：`Preparex`/`PreparexContext` 自动 Rebind，统一使用 `?` 占位符，无需关心底层数据库差异。`PrepareNamed` 不受此限制
- **零开销原则**：未注册 Hook 时，Hook 路径零开销；对于 MySQL 等 `QUESTION` 类型数据库，自动 Rebind 为 no-op（仅比较 bindType 常量）
- **自动 Rebind**：所有查询方法（包括 `Preparex`/`PreparexContext`）始终执行 Rebind。对于 MySQL/SQLite（已使用 `?`），Rebind 直接返回原字符串；对于 PostgreSQL 等非 `QUESTION` 数据库，如果 query 中不含 `?`（如已经是 `$1` 格式），也会通过快速路径直接返回，避免无意义的内存分配。双重 Rebind 完全安全且零开销
- **切片参数检测**：`needsInRewrite` 通过反射检查参数类型，对于非切片参数仅做类型判断（纳秒级）
- **Mapper 缓存**：字段映射结果在首次使用后被缓存，后续查询直接复用
- **Hook 执行**：Hook 在查询前后同步执行，适合轻量级操作（如计数、打日志）；重量级操作建议异步化

### 关于 NameMapper

`NameMapper` 是全局变量，控制字段名到列名的映射规则，默认为 `strings.ToLower`。

> **并发安全警告**：`NameMapper` 的读写不是并发安全的。建议仅在 `init()` 函数中设置，运行时修改可能导致 data race。如果需要在运行时使用不同的映射策略，请通过 `DB.MapperFunc()` 为每个 DB 实例单独设置。

## 许可证

MIT License

基于 [jmoiron/sqlx](https://github.com/jmoiron/sqlx) 项目进行的现代化增强，感谢原作者 Jason Moiron 的出色工作。

请在提交 Pull Request 前阅读我们的[贡献指南](CONTRIBUTING.md)。  
版本变更请查看 [CHANGELOG.md](CHANGELOG.md)。
