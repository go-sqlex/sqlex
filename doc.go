// Package sqlex is a modern enhancement of database/sql based on jmoiron/sqlx.
//
// sqlex's design philosophy is SQL-centric: developers write SQL directly, and sqlex
// handles parameter binding and result mapping. No chain APIs, implicit query building,
// or ORM-style abstractions are introduced.
//
// # Core Enhancements
//
//   - Hook aspects: onion-model SQL interceptors covering query/exec/transaction lifecycle
//   - JSONValue[T]: generic JSON column type, NULL-safe
//   - Auto Rebind: all query methods use ? placeholders uniformly, auto-converting to target database format
//   - Transparent IN: Select/Get/NamedGet/NamedSelect auto-expand slice parameters
//   - StrictMode: optional strict mode detecting query result / struct field mismatches
//   - CloseWithErr: auto-commits or rolls back transactions based on error; result reported via Hook
//   - NamedExt/BindExt: unified interfaces, DB and Tx are interchangeable
//   - Functional Options: Open/Connect support WithHooks/WithStrictMode etc.
//
// # Quick Start
//
//	db, err := sqlex.Connect("postgres", dsn,
//	    sqlex.WithStrictMode(),
//	)
//
//	var users []User
//	err = db.Select(&users, "SELECT * FROM users WHERE age > ?", 18)
//
//	// Named parameters + auto IN expansion
//	err = db.NamedSelect(&users, "SELECT * FROM users WHERE id IN (:ids)",
//	    map[string]any{"ids": []int{1, 2, 3}})
//
// # Supported Databases
//
// PostgreSQL, MySQL, SQLite, Oracle, SQL Server
package sqlex
