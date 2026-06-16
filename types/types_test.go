package types

import (
	"encoding/json"
	"testing"
)

func TestGzipText(t *testing.T) {
	g := GzippedText("Hello, world")
	v, err := g.Value()
	if err != nil {
		t.Errorf("Was not expecting an error")
	}
	err = (&g).Scan(v)
	if err != nil {
		t.Errorf("Was not expecting an error")
	}
	if string(g) != "Hello, world" {
		t.Errorf("Was expecting the string we sent in (Hello World), got %s", string(g))
	}
}

func TestJSONText(t *testing.T) {
	j := JSONText(`{"foo": 1, "bar": 2}`)
	v, err := j.Value()
	if err != nil {
		t.Errorf("Was not expecting an error")
	}
	err = (&j).Scan(v)
	if err != nil {
		t.Errorf("Was not expecting an error")
	}
	m := map[string]any{}
	j.Unmarshal(&m)

	if m["foo"].(float64) != 1 || m["bar"].(float64) != 2 {
		t.Errorf("Expected valid json but got some garbage instead? %#v", m)
	}

	j = JSONText(`{"foo": 1, invalid, false}`)
	_, err = j.Value()
	if err == nil {
		t.Errorf("Was expecting invalid json to fail!")
	}

	j = JSONText("")
	v, err = j.Value()
	if err != nil {
		t.Errorf("Was not expecting an error")
	}

	err = (&j).Scan(v)
	if err != nil {
		t.Errorf("Was not expecting an error")
	}

	j = JSONText(nil)
	v, err = j.Value()
	if err != nil {
		t.Errorf("Was not expecting an error")
	}

	err = (&j).Scan(v)
	if err != nil {
		t.Errorf("Was not expecting an error")
	}
}

func TestNullJSONText(t *testing.T) {
	j := NullJSONText{}
	err := j.Scan(`{"foo": 1, "bar": 2}`)
	if err != nil {
		t.Errorf("Was not expecting an error")
	}
	v, err := j.Value()
	if err != nil {
		t.Errorf("Was not expecting an error")
	}
	err = (&j).Scan(v)
	if err != nil {
		t.Errorf("Was not expecting an error")
	}
	m := map[string]any{}
	j.Unmarshal(&m)

	if m["foo"].(float64) != 1 || m["bar"].(float64) != 2 {
		t.Errorf("Expected valid json but got some garbage instead? %#v", m)
	}

	j = NullJSONText{}
	err = j.Scan(nil)
	if err != nil {
		t.Errorf("Was not expecting an error")
	}
	if j.Valid != false {
		t.Errorf("Expected valid to be false, but got true")
	}
}

func TestNullJSONText_MarshalJSON_Valid(t *testing.T) {
	j := NullJSONText{}
	_ = j.Scan(`{"name":"test","value":42}`)

	data, err := json.Marshal(j)
	if err != nil {
		t.Fatalf("Was not expecting an error, got: %v", err)
	}

	// Should output the original JSON content, not a struct including the Valid field
	m := map[string]any{}
	if err = json.Unmarshal(data, &m); err != nil {
		t.Fatalf("Output should be valid json, got: %s", data)
	}
	if m["name"] != "test" || m["value"].(float64) != 42 {
		t.Errorf("Expected original json content, got: %s", data)
	}
}

func TestNullJSONText_MarshalJSON_Null(t *testing.T) {
	j := NullJSONText{}
	_ = j.Scan(nil) // Valid = false

	data, err := json.Marshal(j)
	if err != nil {
		t.Fatalf("Was not expecting an error, got: %v", err)
	}
	if string(data) != "null" {
		t.Errorf("Expected null, got: %s", data)
	}
}

func TestNullJSONText_UnmarshalJSON_Valid(t *testing.T) {
	var j NullJSONText
	err := json.Unmarshal([]byte(`{"key":"val"}`), &j)
	if err != nil {
		t.Fatalf("Was not expecting an error, got: %v", err)
	}
	if !j.Valid {
		t.Error("Expected Valid=true after unmarshalling valid json")
	}

	m := map[string]any{}
	j.Unmarshal(&m)
	if m["key"] != "val" {
		t.Errorf("Expected key=val, got: %v", m["key"])
	}
}

func TestNullJSONText_UnmarshalJSON_Null(t *testing.T) {
	// First set a valid value
	j := NullJSONText{JSONText: JSONText(`{"old":"data"}`), Valid: true}

	err := json.Unmarshal([]byte("null"), &j)
	if err != nil {
		t.Fatalf("Was not expecting an error, got: %v", err)
	}
	if j.Valid {
		t.Error("Expected Valid=false after unmarshalling null")
	}
}

func TestNullJSONText_JSON_RoundTrip(t *testing.T) {
	// Test embedding into an outer struct
	type Wrapper struct {
		Name string       `json:"name"`
		Data NullJSONText `json:"data"`
	}

	// Valid 场景的 round-trip
	w := Wrapper{Name: "test"}
	w.Data = NullJSONText{}
	_ = w.Data.Scan(`{"x":1}`)

	data, err := json.Marshal(w)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var w2 Wrapper
	if err = json.Unmarshal(data, &w2); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if !w2.Data.Valid {
		t.Error("Expected Data.Valid=true after round-trip")
	}
	m := map[string]any{}
	w2.Data.Unmarshal(&m)
	if m["x"].(float64) != 1 {
		t.Errorf("Expected x=1, got: %v", m["x"])
	}

	// Null 场景的 round-trip
	w = Wrapper{Name: "empty"}
	_ = w.Data.Scan(nil)

	data, err = json.Marshal(w)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	// Should contain "data":null
	expected := `{"name":"empty","data":null}`
	if string(data) != expected {
		t.Errorf("Expected %s, got: %s", expected, data)
	}

	var w3 Wrapper
	if err = json.Unmarshal(data, &w3); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if w3.Data.Valid {
		t.Error("Expected Data.Valid=false after null round-trip")
	}
}

func TestBitBool(t *testing.T) {
	// Test true value
	var b BitBool = true

	v, err := b.Value()
	if err != nil {
		t.Errorf("Cannot return error")
	}
	err = (&b).Scan(v)
	if err != nil {
		t.Errorf("Was not expecting an error")
	}
	if !b {
		t.Errorf("Was expecting the bool we sent in (true), got %v", b)
	}

	// Test false value
	b = false

	v, err = b.Value()
	if err != nil {
		t.Errorf("Cannot return error")
	}
	err = (&b).Scan(v)
	if err != nil {
		t.Errorf("Was not expecting an error")
	}
	if b {
		t.Errorf("Was expecting the bool we sent in (false), got %v", b)
	}
}
