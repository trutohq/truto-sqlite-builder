// Port of src/sql.test.ts, case for case, in the reference's order.
//
// Subtest names match the reference's `it` names so the two suites can be
// diffed. Cases that the Go type system makes unreachable are marked
// TYPE-ENFORCED and assert the equivalent Go guarantee rather than being
// dropped; TRACKER.md counts them separately.
package sqlb

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------- basics ---

func TestSQL_BasicFunctionality(t *testing.T) {
	t.Run("should create simple queries", func(t *testing.T) {
		q, err := Q("SELECT * FROM users")
		wantNoErr(t, err)
		wantText(t, q.Text(), "SELECT * FROM users")
		wantValues(t, q.Values())
	})

	t.Run("should handle string values", func(t *testing.T) {
		q, err := Q("SELECT * FROM users WHERE name = ?", "John")
		wantNoErr(t, err)
		wantText(t, q.Text(), "SELECT * FROM users WHERE name = ?")
		wantValues(t, q.Values(), "John")
	})

	t.Run("should handle numeric values", func(t *testing.T) {
		q, err := Q("SELECT * FROM users WHERE id = ?", 42)
		wantNoErr(t, err)
		wantText(t, q.Text(), "SELECT * FROM users WHERE id = ?")
		wantValues(t, q.Values(), 42)
	})

	t.Run("should handle boolean values", func(t *testing.T) {
		q, err := Q("SELECT * FROM users WHERE active = ?", true)
		wantNoErr(t, err)
		wantText(t, q.Text(), "SELECT * FROM users WHERE active = ?")
		wantValues(t, q.Values(), true)
	})

	t.Run("should handle null values", func(t *testing.T) {
		q, err := Q("SELECT * FROM users WHERE deleted_at = ?", nil)
		wantNoErr(t, err)
		wantText(t, q.Text(), "SELECT * FROM users WHERE deleted_at = ?")
		wantValues(t, q.Values(), nil)
	})

	// The reference distinguishes null from undefined and maps both to null
	// (src/sql.ts:29). Go has one nil; [Undefined] carries the other, and it maps
	// to nil here just as it does there.
	t.Run("should handle undefined values as null", func(t *testing.T) {
		q, err := Q("SELECT * FROM users WHERE deleted_at = ?", Undefined)
		wantNoErr(t, err)
		wantText(t, q.Text(), "SELECT * FROM users WHERE deleted_at = ?")
		wantValues(t, q.Values(), nil)
	})

	t.Run("should handle Date values", func(t *testing.T) {
		// new Date('2023-12-25T10:30:00') has no zone designator, so JavaScript
		// reads it as local time; time.Local is the faithful equivalent.
		date := time.Date(2023, 12, 25, 10, 30, 0, 0, time.Local)
		q, err := Q("SELECT * FROM users WHERE created_at = ?", date)
		wantNoErr(t, err)
		wantText(t, q.Text(), "SELECT * FROM users WHERE created_at = ?")
		wantValues(t, q.Values(), "2023-12-25 10:30:00")
	})

	t.Run("should handle multiple values", func(t *testing.T) {
		q, err := Q("SELECT * FROM users WHERE name = ? AND age = ? AND active = ?", "John", 30, true)
		wantNoErr(t, err)
		wantText(t, q.Text(), "SELECT * FROM users WHERE name = ? AND age = ? AND active = ?")
		wantValues(t, q.Values(), "John", 30, true)
	})

	// TYPE-ENFORCED. The reference freezes the result object and its values array
	// so a caller cannot mutate a fragment after the integrity checks that
	// produced it (src/fragment.ts:29-32). Here a Fragment is a value type whose
	// fields are unexported: assignment copies it and nothing outside the package
	// can reach in.
	t.Run("should return frozen objects (TYPE-ENFORCED)", func(t *testing.T) {
		q, err := Q("SELECT * FROM users WHERE id = ?", 1)
		wantNoErr(t, err)
		copied := q
		wantText(t, copied.Text(), q.Text())
		if ft := reflect.TypeOf(Fragment{}); ft.NumField() != 3 {
			t.Fatalf("Fragment gained a field; check none are exported")
		}
		for i := range reflect.TypeOf(Fragment{}).NumField() {
			if f := reflect.TypeOf(Fragment{}).Field(i); f.IsExported() {
				t.Errorf("Fragment.%s is exported; fragments become forgeable", f.Name)
			}
		}
	})
}

// ----------------------------------------------------------------- ident ---

func TestSQL_Ident(t *testing.T) {
	t.Run("should quote valid identifiers", func(t *testing.T) {
		f := wantOK(t, Ident("users"))
		wantText(t, f.Text(), `"users"`)
		wantValues(t, f.Values())
	})

	t.Run("should handle identifiers with underscores", func(t *testing.T) {
		wantText(t, wantOK(t, Ident("user_name")).Text(), `"user_name"`)
	})

	t.Run("should handle identifiers starting with underscore", func(t *testing.T) {
		wantText(t, wantOK(t, Ident("_private")).Text(), `"_private"`)
	})

	t.Run("should work in queries", func(t *testing.T) {
		q, err := Q("SELECT * FROM ?", Ident("users"))
		wantNoErr(t, err)
		wantText(t, q.Text(), `SELECT * FROM "users"`)
		wantValues(t, q.Values())
	})

	t.Run("should reject empty strings", func(t *testing.T) {
		wantFragErr(t, Ident(""), "Identifier must be a non-empty string")
	})

	// TYPE-ENFORCED. Ident takes a string; there is no non-string to pass.
	t.Run("should reject non-strings (TYPE-ENFORCED)", func(t *testing.T) {
		wantFragErr(t, Ident("123"), "Invalid identifier")
	})

	t.Run("should handle qualified identifiers", func(t *testing.T) {
		wantText(t, wantOK(t, Ident("u.name")).Text(), `"u"."name"`)
	})

	t.Run("should handle multi-level qualified identifiers", func(t *testing.T) {
		wantText(t, wantOK(t, Ident("schema.table.column")).Text(), `"schema"."table"."column"`)
	})

	t.Run("should work with qualified identifiers in queries", func(t *testing.T) {
		q, err := Q("SELECT ?, ? FROM users u JOIN posts p ON u.id = p.user_id", Ident("u.name"), Ident("p.title"))
		wantNoErr(t, err)
		wantText(t, q.Text(), `SELECT "u"."name", "p"."title" FROM users u JOIN posts p ON u.id = p.user_id`)
		wantValues(t, q.Values())
	})

	t.Run("should handle qualified identifiers in arrays", func(t *testing.T) {
		f := wantOK(t, Idents("u.id", "u.name", "p.title", "p.created_at"))
		wantText(t, f.Text(), `"u"."id", "u"."name", "p"."title", "p"."created_at"`)
		wantValues(t, f.Values())
	})

	t.Run("should handle mixed simple and qualified identifiers in arrays", func(t *testing.T) {
		f := wantOK(t, Idents("name", "u.email", "created_at"))
		wantText(t, f.Text(), `"name", "u"."email", "created_at"`)
	})

	t.Run("should reject invalid identifiers", func(t *testing.T) {
		for _, bad := range []string{
			"123invalid", "user-name", "user name", "user.", ".name", "user..name", "user.123invalid",
		} {
			wantFragErr(t, Ident(bad), "Invalid identifier")
		}
	})

	t.Run("should quote arrays of valid identifiers", func(t *testing.T) {
		f := wantOK(t, Idents("name", "email", "age"))
		wantText(t, f.Text(), `"name", "email", "age"`)
		wantValues(t, f.Values())
	})

	t.Run("should handle arrays with single identifier", func(t *testing.T) {
		wantText(t, wantOK(t, Idents("users")).Text(), `"users"`)
	})

	t.Run("should handle arrays with identifiers containing underscores", func(t *testing.T) {
		f := wantOK(t, Idents("user_name", "email_address", "_private"))
		wantText(t, f.Text(), `"user_name", "email_address", "_private"`)
	})

	t.Run("should work with arrays in SELECT queries", func(t *testing.T) {
		q, err := Q("SELECT ? FROM users", Idents("name", "email", "created_at"))
		wantNoErr(t, err)
		wantText(t, q.Text(), `SELECT "name", "email", "created_at" FROM users`)
		wantValues(t, q.Values())
	})

	t.Run("should work with arrays in INSERT queries", func(t *testing.T) {
		q, err := Q("INSERT INTO users (?) VALUES (?, ?, ?)",
			Idents("name", "email", "age"), "John", "john@example.com", 30)
		wantNoErr(t, err)
		wantText(t, q.Text(), `INSERT INTO users ("name", "email", "age") VALUES (?, ?, ?)`)
		wantValues(t, q.Values(), "John", "john@example.com", 30)
	})

	t.Run("should reject empty arrays", func(t *testing.T) {
		wantFragErr(t, Idents(), "Identifier array cannot be empty")
	})

	t.Run("should reject arrays with invalid identifiers", func(t *testing.T) {
		wantFragErr(t, Idents("valid", "123invalid"), "Invalid identifier: 123invalid")
		wantFragErr(t, Idents("name", "user-name"), "Invalid identifier: user-name")
		wantFragErr(t, Idents("email", "user name"), "Invalid identifier: user name")
		wantFragErr(t, Idents("valid", "user."), "Invalid identifier: user.")
	})

	t.Run("should reject arrays with invalid non-string/non-fragment elements", func(t *testing.T) {
		wantFragErr(t, Idents("name", 123), "Array items must be strings or SQL fragments")
		wantFragErr(t, Idents("name", nil), "Array items must be strings or SQL fragments")
		wantFragErr(t, Idents("name", ""), "All identifiers must be non-empty strings")
	})

	t.Run("should handle mixed arrays with strings and SQL fragments", func(t *testing.T) {
		f := wantOK(t, Idents("id", "name", Raw("UPPER(email) as email_upper")))
		wantText(t, f.Text(), `"id", "name", UPPER(email) as email_upper`)
		wantValues(t, f.Values())
	})

	t.Run("should handle mixed arrays with fragments that have values", func(t *testing.T) {
		caseFrag, err := Q("CASE WHEN role = ? THEN 'Admin User' ELSE name END as display_name", "admin")
		wantNoErr(t, err)
		f := wantOK(t, Idents("id", "name", caseFrag))
		wantText(t, f.Text(), `"id", "name", CASE WHEN role = ? THEN 'Admin User' ELSE name END as display_name`)
		wantValues(t, f.Values(), "admin")
	})

	t.Run("should work with mixed arrays in SELECT queries", func(t *testing.T) {
		q, err := Q("SELECT ? FROM users GROUP BY id, name", Idents("id", "name", Raw("COUNT(*) as total")))
		wantNoErr(t, err)
		wantText(t, q.Text(), `SELECT "id", "name", COUNT(*) as total FROM users GROUP BY id, name`)
		wantValues(t, q.Values())
	})

	t.Run("should handle complex mixed arrays", func(t *testing.T) {
		caseFrag, err := Q("CASE WHEN u.status = ? THEN 'Active' ELSE 'Inactive' END as status_label", "active")
		wantNoErr(t, err)
		f := wantOK(t, Idents("id", "name", Raw("u.email"), caseFrag, Raw("COUNT(p.id) as post_count")))
		wantText(t, f.Text(),
			`"id", "name", u.email, CASE WHEN u.status = ? THEN 'Active' ELSE 'Inactive' END as status_label, COUNT(p.id) as post_count`)
		wantValues(t, f.Values(), "active")
	})

	t.Run("should validate string identifiers in mixed arrays", func(t *testing.T) {
		wantFragErr(t, Idents("valid", Raw("COUNT(*)"), "123invalid"), "Invalid identifier: 123invalid")
		wantFragErr(t, Idents("u.valid", Raw("COUNT(*)"), "user."), "Invalid identifier: user.")
	})
}

func TestSQL_Ident_InjectionPrevention(t *testing.T) {
	t.Run("should allow SQL keywords as quoted identifiers (valid behavior)", func(t *testing.T) {
		for _, kw := range []string{
			"SELECT", "INSERT", "UPDATE", "DELETE", "DROP", "CREATE", "ALTER",
			"TRUNCATE", "EXEC", "UNION", "WHERE", "FROM", "JOIN", "ORDER",
			"GROUP", "HAVING",
		} {
			wantText(t, wantOK(t, Ident(kw)).Text(), `"`+kw+`"`)
			lower := strings.ToLower(kw)
			wantText(t, wantOK(t, Ident(lower)).Text(), `"`+lower+`"`)
		}
	})

	reject := func(t *testing.T, ids []string) {
		t.Helper()
		for _, id := range ids {
			wantFragErr(t, Ident(id), "Invalid identifier")
		}
	}

	t.Run("should reject identifiers with SQL comments", func(t *testing.T) {
		reject(t, []string{
			"name--", "name-- comment", "name/*comment*/", "name/*", "*/name",
			"na--me", "na/*me*/", "table-- DROP TABLE users", "column/* malicious comment */",
		})
	})

	t.Run("should reject identifiers with semicolons", func(t *testing.T) {
		reject(t, []string{
			"name;", "name; DROP TABLE users", "name;DROP TABLE users", ";name",
			"na;me", "table; DELETE FROM users",
		})
	})

	t.Run("should reject identifiers with quotes", func(t *testing.T) {
		reject(t, []string{
			"name'", "name' OR 1=1", "name' UNION SELECT", "'name", "na'me",
			`name"`, `name" OR 1=1`, `"name`, `na"me`, "name'; DROP TABLE users; --",
		})
	})

	t.Run("should reject identifiers with parentheses and function calls", func(t *testing.T) {
		reject(t, []string{
			"name()", "DROP()", "EXEC()", "name(1)", "name)", "(name", "na(me",
			"name)me", "SUBSTRING(password", "CONCAT(name, password)",
		})
	})

	t.Run("should reject identifiers with arithmetic and comparison operators", func(t *testing.T) {
		reject(t, []string{
			"name+1", "name-1", "name*1", "name/1", "name=1", "name<>1", "name!=1",
			"name>1", "name<1", "name>=1", "name<=1", "name%1", "name^1", "name&1", "name|1",
		})
	})

	t.Run("should reject identifiers with whitespace characters", func(t *testing.T) {
		reject(t, []string{
			"name ", " name", "na me", "name\t", "\tname", "na\tme",
			"name\n", "\nname", "na\nme", "name\r", "\rname", "na\rme",
		})
	})

	t.Run("should reject identifiers with special characters", func(t *testing.T) {
		reject(t, []string{
			"name@", "name#", "name$", "name%", "name&", "name*", "name+", "name-",
			"name=", "name[", "name]", "name{", "name}", "name|", `name\`, "name/",
			"name?", "name!", "name~", "name`",
		})
	})

	t.Run("should reject qualified identifiers with SQL injection attempts", func(t *testing.T) {
		reject(t, []string{
			"table.name; DROP TABLE users",
			"table.name-- comment",
			"table.name/*comment*/",
			"table.name' OR 1=1",
			`table.name" OR 1=1`,
			"table.name()",
			"table.name=1",
			"table.name OR 1=1",
			"table.name UNION SELECT",
			"table.name; EXEC",
			"2' AND ORD(MID((SELECT DISTINCT(IFNULL(CAST(schema_name AS NCHAR),0x20)) FROM INFORMATION_SCHEMA.SCHEMATA LIMIT 5,1),5,1))>1 AND 'vFAF'='vFAF",
		})
	})

	t.Run("should allow valid qualified identifiers with SQL keywords", func(t *testing.T) {
		for _, id := range []string{"DROP.TABLE.users", "SELECT.FROM.users", "ORDER.BY.name"} {
			wantText(t, wantOK(t, Ident(id)).Text(), quoteParts(id))
		}
	})

	t.Run("should reject arrays containing SQL injection attempts", func(t *testing.T) {
		for _, pair := range [][2]string{
			{"valid", "name; DROP TABLE users"},
			{"valid", "name' OR 1=1"},
			{"valid", "name-- comment"},
			{"valid", "name/*comment*/"},
			{"valid", "name()"},
			{"valid", "name=1"},
			{"table.valid", "table.name-- comment"},
			{"table.valid", "table.name' OR 1=1"},
		} {
			wantFragErr(t, Idents(pair[0], pair[1]), "Invalid identifier")
		}
	})

	t.Run("should reject Unicode and encoded injection attempts", func(t *testing.T) {
		reject(t, []string{
			"name\u0000",
			"name\u0027",
			"name\u0022",
			"name\u003B",
			"name\u002D\u002D",
			"name\uFEFF",
			"name\u200B",
		})
	})

	t.Run("should reject identifiers that could break out of quotes", func(t *testing.T) {
		reject(t, []string{
			`name" + "`, "name' + '", `name" || "`, "name' || '",
			`name" CONCAT "`, "name' CONCAT '", `name"+"`, "name'+'",
		})
	})

	t.Run("should properly quote valid identifiers to prevent misinterpretation", func(t *testing.T) {
		for _, id := range []string{
			"user_id", "table_name", "column_name", "_private", "u", "user123",
			"User_Name_123", "u.name", "users.id", "schema.table.column",
		} {
			f := wantOK(t, Ident(id))
			wantText(t, f.Text(), quoteParts(id))
			wantValues(t, f.Values())

			q, err := Q("SELECT ? FROM users", Ident(id))
			wantNoErr(t, err)
			wantText(t, q.Text(), "SELECT "+quoteParts(id)+" FROM users")
			wantValues(t, q.Values())
		}
	})

	t.Run("should maintain security when used in real query contexts", func(t *testing.T) {
		maliciousTable := "users; DROP TABLE users; --"
		maliciousColumn := "name' OR 1=1 --"

		_, err := Q("SELECT * FROM ?", Ident(maliciousTable))
		wantErr(t, err, "Invalid identifier")
		_, err = Q("SELECT ? FROM users", Ident(maliciousColumn))
		wantErr(t, err, "Invalid identifier")
		_, err = Q("SELECT ? FROM users", Idents(maliciousColumn, "email"))
		wantErr(t, err, "Invalid identifier")
	})

	t.Run("should demonstrate comprehensive injection protection in realistic scenarios", func(t *testing.T) {
		for _, in := range []string{
			"'; DROP TABLE users; --",
			`" OR 1=1 --`,
			"admin/**/AND/**/password",
			`users; EXEC xp_cmdshell("format c:")`,
			"users' UNION SELECT password FROM admin --",
			"table/*comment*/name",
			"column_name--",
			"name`OR`1=1",
			"users WHERE 1=1",
			`name\x00\x27`,
		} {
			_, err := Q("SELECT ? FROM users", Ident(in))
			wantErr(t, err, "Invalid identifier")
			_, err = Q("INSERT INTO ? (name) VALUES (?)", Ident(in), "test")
			wantErr(t, err, "Invalid identifier")
			_, err = Q("UPDATE users SET ? = ?", Ident(in), "value")
			wantErr(t, err, "Invalid identifier")
			_, err = Q("SELECT ? FROM users", Idents("id", in, "email"))
			wantErr(t, err, "Invalid identifier")
			_, err = Q("SELECT ? FROM users", Ident("table."+in))
			wantErr(t, err, "Invalid identifier")
		}
	})

	t.Run("should allow legitimate identifiers that might look suspicious but are valid", func(t *testing.T) {
		for _, id := range []string{
			"SELECT", "user_id_123", "_internal_column", "Column_With_Underscores",
			"u.user_name", "schema.table.column", "UserTable", "ORDER",
		} {
			f := wantOK(t, Ident(id))
			wantText(t, f.Text(), quoteParts(id))
			q, err := Q("SELECT ? FROM users", Ident(id))
			wantNoErr(t, err)
			wantText(t, q.Text(), "SELECT "+quoteParts(id)+" FROM users")
			wantValues(t, q.Values())
		}
	})
}

func quoteParts(id string) string {
	parts := strings.Split(id, ".")
	for i, p := range parts {
		parts[i] = `"` + p + `"`
	}
	return strings.Join(parts, ".")
}

// -------------------------------------------------------------------- in ---

func TestSQL_In(t *testing.T) {
	t.Run("should create IN clauses", func(t *testing.T) {
		f := wantOK(t, In(1, 2, 3))
		wantText(t, f.Text(), "(?,?,?)")
		wantValues(t, f.Values(), 1, 2, 3)
	})

	t.Run("should work in queries", func(t *testing.T) {
		q, err := Q("SELECT * FROM users WHERE id IN ?", In(1, 2, 3))
		wantNoErr(t, err)
		wantText(t, q.Text(), "SELECT * FROM users WHERE id IN (?,?,?)")
		wantValues(t, q.Values(), 1, 2, 3)
	})

	t.Run("should handle mixed types", func(t *testing.T) {
		f := wantOK(t, In("a", 1, nil, true))
		wantText(t, f.Text(), "(?,?,?,?)")
		wantValues(t, f.Values(), "a", 1, nil, true)
	})

	t.Run("should reject empty arrays", func(t *testing.T) {
		wantFragErr(t, In(), "sql.in() cannot be used with empty arrays")
	})

	// TYPE-ENFORCED. In is variadic and InSlice is generic over a slice; there is
	// no non-array to pass.
	t.Run("should reject non-arrays (TYPE-ENFORCED)", func(t *testing.T) {
		wantFragErr(t, InSlice([]string{}), "sql.in() cannot be used with empty arrays")
	})

	t.Run("should warn for large arrays", func(t *testing.T) {
		large := make([]int, 1001)
		for i := range large {
			large[i] = i
		}
		logged := captureWarnings(t, func() { wantOK(t, InSlice(large)) })
		want := "sql.in(): Large array with 1001 items. Consider using temporary tables for better performance."
		if len(logged) != 1 || logged[0] != want {
			t.Errorf("warnings = %#v, want exactly [%q]", logged, want)
		}
	})

	t.Run("InSlice accepts a typed slice", func(t *testing.T) {
		f := wantOK(t, InSlice([]string{"a", "b"}))
		wantText(t, f.Text(), "(?,?)")
		wantValues(t, f.Values(), "a", "b")
	})
}

// ------------------------------------------------------------------- raw ---

func TestSQL_Raw(t *testing.T) {
	t.Run("should create raw SQL fragments", func(t *testing.T) {
		f := wantOK(t, Raw("NOW()"))
		wantText(t, f.Text(), "NOW()")
		wantValues(t, f.Values())
	})

	t.Run("should work in queries", func(t *testing.T) {
		q, err := Q("SELECT * FROM users WHERE created_at > ?", Raw("NOW() - INTERVAL 1 DAY"))
		wantNoErr(t, err)
		wantText(t, q.Text(), "SELECT * FROM users WHERE created_at > NOW() - INTERVAL 1 DAY")
		wantValues(t, q.Values())
	})

	// TYPE-ENFORCED. Raw takes a string.
	t.Run("should reject non-strings (TYPE-ENFORCED)", func(t *testing.T) {
		wantOK(t, Raw(""))
	})
}

// ------------------------------------------------------------------ blob ---

func TestSQL_Blob(t *testing.T) {
	t.Run("should create BLOB fragments from Buffer", func(t *testing.T) {
		buf := []byte("test data")
		f := wantOK(t, Blob(buf))
		wantText(t, f.Text(), "?")
		wantValues(t, f.Values(), buf)
	})

	t.Run("should create BLOB fragments from Uint8Array", func(t *testing.T) {
		arr := []byte{1, 2, 3, 4}
		f := wantOK(t, Blob(arr))
		wantText(t, f.Text(), "?")
		wantValues(t, f.Values(), arr)
	})

	t.Run("should work in queries with Buffer", func(t *testing.T) {
		buf := []byte("test data")
		q, err := Q("INSERT INTO files (data) VALUES (?)", Blob(buf))
		wantNoErr(t, err)
		wantText(t, q.Text(), "INSERT INTO files (data) VALUES (?)")
		wantValues(t, q.Values(), buf)
	})

	t.Run("should work in queries with Uint8Array", func(t *testing.T) {
		arr := []byte{1, 2, 3, 4}
		q, err := Q("UPDATE files SET data = ? WHERE id = ?", Blob(arr), 1)
		wantNoErr(t, err)
		wantText(t, q.Text(), "UPDATE files SET data = ? WHERE id = ?")
		wantValues(t, q.Values(), arr, 1)
	})

	// The reference rejects a string, a number and null. Go's signature rejects
	// the first two at compile time; a nil slice is the null case and is rejected
	// at run time.
	t.Run("should reject non-binary data", func(t *testing.T) {
		wantFragErr(t, Blob(nil), "sql.blob() requires a Buffer or Uint8Array")
	})

	t.Run("should handle empty buffers", func(t *testing.T) {
		f := wantOK(t, Blob([]byte{}))
		wantText(t, f.Text(), "?")
		if got := f.Values(); len(got) != 1 || !bytes.Equal(got[0].([]byte), []byte{}) {
			t.Errorf("values = %#v", got)
		}
	})

	t.Run("should handle large binary data", func(t *testing.T) {
		large := bytes.Repeat([]byte{0xaa}, 10000)
		f := wantOK(t, Blob(large))
		wantText(t, f.Text(), "?")
		wantValues(t, f.Values(), large)
	})
}

// ------------------------------------------------------------------ join ---

func TestSQL_Join(t *testing.T) {
	t.Run("should join fragments with default separator", func(t *testing.T) {
		f := wantOK(t, Join([]Fragment{Ident("name"), Ident("email"), Ident("age")}, ", "))
		wantText(t, f.Text(), `"name", "email", "age"`)
		wantValues(t, f.Values())
	})

	t.Run("should join fragments with custom separator", func(t *testing.T) {
		f := wantOK(t, Join([]Fragment{Ident("name"), Ident("email")}, " AND "))
		wantText(t, f.Text(), `"name" AND "email"`)
	})

	t.Run("should handle fragments with values", func(t *testing.T) {
		a, err := Q("name = ?", "John")
		wantNoErr(t, err)
		b, err := Q("age = ?", 30)
		wantNoErr(t, err)
		f := wantOK(t, Join([]Fragment{a, b}, " AND "))
		wantText(t, f.Text(), "name = ? AND age = ?")
		wantValues(t, f.Values(), "John", 30)
	})

	t.Run("should work in queries", func(t *testing.T) {
		a, err := Q("name = ?", "John")
		wantNoErr(t, err)
		b, err := Q("age = ?", 30)
		wantNoErr(t, err)
		q, err := Q("SELECT * FROM users WHERE ?", Join([]Fragment{a, b}, " AND "))
		wantNoErr(t, err)
		wantText(t, q.Text(), "SELECT * FROM users WHERE name = ? AND age = ?")
		wantValues(t, q.Values(), "John", 30)
	})

	t.Run("should handle empty arrays", func(t *testing.T) {
		f := wantOK(t, Join(nil, ", "))
		wantText(t, f.Text(), "")
		wantValues(t, f.Values())
	})

	// TYPE-ENFORCED. Join takes []Fragment.
	t.Run("should reject non-arrays (TYPE-ENFORCED)", func(t *testing.T) {
		wantOK(t, Join([]Fragment{}, ", "))
	})
}

// -------------------------------------------------------- security checks ---

func TestSQL_SecurityFeatures(t *testing.T) {
	t.Run("should reject stacked queries", func(t *testing.T) {
		_, err := Q("SELECT * FROM users; DROP TABLE users;")
		wantErr(t, err, "Stacked queries are not allowed")
		_, err = Q("SELECT * FROM users;\nDROP TABLE users;")
		wantErr(t, err, "Stacked queries are not allowed")
		_, err = Q("SELECT * FROM users;\tDROP TABLE users;")
		wantErr(t, err, "Stacked queries are not allowed")
	})

	t.Run("should allow semicolons in strings", func(t *testing.T) {
		q, err := Q("SELECT * FROM users WHERE comment = ?", "Hello; World")
		wantNoErr(t, err)
		wantText(t, q.Text(), "SELECT * FROM users WHERE comment = ?")
		wantValues(t, q.Values(), "Hello; World")
	})

	t.Run("should reject queries that are too long", func(t *testing.T) {
		long := "SELECT * FROM users WHERE " + strings.Repeat("a", 102500)
		_, err := Q("?", Raw(long))
		wantErr(t, err, "max: 102400")
		wantErr(t, err, "Query too long")
	})

	t.Run("should reject Buffer values in interpolation", func(t *testing.T) {
		_, err := Q("SELECT * FROM users WHERE data = ?", []byte("test"))
		wantErr(t, err, "Buffer/Uint8Array values must be used with sql.blob() for safe BLOB handling")
	})

	t.Run("should reject Uint8Array values in interpolation", func(t *testing.T) {
		_, err := Q("SELECT * FROM users WHERE data = ?", []byte{1, 2, 3})
		wantErr(t, err, "Buffer/Uint8Array values must be used with sql.blob() for safe BLOB handling")
	})

	t.Run("should reject unsupported value types", func(t *testing.T) {
		_, err := Q("SELECT * FROM users WHERE data = ?", make(chan int))
		wantErr(t, err, "Unsupported value type")
	})
}

// ------------------------------------------------------ injection corpus ---

// injectionCorpus asserts a hostile string is always bound, never spliced.
func injectionCorpus(t *testing.T, inputs []string) {
	t.Helper()
	for _, in := range inputs {
		q, err := Q("SELECT * FROM users WHERE name = ?", in)
		wantNoErr(t, err)
		wantText(t, q.Text(), "SELECT * FROM users WHERE name = ?")
		wantValues(t, q.Values(), in)
	}
}

func TestSQL_InjectionPrevention(t *testing.T) {
	t.Run("should prevent basic SQL injection", func(t *testing.T) {
		injectionCorpus(t, []string{"'; DROP TABLE users; --"})
	})

	t.Run("should prevent OR 1=1 injection", func(t *testing.T) {
		injectionCorpus(t, []string{"' OR 1=1 --"})
	})

	t.Run("should prevent UNION injection", func(t *testing.T) {
		injectionCorpus(t, []string{"' UNION SELECT password FROM admin --"})
	})

	t.Run("should handle special characters safely", func(t *testing.T) {
		injectionCorpus(t, []string{`'"\%_`})
	})

	t.Run("should prevent comment-based injections", func(t *testing.T) {
		injectionCorpus(t, []string{
			"admin'--",
			"admin'/*comment*/",
			"admin' -- comment",
			"admin'/* multi\nline\ncomment */",
			"admin'#comment",
			"admin';-- comment",
			"'; /* comment */ SELECT * FROM admin --",
		})
	})

	t.Run("should prevent various UNION-based injections", func(t *testing.T) {
		injectionCorpus(t, []string{
			"' UNION SELECT NULL, username, password FROM admin --",
			"' UNION ALL SELECT 1,2,3,4 --",
			"' UNION SELECT database(), user(), version() --",
			"' UNION SELECT 1,table_name,3 FROM information_schema.tables --",
			"' UNION SELECT CONCAT(username,':',password) FROM admin --",
			"' OR 1=1 UNION SELECT NULL, password FROM users --",
			"' UNION SELECT * FROM (SELECT password FROM admin) AS t --",
		})
	})

	t.Run("should prevent boolean-based blind injections", func(t *testing.T) {
		injectionCorpus(t, []string{
			"' AND 1=1 --",
			"' AND 1=2 --",
			"' AND (SELECT COUNT(*) FROM admin) > 0 --",
			"' AND (SELECT SUBSTR(password,1,1) FROM admin WHERE username='admin')='a' --",
			"' AND ASCII(SUBSTR((SELECT password FROM admin LIMIT 1),1,1))=97 --",
			"' AND LENGTH((SELECT password FROM admin LIMIT 1))=8 --",
			"' AND EXISTS(SELECT * FROM admin WHERE username='admin') --",
			"' AND 'a'='a",
			"' AND 'a'='b",
			"2' AND ORD(MID((SELECT DISTINCT(IFNULL(CAST(schema_name AS NCHAR),0x20)) FROM INFORMATION_SCHEMA.SCHEMATA LIMIT 5,1),5,1))>1 AND 'vFAF'='vFAF",
		})
	})

	t.Run("should prevent time-based blind injections", func(t *testing.T) {
		injectionCorpus(t, []string{
			"'; WAITFOR DELAY '00:00:05' --",
			"' AND (SELECT SLEEP(5)) --",
			"' OR (SELECT pg_sleep(5)) --",
			"'; SELECT pg_sleep(5); --",
			"' AND IF(1=1, SLEEP(5), 0) --",
			"' UNION SELECT SLEEP(5) --",
			"'; IF(1=1) WAITFOR DELAY '0:0:5' --",
		})
	})

	t.Run("should prevent error-based injections", func(t *testing.T) {
		injectionCorpus(t, []string{
			"' AND (SELECT * FROM (SELECT COUNT(*),CONCAT(version(),FLOOR(RAND(0)*2))x FROM information_schema.tables GROUP BY x)a) --",
			"' AND EXP(~(SELECT * FROM (SELECT USER())a)) --",
			"' AND GTID_SUBSET(CONCAT(0x7e,(SELECT database()),0x7e),1) --",
			"' AND (SELECT * FROM information_schema.tables FOR XML PATH('')) --",
			"' UNION SELECT 1/0 --",
			"' AND (SELECT password FROM admin WHERE username='admin' AND 1=CAST('a' AS INTEGER)) --",
		})
	})

	t.Run("should prevent authentication bypass attempts", func(t *testing.T) {
		for _, in := range []string{
			"admin'--", "admin'/*", "' OR 1=1#", "' OR 1=1--", "' OR '1'='1",
			"' OR '1'='1'--", "' OR '1'='1'/*", "') OR ('1'='1", "') OR ('1'='1'--",
			"admin') --", "admin') #", "') OR 1=1 --", "' OR 'x'='x", "' OR a=a--",
			"' or true--", "' OR 'something' like 'some%'", "' OR 2 > 1--",
			"' OR 'text' = N'text'", "' OR 'something' = 'something'",
			"' OR 'text' like 'text'", "' OR 1=1 LIMIT 1--",
		} {
			q, err := Q("SELECT * FROM users WHERE username = ?", in)
			wantNoErr(t, err)
			wantText(t, q.Text(), "SELECT * FROM users WHERE username = ?")
			wantValues(t, q.Values(), in)
		}
	})

	t.Run("should prevent subquery-based injections", func(t *testing.T) {
		injectionCorpus(t, []string{
			"' AND (SELECT COUNT(*) FROM admin) > 0 --",
			"' AND (SELECT password FROM admin WHERE username='admin') LIKE 'a%' --",
			"' OR (SELECT user FROM mysql.user WHERE user='root') = 'root' --",
			"' AND (SELECT table_name FROM information_schema.tables WHERE table_schema=database() LIMIT 1) = 'admin' --",
			"' OR EXISTS(SELECT 1 FROM admin WHERE username='admin' AND password='pass') --",
			"' UNION SELECT (SELECT password FROM admin WHERE username='admin') --",
			"' AND 1=(SELECT COUNT(*) FROM tabname); --",
		})
	})

	t.Run("should prevent function-based injections", func(t *testing.T) {
		injectionCorpus(t, []string{
			"'; SELECT LOAD_FILE('/etc/passwd') --",
			"'; SELECT INTO OUTFILE '/tmp/exploit.txt' --",
			"' UNION SELECT user(), database(), version() --",
			"' OR ASCII(SUBSTR(password,1,1))=65 --",
			"' AND CHAR_LENGTH(password)=8 --",
			"' UNION SELECT HEX(password) FROM admin --",
			"' OR MD5(password)='5d41402abc4b2a76b9719d911017c592' --",
			"'; EXEC xp_cmdshell('dir') --",
			"'; EXEC master..xp_regread @rootkey='HKEY_LOCAL_MACHINE' --",
			"' UNION SELECT @@version --",
			"' UNION SELECT current_user() --",
		})
	})

	t.Run("should prevent second-order injections", func(t *testing.T) {
		for _, in := range []string{
			"admin'; DROP TABLE users; SELECT 'data",
			"test'; INSERT INTO admin (username, password) VALUES ('hacker', 'pass'); SELECT 'data",
			"user'; UPDATE admin SET password='newpass' WHERE username='admin'; SELECT 'data",
			"data'; TRUNCATE TABLE logs; SELECT 'normal",
		} {
			insert, err := Q("INSERT INTO user_data (content) VALUES (?)", in)
			wantNoErr(t, err)
			wantText(t, insert.Text(), "INSERT INTO user_data (content) VALUES (?)")
			wantValues(t, insert.Values(), in)

			sel, err := Q("SELECT * FROM user_data WHERE content = ?", in)
			wantNoErr(t, err)
			wantText(t, sel.Text(), "SELECT * FROM user_data WHERE content = ?")
			wantValues(t, sel.Values(), in)
		}
	})

	t.Run("should prevent Unicode and encoding-based injections", func(t *testing.T) {
		injectionCorpus(t, []string{
			"admin\u0027 OR 1=1--",
			"admin\u0022 OR 1=1--",
			"admin\u003B DROP TABLE users--",
			"admin\u002D\u002D",
			"admin\uFF07 OR 1=1--",
			"admin\u2019 OR 1=1--",
			"admin%27 OR 1=1--",
			"admin%3B DROP TABLE users--",
			"admin\x27 OR 1=1--",
			"admin\x00",
			"admin\x00\x27 OR 1=1--",
		})
	})

	t.Run("should prevent database-specific injections", func(t *testing.T) {
		injectionCorpus(t, []string{
			"' OR 1=1 INTO OUTFILE '/tmp/file.txt' --",
			"' UNION SELECT @@version, @@datadir --",
			"'; SET @sql = CONCAT('DROP TABLE ', 'users'); PREPARE stmt FROM @sql; EXECUTE stmt; --",
			"'; COPY users TO '/tmp/users.csv' --",
			"' OR 1=1; CREATE OR REPLACE FUNCTION evil() RETURNS void AS $$ BEGIN RAISE NOTICE 'pwned'; END; $$ LANGUAGE plpgsql --",
			"' UNION SELECT sql FROM sqlite_master --",
			"'; ATTACH DATABASE '/tmp/evil.db' AS evil; --",
			"'; EXEC xp_cmdshell('net user hacker pass123 /add') --",
			"'; EXEC sp_configure 'show advanced options', 1 --",
			"' UNION SELECT @@servername, @@version --",
			"' UNION SELECT banner FROM v$version --",
			"'; GRANT DBA TO PUBLIC --",
		})
	})

	t.Run("should prevent advanced evasion techniques", func(t *testing.T) {
		injectionCorpus(t, []string{
			"' Or 1=1 --",
			"' oR 1=1 --",
			"' UnIoN sElEcT --",
			"'\t\tOR\t\t1=1--",
			"'\n\nOR\n\n1=1--",
			"'\r\rOR\r\r1=1--",
			"' /**/OR/**/1=1--",
			"'/*comment*/OR/*comment*/1=1--",
			"' + 'OR' + ' 1=1 --",
			"' || 'OR' || ' 1=1 --",
			"' CONCAT('OR', ' 1=1') --",
			"' OR ASCII('A')=65 --",
			"' OR CHAR(65)='A' --",
			"' OR HEX('41')='A' --",
			"'/*!32302 OR*/ 1=1 --",
			"'/*!50000 UNION SELECT*/ 1,2,3 --",
		})
	})

	t.Run("should prevent multiple statement attacks", func(t *testing.T) {
		injectionCorpus(t, []string{
			"admin'; DROP TABLE users; SELECT 'done",
			"admin'; INSERT INTO admin VALUES ('hacker', 'pass'); SELECT 'done",
			"admin'; UPDATE users SET admin=1 WHERE username='hacker'; SELECT 'done",
			"admin'; DELETE FROM logs; SELECT 'done",
			"admin'; TRUNCATE TABLE sessions; SELECT 'done",
			"admin'; CREATE TABLE backdoor (id INT); SELECT 'done",
			"admin'; ALTER TABLE users ADD COLUMN backdoor TEXT; SELECT 'done",
		})
	})

	t.Run("should safely handle complex legitimate data that might look suspicious", func(t *testing.T) {
		for _, in := range []string{
			"O'Reilly",
			"Jean-Luc O'Connor",
			"SELECT * FROM my_table",
			"UPDATE SET name = 'value'",
			"User input: DROP TABLE users",
			"Email: user@domain.com; Note: important",
			"Comment: /* This is a comment */",
			"Formula: a = b AND c = d",
			`Path: C:\Users\admin\file.txt`,
			"Version: 1.0.0 UNION compatible",
		} {
			q, err := Q("SELECT * FROM users WHERE description = ?", in)
			wantNoErr(t, err)
			wantText(t, q.Text(), "SELECT * FROM users WHERE description = ?")
			wantValues(t, q.Values(), in)

			ins, err := Q("INSERT INTO posts (content) VALUES (?)", in)
			wantNoErr(t, err)
			wantText(t, ins.Text(), "INSERT INTO posts (content) VALUES (?)")
			wantValues(t, ins.Values(), in)
		}
	})

	t.Run("should demonstrate protection across different query types", func(t *testing.T) {
		in := "'; DROP TABLE users; --"

		sel, err := Q("SELECT * FROM users WHERE name = ?", in)
		wantNoErr(t, err)
		wantText(t, sel.Text(), "SELECT * FROM users WHERE name = ?")
		wantValues(t, sel.Values(), in)

		ins, err := Q("INSERT INTO users (name) VALUES (?)", in)
		wantNoErr(t, err)
		wantText(t, ins.Text(), "INSERT INTO users (name) VALUES (?)")
		wantValues(t, ins.Values(), in)

		upd, err := Q("UPDATE users SET name = ? WHERE id = ?", in, 1)
		wantNoErr(t, err)
		wantText(t, upd.Text(), "UPDATE users SET name = ? WHERE id = ?")
		wantValues(t, upd.Values(), in, 1)

		del, err := Q("DELETE FROM users WHERE name = ?", in)
		wantNoErr(t, err)
		wantText(t, del.Text(), "DELETE FROM users WHERE name = ?")
		wantValues(t, del.Values(), in)

		join, err := Q(`
        SELECT u.name, p.title
        FROM users u
        JOIN posts p ON u.id = p.user_id
        WHERE u.name = ? AND p.status = ?
      `, in, "active")
		wantNoErr(t, err)
		wantText(t, strings.TrimSpace(collapseSpace(join.Text())),
			"SELECT u.name, p.title FROM users u JOIN posts p ON u.id = p.user_id WHERE u.name = ? AND p.status = ?")
		wantValues(t, join.Values(), in, "active")
	})
}

func collapseSpace(s string) string {
	var b strings.Builder
	prevSpace := false
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\v' || r == '\f' {
			if !prevSpace {
				b.WriteByte(' ')
			}
			prevSpace = true
			continue
		}
		prevSpace = false
		b.WriteRune(r)
	}
	return b.String()
}

// ------------------------------------------------------------ edge cases ---

func TestSQL_EdgeCases(t *testing.T) {
	t.Run("should handle empty template strings", func(t *testing.T) {
		q, err := Q("")
		wantNoErr(t, err)
		wantText(t, q.Text(), "")
		wantValues(t, q.Values())
	})

	t.Run("should handle only interpolation", func(t *testing.T) {
		q, err := Q("?", "test")
		wantNoErr(t, err)
		wantText(t, q.Text(), "?")
		wantValues(t, q.Values(), "test")
	})

	t.Run("should handle Date edge cases", func(t *testing.T) {
		date := time.Date(2023, 1, 1, 0, 0, 0, 0, time.Local)
		q, err := Q("SELECT * FROM users WHERE created_at = ?", date)
		wantNoErr(t, err)
		wantText(t, q.Text(), "SELECT * FROM users WHERE created_at = ?")
		wantValues(t, q.Values(), "2023-01-01 00:00:00")
	})

	t.Run("should handle nested template literals", func(t *testing.T) {
		condition, err := Q("name = ?", "John")
		wantNoErr(t, err)
		q, err := Q("SELECT * FROM ? WHERE ?", Ident("users"), condition)
		wantNoErr(t, err)
		wantText(t, q.Text(), `SELECT * FROM "users" WHERE name = ?`)
		wantValues(t, q.Values(), "John")
	})
}

// -------------------------------------------------------- complex queries ---

func TestSQL_ComplexQueries(t *testing.T) {
	t.Run("should handle INSERT queries", func(t *testing.T) {
		q, err := Q(`
        INSERT INTO users (name, email, age)
        VALUES (?, ?, ?)
      `, "John Doe", "john@example.com", 30)
		wantNoErr(t, err)
		wantText(t, strings.TrimSpace(q.Text()),
			"INSERT INTO users (name, email, age)\n        VALUES (?, ?, ?)")
		wantValues(t, q.Values(), "John Doe", "john@example.com", 30)
	})

	t.Run("should handle UPDATE queries", func(t *testing.T) {
		// The trailing space after "users" is in the reference's template literal
		// (src/sql.test.ts:1350) and is part of the expected output.
		q, err := Q(`
        UPDATE users 
        SET name = ?, age = ?
        WHERE id = ?
      `, "Jane Doe", 25, 1)
		wantNoErr(t, err)
		wantText(t, strings.TrimSpace(q.Text()),
			"UPDATE users \n        SET name = ?, age = ?\n        WHERE id = ?")
		wantValues(t, q.Values(), "Jane Doe", 25, 1)
	})

	t.Run("should handle complex SELECT with JOINs", func(t *testing.T) {
		q, err := Q(`
        SELECT u.name, p.title
        FROM ? u
        JOIN ? p ON u.id = p.user_id
        WHERE u.id = ? AND p.status = ?
      `, Ident("users"), Ident("posts"), 1, "active")
		wantNoErr(t, err)
		wantText(t, strings.TrimSpace(q.Text()),
			"SELECT u.name, p.title\n        FROM \"users\" u\n        JOIN \"posts\" p ON u.id = p.user_id\n        WHERE u.id = ? AND p.status = ?")
		wantValues(t, q.Values(), 1, "active")
	})
}

// -------------------------------------------------- Go-specific mechanics ---

func TestSQL_SlotArity(t *testing.T) {
	// The tagged template makes an arity mismatch impossible; Q's format cannot,
	// so it has to check. Fail closed on both sides.
	t.Run("too few arguments", func(t *testing.T) {
		_, err := Q("SELECT * FROM t WHERE a = ? AND b = ?", 1)
		wantErr(t, err, "2 interpolation slots")
	})
	t.Run("too many arguments", func(t *testing.T) {
		_, err := Q("SELECT * FROM t WHERE a = ?", 1, 2)
		wantErr(t, err, "1 interpolation slot")
	})
	t.Run("a ? inside a string literal is not a slot", func(t *testing.T) {
		q, err := Q("SELECT 'a?b' AS s, ? AS v", 1)
		wantNoErr(t, err)
		wantText(t, q.Text(), "SELECT 'a?b' AS s, ? AS v")
		wantValues(t, q.Values(), 1)
	})
	t.Run("a ? inside a comment is not a slot", func(t *testing.T) {
		q, err := Q("SELECT 1 /* ? */, ?", 2)
		wantNoErr(t, err)
		wantText(t, q.Text(), "SELECT 1 /* ? */, ?")
		wantValues(t, q.Values(), 2)
	})
	t.Run("a ? inside a bracket identifier is not a slot", func(t *testing.T) {
		q, err := Q("SELECT [a?] FROM t")
		wantNoErr(t, err)
		wantText(t, q.Text(), "SELECT [a?] FROM t")
	})
}

func TestSQL_ErrorsPropagateInArgumentOrder(t *testing.T) {
	// In the reference, sql.ident() and friends run while the template's
	// expressions are being evaluated — before the tag body executes at all. So a
	// failing helper always wins over a failing value conversion no matter where
	// each sits, and among helpers the leftmost wins. Getting this wrong means a
	// caller sees a different error message than production produces.
	_, err := Q("SELECT ?, ? FROM t", Ident("bad-one"), Ident("bad-two"))
	wantErr(t, err, "bad-one")

	// A helper error beats a value error even when the value comes first.
	_, err = Q("SELECT ?, ? FROM t", make(chan int), Ident("bad-two"))
	wantErr(t, err, "bad-two")

	_, err = Q("SELECT ?, ? FROM t", Ident("bad-one"), make(chan int))
	wantErr(t, err, "bad-one")

	// With no helper involved, the leftmost value error wins.
	_, err = Q("SELECT ?, ? FROM t", make(chan int), func() {})
	wantErr(t, err, "chan int")
}

func TestSQL_ValueAcceptsGoNumericTypes(t *testing.T) {
	for _, v := range []any{
		int(1), int8(1), int16(1), int32(1), int64(1),
		uint(1), uint8(1), uint16(1), uint32(1), uint64(1),
		float32(1.5), float64(1.5), true, "s",
	} {
		got, err := Value(v)
		wantNoErr(t, err)
		if got == nil {
			t.Errorf("Value(%T) returned nil", v)
		}
	}
}

func TestSQL_ValueDereferencesPointers(t *testing.T) {
	s := "x"
	got, err := Value(&s)
	wantNoErr(t, err)
	if got != "x" {
		t.Errorf("Value(*string) = %#v, want %q", got, "x")
	}

	var nilp *string
	got, err = Value(nilp)
	wantNoErr(t, err)
	if got != nil {
		t.Errorf("Value((*string)(nil)) = %#v, want nil", got)
	}
}

func TestSQL_DateYearIsNotZeroPadded(t *testing.T) {
	// String(date.getFullYear()) does not pad; Go's "2006" layout does. Verified
	// against the reference 2026-09-06: year 999 formats as "999-01-02 03:04:05".
	q, err := Q("?", time.Date(999, 1, 2, 3, 4, 5, 0, time.Local))
	wantNoErr(t, err)
	wantValues(t, q.Values(), "999-01-02 03:04:05")

	q, err = Q("?", time.Date(12345, 1, 2, 3, 4, 5, 0, time.Local))
	wantNoErr(t, err)
	wantValues(t, q.Values(), "12345-01-02 03:04:05")
}
