package types

import (
	"database/sql/driver"
	"encoding/json"
)

// JSONValue is a generic JSON column type that maps database JSON fields directly to Go strongly-typed structs.
// T can be any JSON-serializable type (struct, slice, map, pointer, etc.).
type JSONValue[T any] struct {
	Val   T
	Valid bool
}

// NewJSONValue creates a valid JSONValue.
func NewJSONValue[T any](val T) JSONValue[T] {
	return JSONValue[T]{Val: val, Valid: true}
}

// Scan implements the sql.Scanner interface.
func (j *JSONValue[T]) Scan(src any) error {
	if src == nil {
		j.Valid = false
		var zero T
		j.Val = zero
		return nil
	}

	var data []byte
	switch v := src.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		// Try JSON encoding then decoding
		var err error
		data, err = json.Marshal(src)
		if err != nil {
			return err
		}
	}

	j.Valid = true
	return json.Unmarshal(data, &j.Val)
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
func (j *JSONValue[T]) UnmarshalJSON(data []byte) error {
	if string(data) == "null" || len(data) == 0 {
		j.Valid = false
		var zero T
		j.Val = zero
		return nil
	}
	j.Valid = true
	return json.Unmarshal(data, &j.Val)
}
