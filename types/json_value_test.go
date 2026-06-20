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
