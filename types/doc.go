// Package types provides some useful types which implement the `sql.Scanner`
// and `driver.Valuer` interfaces, suitable for use as scan and value targets with
// database/sql.
//
//   - GzippedText: transparently gzips on write and gunzips on scan.
//   - JSONText / NullJSONText: raw JSON bytes, validated on write.
//   - JSONValue[T]: a JSON column decoded directly into a typed Go value.
//   - BitBool: a bool stored as BIT(1).
package types
