package sqlex

import (
	"bytes"
	"database/sql/driver"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"sync"

	"github.com/go-sqlex/sqlex/reflectx"
)

// Bindvar types supported by Rebind, BindMap and BindStruct.
const (
	UNKNOWN = iota
	QUESTION
	DOLLAR
	NAMED
	AT
)

var defaultBinds = map[int][]string{
	DOLLAR:   {"postgres", "pgx", "pq-timeouts", "cloudsqlpostgres", "ql", "nrpostgres", "cockroach"},
	QUESTION: {"mysql", "sqlite3", "nrmysql", "nrsqlite3"},
	NAMED:    {"oci8", "ora", "goracle", "godror"},
	AT:       {"sqlserver"},
}

var binds sync.Map

func init() {
	for bind, drivers := range defaultBinds {
		for _, driver := range drivers {
			BindDriver(driver, bind)
		}
	}

}

// BindType returns the bindtype for a given database given a drivername.
func BindType(driverName string) int {
	itype, ok := binds.Load(driverName)
	if !ok {
		return UNKNOWN
	}
	return itype.(int)
}

// BindDriver sets the BindType for driverName to bindType.
func BindDriver(driverName string, bindType int) {
	binds.Store(driverName, bindType)
}

// Rebind a query from the default bindtype (QUESTION) to the target bindtype.
//
// Lexical skip: ? inside single/double/backtick-quoted strings, dollar-quoted strings, and
// line/block comments will not be replaced; `\?` and `??` output a literal ?.
// Rules are symmetric with compileNamedQuery / In.
func Rebind(bindType int, query string) string {
	switch bindType {
	case QUESTION, UNKNOWN:
		return query
	}

	// Fast path: when query contains no ?, skip lexical scanning (already-rebound queries,
	// pure DDL, etc. often hit this path) and return directly to avoid the subsequent
	// make([]byte) allocation.
	if strings.IndexByte(query, '?') < 0 {
		return query
	}

	// Add space enough for 10 params before we have to allocate
	var (
		rqb = make([]byte, 0, len(query)+10)
		j   int
	)

	for i := 0; i < len(query); {
		// Unified lexical scan: if query[i] is the start of a lexical segment to skip,
		// copy the entire segment verbatim
		if end, _, skip := scanSkipSegment(query, i); skip {
			rqb = append(rqb, query[i:end]...)
			i = end
			continue
		}

		if query[i] == '?' {
			// Skip escaped ? (\? or ??)
			if i > 0 && query[i-1] == '\\' {
				// \? -> output ? (replace the previously appended '\')
				rqb = rqb[:len(rqb)-1] // Remove the already-appended '\'
				rqb = append(rqb, '?')
				i++
				continue
			}
			if i+1 < len(query) && query[i+1] == '?' {
				// ?? -> output ?
				rqb = append(rqb, '?')
				i += 2 // Skip both ?s
				continue
			}

			switch bindType {
			case DOLLAR:
				rqb = append(rqb, '$')
			case NAMED:
				rqb = append(rqb, ':', 'a', 'r', 'g')
			case AT:
				rqb = append(rqb, '@', 'p')
			}

			j++
			rqb = strconv.AppendInt(rqb, int64(j), 10)
			i++
		} else {
			rqb = append(rqb, query[i])
			i++
		}
	}

	return string(rqb)
}

// Experimental implementation of Rebind which uses a bytes.Buffer.  The code is
// much simpler and should be more resistant to odd unicode, but it is twice as
// slow.  Kept here for benchmarking purposes and to possibly replace Rebind if
// problems arise with its somewhat naive handling of unicode.
func rebindBuff(bindType int, query string) string {
	if bindType != DOLLAR {
		return query
	}

	b := make([]byte, 0, len(query))
	rqb := bytes.NewBuffer(b)
	j := 1
	for _, r := range query {
		if r == '?' {
			rqb.WriteRune('$')
			rqb.WriteString(strconv.Itoa(j))
			j++
		} else {
			rqb.WriteRune(r)
		}
	}

	return rqb.String()
}

func asSliceForIn(i any) (v reflect.Value, ok bool) {
	if i == nil {
		return reflect.Value{}, false
	}

	v = reflect.ValueOf(i)
	t := reflectx.Deref(v.Type())

	// Only expand slices
	if t.Kind() != reflect.Slice {
		return reflect.Value{}, false
	}

	// []byte is a driver.Value type so it should not be expanded
	if t == reflect.TypeOf([]byte{}) {
		return reflect.Value{}, false

	}

	return v, true
}

// valueArg / listArg are internal wrapper types for AsValue / AsList.
// Unexported struct types prevent users from bypassing the helpers and constructing them directly.
type valueArg struct{ v any }
type listArg struct{ v any }

// AsValue wraps a value, telling In/autoIn not to expand it into an IN list;
// it is passed as a single argument as-is.
//
// Use case: INSERT/UPDATE slice field values, PG's ANY(?)/ALL(?) and other patterns
// that might be misidentified as the (?) form.
//
//	db.Exec("INSERT INTO t (col) VALUES (?)", sqlex.AsValue([]int{1, 2, 3}))
//	db.Select(&rows, "WHERE id = ANY(?)", sqlex.AsValue(pq.Array([]int{1,2,3})))
func AsValue(v any) any { return valueArg{v: v} }

// AsList wraps a slice, forcing expansion into an IN list (even if ? is not in the (?) form).
// Returns an error from In if the argument is not a slice or is an empty slice.
//
//	db.Exec("WHERE x = ?", sqlex.AsList([]int{1, 2, 3}))
//	// Expands to "WHERE x = ?, ?, ?"
func AsList(slice any) any { return listArg{v: slice} }

// In expands slice values in args, returning the modified query string
// and a new arg list that can be executed by a database. The `query` should
// use the `?` bindVar.  The return value uses the `?` bindVar.
//
// Lexical skip: ? inside single/double/backtick-quoted strings, dollar-quoted strings, and
// line/block comments will not be recognized as placeholders; `\?` and `??` output a literal ?.
// Lexical rules are symmetric with Rebind.
//
// Slice expansion rules ("strict (?) context recognition"):
//   - ? in strict (?) form (only ? and optional ASCII whitespace between ( and )) + slice -> expand to ?, ?, ?
//   - ? elsewhere + slice -> no expansion, passed as a single value to the driver
//   - sqlex.AsValue(v)    -> force no expansion (even if ? is in (?) form)
//   - sqlex.AsList(slice) -> force expansion (even if ? is not in (?) form)
//   - driver.Valuer       -> .Value() is called first, then the above rules apply
//   - []byte              -> treated as a single value (standard driver.Value type)
func In(query string, args ...any) (string, []any, error) {
	type argMeta struct {
		v           reflect.Value
		i           any
		length      int
		forceSingle bool // AsValue
		forceExpand bool // AsList
	}

	var flatArgsCount int
	var needRewrite bool

	var stackMeta [32]argMeta

	var meta []argMeta
	if len(args) <= len(stackMeta) {
		meta = stackMeta[:len(args)]
	} else {
		meta = make([]argMeta, len(args))
	}

	for i, arg := range args {
		if r, ok := arg.(valueArg); ok {
			meta[i].i = r.v
			meta[i].forceSingle = true
			needRewrite = true // AsValue wrapper needs unwrapping; must go through main loop
			flatArgsCount++
			continue
		}
		if s, ok := arg.(listArg); ok {
			v, ok := asSliceForIn(s.v)
			if !ok {
				return "", nil, errors.New("sqlex.AsList: argument is not a slice or array")
			}
			if v.Len() == 0 {
				return "", nil, errors.New("sqlex.AsList: empty slice")
			}
			meta[i].v = v
			meta[i].length = v.Len()
			meta[i].forceExpand = true
			needRewrite = true
			flatArgsCount += v.Len()
			continue
		}

		if a, ok := arg.(driver.Valuer); ok {
			var err error
			arg, err = a.Value()
			if err != nil {
				return "", nil, err
			}
		}

		if v, ok := asSliceForIn(arg); ok {
			meta[i].length = v.Len()
			meta[i].v = v

			needRewrite = true
			flatArgsCount += meta[i].length
			// Empty slices are not reported here — deferred to the main loop for (?) form detection
		} else {
			meta[i].i = arg
			flatArgsCount++
		}
	}

	// No rewrite needed: return as-is. Note that this fast path skips the argument
	// count validation below; count mismatches will surface at the driver layer.
	if !needRewrite {
		return query, args, nil
	}

	newArgs := make([]any, 0, flatArgsCount)

	var buf strings.Builder
	buf.Grow(len(query) + len(", ?")*flatArgsCount)

	var arg int
	pos := 0
	for {
		idx, inParen := nextPlaceholder(query, pos)
		if idx < 0 {
			break
		}
		if arg >= len(meta) {
			return "", nil, errors.New("number of bindVars exceeds arguments")
		}

		// [pos, idx] includes skipped lexical ranges; write them verbatim (including the current ?)
		buf.WriteString(query[pos : idx+1])

		argMeta := meta[arg]
		arg++

		// Expansion logic:
		//   forceSingle (AsValue)     -> never expand
		//   forceExpand (AsList)      -> always expand
		//   default                   -> expand when length>0 and inParen
		shouldExpand := argMeta.length > 0 &&
			!argMeta.forceSingle &&
			(inParen || argMeta.forceExpand)

		// Strict (?) form + empty slice -> reject (generating IN () is invalid SQL)
		if argMeta.v.IsValid() && argMeta.length == 0 &&
			!argMeta.forceSingle && inParen {
			return "", nil, errors.New("sqlex: empty slice cannot be expanded into IN ()")
		}

		if !shouldExpand {
			if argMeta.v.IsValid() {
				// Slice not expanded (including AsValue / non-(?) form empty slice) -> entire slice as single value
				newArgs = append(newArgs, argMeta.v.Interface())
			} else {
				newArgs = append(newArgs, argMeta.i)
			}
		} else {
			// Expand: current ? already written, append length-1 more ", ?"
			for si := 1; si < argMeta.length; si++ {
				buf.WriteString(", ?")
			}
			newArgs = appendReflectSlice(newArgs, argMeta.v, argMeta.length)
		}

		pos = idx + 1
	}

	buf.WriteString(query[pos:])

	if arg < len(meta) {
		return "", nil, errors.New("number of bindVars less than number arguments")
	}

	return buf.String(), newArgs, nil
}

// nextPlaceholder finds the next real ? placeholder position in query starting from start.
// It automatically skips string literals, double/backtick-quoted identifiers, dollar-quoted strings,
// line/block comments, and \? and ?? escapes. Returns idx=-1 if not found.
//
// inParen indicates whether the ? is in the strict (?) form: only one ? and optional ASCII
// whitespace (' ' / '\t' / '\n' / '\r') are allowed between ( and ). This is the core
// heuristic sqlex uses to distinguish IN list context from other contexts.
//
// Known edge case: ANY(?) / ALL(?) / func(?) and other "preceded by a letter" patterns are
// still recognized as (?); users must use sqlex.AsValue to explicitly suppress expansion.
func nextPlaceholder(query string, start int) (idx int, inParen bool) {
	afterParen := false // left side has seen ( + only ASCII whitespace
	for i := start; i < len(query); {
		if end, _, skip := scanSkipSegment(query, i); skip {
			afterParen = false // Lexical segment breaks the adjacency of ( and ?
			i = end
			continue
		}

		c := query[i]
		if c == '?' {
			// \? / ?? escape: skip
			if i > 0 && query[i-1] == '\\' {
				afterParen = false
				i++
				continue
			}
			if i+1 < len(query) && query[i+1] == '?' {
				afterParen = false
				i += 2
				continue
			}
			if afterParen && hasMatchingCloseParen(query, i+1) {
				return i, true
			}
			return i, false
		}

		// Maintain afterParen: ( enters, whitespace keeps, anything else leaves
		switch {
		case c == '(':
			afterParen = true
		case isASCIISpace(c):
			// keep
		default:
			afterParen = false
		}
		i++
	}
	return -1, false
}

// isASCIISpace only recognizes space / Tab / newline / carriage return, covering 99.99% of real SQL scenarios.
func isASCIISpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

// hasMatchingCloseParen looks ahead from start, skipping ASCII whitespace, and checks whether
// the next non-whitespace character is ). Does not skip comments/strings — those characters
// will cause detection to fail; users can use sqlex.AsList to force expansion.
func hasMatchingCloseParen(query string, start int) bool {
	for j := start; j < len(query); j++ {
		c := query[j]
		if isASCIISpace(c) {
			continue
		}
		return c == ')'
	}
	return false
}

func appendReflectSlice(args []any, v reflect.Value, vlen int) []any {
	switch val := v.Interface().(type) {
	case []any:
		args = append(args, val...)
	case []int:
		for i := range val {
			args = append(args, val[i])
		}
	case []string:
		for i := range val {
			args = append(args, val[i])
		}
	default:
		for si := 0; si < vlen; si++ {
			args = append(args, v.Index(si).Interface())
		}
	}

	return args
}
