package types

import (
	"database/sql/driver"
	"encoding/json"
)

// JSONValue is a generic JSON column type that maps database JSON fields directly to Go strongly-typed structs.
// T can be any JSON-serializable type (struct, slice, map, pointer, etc.). Types encoding/json cannot
// handle (chan, func, complex) compile fine but fail at runtime in Value/MarshalJSON.
//
// Valid decides between a JSON document and SQL NULL. Prefer the constructors
// over a struct literal: JSONValue[T]{Val: v} compiles but, with Valid left
// false, silently writes SQL NULL. The zero value is SQL NULL.
type JSONValue[T any] struct {
	Val   T
	Valid bool
}

// NewJSONValue creates a valid JSONValue, which always writes val.
// Use NewJSONValueOrNull when NULL depends on a condition; the zero value
// JSONValue[T]{} writes SQL NULL.
func NewJSONValue[T any](val T) JSONValue[T] {
	return JSONValue[T]{Val: val, Valid: true}
}

// NewJSONValueOrNull creates a valid JSONValue when ok is true, and a value
// that writes SQL NULL when ok is false — the one-line ternary form:
//
//	row.Config = types.NewJSONValueOrNull(cfg, req.EnableConfig)
//
// Note that val is ignored when ok is false, matching the meaning of "or null".
func NewJSONValueOrNull[T any](val T, ok bool) JSONValue[T] {
	if !ok {
		return JSONValue[T]{}
	}
	return JSONValue[T]{Val: val, Valid: true}
}

// Scan implements the sql.Scanner interface.
//
// Val is reset before decoding, so reusing the same JSONValue across rows
// never merges fields or map keys from a previous row. Valid is true only
// when decoding succeeds; on error Val is left at its zero value.
func (j *JSONValue[T]) Scan(src any) error {
	var zero T
	j.Val, j.Valid = zero, false
	if src == nil {
		return nil
	}

	var data []byte
	switch v := src.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		// Driver returned a non-text value (e.g. int64, bool): round-trip it through JSON.
		var err error
		if data, err = json.Marshal(src); err != nil {
			return err
		}
	}

	if err := json.Unmarshal(data, &j.Val); err != nil {
		j.Val = zero
		return err
	}
	j.Valid = true
	return nil
}

// Value implements the driver.Valuer interface.
func (j JSONValue[T]) Value() (driver.Value, error) {
	if !j.Valid {
		return nil, nil
	}
	data, err := json.Marshal(j.Val)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// MarshalJSON implements the json.Marshaler interface.
func (j JSONValue[T]) MarshalJSON() ([]byte, error) {
	if !j.Valid {
		return []byte("null"), nil
	}
	return json.Marshal(j.Val)
}

// UnmarshalJSON implements the json.Unmarshaler interface.
// Like encoding/json, it decodes into the existing Val (fields merge);
// Valid is true only when decoding succeeds.
func (j *JSONValue[T]) UnmarshalJSON(data []byte) error {
	if string(data) == "null" || len(data) == 0 {
		j.Valid = false
		var zero T
		j.Val = zero
		return nil
	}
	err := json.Unmarshal(data, &j.Val)
	j.Valid = err == nil
	return err
}
