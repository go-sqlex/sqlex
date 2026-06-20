package sqlex

import (
	"database/sql"
	"errors"
	"reflect"

	"github.com/go-sqlex/sqlex/reflectx"
)

// Rows is a wrapper around sql.Rows which caches costly reflect operations
// during a looped StructScan
type Rows struct {
	*sql.Rows
	Mapper *reflectx.Mapper
	strict bool
	// these fields cache memory use for a rows during iteration w/ structScan
	started bool
	fields  [][]int
	values  []any
}

// GetMapper returns the Mapper for this Rows.
func (r *Rows) GetMapper() *reflectx.Mapper {
	return r.Mapper
}

// SliceScan using this Rows.
func (r *Rows) SliceScan() ([]any, error) {
	return SliceScan(r)
}

// MapScan using this Rows.
func (r *Rows) MapScan(dest map[string]any) error {
	return MapScan(r, dest)
}

// StructScan is like sql.Rows.Scan, but scans a single Row into a single Struct.
// Use this and iterate over Rows manually when the memory load of Select() might be
// prohibitive.  *Rows.StructScan caches the reflect work of matching up column
// positions to fields to avoid that overhead per scan, which means it is not safe
// to run StructScan on the same Rows instance with different struct types.
//
// Note: Rows, like database/sql.Rows, is not safe for concurrent use.
// Do not call StructScan on the same Rows instance from multiple goroutines.
func (r *Rows) StructScan(dest any) error {
	v := reflect.ValueOf(dest)

	if v.Kind() != reflect.Ptr {
		return errors.New("must pass a pointer, not a value, to StructScan destination")
	}

	v = v.Elem()

	if !r.started {
		columns, err := r.Columns()
		if err != nil {
			return err
		}
		m := r.Mapper

		r.fields = m.TraversalsByName(v.Type(), columns)
		// In strict mode, check for missing fields
		if r.strict {
			if err := checkMissingFields(r.fields, columns, dest); err != nil {
				return err
			}
		}
		r.values = make([]any, len(columns))
		r.started = true
	}

	err := fieldsByTraversal(v, r.fields, r.values, true)
	if err != nil {
		return err
	}
	// scan into the struct field pointers and append to our results
	err = r.Scan(r.values...)
	if err != nil {
		return err
	}
	return r.Err()
}
