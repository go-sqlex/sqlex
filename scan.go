package sqlex

import (
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/go-sqlex/sqlex/reflectx"
)

// ColScanner is an interface used by MapScan and SliceScan
type ColScanner interface {
	Columns() ([]string, error)
	Scan(dest ...any) error
	Err() error
}

type rowsi interface {
	Close() error
	Columns() ([]string, error)
	Err() error
	Next() bool
	Scan(...any) error
}

// structOnlyError returns an error appropriate for type when a non-scannable
// struct is expected but something else is given
func structOnlyError(t reflect.Type) error {
	isStruct := t.Kind() == reflect.Struct
	isScanner := reflect.PointerTo(t).Implements(_scannerInterface)
	if !isStruct {
		return fmt.Errorf("expected %s but got %s", reflect.Struct, t.Kind())
	}
	if isScanner {
		return fmt.Errorf("structscan expects a struct dest but the provided struct type %s implements scanner", t.Name())
	}
	return fmt.Errorf("expected a struct, but struct %s has no exported fields", t.Name())
}

// scanAll scans all rows into a destination, which must be a slice of any
// type.  It resets the slice length to zero before appending each element to
// the slice.  If the destination slice type is a Struct, then StructScan will
// be used on each row.  If the destination is some other kind of base type,
// then each row must only have one column which can scan into that type.  This
// allows you to do something like:
//
//	rows, _ := db.Query("select id from people;")
//	var ids []int
//	scanAll(rows, &ids, false)
//
// and ids will be a list of the id results.
func scanAll(rows rowsi, dest any, structOnly bool) error {
	var v, vp reflect.Value

	value := reflect.ValueOf(dest)

	// json.Unmarshal returns errors for these
	if value.Kind() != reflect.Ptr {
		return errors.New("must pass a pointer, not a value, to StructScan destination")
	}
	if value.IsNil() {
		return errors.New("nil pointer passed to StructScan destination")
	}
	direct := reflect.Indirect(value)

	slice, err := baseType(value.Type(), reflect.Slice)
	if err != nil {
		return err
	}
	direct.SetLen(0)

	var (
		isPtr     = slice.Elem().Kind() == reflect.Ptr
		base      = reflectx.Deref(slice.Elem())
		scannable = isScannable(base)
	)

	// Reject sql.RawBytes to prevent buffer-reuse corruption. See #931.
	if containsRawBytes(base) {
		return errors.New("sqlex: sql.RawBytes is not allowed in Select/ScanAll (use []byte instead)")
	}

	if structOnly && scannable {
		return structOnlyError(base)
	}

	columns, err := rows.Columns()
	if err != nil {
		return err
	}

	// if it's a base type make sure it only has 1 column;  if not return an error
	if scannable && len(columns) > 1 {
		return fmt.Errorf("non-struct dest type %s with >1 columns (%d)", base.Kind(), len(columns))
	}

	if !scannable {
		var values []any
		var m *reflectx.Mapper
		var strict bool

		switch rows := rows.(type) {
		case *Rows:
			m = rows.Mapper
			strict = rows.strict
		default:
			m = mapper()
			strict = false // default lenient mode
		}

		fields := m.TraversalsByName(base, columns)
		// In strict mode, check for missing fields
		if strict {
			if err := checkMissingFields(fields, columns, dest); err != nil {
				return err
			}
		}
		values = make([]any, len(columns))

		for rows.Next() {
			// create a new struct type (which returns PtrTo) and indirect it
			vp = reflect.New(base)
			v = reflect.Indirect(vp)

			err = fieldsByTraversal(v, fields, values, true)
			if err != nil {
				return err
			}

			// scan into the struct field pointers and append to our results
			err = rows.Scan(values...)
			if err != nil {
				return err
			}

			if isPtr {
				direct.Set(reflect.Append(direct, vp))
			} else {
				direct.Set(reflect.Append(direct, v))
			}
		}
	} else {
		for rows.Next() {
			vp = reflect.New(base)
			err = rows.Scan(vp.Interface())
			if err != nil {
				return err
			}
			// append
			if isPtr {
				direct.Set(reflect.Append(direct, vp))
			} else {
				direct.Set(reflect.Append(direct, reflect.Indirect(vp)))
			}
		}
	}

	return rows.Err()
}

// StructScan all rows from an sql.Rows or an sqlex.Rows into the dest slice.
// StructScan will scan in the entire rows result, so if you do not want to
// allocate structs for the entire result, use Queryx and see sqlex.Rows.StructScan.
// If rows is sqlex.Rows, it will use its mapper, otherwise it will use the default.
//
// Deprecated: Use ScanAll instead, which has a clearer name.
func StructScan(rows rowsi, dest any) error {
	return scanAll(rows, dest, true)
}

// ScanAll scans all rows from an sql.Rows or an sqlex.Rows into the dest slice.
// ScanAll is the preferred alias for StructScan with a clearer naming convention.
// If rows is sqlex.Rows, it will use its mapper, otherwise it will use the default.
func ScanAll(rows rowsi, dest any) error {
	return scanAll(rows, dest, true)
}

// SliceScan a row, returning a []any with values similar to MapScan.
// This function is primarily intended for use where the number of columns
// is not known.  Because you can pass an []any directly to Scan,
// it's recommended that you do that as it will not have to allocate new
// slices per row.
func SliceScan(r ColScanner) ([]any, error) {
	// ignore r.started, since we needn't use reflect for anything.
	columns, err := r.Columns()
	if err != nil {
		return []any{}, err
	}

	values := make([]any, len(columns))
	for i := range values {
		values[i] = new(any)
	}

	err = r.Scan(values...)

	if err != nil {
		return values, err
	}

	for i := range columns {
		values[i] = *(values[i].(*any))
	}

	return values, r.Err()
}

// MapScan scans a single Row into the dest map[string]any.
// Use this to get results for SQL that might not be under your control
// (for instance, if you're building an interface for an SQL server that
// executes SQL from input).  Please do not use this as a primary interface!
// This will modify the map sent to it in place, so reuse the same map with
// care.  Columns which occur more than once in the result will overwrite
// each other!
func MapScan(r ColScanner, dest map[string]any) error {
	// ignore r.started, since we needn't use reflect for anything.
	columns, err := r.Columns()
	if err != nil {
		return err
	}

	values := make([]any, len(columns))
	for i := range values {
		values[i] = new(any)
	}

	err = r.Scan(values...)
	if err != nil {
		return err
	}

	for i, column := range columns {
		dest[column] = *(values[i].(*any))
	}

	return r.Err()
}

// reflect helpers

func baseType(t reflect.Type, expected reflect.Kind) (reflect.Type, error) {
	t = reflectx.Deref(t)
	if t.Kind() != expected {
		return nil, fmt.Errorf("expected %s but got %s", expected, t.Kind())
	}
	return t, nil
}

var rawBytesType = reflect.TypeOf(sql.RawBytes{})

// containsRawBytes checks if type t is sql.RawBytes or contains it as a struct field.
func containsRawBytes(t reflect.Type) bool {
	t = reflectx.Deref(t)
	if t == rawBytesType {
		return true
	}
	if t.Kind() == reflect.Struct {
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if !f.IsExported() && !f.Anonymous {
				continue
			}
			if containsRawBytes(f.Type) {
				return true
			}
		}
	}
	return false
}

// fieldsByName fills a values interface with fields from the passed value based
// on the traversals in int.  If ptrs is true, return addresses instead of values.
// We write this instead of using FieldsByName to save allocations and map lookups
// when iterating over many rows.  Empty traversals will get an interface pointer.
// Because of the necessity of requesting ptrs or values, it's considered a bit too
// specialized for inclusion in reflectx itself.
func fieldsByTraversal(v reflect.Value, traversals [][]int, values []any, ptrs bool) error {
	v = reflect.Indirect(v)
	if v.Kind() != reflect.Struct {
		return errors.New("argument not a struct")
	}

	for i, traversal := range traversals {
		if len(traversal) == 0 {
			values[i] = new(any)
			continue
		}
		f := reflectx.FieldByIndexes(v, traversal)
		if ptrs {
			values[i] = f.Addr().Interface()
		} else {
			values[i] = f.Interface()
		}
	}
	return nil
}

// checkMissingFields checks whether there are missing field mappings in traversals.
// If there are missing fields, it returns a detailed error including the missing column names,
// column indices, and target type.
func checkMissingFields(traversals [][]int, columns []string, dest any) error {
	var missing []string
	for i, t := range traversals {
		if len(t) == 0 {
			missing = append(missing, fmt.Sprintf("%s (index %d)", columns[i], i))
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing destination name %s in %T, columns: %v", strings.Join(missing, ", "), dest, columns)
	}
	return nil
}
