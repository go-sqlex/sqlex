package sqlex

import "testing"

// TestWithStrictMode verifies the Opt advertised in doc.go actually exists and
// enables strict mode (#19).
func TestWithStrictMode(t *testing.T) {
	db := NewDB(nil, "mysql", WithStrictMode())
	if !db.IsStrict() {
		t.Error("WithStrictMode() should enable strict mode")
	}

	// The default must stay lenient.
	if NewDB(nil, "mysql").IsStrict() {
		t.Error("NewDB without options should be lenient")
	}
}
