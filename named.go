package sqlex

// Named Query Support
//
//  * BindMap - bind query bindvars to map/struct args
//	* NamedExec, NamedQuery - named query w/ struct or map
//  * NamedStmt - a pre-compiled named query which is a prepared statement
//
// Internal Interfaces:
//
//  * compileNamedQuery - rebind a named query, returning a query and list of names
//  * bindArgs, bindMapArgs, bindAnyArgs - given a list of names, return an arglist
//
import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/go-sqlex/sqlex/reflectx"
)

// NamedStmt is a prepared statement that executes named queries.  Prepare it
// how you would execute a NamedQuery, but pass in a struct or map when executing.
// NamedStmt does not support IN slice expansion; use db.NamedSelect instead.
type NamedStmt struct {
	Params      []string
	QueryString string
	Stmt        *Stmt
	strict      bool
}

// Close closes the named statement.
func (n *NamedStmt) Close() error {
	return n.Stmt.Close()
}

// GetMapper returns the Mapper for this NamedStmt.
func (n *NamedStmt) GetMapper() *reflectx.Mapper {
	return n.Stmt.Mapper
}

// Exec executes a named statement using the struct passed.
// Any named placeholder parameters are replaced with fields from arg.
func (n *NamedStmt) Exec(arg any) (sql.Result, error) {
	args, err := bindAnyArgs(n.Params, arg, n.Stmt.Mapper)
	if err != nil {
		return nil, err
	}
	return n.Stmt.Exec(args...)
}

// Query executes a named statement using the struct argument, returning rows.
// Any named placeholder parameters are replaced with fields from arg.
func (n *NamedStmt) Query(arg any) (*sql.Rows, error) {
	args, err := bindAnyArgs(n.Params, arg, n.Stmt.Mapper)
	if err != nil {
		return nil, err
	}
	return n.Stmt.Query(args...)
}

// QueryRow executes a named statement against the database.  Because sqlex cannot
// create a *sql.Row with an error condition pre-set for binding errors, sqlex
// returns a *sqlex.Row instead.
// Any named placeholder parameters are replaced with fields from arg.
func (n *NamedStmt) QueryRow(arg any) *Row {
	args, err := bindAnyArgs(n.Params, arg, n.Stmt.Mapper)
	if err != nil {
		return &Row{err: err}
	}
	return n.Stmt.QueryRowx(args...)
}

// MustExec execs a NamedStmt, panicing on error
// Any named placeholder parameters are replaced with fields from arg.
func (n *NamedStmt) MustExec(arg any) sql.Result {
	res, err := n.Exec(arg)
	if err != nil {
		panic(err)
	}
	return res
}

// Queryx using this NamedStmt
// Any named placeholder parameters are replaced with fields from arg.
func (n *NamedStmt) Queryx(arg any) (*Rows, error) {
	r, err := n.Query(arg)
	if err != nil {
		return nil, err
	}
	return &Rows{Rows: r, Mapper: n.Stmt.Mapper, strict: n.strict}, nil
}

// QueryRowx this NamedStmt.  Because of limitations with QueryRow, this is
// an alias for QueryRow.
// Any named placeholder parameters are replaced with fields from arg.
func (n *NamedStmt) QueryRowx(arg any) *Row {
	return n.QueryRow(arg)
}

// Select using this NamedStmt
// Any named placeholder parameters are replaced with fields from arg.
func (n *NamedStmt) Select(dest any, arg any) error {
	rows, err := n.Queryx(arg)
	if err != nil {
		return err
	}
	// if something happens here, we want to make sure the rows are Closed
	defer rows.Close()
	return scanAll(rows, dest, false)
}

// Get using this NamedStmt
// Any named placeholder parameters are replaced with fields from arg.
func (n *NamedStmt) Get(dest any, arg any) error {
	r := n.QueryRowx(arg)
	return r.scanAny(dest, false)
}

// ExecContext executes a named statement using the struct passed.
// Any named placeholder parameters are replaced with fields from arg.
func (n *NamedStmt) ExecContext(ctx context.Context, arg any) (sql.Result, error) {
	args, err := bindAnyArgs(n.Params, arg, n.Stmt.Mapper)
	if err != nil {
		return nil, err
	}
	return n.Stmt.ExecContext(ctx, args...)
}

// QueryContext executes a named statement using the struct argument, returning rows.
// Any named placeholder parameters are replaced with fields from arg.
func (n *NamedStmt) QueryContext(ctx context.Context, arg any) (*sql.Rows, error) {
	args, err := bindAnyArgs(n.Params, arg, n.Stmt.Mapper)
	if err != nil {
		return nil, err
	}
	return n.Stmt.QueryContext(ctx, args...)
}

// QueryRowContext executes a named statement against the database.  Because sqlex cannot
// create a *sql.Row with an error condition pre-set for binding errors, sqlex
// returns a *sqlex.Row instead.
// Any named placeholder parameters are replaced with fields from arg.
func (n *NamedStmt) QueryRowContext(ctx context.Context, arg any) *Row {
	args, err := bindAnyArgs(n.Params, arg, n.Stmt.Mapper)
	if err != nil {
		return &Row{err: err}
	}
	return n.Stmt.QueryRowxContext(ctx, args...)
}

// MustExecContext execs a NamedStmt, panicing on error
// Any named placeholder parameters are replaced with fields from arg.
func (n *NamedStmt) MustExecContext(ctx context.Context, arg any) sql.Result {
	res, err := n.ExecContext(ctx, arg)
	if err != nil {
		panic(err)
	}
	return res
}

// QueryxContext using this NamedStmt
// Any named placeholder parameters are replaced with fields from arg.
func (n *NamedStmt) QueryxContext(ctx context.Context, arg any) (*Rows, error) {
	r, err := n.QueryContext(ctx, arg)
	if err != nil {
		return nil, err
	}
	return &Rows{Rows: r, Mapper: n.Stmt.Mapper, strict: n.strict}, nil
}

// QueryRowxContext this NamedStmt.  Because of limitations with QueryRow, this is
// an alias for QueryRow.
// Any named placeholder parameters are replaced with fields from arg.
func (n *NamedStmt) QueryRowxContext(ctx context.Context, arg any) *Row {
	return n.QueryRowContext(ctx, arg)
}

// SelectContext using this NamedStmt
// Any named placeholder parameters are replaced with fields from arg.
func (n *NamedStmt) SelectContext(ctx context.Context, dest any, arg any) error {
	rows, err := n.QueryxContext(ctx, arg)
	if err != nil {
		return err
	}
	// if something happens here, we want to make sure the rows are Closed
	defer rows.Close()
	return scanAll(rows, dest, false)
}

// GetContext using this NamedStmt
// Any named placeholder parameters are replaced with fields from arg.
func (n *NamedStmt) GetContext(ctx context.Context, dest any, arg any) error {
	r := n.QueryRowxContext(ctx, arg)
	return r.scanAny(dest, false)
}

// Preparer interface already embeds binder, so there is no need to define a separate namedPreparer interface.
func prepareNamed(p Preparer, query string) (*NamedStmt, error) {
	bindType := BindType(p.DriverName())
	q, args, err := compileNamedQuery([]byte(query), bindType)
	if err != nil {
		return nil, err
	}
	stmt, err := Preparex(p, q)
	if err != nil {
		return nil, err
	}
	return &NamedStmt{
		QueryString: q,
		Params:      args,
		Stmt:        stmt,
		strict:      strictFor(p),
	}, nil
}

// PreparerContext interface already embeds binder, so there is no need to define a separate namedPreparerContext interface.

func prepareNamedContext(ctx context.Context, p PreparerContext, query string) (*NamedStmt, error) {
	bindType := BindType(p.DriverName())
	q, args, err := compileNamedQuery([]byte(query), bindType)
	if err != nil {
		return nil, err
	}
	stmt, err := PreparexContext(ctx, p, q)
	if err != nil {
		return nil, err
	}
	return &NamedStmt{
		QueryString: q,
		Params:      args,
		Stmt:        stmt,
		strict:      strictFor(p),
	}, nil
}

// convertMapStringInterface attempts to convert v to map[string]any.
// Unlike v.(map[string]any), this function works on named types that
// are convertible to map[string]any as well.
func convertMapStringInterface(v any) (map[string]any, bool) {
	var m map[string]any
	mtype := reflect.TypeOf(m)
	t := reflect.TypeOf(v)
	if !t.ConvertibleTo(mtype) {
		return nil, false
	}
	return reflect.ValueOf(v).Convert(mtype).Interface().(map[string]any), true

}

func bindAnyArgs(names []string, arg any, m *reflectx.Mapper) ([]any, error) {
	if maparg, ok := convertMapStringInterface(arg); ok {
		return bindMapArgs(names, maparg)
	}
	return bindArgs(names, arg, m)
}

// bindArgs extracts values from a struct in the order of names. names have already been
// filtered at compile time by compileNamedQueryWith to only include fields that exist in the struct.
func bindArgs(names []string, arg any, m *reflectx.Mapper) ([]any, error) {
	arglist := make([]any, 0, len(names))
	var v reflect.Value
	for v = reflect.ValueOf(arg); v.Kind() == reflect.Ptr; {
		v = v.Elem()
	}
	err := m.TraversalsByNameFunc(v.Type(), names, func(i int, t []int) error {
		if len(t) == 0 {
			return nil // Defensive fallback; should not be reached in theory
		}
		arglist = append(arglist, reflectx.FieldByIndexesReadOnly(v, t).Interface())
		return nil
	})
	return arglist, err
}

// bindMapArgs extracts values from a map in the order of names. names have already been
// filtered at compile time by compileNamedQueryWith to only include keys that exist in the map.
func bindMapArgs(names []string, arg map[string]any) ([]any, error) {
	arglist := make([]any, 0, len(names))
	for _, name := range names {
		if val, ok := arg[name]; ok {
			arglist = append(arglist, val)
		}
	}
	return arglist, nil
}

// nameExistsInMap / nameExistsInStruct provide the nameExists closure for compileNamedQueryWith:
// they determine whether a parameter name actually exists in args (a key check for the error-tolerance path).

func nameExistsInMap(arg map[string]any) func(string) bool {
	return func(name string) bool { _, ok := arg[name]; return ok }
}

func nameExistsInStruct(arg any, m *reflectx.Mapper) func(string) bool {
	var v reflect.Value
	for v = reflect.ValueOf(arg); v.Kind() == reflect.Ptr; {
		v = v.Elem()
	}
	tm := m.TypeMap(v.Type())
	return func(name string) bool { _, ok := tm.Names[name]; return ok }
}

// bindStruct binds named parameters in a query using struct fields. Field names are mapped
// via the `db` tag, consistent with StructScan. For error-tolerance mode, see compileNamedQueryWith.
func bindStruct(bindType int, query string, arg any, m *reflectx.Mapper) (string, []any, error) {
	bound, names, err := compileNamedQueryWith([]byte(query), bindType, nameExistsInStruct(arg, m))
	if err != nil {
		return "", []any{}, err
	}
	arglist, err := bindAnyArgs(names, arg, m)
	if err != nil {
		return "", []any{}, err
	}
	return bound, arglist, nil
}

func findMatchingClosingBracketIndex(s string) int {
	count := 0
	for i := 0; i < len(s); {
		// Skip string literals, comments, quoted identifiers, dollar-quoted strings
		if end, _, skip := scanSkipSegment(s, i); skip {
			i = end
			continue
		}
		ch := s[i]
		if ch == '(' {
			count++
		}
		if ch == ')' {
			count--
			if count == 0 {
				return i
			}
		}
		i++
	}
	return 0
}

// findValuesKeyword finds VALUES keyword position, skipping string literals/comments.
// Does not require ) before VALUES (fixes #898).
func findValuesKeyword(s string) int {
	for i := 0; i < len(s); {
		if end, _, skip := scanSkipSegment(s, i); skip {
			i = end
			continue
		}
		if isKeywordAt(s, i, "VALUES") {
			return i
		}
		i++
	}
	return -1
}

// isKeywordAt checks keyword match at pos with word boundary (case-insensitive).
func isKeywordAt(s string, pos int, keyword string) bool {
	if pos+len(keyword) > len(s) {
		return false
	}
	for j := 0; j < len(keyword); j++ {
		if (s[pos+j] | 0x20) != (keyword[j] | 0x20) {
			return false
		}
	}
	// word boundary check
	if pos > 0 && isIdentByte(s[pos-1]) {
		return false
	}
	end := pos + len(keyword)
	if end < len(s) && isIdentByte(s[end]) {
		return false
	}
	return true
}

func fixBound(bound string, loop int) string {
	valuesPos := findValuesKeyword(bound)
	if valuesPos < 0 {
		return bound
	}

	rest := bound[valuesPos+len("VALUES"):]
	openOffset := strings.IndexByte(rest, '(')
	if openOffset < 0 {
		return bound
	}
	openingBracketIndex := valuesPos + len("VALUES") + openOffset

	index := findMatchingClosingBracketIndex(bound[openingBracketIndex:])
	if index == 0 {
		return bound
	}
	closingBracketIndex := openingBracketIndex + index + 1

	var buffer bytes.Buffer
	buffer.WriteString(bound[0:closingBracketIndex])
	for i := 0; i < loop-1; i++ {
		buffer.WriteString(",")
		buffer.WriteString(bound[openingBracketIndex:closingBracketIndex])
	}
	buffer.WriteString(bound[closingBracketIndex:])
	return buffer.String()
}

// bindArray binds a named parameter query with fields from an array or slice of
// structs argument.
func bindArray(bindType int, query string, arg any, m *reflectx.Mapper) (string, []any, error) {
	// do the initial binding with QUESTION;  if bindType is not question,
	// we can rebind it at the end.
	bound, names, err := compileNamedQuery([]byte(query), QUESTION)
	if err != nil {
		return "", []any{}, err
	}
	arrayValue := reflect.ValueOf(arg)
	arrayLen := arrayValue.Len()
	if arrayLen == 0 {
		return "", []any{}, fmt.Errorf("length of array is 0: %#v", arg)
	}
	var arglist = make([]any, 0, len(names)*arrayLen)
	for i := 0; i < arrayLen; i++ {
		elemArglist, err := bindAnyArgs(names, arrayValue.Index(i).Interface(), m)
		if err != nil {
			return "", []any{}, err
		}
		arglist = append(arglist, elemArglist...)
	}
	if arrayLen > 1 {
		bound = fixBound(bound, arrayLen)
	}
	// adjust binding type if we weren't on question
	if bindType != QUESTION {
		bound = Rebind(bindType, bound)
	}
	return bound, arglist, nil
}

// bindMap binds named parameters in a query using a map. For error-tolerance mode, see compileNamedQueryWith.
func bindMap(bindType int, query string, args map[string]any) (string, []any, error) {
	bound, names, err := compileNamedQueryWith([]byte(query), bindType, nameExistsInMap(args))
	if err != nil {
		return "", []any{}, err
	}
	arglist, err := bindMapArgs(names, args)
	return bound, arglist, err
}

// -- Compilation of Named Queries
//
// Named parameter name rules (ASCII, unified byte scanning, Unicode parameter names are not supported):
//   - First character: letter or underscore [A-Za-z_] (also fixes the old issue where `:123` was recognized as parameter name 123)
//   - Subsequent characters: [A-Za-z0-9_.] (dot supports nested fields :user.name)
//
// Unicode in other SQL positions (table names / column names / string literals) is unaffected;
// byte scanning is inherently safe.

func isBindStartByte(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_'
}

func isBindNameByte(c byte) bool {
	return isBindStartByte(c) || (c >= '0' && c <= '9') || c == '.'
}

// compileNamedQuery is equivalent to compileNamedQueryWith(qs, bindType, nil) — all
// recognized :name are treated as parameters. Use this when the caller does not yet know args
// (e.g. bindArray compiles first, then expands by array length).
func compileNamedQuery(qs []byte, bindType int) (query string, names []string, err error) {
	return compileNamedQueryWith(qs, bindType, nil)
}

// compileNamedQueryWith compiles a named-parameter query into a placeholder query for the target bindType.
//
// Error-tolerance semantics: for each recognized :name, if nameExists==nil or nameExists(name)==true,
// it is converted to a placeholder per bindType and recorded in names; otherwise, the :name literal
// is written back as-is and not counted for numbering. The latter provides a fallback for rare lexical
// false positives (e.g. 'a:6e:...' / 'HH:mm:ss' where the colon truly means string content), allowing
// the original SQL to still be correctly executed by the driver/server. This behavior is consistent
// with GORM's @name handling.
//
// Single-pass compilation — avoids a two-phase "number all + renumber" implementation and its
// associated lexical drift risk.
//
// Lexical skip (reuses lexer.go's scanSkipSegment, shared with Rebind / In):
// :name inside string literals, double/backtick-quoted identifiers, dollar-quoted strings, and
// line/block comments are not recognized as parameters; additionally, :: (PG type cast) and
// := (assignment) are handled as non-parameter uses of colon.
func compileNamedQueryWith(qs []byte, bindType int, nameExists func(string) bool) (query string, names []string, err error) {
	var (
		q          = string(qs)
		rebound    = make([]byte, 0, len(qs))
		currentVar = 1
	)
	names = make([]string, 0, 10)

	// emitParam converts :name to a placeholder for the target bindType and records it in names.
	emitParam := func(name string) {
		names = append(names, name)
		switch bindType {
		case NAMED:
			rebound = append(rebound, ':')
			rebound = append(rebound, name...)
		case QUESTION, UNKNOWN:
			rebound = append(rebound, '?')
		case DOLLAR:
			rebound = append(rebound, '$')
			rebound = strconv.AppendInt(rebound, int64(currentVar), 10)
			currentVar++
		case AT:
			rebound = append(rebound, '@', 'p')
			rebound = strconv.AppendInt(rebound, int64(currentVar), 10)
			currentVar++
		default:
			rebound = append(rebound, '?')
		}
	}

	// emitLiteral writes the :name literal back as-is (error-tolerance: the parameter does not exist in args).
	emitLiteral := func(name string) {
		rebound = append(rebound, ':')
		rebound = append(rebound, name...)
	}

	for i := 0; i < len(q); {
		// Skip string/identifier/dollar-quote/comment segments
		if end, _, skip := scanSkipSegment(q, i); skip {
			rebound = append(rebound, q[i:end]...)
			i = end
			continue
		}

		if q[i] != ':' {
			rebound = append(rebound, q[i])
			i++
			continue
		}

		// q[i] == ':', first exclude :: / := / invalid first character
		if i+1 < len(q) && q[i+1] == ':' { // PG type cast
			rebound = append(rebound, ':', ':')
			i += 2
			continue
		}
		if i+1 < len(q) && q[i+1] == '=' { // Assignment operator
			rebound = append(rebound, ':', '=')
			i += 2
			continue
		}
		if i+1 >= len(q) || !isBindStartByte(q[i+1]) {
			rebound = append(rebound, ':')
			i++
			continue
		}

		// Read parameter name
		j := i + 1
		for j < len(q) && isBindNameByte(q[j]) {
			j++
		}
		name := q[i+1 : j]
		if nameExists == nil || nameExists(name) {
			emitParam(name)
		} else {
			emitLiteral(name)
		}
		i = j
	}

	return string(rebound), names, err
}

// BindNamed binds a struct or a map to a query with named parameters.
//
// Deprecated: It is recommended to use db.NamedSelect / NamedExec / NamedQuery etc. directly,
// as they already include Rebind/Hook/StrictMode aspects automatically, eliminating the need
// to call BindNamed manually. Use this function only when you specifically need to control
// bindType yourself (rare).
func BindNamed(bindType int, query string, arg any) (string, []any, error) {
	return bindNamedMapper(bindType, query, arg, mapper())
}

// Named takes a query using named parameters and an argument and
// returns a new query with a list of args that can be executed by
// a database.  The return value uses the `?` bindvar.
//
// Deprecated: It is recommended to use db.NamedSelect / NamedExec / NamedQuery etc. directly,
// as they already include Rebind/Hook/StrictMode aspects automatically. Use this function only
// in dynamic SQL composition scenarios (e.g. multi-segment UNION, dynamic WHERE condition merging)
// that cannot be expressed with the higher-level methods.
func Named(query string, arg any) (string, []any, error) {
	return bindNamedMapper(QUESTION, query, arg, mapper())
}

func bindNamedMapper(bindType int, query string, arg any, m *reflectx.Mapper) (string, []any, error) {
	t := reflect.TypeOf(arg)
	k := t.Kind()
	switch {
	case k == reflect.Map && t.Key().Kind() == reflect.String:
		m, ok := convertMapStringInterface(arg)
		if !ok {
			return "", nil, fmt.Errorf("sqlex.bindNamedMapper: unsupported map type: %T", arg)
		}
		return bindMap(bindType, query, m)
	case k == reflect.Array || k == reflect.Slice:
		return bindArray(bindType, query, arg, m)
	default:
		return bindStruct(bindType, query, arg, m)
	}
}

// NamedQuery binds a named query and then runs Query on the result using the
// provided Ext (sqlex.Tx, sqlex.Db).  It works with both structs and with
// map[string]any types.
func NamedQuery(e Ext, query string, arg any) (*Rows, error) {
	q, args, err := bindNamedMapper(QUESTION, query, arg, mapperFor(e))
	if err != nil {
		return nil, err
	}
	return e.Queryx(q, args...)
}

// NamedExec uses BindStruct to get a query executable by the driver and
// then runs Exec on the result.  Returns an error from the binding
// or the query execution itself.
func NamedExec(e Ext, query string, arg any) (sql.Result, error) {
	q, args, err := bindNamedMapper(QUESTION, query, arg, mapperFor(e))
	if err != nil {
		return nil, err
	}
	return e.Exec(q, args...)
}

// NamedQueryContext binds a named query and then runs Query on the result using the
// provided Ext (sqlex.Tx, sqlex.Db).  It works with both structs and with
// map[string]any types.
func NamedQueryContext(ctx context.Context, e ExtContext, query string, arg any) (*Rows, error) {
	q, args, err := bindNamedMapper(QUESTION, query, arg, mapperFor(e))
	if err != nil {
		return nil, err
	}
	return e.QueryxContext(ctx, q, args...)
}

// NamedExecContext uses BindStruct to get a query executable by the driver and
// then runs Exec on the result.  Returns an error from the binding
// or the query execution itself.
func NamedExecContext(ctx context.Context, e ExtContext, query string, arg any) (sql.Result, error) {
	q, args, err := bindNamedMapper(QUESTION, query, arg, mapperFor(e))
	if err != nil {
		return nil, err
	}
	return e.ExecContext(ctx, q, args...)
}

// --- Transparent IN query helper functions ---
//
// Design note: All top-level NamedXxx functions and DB / Tx / Conn NamedXxx methods
// only do "named -> ? conversion + forwarding"; they do not perform autoIn / Rebind / Hook
// themselves — these are guaranteed by the downstream Queryer / Execer interface implementations
// (see interface contracts). This avoids multiple autoIn calls and keeps each layer's
// responsibility single.

// needsInRewrite determines whether the arguments need to go through the In path.
//
// Must go through In:
//  1. Plain slice/array (to be expanded)
//  2. sqlex.AsValue / sqlex.AsList wrappers (must unwrap the valueArg/listArg outer shell,
//     otherwise the driver will report "unsupported type")
//
// Not handled edge cases (YAGNI): slice pointers / Valuer returning a slice (violates driver.Value spec).
func needsInRewrite(args []any) bool {
	for _, arg := range args {
		switch arg.(type) {
		case valueArg, listArg:
			return true
		}
		if _, ok := arg.(driver.Valuer); ok {
			continue
		}
		if _, ok := arg.([]byte); ok {
			continue
		}
		v := reflect.ValueOf(arg)
		if v.IsValid() && (v.Kind() == reflect.Slice || v.Kind() == reflect.Array) {
			return true
		}
	}
	return false
}

// autoIn automatically detects slice arguments and expands IN (?) placeholders.
// Returns the original query and args unchanged if no slice arguments are present (zero overhead).
func autoIn(query string, args ...any) (string, []any, error) {
	if needsInRewrite(args) {
		var err error
		query, args, err = In(query, args...)
		if err != nil {
			return "", nil, err
		}
	}
	return query, args, nil
}

// --- DB Named convenience methods ---

// NamedGetContext executes a query with named parameters and scans a single row result.
func (db *DB) NamedGetContext(ctx context.Context, dest any, query string, param any) error {
	q, args, err := bindNamedMapper(QUESTION, query, param, db.Mapper)
	if err != nil {
		return err
	}
	return db.GetContext(ctx, dest, q, args...)
}

// NamedGet executes a query with named parameters and scans a single row result.
func (db *DB) NamedGet(dest any, query string, param any) error {
	return db.NamedGetContext(context.Background(), dest, query, param)
}

// NamedSelectContext executes a query with named parameters and scans the result set into a slice.
func (db *DB) NamedSelectContext(ctx context.Context, dest any, query string, param any) error {
	q, args, err := bindNamedMapper(QUESTION, query, param, db.Mapper)
	if err != nil {
		return err
	}
	return db.SelectContext(ctx, dest, q, args...)
}

// NamedSelect executes a query with named parameters and scans the result set into a slice.
func (db *DB) NamedSelect(dest any, query string, param any) error {
	return db.NamedSelectContext(context.Background(), dest, query, param)
}

// --- Tx Named convenience methods ---

// NamedGetContext executes a query with named parameters and scans a single row result.
func (tx *Tx) NamedGetContext(ctx context.Context, dest any, query string, param any) error {
	q, args, err := bindNamedMapper(QUESTION, query, param, tx.Mapper)
	if err != nil {
		return err
	}
	return tx.GetContext(ctx, dest, q, args...)
}

// NamedGet executes a query with named parameters and scans a single row result.
func (tx *Tx) NamedGet(dest any, query string, param any) error {
	return tx.NamedGetContext(context.Background(), dest, query, param)
}

// NamedSelectContext executes a query with named parameters and scans the result set into a slice.
func (tx *Tx) NamedSelectContext(ctx context.Context, dest any, query string, param any) error {
	q, args, err := bindNamedMapper(QUESTION, query, param, tx.Mapper)
	if err != nil {
		return err
	}
	return tx.SelectContext(ctx, dest, q, args...)
}

// NamedSelect executes a query with named parameters and scans the result set into a slice.
func (tx *Tx) NamedSelect(dest any, query string, param any) error {
	return tx.NamedSelectContext(context.Background(), dest, query, param)
}

// --- Conn Named convenience methods ---

// NamedGetContext executes a query with named parameters and scans a single row result.
func (c *Conn) NamedGetContext(ctx context.Context, dest any, query string, param any) error {
	q, args, err := bindNamedMapper(QUESTION, query, param, c.Mapper)
	if err != nil {
		return err
	}
	return c.GetContext(ctx, dest, q, args...)
}

// NamedSelectContext executes a query with named parameters and scans the result set into a slice.
func (c *Conn) NamedSelectContext(ctx context.Context, dest any, query string, param any) error {
	q, args, err := bindNamedMapper(QUESTION, query, param, c.Mapper)
	if err != nil {
		return err
	}
	return c.SelectContext(ctx, dest, q, args...)
}
