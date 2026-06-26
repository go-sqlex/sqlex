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

	if strings.IndexByte(query, '?') < 0 {
		return query
	}

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

// rebindBuff is kept for benchmarking comparison.
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

// callValuerValue mirrors database/sql.callValuerValue. See #952.
func callValuerValue(vr driver.Valuer) (driver.Value, error) {
	if rv := reflect.ValueOf(vr); rv.Kind() == reflect.Ptr && rv.IsNil() {
		return nil, nil
	}
	return vr.Value()
}

func asSliceForIn(i any) (v reflect.Value, ok bool) {
	if i == nil {
		return reflect.Value{}, false
	}

	v = reflect.ValueOf(i)
	t := reflectx.Deref(v.Type())

	if t.Kind() != reflect.Slice {
		return reflect.Value{}, false
	}

	if t == reflect.TypeOf([]byte{}) {
		return reflect.Value{}, false
	}

	// Nil pointer slice: treat as empty slice; non-nil: dereference to Slice Value.
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return reflect.Zero(t), true
		}
		v = v.Elem()
	}

	return v, true
}

// valueArg/listArg are internal wrappers for AsValue/AsList.
type valueArg struct{ v any }
type listArg struct{ v any }

// AsValue tells In/autoIn not to expand the value into an IN list.
func AsValue(v any) any { return valueArg{v: v} }

// AsList forces slice expansion even without IN (?) context.
func AsList(slice any) any { return listArg{v: slice} }

// In expands slice values in args, returning the modified query string
// and a new arg list that can be executed by a database. The `query` should
// use the `?` bindVar.  The return value uses the `?` bindVar.
//
// Lexical skip: ? inside single/double/backtick-quoted strings, dollar-quoted strings, and
// line/block comments will not be recognized as placeholders; `\?` and `??` output a literal ?.
// Lexical rules are symmetric with Rebind.
//
// Slice expansion rules ("IN list context recognition"):
//   - IN (?) (strict (?) form + preceded by IN keyword, including NOT IN) + slice -> expand
//   - Other positions (ANY(?)/VALUES(?)/func(?) etc.) + slice -> single value, no expand
//   - sqlex.AsValue(v)    -> force no expand
//   - sqlex.AsList(slice) -> force expand
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
			arg, err = callValuerValue(a)
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
		//   default                   -> expand when length>0 and inParen (IN (?) context)
		shouldExpand := argMeta.length > 0 &&
			!argMeta.forceSingle &&
			(inParen || argMeta.forceExpand)

		// IN (?) context + empty slice -> reject (generating IN () is invalid SQL)
		if argMeta.v.IsValid() && argMeta.length == 0 &&
			!argMeta.forceSingle && inParen {
			return "", nil, errors.New("sqlex: empty slice cannot be expanded into IN ()")
		}

		if !shouldExpand {
			if argMeta.v.IsValid() {
				// Slice not expanded (including AsValue / non-IN(?) context empty slice) -> entire slice as single value
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

// nextPlaceholder finds the next ? placeholder, skipping string literals/comments.
// Returns inParen=true when ? is in IN (?) context.
func nextPlaceholder(query string, start int) (idx int, inParen bool) {
	parenPos := -1 // position of the most recent (; -1 = no adjacent (
	for i := start; i < len(query); {
		if end, _, skip := scanSkipSegment(query, i); skip {
			parenPos = -1 // lexical segment breaks adjacency
			i = end
			continue
		}

		c := query[i]
		if c == '?' {
			// \? / ?? escape: skip
			if i > 0 && query[i-1] == '\\' {
				parenPos = -1
				i++
				continue
			}
			if i+1 < len(query) && query[i+1] == '?' {
				parenPos = -1
				i += 2
				continue
			}
			// IN list context: ( adjacent to ?, ? followed by ), and ( preceded by IN
			if parenPos >= 0 && hasMatchingCloseParen(query, i+1) &&
				precededByIn(query, parenPos) {
				return i, true
			}
			return i, false
		}

		// Maintain parenPos: ( records, whitespace keeps, others reset
		switch {
		case c == '(':
			parenPos = i
		case isASCIISpace(c):
			// keep
		default:
			parenPos = -1
		}
		i++
	}
	return -1, false
}

// isASCIISpace only recognizes space / Tab / newline / carriage return, covering 99.99% of real SQL scenarios.
func isASCIISpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

// isIdentByte checks if a byte is a valid SQL identifier character.
func isIdentByte(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9') || c == '_'
}

// hasMatchingCloseParen checks if the next non-whitespace char is ).
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

// precededByIn checks if the token before ( is the IN keyword.
func precededByIn(query string, parenPos int) bool {
	j := parenPos - 1
	for j >= 0 && isASCIISpace(query[j]) {
		j--
	}
	end := j
	for j >= 0 && isIdentByte(query[j]) {
		j--
	}
	tokenStart := j + 1
	if end-tokenStart+1 != 2 { // token length must be exactly 2
		return false
	}
	if c0, c1 := query[tokenStart], query[tokenStart+1]; (c0|0x20) != 'i' || (c1|0x20) != 'n' {
		return false
	}
	return tokenStart == 0 || query[tokenStart-1] != '.'
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
