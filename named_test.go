package sqlex

import (
	"database/sql"
	"fmt"
	"strings"
	"testing"
)

func TestCompileQuery(t *testing.T) {
	table := []struct {
		Q, R, D, T, N string
		V             []string
	}{
		// basic test for named parameters, invalid char ',' terminating
		{
			Q: `INSERT INTO foo (a,b,c,d) VALUES (:name, :age, :first, :last)`,
			R: `INSERT INTO foo (a,b,c,d) VALUES (?, ?, ?, ?)`,
			D: `INSERT INTO foo (a,b,c,d) VALUES ($1, $2, $3, $4)`,
			T: `INSERT INTO foo (a,b,c,d) VALUES (@p1, @p2, @p3, @p4)`,
			N: `INSERT INTO foo (a,b,c,d) VALUES (:name, :age, :first, :last)`,
			V: []string{"name", "age", "first", "last"},
		},
		// This query tests a named parameter ending the string as well as numbers
		{
			Q: `SELECT * FROM a WHERE first_name=:name1 AND last_name=:name2`,
			R: `SELECT * FROM a WHERE first_name=? AND last_name=?`,
			D: `SELECT * FROM a WHERE first_name=$1 AND last_name=$2`,
			T: `SELECT * FROM a WHERE first_name=@p1 AND last_name=@p2`,
			N: `SELECT * FROM a WHERE first_name=:name1 AND last_name=:name2`,
			V: []string{"name1", "name2"},
		},
		{
			Q: `SELECT "::foo" FROM a WHERE first_name=:name1 AND last_name=:name2`,
			R: `SELECT "::foo" FROM a WHERE first_name=? AND last_name=?`,
			D: `SELECT "::foo" FROM a WHERE first_name=$1 AND last_name=$2`,
			T: `SELECT "::foo" FROM a WHERE first_name=@p1 AND last_name=@p2`,
			N: `SELECT "::foo" FROM a WHERE first_name=:name1 AND last_name=:name2`,
			V: []string{"name1", "name2"},
		},
		{
			Q: `SELECT 'a::b::c' || first_name, '::::ABC::_::' FROM person WHERE first_name=:first_name AND last_name=:last_name`,
			R: `SELECT 'a::b::c' || first_name, '::::ABC::_::' FROM person WHERE first_name=? AND last_name=?`,
			D: `SELECT 'a::b::c' || first_name, '::::ABC::_::' FROM person WHERE first_name=$1 AND last_name=$2`,
			T: `SELECT 'a::b::c' || first_name, '::::ABC::_::' FROM person WHERE first_name=@p1 AND last_name=@p2`,
			N: `SELECT 'a::b::c' || first_name, '::::ABC::_::' FROM person WHERE first_name=:first_name AND last_name=:last_name`,
			V: []string{"first_name", "last_name"},
		},
		{
			Q: `SELECT @name := "name", :age, :first, :last`,
			R: `SELECT @name := "name", ?, ?, ?`,
			D: `SELECT @name := "name", $1, $2, $3`,
			N: `SELECT @name := "name", :age, :first, :last`,
			T: `SELECT @name := "name", @p1, @p2, @p3`,
			V: []string{"age", "first", "last"},
		},
		// Behavior change: sqlex does not support Unicode named parameter names (:あ/:キコ/:名前);
		// their first character is non-ASCII bindStart, so they are preserved as-is; only ASCII :b is recognized.
		// Unicode in other SQL positions (table names/column names/string values) is unaffected.
		{
			Q: `INSERT INTO foo (a,b,c,d) VALUES (:あ, :b, :キコ, :名前)`,
			R: `INSERT INTO foo (a,b,c,d) VALUES (:あ, ?, :キコ, :名前)`,
			D: `INSERT INTO foo (a,b,c,d) VALUES (:あ, $1, :キコ, :名前)`,
			T: `INSERT INTO foo (a,b,c,d) VALUES (:あ, @p1, :キコ, :名前)`,
			N: `INSERT INTO foo (a,b,c,d) VALUES (:あ, :b, :キコ, :名前)`,
			V: []string{"b"},
		},
		// Issue #872: Colons inside IPv6 address string literals should not be parsed as named parameters
		{
			Q: `INSERT INTO users (name, ipv6) VALUES (:name, '2001:0db8:85a3:0000:0000:8a2e:0370:7334')`,
			R: `INSERT INTO users (name, ipv6) VALUES (?, '2001:0db8:85a3:0000:0000:8a2e:0370:7334')`,
			D: `INSERT INTO users (name, ipv6) VALUES ($1, '2001:0db8:85a3:0000:0000:8a2e:0370:7334')`,
			T: `INSERT INTO users (name, ipv6) VALUES (@p1, '2001:0db8:85a3:0000:0000:8a2e:0370:7334')`,
			N: `INSERT INTO users (name, ipv6) VALUES (:name, '2001:0db8:85a3:0000:0000:8a2e:0370:7334')`,
			V: []string{"name"},
		},
		// Time format string literal
		{
			Q: `SELECT * FROM events WHERE created_at > :start_time AND format = 'HH:mm:ss'`,
			R: `SELECT * FROM events WHERE created_at > ? AND format = 'HH:mm:ss'`,
			D: `SELECT * FROM events WHERE created_at > $1 AND format = 'HH:mm:ss'`,
			T: `SELECT * FROM events WHERE created_at > @p1 AND format = 'HH:mm:ss'`,
			N: `SELECT * FROM events WHERE created_at > :start_time AND format = 'HH:mm:ss'`,
			V: []string{"start_time"},
		},
		// Time interval query: interval '01:30:00' and AT TIME ZONE 'utc'
		{
			Q: `SELECT * FROM testtable WHERE timeposted BETWEEN (now() AT TIME ZONE 'utc') AND (now() AT TIME ZONE 'utc') - interval '01:30:00' AND id = :id`,
			R: `SELECT * FROM testtable WHERE timeposted BETWEEN (now() AT TIME ZONE 'utc') AND (now() AT TIME ZONE 'utc') - interval '01:30:00' AND id = ?`,
			D: `SELECT * FROM testtable WHERE timeposted BETWEEN (now() AT TIME ZONE 'utc') AND (now() AT TIME ZONE 'utc') - interval '01:30:00' AND id = $1`,
			T: `SELECT * FROM testtable WHERE timeposted BETWEEN (now() AT TIME ZONE 'utc') AND (now() AT TIME ZONE 'utc') - interval '01:30:00' AND id = @p1`,
			N: `SELECT * FROM testtable WHERE timeposted BETWEEN (now() AT TIME ZONE 'utc') AND (now() AT TIME ZONE 'utc') - interval '01:30:00' AND id = :id`,
			V: []string{"id"},
		},
		// IPv6 shorthand ::1
		{
			Q: `SELECT * FROM hosts WHERE ip = '::1' AND id = :id`,
			R: `SELECT * FROM hosts WHERE ip = '::1' AND id = ?`,
			D: `SELECT * FROM hosts WHERE ip = '::1' AND id = $1`,
			T: `SELECT * FROM hosts WHERE ip = '::1' AND id = @p1`,
			N: `SELECT * FROM hosts WHERE ip = '::1' AND id = :id`,
			V: []string{"id"},
		},
		// IPv6 fe80::1
		{
			Q: `SELECT * FROM hosts WHERE ip = 'fe80::1' AND id = :id`,
			R: `SELECT * FROM hosts WHERE ip = 'fe80::1' AND id = ?`,
			D: `SELECT * FROM hosts WHERE ip = 'fe80::1' AND id = $1`,
			T: `SELECT * FROM hosts WHERE ip = 'fe80::1' AND id = @p1`,
			N: `SELECT * FROM hosts WHERE ip = 'fe80::1' AND id = :id`,
			V: []string{"id"},
		},
		// SQL standard escaped quote '': :test inside 'it''s a :test' is within a string and should not be parsed
		{
			Q: `SELECT * FROM t WHERE name = 'it''s a :test' AND id = :id`,
			R: `SELECT * FROM t WHERE name = 'it''s a :test' AND id = ?`,
			D: `SELECT * FROM t WHERE name = 'it''s a :test' AND id = $1`,
			T: `SELECT * FROM t WHERE name = 'it''s a :test' AND id = @p1`,
			N: `SELECT * FROM t WHERE name = 'it''s a :test' AND id = :id`,
			V: []string{"id"},
		},
		// -- Colons inside line comments should not be parsed
		{
			Q: "SELECT * FROM t -- comment :fake\nWHERE id = :id",
			R: "SELECT * FROM t -- comment :fake\nWHERE id = ?",
			D: "SELECT * FROM t -- comment :fake\nWHERE id = $1",
			T: "SELECT * FROM t -- comment :fake\nWHERE id = @p1",
			N: "SELECT * FROM t -- comment :fake\nWHERE id = :id",
			V: []string{"id"},
		},
		// /* */ Colons inside block comments should not be parsed
		{
			Q: `SELECT * FROM t /* :fake comment */ WHERE id = :id`,
			R: `SELECT * FROM t /* :fake comment */ WHERE id = ?`,
			D: `SELECT * FROM t /* :fake comment */ WHERE id = $1`,
			T: `SELECT * FROM t /* :fake comment */ WHERE id = @p1`,
			N: `SELECT * FROM t /* :fake comment */ WHERE id = :id`,
			V: []string{"id"},
		},
		// Mixed scenario: string colon + comment colon + normal named parameter
		{
			Q: "SELECT * FROM t /* skip :x */ WHERE ip = '::1' AND name = 'it''s :y' -- :z\nAND id = :id AND age > :age",
			R: "SELECT * FROM t /* skip :x */ WHERE ip = '::1' AND name = 'it''s :y' -- :z\nAND id = ? AND age > ?",
			D: "SELECT * FROM t /* skip :x */ WHERE ip = '::1' AND name = 'it''s :y' -- :z\nAND id = $1 AND age > $2",
			T: "SELECT * FROM t /* skip :x */ WHERE ip = '::1' AND name = 'it''s :y' -- :z\nAND id = @p1 AND age > @p2",
			N: "SELECT * FROM t /* skip :x */ WHERE ip = '::1' AND name = 'it''s :y' -- :z\nAND id = :id AND age > :age",
			V: []string{"id", "age"},
		},
		// Colons inside double-quoted identifiers should not be parsed as named parameters (PostgreSQL identifier quoting)
		{
			Q: `SELECT "col:name" FROM t WHERE id = :id`,
			R: `SELECT "col:name" FROM t WHERE id = ?`,
			D: `SELECT "col:name" FROM t WHERE id = $1`,
			T: `SELECT "col:name" FROM t WHERE id = @p1`,
			N: `SELECT "col:name" FROM t WHERE id = :id`,
			V: []string{"id"},
		},
		// Double-quoted identifier with escaped double quote ""
		{
			Q: `SELECT "col""with:colon" FROM t WHERE id = :id`,
			R: `SELECT "col""with:colon" FROM t WHERE id = ?`,
			D: `SELECT "col""with:colon" FROM t WHERE id = $1`,
			T: `SELECT "col""with:colon" FROM t WHERE id = @p1`,
			N: `SELECT "col""with:colon" FROM t WHERE id = :id`,
			V: []string{"id"},
		},
		// Double-quoted identifier and single-quoted string literal mixed
		{
			Q: `SELECT "col:a" FROM t WHERE name = 'val:b' AND id = :id`,
			R: `SELECT "col:a" FROM t WHERE name = 'val:b' AND id = ?`,
			D: `SELECT "col:a" FROM t WHERE name = 'val:b' AND id = $1`,
			T: `SELECT "col:a" FROM t WHERE name = 'val:b' AND id = @p1`,
			N: `SELECT "col:a" FROM t WHERE name = 'val:b' AND id = :id`,
			V: []string{"id"},
		},
		// When PostgreSQL type cast immediately follows a named parameter, :: should not be included in the parameter name
		{
			Q: `SELECT * FROM t WHERE id = :id::int`,
			R: `SELECT * FROM t WHERE id = ?::int`,
			D: `SELECT * FROM t WHERE id = $1::int`,
			T: `SELECT * FROM t WHERE id = @p1::int`,
			N: `SELECT * FROM t WHERE id = :id::int`,
			V: []string{"id"},
		},
		{
			Q: `SELECT :val::text AS val`,
			R: `SELECT ?::text AS val`,
			D: `SELECT $1::text AS val`,
			T: `SELECT @p1::text AS val`,
			N: `SELECT :val::text AS val`,
			V: []string{"val"},
		},
		{
			Q: `SELECT * FROM t WHERE id = :id::int AND name = :name`,
			R: `SELECT * FROM t WHERE id = ?::int AND name = ?`,
			D: `SELECT * FROM t WHERE id = $1::int AND name = $2`,
			T: `SELECT * FROM t WHERE id = @p1::int AND name = @p2`,
			N: `SELECT * FROM t WHERE id = :id::int AND name = :name`,
			V: []string{"id", "name"},
		},
		// When a named parameter is immediately followed by a comment, the parameter name should end first, then colons in the comment should be skipped
		{
			Q: "SELECT * FROM t WHERE id = :id -- comment :fake\nAND name = :name",
			R: "SELECT * FROM t WHERE id = ? -- comment :fake\nAND name = ?",
			D: "SELECT * FROM t WHERE id = $1 -- comment :fake\nAND name = $2",
			T: "SELECT * FROM t WHERE id = @p1 -- comment :fake\nAND name = @p2",
			N: "SELECT * FROM t WHERE id = :id -- comment :fake\nAND name = :name",
			V: []string{"id", "name"},
		},
		{
			Q: `SELECT * FROM t WHERE id = :id/* comment :fake */ AND name = :name`,
			R: `SELECT * FROM t WHERE id = ?/* comment :fake */ AND name = ?`,
			D: `SELECT * FROM t WHERE id = $1/* comment :fake */ AND name = $2`,
			T: `SELECT * FROM t WHERE id = @p1/* comment :fake */ AND name = @p2`,
			N: `SELECT * FROM t WHERE id = :id/* comment :fake */ AND name = :name`,
			V: []string{"id", "name"},
		},
	}

	for _, test := range table {
		qr, names, err := compileNamedQuery([]byte(test.Q), QUESTION)
		if err != nil {
			t.Error(err)
		}
		if qr != test.R {
			t.Errorf("expected %s, got %s", test.R, qr)
		}
		if len(names) != len(test.V) {
			t.Errorf("expected %#v, got %#v", test.V, names)
		} else {
			for i, name := range names {
				if name != test.V[i] {
					t.Errorf("expected %dth name to be %s, got %s", i+1, test.V[i], name)
				}
			}
		}
		qd, _, _ := compileNamedQuery([]byte(test.Q), DOLLAR)
		if qd != test.D {
			t.Errorf("\nexpected: `%s`\ngot:      `%s`", test.D, qd)
		}

		qt, _, _ := compileNamedQuery([]byte(test.Q), AT)
		if qt != test.T {
			t.Errorf("\nexpected: `%s`\ngot:      `%s`", test.T, qt)
		}

		qq, _, _ := compileNamedQuery([]byte(test.Q), NAMED)
		if qq != test.N {
			t.Errorf("\nexpected: `%s`\ngot:      `%s`\n(len: %d vs %d)", test.N, qq, len(test.N), len(qq))
		}
	}
}

type Test struct {
	t *testing.T
}

func (t Test) Error(err error, msg ...any) {
	t.t.Helper()
	if err != nil {
		if len(msg) == 0 {
			t.t.Error(err)
		} else {
			t.t.Error(msg...)
		}
	}
}

func (t Test) Errorf(err error, format string, args ...any) {
	t.t.Helper()
	if err != nil {
		t.t.Errorf(format, args...)
	}
}

func TestEscapedColons(t *testing.T) {
	// Test correct handling of colons inside string literals
	var qs = `SELECT * FROM testtable WHERE timeposted BETWEEN (now() AT TIME ZONE 'utc') AND
	(now() AT TIME ZONE 'utc') - interval '01:30:00') AND id = :id`
	query, names, err := compileNamedQuery([]byte(qs), DOLLAR)
	if err != nil {
		t.Error("Didn't handle colons correctly when inside a string")
	}
	if len(names) != 1 || names[0] != "id" {
		t.Errorf("expected names=[id], got %v", names)
	}
	// Ensure colons inside string literals are preserved as-is
	if !containsString(query, "'01:30:00'") {
		t.Errorf("string literal '01:30:00' was corrupted in query: %s", query)
	}
	if !containsString(query, "'utc'") {
		t.Errorf("string literal 'utc' was corrupted in query: %s", query)
	}

	// Test SQL standard escaped quotes
	qs2 := `SELECT * FROM t WHERE name = 'it''s a :test' AND id = :id`
	_, names2, err2 := compileNamedQuery([]byte(qs2), DOLLAR)
	if err2 != nil {
		t.Errorf("Didn't handle escaped quotes correctly: %v", err2)
	}
	if len(names2) != 1 || names2[0] != "id" {
		t.Errorf("expected names=[id], got %v (escaped quote test)", names2)
	}

	// Test colons inside line comments
	qs3 := "SELECT * FROM t -- comment with :fake_param\nWHERE id = :real_id"
	_, names3, err3 := compileNamedQuery([]byte(qs3), DOLLAR)
	if err3 != nil {
		t.Errorf("Didn't handle line comment correctly: %v", err3)
	}
	if len(names3) != 1 || names3[0] != "real_id" {
		t.Errorf("expected names=[real_id], got %v (line comment test)", names3)
	}

	// Test colons inside block comments
	qs4 := `SELECT * FROM t /* :fake_param in block comment */ WHERE id = :real_id`
	_, names4, err4 := compileNamedQuery([]byte(qs4), DOLLAR)
	if err4 != nil {
		t.Errorf("Didn't handle block comment correctly: %v", err4)
	}
	if len(names4) != 1 || names4[0] != "real_id" {
		t.Errorf("expected names=[real_id], got %v (block comment test)", names4)
	}

	// Test consecutive multiple colons :::
	qs5 := `SELECT ':::'::text, id FROM t WHERE id = :id`
	_, names5, err5 := compileNamedQuery([]byte(qs5), DOLLAR)
	if err5 != nil {
		t.Errorf("Didn't handle triple colons correctly: %v", err5)
	}
	if len(names5) != 1 || names5[0] != "id" {
		t.Errorf("expected names=[id], got %v (triple colons test)", names5)
	}
}

// containsString is a helper function that checks whether s contains substr
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && strings.Contains(s, substr)
}

func TestNamedQueries(t *testing.T) {
	RunWithSchema(defaultSchema, t, func(db *DB, t *testing.T, now string) {
		loadDefaultFixture(db, t)
		test := Test{t}
		var ns *NamedStmt
		var err error

		// Check that invalid preparations fail
		_, err = db.PrepareNamed("SELECT * FROM person WHERE first_name=:first:name")
		if err == nil {
			t.Error("Expected an error with invalid prepared statement.")
		}

		_, err = db.PrepareNamed("invalid sql")
		if err == nil {
			t.Error("Expected an error with invalid prepared statement.")
		}

		// Check closing works as anticipated
		ns, err = db.PrepareNamed("SELECT * FROM person WHERE first_name=:first_name")
		test.Error(err)
		err = ns.Close()
		test.Error(err)

		ns, err = db.PrepareNamed(`
			SELECT first_name, last_name, email 
			FROM person WHERE first_name=:first_name AND email=:email`)
		test.Error(err)

		// test Queryx w/ uses Query
		p := Person{FirstName: "Jason", LastName: "Moiron", Email: "jmoiron@jmoiron.net"}

		rows, err := ns.Queryx(p)
		test.Error(err)
		for rows.Next() {
			var p2 Person
			rows.StructScan(&p2)
			if p.FirstName != p2.FirstName {
				t.Errorf("got %s, expected %s", p.FirstName, p2.FirstName)
			}
			if p.LastName != p2.LastName {
				t.Errorf("got %s, expected %s", p.LastName, p2.LastName)
			}
			if p.Email != p2.Email {
				t.Errorf("got %s, expected %s", p.Email, p2.Email)
			}
		}

		// test Select
		people := make([]Person, 0, 5)
		err = ns.Select(&people, p)
		test.Error(err)

		if len(people) != 1 {
			t.Errorf("got %d results, expected %d", len(people), 1)
		}
		if p.FirstName != people[0].FirstName {
			t.Errorf("got %s, expected %s", p.FirstName, people[0].FirstName)
		}
		if p.LastName != people[0].LastName {
			t.Errorf("got %s, expected %s", p.LastName, people[0].LastName)
		}
		if p.Email != people[0].Email {
			t.Errorf("got %s, expected %s", p.Email, people[0].Email)
		}

		// test struct batch inserts
		sls := []Person{
			{FirstName: "Ardie", LastName: "Savea", Email: "asavea@ab.co.nz"},
			{FirstName: "Sonny Bill", LastName: "Williams", Email: "sbw@ab.co.nz"},
			{FirstName: "Ngani", LastName: "Laumape", Email: "nlaumape@ab.co.nz"},
		}

		insert := fmt.Sprintf(
			"INSERT INTO person (first_name, last_name, email, added_at) VALUES (:first_name, :last_name, :email, %v)\n",
			now,
		)
		_, err = db.NamedExec(insert, sls)
		test.Error(err)

		// test map batch inserts
		slsMap := []map[string]any{
			{"first_name": "Ardie", "last_name": "Savea", "email": "asavea@ab.co.nz"},
			{"first_name": "Sonny Bill", "last_name": "Williams", "email": "sbw@ab.co.nz"},
			{"first_name": "Ngani", "last_name": "Laumape", "email": "nlaumape@ab.co.nz"},
		}

		_, err = db.NamedExec(`INSERT INTO person (first_name, last_name, email)
			VALUES (:first_name, :last_name, :email) ;--`, slsMap)
		test.Error(err)

		type A map[string]any

		typedMap := []A{
			{"first_name": "Ardie", "last_name": "Savea", "email": "asavea@ab.co.nz"},
			{"first_name": "Sonny Bill", "last_name": "Williams", "email": "sbw@ab.co.nz"},
			{"first_name": "Ngani", "last_name": "Laumape", "email": "nlaumape@ab.co.nz"},
		}

		_, err = db.NamedExec(`INSERT INTO person (first_name, last_name, email)
			VALUES (:first_name, :last_name, :email) ;--`, typedMap)
		test.Error(err)

		for _, p := range sls {
			dest := Person{}
			err = db.Get(&dest, "SELECT * FROM person WHERE email=?", p.Email)
			test.Error(err)
			if dest.Email != p.Email {
				t.Errorf("expected %s, got %s", p.Email, dest.Email)
			}
		}

		// test Exec
		ns, err = db.PrepareNamed(`
			INSERT INTO person (first_name, last_name, email)
			VALUES (:first_name, :last_name, :email)`)
		test.Error(err)

		js := Person{
			FirstName: "Julien",
			LastName:  "Savea",
			Email:     "jsavea@ab.co.nz",
		}
		_, err = ns.Exec(js)
		test.Error(err)

		// Make sure we can pull him out again
		p2 := Person{}
		db.Get(&p2, "SELECT * FROM person WHERE email=?", js.Email)
		if p2.Email != js.Email {
			t.Errorf("expected %s, got %s", js.Email, p2.Email)
		}

		// test Txn NamedStmts
		tx := db.MustBegin()
		txns := tx.NamedStmt(ns)

		// We're going to add Steven in this txn
		sl := Person{
			FirstName: "Steven",
			LastName:  "Luatua",
			Email:     "sluatua@ab.co.nz",
		}

		_, err = txns.Exec(sl)
		test.Error(err)
		// then rollback...
		tx.Rollback()
		// looking for Steven after a rollback should fail
		err = db.Get(&p2, "SELECT * FROM person WHERE email=?", sl.Email)
		if err != sql.ErrNoRows {
			t.Errorf("expected no rows error, got %v", err)
		}

		// now do the same, but commit
		tx = db.MustBegin()
		txns = tx.NamedStmt(ns)
		_, err = txns.Exec(sl)
		test.Error(err)
		tx.Commit()

		// looking for Steven after a Commit should succeed
		err = db.Get(&p2, "SELECT * FROM person WHERE email=?", sl.Email)
		test.Error(err)
		if p2.Email != sl.Email {
			t.Errorf("expected %s, got %s", sl.Email, p2.Email)
		}

	})
}

func TestFixBounds(t *testing.T) {
	table := []struct {
		name, query, expect string
		loop                int
	}{
		{
			name:   `named syntax`,
			query:  `INSERT INTO foo (a,b,c,d) VALUES (:name, :age, :first, :last)`,
			expect: `INSERT INTO foo (a,b,c,d) VALUES (:name, :age, :first, :last),(:name, :age, :first, :last)`,
			loop:   2,
		},
		{
			name:   `mysql syntax`,
			query:  `INSERT INTO foo (a,b,c,d) VALUES (?, ?, ?, ?)`,
			expect: `INSERT INTO foo (a,b,c,d) VALUES (?, ?, ?, ?),(?, ?, ?, ?)`,
			loop:   2,
		},
		{
			name:   `named syntax w/ trailer`,
			query:  `INSERT INTO foo (a,b,c,d) VALUES (:name, :age, :first, :last) ;--`,
			expect: `INSERT INTO foo (a,b,c,d) VALUES (:name, :age, :first, :last),(:name, :age, :first, :last) ;--`,
			loop:   2,
		},
		{
			name:   `mysql syntax w/ trailer`,
			query:  `INSERT INTO foo (a,b,c,d) VALUES (?, ?, ?, ?) ;--`,
			expect: `INSERT INTO foo (a,b,c,d) VALUES (?, ?, ?, ?),(?, ?, ?, ?) ;--`,
			loop:   2,
		},
		{
			name:   `not found test`,
			query:  `INSERT INTO foo (a,b,c,d) (:name, :age, :first, :last)`,
			expect: `INSERT INTO foo (a,b,c,d) (:name, :age, :first, :last)`,
			loop:   2,
		},
		{
			name:   `found twice test`,
			query:  `INSERT INTO foo (a,b,c,d) VALUES (:name, :age, :first, :last) VALUES (:name, :age, :first, :last)`,
			expect: `INSERT INTO foo (a,b,c,d) VALUES (:name, :age, :first, :last),(:name, :age, :first, :last) VALUES (:name, :age, :first, :last)`,
			loop:   2,
		},
		{
			name:   `nospace`,
			query:  `INSERT INTO foo (a,b) VALUES(:a, :b)`,
			expect: `INSERT INTO foo (a,b) VALUES(:a, :b),(:a, :b)`,
			loop:   2,
		},
		{
			name:   `lowercase`,
			query:  `INSERT INTO foo (a,b) values(:a, :b)`,
			expect: `INSERT INTO foo (a,b) values(:a, :b),(:a, :b)`,
			loop:   2,
		},
		{
			name:   `on duplicate key using VALUES`,
			query:  `INSERT INTO foo (a,b) VALUES (:a, :b) ON DUPLICATE KEY UPDATE a=VALUES(a)`,
			expect: `INSERT INTO foo (a,b) VALUES (:a, :b),(:a, :b) ON DUPLICATE KEY UPDATE a=VALUES(a)`,
			loop:   2,
		},
		{
			name:   `single column`,
			query:  `INSERT INTO foo (a) VALUES (:a)`,
			expect: `INSERT INTO foo (a) VALUES (:a),(:a)`,
			loop:   2,
		},
		{
			name:   `call now`,
			query:  `INSERT INTO foo (a, b) VALUES (:a, NOW())`,
			expect: `INSERT INTO foo (a, b) VALUES (:a, NOW()),(:a, NOW())`,
			loop:   2,
		},
		{
			name:   `two level depth function call`,
			query:  `INSERT INTO foo (a, b) VALUES (:a, YEAR(NOW()))`,
			expect: `INSERT INTO foo (a, b) VALUES (:a, YEAR(NOW())),(:a, YEAR(NOW()))`,
			loop:   2,
		},
		{
			name:   `missing closing bracket`,
			query:  `INSERT INTO foo (a, b) VALUES (:a, YEAR(NOW())`,
			expect: `INSERT INTO foo (a, b) VALUES (:a, YEAR(NOW())`,
			loop:   2,
		},
		{
			name:   `table with "values" at the end`,
			query:  `INSERT INTO table_values (a, b) VALUES (:a, :b)`,
			expect: `INSERT INTO table_values (a, b) VALUES (:a, :b),(:a, :b)`,
			loop:   2,
		},
		{
			name: `multiline indented query`,
			query: `INSERT INTO foo (
		a,
		b,
		c,
		d
	) VALUES (
		:name,
		:age,
		:first,
		:last
	)`,
			expect: `INSERT INTO foo (
		a,
		b,
		c,
		d
	) VALUES (
		:name,
		:age,
		:first,
		:last
	),(
		:name,
		:age,
		:first,
		:last
	)`,
			loop: 2,
		},
	}

	for _, tc := range table {
		t.Run(tc.name, func(t *testing.T) {
			res := fixBound(tc.query, tc.loop)
			if res != tc.expect {
				t.Errorf("mismatched results")
			}
		})
	}
}

// TestNamedParamTolerance tests tolerance handling when named parameters do not exist (Issue #892)
func TestNamedParamTolerance(t *testing.T) {
	t.Run("tolerance_when_map_parameter_does_not_exist", func(t *testing.T) {
		// All parameters do not exist; should not error, :name preserved as-is
		query := `SELECT * FROM users WHERE name = :name`
		bound, arglist, err := bindMap(QUESTION, query, map[string]any{})
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if len(arglist) != 0 {
			t.Errorf("expected 0 args, got %d: %v", len(arglist), arglist)
		}
		// :name in the query should be preserved as-is
		if !strings.Contains(bound, ":name") {
			t.Errorf("expected :name to remain in query, got: %s", bound)
		}
	})

	t.Run("some_map_parameters_exist", func(t *testing.T) {
		// :name exists, :min_age does not
		query := `SELECT * FROM users WHERE name = :name AND age > :min_age`
		bound, arglist, err := bindMap(QUESTION, query, map[string]any{"name": "Alice"})
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if len(arglist) != 1 {
			t.Errorf("expected 1 arg, got %d: %v", len(arglist), arglist)
		}
		if arglist[0] != "Alice" {
			t.Errorf("expected first arg to be 'Alice', got: %v", arglist[0])
		}
		// :name 应被替换为 ?，:min_age 应保持原样
		if strings.Contains(bound, ":name") {
			t.Errorf("expected :name to be replaced, got: %s", bound)
		}
		if !strings.Contains(bound, ":min_age") {
			t.Errorf("expected :min_age to remain in query, got: %s", bound)
		}
	})

	t.Run("all_map_parameters_exist", func(t *testing.T) {
		// Normal scenario, should work normally
		query := `SELECT * FROM users WHERE name = :name AND age > :min_age`
		bound, arglist, err := bindMap(QUESTION, query, map[string]any{"name": "Alice", "min_age": 18})
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if len(arglist) != 2 {
			t.Errorf("expected 2 args, got %d: %v", len(arglist), arglist)
		}
		if strings.Contains(bound, ":name") || strings.Contains(bound, ":min_age") {
			t.Errorf("expected all params to be replaced, got: %s", bound)
		}
	})

	t.Run("tolerance_when_struct_field_does_not_exist", func(t *testing.T) {
		type PartialUser struct {
			Name string `db:"name"`
		}
		// :min_age does not exist in the struct
		query := `SELECT * FROM users WHERE name = :name AND age > :min_age`
		bound, arglist, err := bindStruct(QUESTION, query, PartialUser{Name: "Bob"}, mapper())
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if len(arglist) != 1 {
			t.Errorf("expected 1 arg, got %d: %v", len(arglist), arglist)
		}
		if arglist[0] != "Bob" {
			t.Errorf("expected first arg to be 'Bob', got: %v", arglist[0])
		}
		// :name 应被替换为 ?，:min_age 应保持原样
		if strings.Contains(bound, ":name") {
			t.Errorf("expected :name to be replaced, got: %s", bound)
		}
		if !strings.Contains(bound, ":min_age") {
			t.Errorf("expected :min_age to remain in query, got: %s", bound)
		}
	})

	t.Run("named_parameter_form_wrapped_in_string_should_be_skipped", func(t *testing.T) {
		// ':name' is inside quotes, should be skipped
		query := `SELECT * FROM users WHERE name = ':name'`
		bound, arglist, err := bindMap(QUESTION, query, map[string]any{})
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if len(arglist) != 0 {
			t.Errorf("expected 0 args, got %d: %v", len(arglist), arglist)
		}
		// ':name' inside the string should be preserved as-is
		if !strings.Contains(bound, "':name'") {
			t.Errorf("expected ':name' to remain as string literal, got: %s", bound)
		}
	})

	t.Run("DOLLAR_type_some_parameters_exist", func(t *testing.T) {
		// Test DOLLAR type placeholder fallback
		query := `SELECT * FROM users WHERE name = :name AND age > :min_age AND city = :city`
		bound, arglist, err := bindMap(DOLLAR, query, map[string]any{"name": "Alice", "city": "NYC"})
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if len(arglist) != 2 {
			t.Errorf("expected 2 args, got %d: %v", len(arglist), arglist)
		}
		// :min_age should be preserved as-is
		if !strings.Contains(bound, ":min_age") {
			t.Errorf("expected :min_age to remain in query, got: %s", bound)
		}
		// $1 and $2 should exist (name and city are correctly numbered)
		if !strings.Contains(bound, "$1") || !strings.Contains(bound, "$2") {
			t.Errorf("expected $1 and $2 in query, got: %s", bound)
		}
	})

	t.Run("fields_in_args_not_referenced_in_SQL_should_be_ignored", func(t *testing.T) {
		// Map has extra fields, should be silently ignored
		query := `SELECT * FROM users WHERE name = :name`
		bound, arglist, err := bindMap(QUESTION, query, map[string]any{"name": "Alice", "unused": "field"})
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if len(arglist) != 1 {
			t.Errorf("expected 1 arg, got %d: %v", len(arglist), arglist)
		}
		if !strings.Contains(bound, "?") {
			t.Errorf("expected ? placeholder, got: %s", bound)
		}
	})

	// End-to-end regression: question mark inside string literal + missing named parameter. compileNamedQueryWith
	// compiles in a single pass: reuses scanSkipSegment to skip ? inside string literals, and preserves
	// missing :missing as a literal. There is no "second lexical scan" drift risk.
	t.Run("question_mark_inside_string_literal_coexists_with_missing_named_parameter", func(t *testing.T) {
		query := `SELECT 'are you sure?' AS note FROM t WHERE id = :missing`
		bound, arglist, err := bindMap(QUESTION, query, map[string]any{})
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if len(arglist) != 0 {
			t.Errorf("expected 0 args, got %d: %v", len(arglist), arglist)
		}
		// String literal must be preserved as-is
		if !strings.Contains(bound, "'are you sure?'") {
			t.Errorf("string literal corrupted, got: %s", bound)
		}
		// Missing parameter is restored as :missing
		if !strings.Contains(bound, ":missing") {
			t.Errorf("expected :missing to be restored, got: %s", bound)
		}
	})
}
