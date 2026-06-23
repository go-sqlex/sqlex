---
name: sqlex
description: sqlex 是基于 jmoiron/sqlx 的 Go database/sql 现代化增强封装库，提供结构体扫描、命名参数查询、
  Hook 切面、泛型 JSONValue[T] JSON 列类型、自动 IN 展开、跨数据库统一 `?` 占位符等能力。
  当用户涉及以下任何场景时，务必使用此技能：sqlex 库的使用、开发、调试、迁移、Bug 修复，
  Go 数据库操作（Get/Select/Exec/Queryx），SQL 查询结果映射结构体（StructScan/MapScan/SliceScan），
  NamedGet/NamedSelect/NamedExec 命名参数查询，IN 子句展开，
  数据库 Hook/拦截器/中间件/SQL 切面，JSONValue JSON 列类型，
  事务管理 CloseWithErr/ExecFunc，NamedExt/BindExt 统一接口，
  Rebind 占位符转换，从 jmoiron/sqlx 迁移，
  PostgreSQL/MySQL/SQLite/SQL Server 跨数据库兼容，Preparex 预编译语句，PrepareNamed 命名预编译，
  reflectx 反射映射。
  即使用户只是模糊提到 "Go SQL 封装"、"sqlx 增强"、"数据库结构体映射"、"database/sql 扩展"、
  "数据库查询封装"、"SQL 参数绑定"、"数据库中间件" 等概念，也应优先查阅此技能。
---

[English](SKILL.md) | **中文**

# sqlex — AI 助手快速参考

> 本文档是 AI 编程助手的决策参考，聚焦"用什么 API、怎么用、避什么坑"。
> 完整文档（安装、测试、迁移指南等）见 [README.md](README.md)。

## 1. 核心概念速查

**定位**：Go `database/sql` 增强封装（非 ORM），基于 jmoiron/sqlx 升级。
**模块路径**：`github.com/go-sqlex/sqlex`
**Go 版本**：1.21+ | **数据库**：PostgreSQL、MySQL、SQLite、Oracle、SQL Server

**关键设计**：
- 统一 `?` 占位符，框架自动 Rebind 为 `$N`/`@pN`/`:argN`
- DB / Tx / Conn 三者接口对齐（`BindExt` + `NamedExt`），函数签名接受任一
- Hook 洋葱模型，未注册时零开销
- StrictMode 默认宽松（与 sqlx `Unsafe()` 一致），可开启严格检查

## 2. API 决策树

```
需要查询？
├─ 单行 → db.Get(&dest, query, args...)       或 db.NamedGet(&dest, query, arg)
├─ 多行 → db.Select(&dest, query, args...)    或 db.NamedSelect(&dest, query, arg)
└─ 原始行 → db.Queryx / db.QueryRowx          或 db.NamedQuery

需要执行？
├─ 普通执行 → db.Exec / db.MustExec            或 db.NamedExec
└─ 事务内 → tx.Exec / tx.CloseWithErr(err)     或 tx.NamedExec

需要预编译？
├─ 位置参数 → db.Preparex / db.PreparexContext
└─ 命名参数 → db.PrepareNamed / db.PrepareNamedContext

需要单连接？ → db.Connx(ctx) → conn（与 DB/Tx 接口对齐）

需要 JSON 列？ → types.JSONValue[T]（泛型，替代 JSONText）
```

## 3. 用法精要

### 3.1 连接

```go
db, err := sqlex.Connect("postgres", dsn)   // 含 Ping
db, err := sqlex.Open("mysql", dsn)          // 不 Ping
db := sqlex.MustConnect("sqlite3", ":memory:") // 失败 panic
```

### 3.2 查询（统一 `?` 占位符）

```go
// 位置参数 — 框架自动 Rebind，MySQL/PG/SQLite/SQL Server 统一写法
db.Get(&user, "SELECT * FROM users WHERE id = ?", 1)
db.Select(&users, "SELECT * FROM users WHERE age > ?", 18)

// 命名参数 — 支持 struct 或 map[string]any
db.NamedGet(&user, `SELECT * FROM users WHERE name = :name`, map[string]any{"name": "Alice"})
db.NamedSelect(&users, `SELECT * FROM users WHERE age > :min_age`, map[string]any{"min_age": 18})
db.NamedExec(`INSERT INTO users (name, email) VALUES (:name, :email)`, User{Name: "Alice", Email: "a@b.c"})
```

### 3.3 IN 查询（自动展开）

```go
// 位置参数：自动检测切片 + IN 列表语境识别
db.Select(&users, "SELECT * FROM users WHERE id IN (?)", []int{1, 2, 3})

// 命名参数：内置 IN 展开
db.NamedSelect(&users, `SELECT * FROM users WHERE id IN (:ids)`, map[string]any{"ids": []int{1, 2, 3}})
```

**IN 列表语境识别规则**：切片自动展开需同时满足 ① 严格 `(?)` 形态（`(` 与 `)` 之间仅一个 `?` 加可选空白）② `(` 前紧邻的完整标识符是 `IN`（大小写不敏感，含 `NOT IN`）。其他 `(?)` 语境一律视为单值，**无需 `AsValue` 兜底**。

| SQL 形态 | 切片参数 | 行为 |
|---|---|---|
| `IN (?)` / `NOT IN (?)` | 切片 | ✅ 展开 |
| `IN (?, ?, ?)` | 多个标量 | 不展开 |
| `WHERE x = ?` | 切片 | 不展开（整体下发） |
| `ANY(?)` / `ALL(?)` / `VALUES (?)` / `func(?)` | 切片 | 不展开（整体下发，正确行为） |
| `col_in (?)` / `t.in (?)` | 切片 | 不展开（完整 token 比较，不误判 IN） |

**逃生通道**：`sqlex.AsValue(v)` 强制不展开（即使处于 IN 语境）| `sqlex.AsList(slice)` 强制展开（即使不在 IN 语境，如 `ANY(?)` 想展开成列表）

**已知边界**：`IN /* 注释 */ (?)` 会识别不到 IN 而不展开，此种写法极罕见，必要时用 `AsList` 兜底。

### 3.4 事务管理

```go
tx, err := db.Beginx()
if err != nil {
    return err
}
defer func() { tx.CloseWithErr(err) }()  // err==nil → Commit, err!=nil → Rollback

_, err = tx.NamedExec(`INSERT INTO users (name) VALUES (:name)`, User{Name: "Bob"})
if err != nil {
    return err  // defer 中自动 Rollback
}
return nil      // defer 中自动 Commit
```

### 3.5 Conn（单连接）

```go
conn, err := db.Connx(ctx)
defer conn.Close()
// Conn 与 DB/Tx 接口完全对齐
conn.Get(&user, "SELECT * FROM users WHERE id = ?", 1)
conn.NamedGet(&user, `SELECT * FROM users WHERE name = :name`, map[string]any{"name": "Alice"})
```

### 3.6 JSONValue[T]

```go
import "github.com/go-sqlex/sqlex/types"

type Config struct {
    ID       int                       `db:"id"`
    Settings types.JSONValue[Settings] `db:"settings"`
}
cfg := Config{Settings: types.NewJSONValue(Settings{Theme: "dark", FontSize: 14})}
if cfg.Settings.Valid {
    theme := cfg.Settings.Val.Theme // "dark"
}
// !Valid 时 Val 为零值
```

### 3.7 Hook 切面

```go
type MetricsHook struct{}
func (h *MetricsHook) BeforeQuery(ctx context.Context, event *sqlex.QueryEvent) context.Context {
    return ctx
}
func (h *MetricsHook) AfterQuery(ctx context.Context, event *sqlex.QueryEvent) {
    recordMetric(event.Query, event.Duration, event.Error)
}
db.AddHook(&MetricsHook{})
// Hook 也适用于 Tx/Conn（自动继承）
// 多个 Hook：BeforeQuery 正序，AfterQuery 反序（洋葱模型）
```

**按条件过滤**：sqlex 不内置过滤器，用装饰器包装即可：
```go
// 仅慢查询触发
db.AddHook(SlowOnly(&AlertHook{}, 500*time.Millisecond))
// SlowOnly / OnError 等装饰器自行实现，几行代码即可
```

## 4. 最佳实践与常见陷阱

### ✅ 推荐做法

1. **使用统一 `?` 占位符** — 所有查询方法自动 Rebind
2. **事务使用 `CloseWithErr`** — `defer func() { tx.CloseWithErr(err) }()`
3. **生产环境使用 Context 方法** — `GetContext`/`SelectContext` 支持超时控制
4. **NamedSelect + IN** — 无需手动调用 `In()`
5. **ANY(?)/VALUES(?) 自动安全** — 默认不再展开，无需 `AsValue` 兜底
6. **初始化时注册 Hook** — Tx/Conn 自动继承 DB 的 Hook
7. **PostgreSQL JSONB `?` 操作符** — 用 `??` 转义
8. **StrictMode 按需开启** — 默认宽松；开发时 `db.SetStrict(true)` 助查问题

### ⚠️ 常见陷阱

1. **Preparex 已自动 Rebind** — 无需手动区分数据库类型
2. **NameMapper 只能在 `init()` 设置** — 运行时用 `DB.MapperFunc()` 替代
3. **Tx 非并发安全** — 用 `Tx.ExecFunc()` 做并发保护
4. **Hook 同步执行** — 重量级操作应在 Hook 内异步
5. **命名参数名限 ASCII** — `[A-Za-z_][A-Za-z0-9_.]*`；数字开头 `:123` 不被识别
6. **StrictMode 默认宽松** — 与 sqlx `db.Unsafe()` 行为一致

## 5. 与 jmoiron/sqlx 的差异

### 新增能力

| 特性 | 说明 |
|------|------|
| Hook 切面 | `AddHook` 可插拔 SQL 执行拦截器（洋葱模型） |
| JSONValue[T] | 泛型 JSON 列类型 |
| NamedGet/NamedSelect | 便捷命名参数查询（内置 IN 展开） |
| CloseWithErr | 根据 error 自动 Commit/Rollback |
| ExecFunc | Tx 互斥锁保护下执行函数 |
| NamedExt/BindExt | DB/Tx 统一编程接口 |
| Select/Get 自动 IN | 检测切片参数 + IN 列表语境识别（仅 `IN (?)` 展开） |
| StrictMode | 默认宽松，可开启严格检查 |
| 自动 Rebind | 所有查询方法自动将 `?` 转换为目标数据库占位符 |
| Conn 增强 | 与 DB/Tx 接口完全对齐 |
| SQL Server 方括号标识符 | `scanBracketIdentifier` 支持 `[col?name]` |

### Bug 修复

- **统一词法扫描器**：Rebind/In/compileNamedQuery 复用 `scanSkipSegment`，消除漂移
- **Rebind 全覆盖**：跳过字符串/注释/PG 双引号/MySQL 反引号/SQL Server 方括号/PG dollar quoting 内的 `?`
- **命名查询对称修复**：跳过字符串/注释/双引号/反引号/方括号/dollar quoting 中的冒号
- **参数名规则收紧**：限 `[A-Za-z_][A-Za-z0-9_.]*`，数字开头不再误判
- **缺失参数原样保留**：兜底解析器误判，不是业务容错
- **ConnectContext 泄漏**、**NamedStmt.Exec 返回值**、**Named 查询 Rebind 缺失**等均已修复

## 6. API 速查表

### 顶层函数

| 函数 | 说明 |
|------|------|
| `Connect/ConnectContext/MustConnect` | 连接数据库（含 Ping） |
| `Open/MustOpen` | 打开连接（不 Ping） |
| `Select/SelectContext` | 查询多行（接受 Queryer） |
| `Get/GetContext` | 查询单行（接受 Queryer） |
| `In` | 展开 IN 切片参数 |
| `AsValue` | 强制不展开（即使处于 IN(?) 语境） |
| `AsList` | 强制展开（即使不在 IN(?) 语境） |
| `Named` | 命名参数绑定 |
| `Rebind` | 转换绑定变量格式 |

### DB 独有方法

`Beginx/BeginTxx`、`Connx`、`AddHook`、`MapperFunc`、`Preparex/PreparexContext`、`PrepareNamed/PrepareNamedContext`、`SetStrict/IsStrict`

### Tx 独有方法

`CloseWithErr(err)`、`ExecFunc(fn)`、`Stmtx/StmtxContext`、`TryStmtx/TryStmtxContext`、`SetStrict/IsStrict`

### DB 和 Tx 共有方法

`Get/GetContext`、`Select/SelectContext`、`Queryx/QueryxContext`、`QueryRowx/QueryRowxContext`、`Exec/ExecContext`、`MustExec/MustExecContext`、`NamedGet/NamedGetContext`、`NamedSelect/NamedSelectContext`、`NamedExec/NamedExecContext`、`NamedQuery/NamedQueryContext`、`Rebind`、`BindNamed`

### Conn 方法（与 DB/Tx 对齐）

**Context 版**：`GetContext`、`SelectContext`、`QueryxContext`、`QueryRowxContext`、`ExecContext`、`MustExecContext`、`NamedGetContext`、`NamedSelectContext`、`NamedExecContext`、`NamedQueryContext`

**非 Context 版**（委托 `context.Background()`）：`Get`、`Select`、`Queryx`、`QueryRowx`、`Exec`、`MustExec`、`NamedGet`、`NamedSelect`、`NamedExec`、`NamedQuery`

**工具方法**：`Rebind`、`BindNamed`、`DriverName`、`SetStrict/IsStrict`、`BeginTxx`、`PreparexContext`、`PrepareNamedContext`

### types 子包

| 类型 | 说明 |
|------|------|
| `JSONValue[T]` | 泛型 JSON 列（Scan/Value + MarshalJSON/UnmarshalJSON；直接访问 Val/Valid 字段） |
| `JSONText` | json.RawMessage 包装，支持 Scan/Value |
| `NullJSONText` | 可空的 JSONText |
| `GzippedText` | 自动 gzip 压缩/解压的 []byte |
| `BitBool` | MySQL BIT(1) 布尔类型 |

### Hook 相关

| 类型 | 说明 |
|------|------|
| `Hook` 接口 | `BeforeQuery(ctx, *QueryEvent) ctx` + `AfterQuery(ctx, *QueryEvent)` |
| `QueryEvent` | 包含 Query, Args, StartTime, Duration, Error, OperationType, RowsAffected, LastInsertID |
| `OpType` | 操作类型枚举：OpQuery/OpExec/OpBegin/OpCommit/OpRollback |
