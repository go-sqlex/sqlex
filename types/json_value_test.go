package types

import (
	"encoding/json"
	"testing"
)

type testAddr struct {
	City   string `json:"city"`
	Street string `json:"street"`
}

func TestJSONValue_ScanNull(t *testing.T) {
	var jv JSONValue[testAddr]
	if err := jv.Scan(nil); err != nil {
		t.Fatal(err)
	}
	if jv.Valid {
		t.Error("expected Valid=false for NULL")
	}
}

func TestJSONValue_ScanBytes(t *testing.T) {
	var jv JSONValue[testAddr]
	if err := jv.Scan([]byte(`{"city":"Beijing","street":"ChangAn"}`)); err != nil {
		t.Fatal(err)
	}
	if !jv.Valid {
		t.Error("expected Valid=true")
	}
	if jv.Val.City != "Beijing" || jv.Val.Street != "ChangAn" {
		t.Errorf("unexpected value: %+v", jv.Val)
	}
}

func TestJSONValue_ScanString(t *testing.T) {
	var jv JSONValue[map[string]int]
	if err := jv.Scan(`{"a":1,"b":2}`); err != nil {
		t.Fatal(err)
	}
	if !jv.Valid || jv.Val["a"] != 1 || jv.Val["b"] != 2 {
		t.Errorf("unexpected value: %+v", jv.Val)
	}
}

func TestJSONValue_Value(t *testing.T) {
	jv := NewJSONValue(testAddr{City: "Shanghai", Street: "NanJing"})
	v, err := jv.Value()
	if err != nil {
		t.Fatal(err)
	}
	data, ok := v.([]byte)
	if !ok {
		t.Fatal("expected []byte")
	}
	var addr testAddr
	if err := json.Unmarshal(data, &addr); err != nil {
		t.Fatal(err)
	}
	if addr.City != "Shanghai" {
		t.Errorf("unexpected city: %s", addr.City)
	}
}

func TestJSONValue_ValueNull(t *testing.T) {
	var jv JSONValue[testAddr]
	v, err := jv.Value()
	if err != nil {
		t.Fatal(err)
	}
	if v != nil {
		t.Error("expected nil for invalid JSONValue")
	}
}

func TestJSONValue_MarshalJSON(t *testing.T) {
	jv := NewJSONValue([]int{1, 2, 3})
	data, err := json.Marshal(jv)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "[1,2,3]" {
		t.Errorf("unexpected json: %s", data)
	}
}

func TestJSONValue_MarshalJSON_Null(t *testing.T) {
	var jv JSONValue[testAddr]
	data, err := json.Marshal(jv)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "null" {
		t.Errorf("expected null, got: %s", data)
	}
}

func TestJSONValue_UnmarshalJSON(t *testing.T) {
	var jv JSONValue[testAddr]
	if err := json.Unmarshal([]byte(`{"city":"Hangzhou","street":"West Lake"}`), &jv); err != nil {
		t.Fatal(err)
	}
	if !jv.Valid || jv.Val.City != "Hangzhou" {
		t.Errorf("unexpected value: %+v", jv)
	}
}

func TestJSONValue_UnmarshalJSON_Null(t *testing.T) {
	jv := NewJSONValue(testAddr{City: "test"})
	if err := json.Unmarshal([]byte("null"), &jv); err != nil {
		t.Fatal(err)
	}
	if jv.Valid {
		t.Error("expected Valid=false after unmarshal null")
	}
}

// TestJSONValue_ScanNonText covers the default branch: drivers may return
// non-text values (e.g. int64 for a JSON number column) (#10 F-0118).
func TestJSONValue_ScanNonText(t *testing.T) {
	var jv JSONValue[int]
	if err := jv.Scan(int64(5)); err != nil {
		t.Fatal(err)
	}
	if !jv.Valid || jv.Val != 5 {
		t.Errorf("unexpected value: %+v", jv)
	}
}

// TestJSONValue_ScanReuseNoMerge guards against fields/keys from a previous
// row leaking into the next when the same JSONValue is reused across rows.
func TestJSONValue_ScanReuseNoMerge(t *testing.T) {
	var m JSONValue[map[string]int]
	if err := m.Scan([]byte(`{"a":1}`)); err != nil {
		t.Fatal(err)
	}
	if err := m.Scan([]byte(`{"b":2}`)); err != nil {
		t.Fatal(err)
	}
	if len(m.Val) != 1 || m.Val["b"] != 2 {
		t.Errorf("map merged across scans: %v", m.Val)
	}

	var s JSONValue[testAddr]
	if err := s.Scan([]byte(`{"city":"A","street":"1"}`)); err != nil {
		t.Fatal(err)
	}
	if err := s.Scan([]byte(`{"city":"B"}`)); err != nil {
		t.Fatal(err)
	}
	if s.Val != (testAddr{City: "B"}) {
		t.Errorf("struct merged across scans: %+v", s.Val)
	}
}

// TestJSONValue_ScanInvalid ensures a decode error does not leave Valid=true.
func TestJSONValue_ScanInvalid(t *testing.T) {
	jv := NewJSONValue(testAddr{City: "stale"})
	if err := jv.Scan([]byte(`not json`)); err == nil {
		t.Fatal("expected error for invalid json")
	}
	if jv.Valid || jv.Val != (testAddr{}) {
		t.Errorf("expected zero invalid value after failed scan, got %+v", jv)
	}

	var u JSONValue[testAddr]
	if err := u.UnmarshalJSON([]byte(`{"city":1}`)); err == nil {
		t.Fatal("expected error for mistyped json")
	}
	if u.Valid {
		t.Error("expected Valid=false after failed UnmarshalJSON")
	}
}

func TestJSONValue_NestedStruct(t *testing.T) {
	type Inner struct {
		Items []testAddr `json:"items"`
	}
	data := `{"items":[{"city":"A","street":"1"},{"city":"B","street":"2"}]}`
	var jv JSONValue[Inner]
	if err := jv.Scan([]byte(data)); err != nil {
		t.Fatal(err)
	}
	if !jv.Valid || len(jv.Val.Items) != 2 || jv.Val.Items[0].City != "A" {
		t.Errorf("unexpected: %+v", jv.Val)
	}
}
