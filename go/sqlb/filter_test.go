// Port of src/filter.test.ts, case for case, in the reference's order.
package sqlb

import (
	"strings"
	"testing"
	"time"
)

// compile is the shorthand every case here uses.
func compile(t *testing.T, f *Filter) Fragment {
	t.Helper()
	got, err := CompileFilter(f)
	wantNoErr(t, err)
	return got
}

func compileErr(t *testing.T, f *Filter, substr string) {
	t.Helper()
	_, err := CompileFilter(f)
	wantErr(t, err, substr)
}

func TestFilter_BasicEquality(t *testing.T) {
	t.Run("should handle simple equality", func(t *testing.T) {
		r := compile(t, NewFilter().Eq("status", "ACTIVE"))
		wantText(t, r.Text(), `(("status" = ?))`)
		wantValues(t, r.Values(), "ACTIVE")
	})

	t.Run("should handle number equality", func(t *testing.T) {
		r := compile(t, NewFilter().Eq("age", 25))
		wantText(t, r.Text(), `(("age" = ?))`)
		wantValues(t, r.Values(), 25)
	})

	t.Run("should handle boolean equality", func(t *testing.T) {
		r := compile(t, NewFilter().Eq("active", true))
		wantText(t, r.Text(), `(("active" = ?))`)
		wantValues(t, r.Values(), true)
	})

	t.Run("should handle null equality with IS NULL", func(t *testing.T) {
		r := compile(t, NewFilter().Eq("deleted_at", nil))
		wantText(t, r.Text(), `(("deleted_at" IS NULL))`)
		wantValues(t, r.Values())
	})

	t.Run("should handle undefined by filtering it out", func(t *testing.T) {
		compileErr(t, NewFilter().Eq("deleted_at", Undefined), "Filter must contain at least one condition")
	})

	t.Run("should handle Date values", func(t *testing.T) {
		r := compile(t, NewFilter().Eq("created_at", time.Date(2023, 12, 25, 10, 30, 0, 0, time.UTC)))
		wantText(t, r.Text(), `(("created_at" = ?))`)
		if len(r.Values()) != 1 {
			t.Fatalf("values = %#v", r.Values())
		}
		if _, ok := r.Values()[0].(string); !ok {
			t.Errorf("value = %#v, want a string", r.Values()[0])
		}
	})
}

func TestFilter_Inequality(t *testing.T) {
	t.Run("should handle not equal operator", func(t *testing.T) {
		r := compile(t, NewFilter().Where("status", NewOps().Ne("DELETED")))
		wantText(t, r.Text(), `(("status" <> ?))`)
		wantValues(t, r.Values(), "DELETED")
	})

	t.Run("should handle not equal with null using IS NOT NULL", func(t *testing.T) {
		r := compile(t, NewFilter().Where("deleted_at", NewOps().Ne(nil)))
		wantText(t, r.Text(), `(("deleted_at" IS NOT NULL))`)
		wantValues(t, r.Values())
	})
}

func TestFilter_Comparison(t *testing.T) {
	t.Run("should handle greater than", func(t *testing.T) {
		r := compile(t, NewFilter().Where("age", NewOps().Gt(18)))
		wantText(t, r.Text(), `(("age" > ?))`)
		wantValues(t, r.Values(), 18)
	})

	t.Run("should handle greater than or equal", func(t *testing.T) {
		r := compile(t, NewFilter().Where("age", NewOps().Gte(21)))
		wantText(t, r.Text(), `(("age" >= ?))`)
		wantValues(t, r.Values(), 21)
	})

	t.Run("should handle less than", func(t *testing.T) {
		r := compile(t, NewFilter().Where("age", NewOps().Lt(65)))
		wantText(t, r.Text(), `(("age" < ?))`)
		wantValues(t, r.Values(), 65)
	})

	t.Run("should handle less than or equal", func(t *testing.T) {
		r := compile(t, NewFilter().Where("age", NewOps().Lte(64)))
		wantText(t, r.Text(), `(("age" <= ?))`)
		wantValues(t, r.Values(), 64)
	})

	t.Run("should handle multiple comparison operators", func(t *testing.T) {
		r := compile(t, NewFilter().Where("age", NewOps().Gt(18).Lt(65)))
		wantText(t, r.Text(), `(("age" > ? AND "age" < ?))`)
		wantValues(t, r.Values(), 18, 65)
	})

	t.Run("should handle range with gte and lte", func(t *testing.T) {
		r := compile(t, NewFilter().Where("score", NewOps().Gte(80).Lte(100)))
		wantText(t, r.Text(), `(("score" >= ? AND "score" <= ?))`)
		wantValues(t, r.Values(), 80, 100)
	})
}

func TestFilter_SetMembership(t *testing.T) {
	t.Run("should handle IN operator", func(t *testing.T) {
		r := compile(t, NewFilter().Where("role", NewOps().In("ADMIN", "EDITOR")))
		wantText(t, r.Text(), `(("role" IN (?,?)))`)
		wantValues(t, r.Values(), "ADMIN", "EDITOR")
	})

	t.Run("should handle NOT IN operator", func(t *testing.T) {
		r := compile(t, NewFilter().Where("role", NewOps().Nin("GUEST", "BANNED")))
		wantText(t, r.Text(), `(("role" NOT IN (?,?)))`)
		wantValues(t, r.Values(), "GUEST", "BANNED")
	})

	t.Run("should handle IN with single value", func(t *testing.T) {
		r := compile(t, NewFilter().Where("status", NewOps().In("ACTIVE")))
		wantText(t, r.Text(), `(("status" IN (?)))`)
		wantValues(t, r.Values(), "ACTIVE")
	})

	t.Run("should handle IN with mixed types", func(t *testing.T) {
		r := compile(t, NewFilter().Where("priority", NewOps().In(1, 2, 3)))
		wantText(t, r.Text(), `(("priority" IN (?,?,?)))`)
		wantValues(t, r.Values(), 1, 2, 3)
	})
}

func TestFilter_NullChecks(t *testing.T) {
	t.Run("should handle exists true (IS NOT NULL)", func(t *testing.T) {
		r := compile(t, NewFilter().Where("email", NewOps().Exists(true)))
		wantText(t, r.Text(), `(("email" IS NOT NULL))`)
		wantValues(t, r.Values())
	})

	t.Run("should handle exists false (IS NULL)", func(t *testing.T) {
		r := compile(t, NewFilter().Where("deleted_at", NewOps().Exists(false)))
		wantText(t, r.Text(), `(("deleted_at" IS NULL))`)
		wantValues(t, r.Values())
	})

	t.Run("should prioritize exists over other operators", func(t *testing.T) {
		r := compile(t, NewFilter().Where("email", NewOps().Exists(true).Like("%@example.com")))
		wantText(t, r.Text(), `(("email" IS NOT NULL))`)
		wantValues(t, r.Values())
	})

	t.Run("exists wins even when it comes last", func(t *testing.T) {
		// 'exists' in operators is checked before the operator loop
		// (src/filter.ts:179), so position does not matter. Verified against the
		// reference 2026-09-06.
		r := compile(t, NewFilter().Where("email", NewOps().Gt(1).Exists(true)))
		wantText(t, r.Text(), `(("email" IS NOT NULL))`)
		wantValues(t, r.Values())
	})

	t.Run("exists is truthiness, not a boolean check", func(t *testing.T) {
		// operators.exists ? … : … — anything truthy means IS NOT NULL, anything
		// falsy (including a present-but-nil value) means IS NULL.
		r := compile(t, NewFilter().Where("a", NewOps().Set("exists", "no")))
		wantText(t, r.Text(), `(("a" IS NOT NULL))`)

		r = compile(t, NewFilter().Where("a", NewOps().Set("exists", 0)))
		wantText(t, r.Text(), `(("a" IS NULL))`)

		r = compile(t, NewFilter().Where("a", NewOps().Set("exists", nil)))
		wantText(t, r.Text(), `(("a" IS NULL))`)

		r = compile(t, NewFilter().Where("a", NewOps().Set("exists", Undefined)))
		wantText(t, r.Text(), `(("a" IS NULL))`)
	})
}

func TestFilter_Like(t *testing.T) {
	t.Run("should handle LIKE operator", func(t *testing.T) {
		r := compile(t, NewFilter().Where("username", NewOps().Like("john%")))
		wantText(t, r.Text(), `(("username" LIKE ?))`)
		wantValues(t, r.Values(), "john%")
	})

	t.Run("should handle ILIKE operator (case-insensitive)", func(t *testing.T) {
		r := compile(t, NewFilter().Where("username", NewOps().ILike("%doe%")))
		wantText(t, r.Text(), `(("username" LIKE ? COLLATE NOCASE))`)
		wantValues(t, r.Values(), "%doe%")
	})

	t.Run("should handle LIKE with special characters", func(t *testing.T) {
		r := compile(t, NewFilter().Where("description", NewOps().Like("test_file%")))
		wantText(t, r.Text(), `(("description" LIKE ?))`)
		wantValues(t, r.Values(), "test_file%")
	})
}

func TestFilter_Regex(t *testing.T) {
	t.Run("should handle regex operator", func(t *testing.T) {
		r := compile(t, NewFilter().Where("email", NewOps().Regex(`^[A-Za-z0-9._%+-]+@example\.com$`)))
		wantText(t, r.Text(), `(("email" REGEXP ?))`)
		wantValues(t, r.Values(), `^[A-Za-z0-9._%+-]+@example\.com$`)
	})

	t.Run("should handle simple regex patterns", func(t *testing.T) {
		r := compile(t, NewFilter().Where("phone", NewOps().Regex(`^\+1`)))
		wantText(t, r.Text(), `(("phone" REGEXP ?))`)
		wantValues(t, r.Values(), `^\+1`)
	})
}

func TestFilter_Logical(t *testing.T) {
	t.Run("should handle AND operator", func(t *testing.T) {
		r := compile(t, NewFilter().And(
			NewFilter().Eq("status", "ACTIVE"),
			NewFilter().Where("age", NewOps().Gte(18)),
		))
		wantText(t, r.Text(), `((("status" = ?) AND ("age" >= ?)))`)
		wantValues(t, r.Values(), "ACTIVE", 18)
	})

	t.Run("should handle OR operator", func(t *testing.T) {
		r := compile(t, NewFilter().Or(
			NewFilter().Where("age", NewOps().Lt(18)),
			NewFilter().Where("age", NewOps().Gte(65)),
		))
		wantText(t, r.Text(), `((("age" < ?) OR ("age" >= ?)))`)
		wantValues(t, r.Values(), 18, 65)
	})

	t.Run("should handle mixed AND and OR", func(t *testing.T) {
		r := compile(t, NewFilter().And(
			NewFilter().Eq("status", "ACTIVE"),
			NewFilter().Or(
				NewFilter().Where("age", NewOps().Lt(18)),
				NewFilter().Where("age", NewOps().Gte(65)),
			),
		))
		wantText(t, r.Text(), `((("status" = ?) AND (("age" < ?) OR ("age" >= ?))))`)
		wantValues(t, r.Values(), "ACTIVE", 18, 65)
	})

	t.Run("should handle implicit AND with multiple fields", func(t *testing.T) {
		r := compile(t, NewFilter().
			Eq("status", "ACTIVE").
			Where("age", NewOps().Gte(18)).
			Where("country", NewOps().In("US", "CA")))
		wantText(t, r.Text(), `((("status" = ?) AND ("age" >= ?) AND ("country" IN (?,?))))`)
		wantValues(t, r.Values(), "ACTIVE", 18, "US", "CA")
	})

	t.Run("and is emitted before or regardless of key order", func(t *testing.T) {
		// Both logical keys are hoisted, and `and` is handled first
		// (src/filter.ts:314-350). Verified against the reference 2026-09-06.
		r := compile(t, NewFilter().
			Or(NewFilter().Eq("a", 1)).
			And(NewFilter().Eq("b", 2)))
		wantText(t, r.Text(), `(((("b" = ?)) AND (("a" = ?))))`)
		wantValues(t, r.Values(), 2, 1)
	})
}

func TestFilter_JSONPath(t *testing.T) {
	t.Run("should handle JSON path equality", func(t *testing.T) {
		r := compile(t, NewFilter().Eq("profile.address.city", "Paris"))
		wantText(t, r.Text(), `((json_extract("profile", ?) = ?))`)
		wantValues(t, r.Values(), "$.address.city", "Paris")
	})

	t.Run("should handle JSON path with null", func(t *testing.T) {
		r := compile(t, NewFilter().Eq("metadata.deleted", nil))
		wantText(t, r.Text(), `((json_extract("metadata", ?) IS NULL))`)
		wantValues(t, r.Values(), "$.deleted")
	})

	t.Run("should handle JSON path with comparison operators", func(t *testing.T) {
		r := compile(t, NewFilter().Where("profile.age", NewOps().Gte(21)))
		wantText(t, r.Text(), `((json_extract("profile", ?) >= ?))`)
		wantValues(t, r.Values(), "$.age", 21)
	})

	t.Run("should handle JSON path with IN operator", func(t *testing.T) {
		r := compile(t, NewFilter().Where("settings.theme", NewOps().In("dark", "light")))
		wantText(t, r.Text(), `((json_extract("settings", ?) IN (?,?)))`)
		wantValues(t, r.Values(), "$.theme", "dark", "light")
	})

	t.Run("should handle deep JSON paths", func(t *testing.T) {
		r := compile(t, NewFilter().Where("profile.social.twitter.followers", NewOps().Gt(1000)))
		wantText(t, r.Text(), `((json_extract("profile", ?) > ?))`)
		wantValues(t, r.Values(), "$.social.twitter.followers", 1000)
	})

	t.Run("should handle JSON path with exists operator", func(t *testing.T) {
		r := compile(t, NewFilter().Where("profile.email", NewOps().Exists(false)))
		wantText(t, r.Text(), `((json_extract("profile", ?) IS NULL))`)
		wantValues(t, r.Values(), "$.email")
	})
}

func TestFilter_ComplexNested(t *testing.T) {
	t.Run("kitchen sink A - Active users with age restrictions by country", func(t *testing.T) {
		r := compile(t, NewFilter().And(
			NewFilter().Eq("status", "ACTIVE"),
			NewFilter().Or(
				NewFilter().Where("age", NewOps().Lt(18)),
				NewFilter().Where("age", NewOps().Gte(65)),
			),
			NewFilter().Where("country", NewOps().In("US", "CA", "GB")),
		))
		wantText(t, r.Text(),
			`((("status" = ?) AND (("age" < ?) OR ("age" >= ?)) AND ("country" IN (?,?,?))))`)
		wantValues(t, r.Values(), "ACTIVE", 18, 65, "US", "CA", "GB")
	})

	t.Run("kitchen sink B - Company filtering with priority tagging", func(t *testing.T) {
		r := compile(t, NewFilter().
			Where("name", NewOps().Like("Acme%")).
			Where("category", NewOps().Nin("ARCHIVED", "DELETED")).
			Where("deleted_at", NewOps().Exists(false)).
			Or(
				NewFilter().Where("tags", NewOps().ILike("%urgent%")),
				NewFilter().Where("priority", NewOps().Gte(8)),
			))
		wantText(t, r.Text(),
			`(((("tags" LIKE ? COLLATE NOCASE) OR ("priority" >= ?)) AND ("name" LIKE ?) AND ("category" NOT IN (?,?)) AND ("deleted_at" IS NULL)))`)
		wantValues(t, r.Values(), "%urgent%", 8, "Acme%", "ARCHIVED", "DELETED")
	})

	t.Run("kitchen sink C - User signup analysis with JSON path queries", func(t *testing.T) {
		r := compile(t, NewFilter().And(
			NewFilter().Where("profile.email", NewOps().Regex(`.*@example\.org$`)),
			NewFilter().Or(
				NewFilter().Eq("signup_source", "REFERRAL"),
				NewFilter().
					Eq("signup_source", "SOCIAL").
					Where("profile.social.followers", NewOps().Gt(10000)),
			),
			NewFilter().Where("age", NewOps().Gte(18).Lte(30)),
		))
		wantText(t, r.Text(),
			`(((json_extract("profile", ?) REGEXP ?) AND (("signup_source" = ?) OR (("signup_source" = ?) AND (json_extract("profile", ?) > ?))) AND ("age" >= ? AND "age" <= ?)))`)
		wantValues(t, r.Values(),
			"$.email", `.*@example\.org$`, "REFERRAL", "SOCIAL", "$.social.followers", 10000, 18, 30)
	})
}

func TestFilter_DeeplyNestedLogical(t *testing.T) {
	t.Run("should handle OR within AND within OR", func(t *testing.T) {
		r := compile(t, NewFilter().Or(
			NewFilter().And(
				NewFilter().Eq("category", "electronics"),
				NewFilter().Or(NewFilter().Eq("brand", "Apple"), NewFilter().Eq("brand", "Samsung")),
				NewFilter().Where("price", NewOps().Lte(1000)),
			),
			NewFilter().And(
				NewFilter().Eq("category", "books"),
				NewFilter().Or(NewFilter().Eq("genre", "fiction"), NewFilter().Eq("genre", "mystery")),
				NewFilter().Where("rating", NewOps().Gte(4)),
			),
		))
		wantText(t, r.Text(),
			`(((("category" = ?) AND (("brand" = ?) OR ("brand" = ?)) AND ("price" <= ?)) OR (("category" = ?) AND (("genre" = ?) OR ("genre" = ?)) AND ("rating" >= ?))))`)
		wantValues(t, r.Values(), "electronics", "Apple", "Samsung", 1000, "books", "fiction", "mystery", 4)
	})

	t.Run("should handle AND within OR within AND", func(t *testing.T) {
		r := compile(t, NewFilter().And(
			NewFilter().Eq("status", "ACTIVE"),
			NewFilter().Or(
				NewFilter().And(
					NewFilter().Eq("department", "engineering"),
					NewFilter().Where("level", NewOps().Gte(5)),
					NewFilter().Where("skills", NewOps().Like("%javascript%")),
				),
				NewFilter().And(
					NewFilter().Eq("department", "design"),
					NewFilter().Where("level", NewOps().Gte(3)),
					NewFilter().Where("portfolio", NewOps().Exists(true)),
				),
			),
			NewFilter().Where("location", NewOps().In("SF", "NYC", "LA")),
		))
		wantText(t, r.Text(),
			`((("status" = ?) AND ((("department" = ?) AND ("level" >= ?) AND ("skills" LIKE ?)) OR (("department" = ?) AND ("level" >= ?) AND ("portfolio" IS NOT NULL))) AND ("location" IN (?,?,?))))`)
		wantValues(t, r.Values(), "ACTIVE", "engineering", 5, "%javascript%", "design", 3, "SF", "NYC", "LA")
	})

	t.Run("should handle triple-nested conditions", func(t *testing.T) {
		r := compile(t, NewFilter().And(
			NewFilter().Eq("user.status", "ACTIVE"),
			NewFilter().Or(
				NewFilter().And(
					NewFilter().Eq("user.type", "premium"),
					NewFilter().Or(
						NewFilter().Eq("billing.plan", "annual"),
						NewFilter().Where("billing.credits", NewOps().Gte(100)),
					),
				),
				NewFilter().And(
					NewFilter().Eq("user.type", "trial"),
					NewFilter().Where("user.trial_days_left", NewOps().Gt(0)),
					NewFilter().Or(
						NewFilter().Where("user.referral_code", NewOps().Exists(true)),
						NewFilter().Where("user.email", NewOps().Regex(`.*@company\.com$`)),
					),
				),
			),
		))
		wantText(t, r.Text(),
			`(((json_extract("user", ?) = ?) AND (((json_extract("user", ?) = ?) AND ((json_extract("billing", ?) = ?) OR (json_extract("billing", ?) >= ?))) OR ((json_extract("user", ?) = ?) AND (json_extract("user", ?) > ?) AND ((json_extract("user", ?) IS NOT NULL) OR (json_extract("user", ?) REGEXP ?))))))`)
		wantValues(t, r.Values(),
			"$.status", "ACTIVE", "$.type", "premium", "$.plan", "annual", "$.credits", 100,
			"$.type", "trial", "$.trial_days_left", 0, "$.referral_code", "$.email", `.*@company\.com$`)
	})

	t.Run("should handle complex real-world user permissions filter", func(t *testing.T) {
		r := compile(t, NewFilter().And(
			NewFilter().Eq("user.active", true),
			NewFilter().Or(
				NewFilter().Eq("user.role", "ADMIN"),
				NewFilter().And(
					NewFilter().Eq("user.role", "MANAGER"),
					NewFilter().Where("resource.department", NewOps().In("${user.department}")),
				),
				NewFilter().And(
					NewFilter().Eq("user.role", "USER"),
					NewFilter().Or(
						NewFilter().Eq("resource.owner_id", "${user.id}"),
						NewFilter().And(
							NewFilter().Eq("resource.shared", true),
							NewFilter().Where("resource.visibility", NewOps().In("public", "team")),
						),
						NewFilter().And(
							NewFilter().Where("resource.team_id", NewOps().Exists(true)),
							NewFilter().Where("user.team_memberships", NewOps().Like("%${resource.team_id}%")),
						),
					),
				),
			),
		))
		wantText(t, r.Text(),
			`(((json_extract("user", ?) = ?) AND ((json_extract("user", ?) = ?) OR ((json_extract("user", ?) = ?) AND (json_extract("resource", ?) IN (?))) OR ((json_extract("user", ?) = ?) AND ((json_extract("resource", ?) = ?) OR ((json_extract("resource", ?) = ?) AND (json_extract("resource", ?) IN (?,?))) OR ((json_extract("resource", ?) IS NOT NULL) AND (json_extract("user", ?) LIKE ?)))))))`)
		wantValues(t, r.Values(),
			"$.active", true, "$.role", "ADMIN", "$.role", "MANAGER", "$.department",
			"${user.department}", "$.role", "USER", "$.owner_id", "${user.id}", "$.shared",
			true, "$.visibility", "public", "team", "$.team_id", "$.team_memberships",
			"%${resource.team_id}%")
	})

	t.Run("should handle e-commerce product search filter", func(t *testing.T) {
		r := compile(t, NewFilter().And(
			NewFilter().Eq("available", true),
			NewFilter().Where("price", NewOps().Gte(10).Lte(500)),
			NewFilter().Or(
				NewFilter().And(
					NewFilter().Eq("category", "electronics"),
					NewFilter().Or(
						NewFilter().Where("brand", NewOps().In("Apple", "Samsung", "Google")),
						NewFilter().And(
							NewFilter().Eq("brand", "Other"),
							NewFilter().Where("rating", NewOps().Gte(4.5)),
							NewFilter().Where("reviews_count", NewOps().Gte(100)),
						),
					),
				),
				NewFilter().And(
					NewFilter().Eq("category", "clothing"),
					NewFilter().Or(
						NewFilter().And(
							NewFilter().Eq("attributes.gender", "unisex"),
							NewFilter().Where("attributes.size", NewOps().In("S", "M", "L")),
						),
						NewFilter().And(
							NewFilter().Where("attributes.style", NewOps().Like("%casual%")),
							NewFilter().Where("discount_percentage", NewOps().Gte(20)),
						),
					),
				),
				NewFilter().And(
					NewFilter().Eq("category", "books"),
					NewFilter().Or(
						NewFilter().Where("genre", NewOps().In("fiction", "mystery", "sci-fi")),
						NewFilter().And(
							NewFilter().Eq("genre", "non-fiction"),
							NewFilter().Eq("metadata.bestseller", true),
						),
					),
					NewFilter().Where("language", NewOps().In("english", "spanish")),
				),
			),
		))
		wantText(t, r.Text(),
			`((("available" = ?) AND ("price" >= ? AND "price" <= ?) AND ((("category" = ?) AND (("brand" IN (?,?,?)) OR (("brand" = ?) AND ("rating" >= ?) AND ("reviews_count" >= ?)))) OR (("category" = ?) AND (((json_extract("attributes", ?) = ?) AND (json_extract("attributes", ?) IN (?,?,?))) OR ((json_extract("attributes", ?) LIKE ?) AND ("discount_percentage" >= ?)))) OR (("category" = ?) AND (("genre" IN (?,?,?)) OR (("genre" = ?) AND (json_extract("metadata", ?) = ?))) AND ("language" IN (?,?))))))`)
		wantValues(t, r.Values(),
			true, 10, 500, "electronics", "Apple", "Samsung", "Google", "Other", 4.5, 100,
			"clothing", "$.gender", "unisex", "$.size", "S", "M", "L", "$.style", "%casual%", 20,
			"books", "fiction", "mystery", "sci-fi", "non-fiction", "$.bestseller", true,
			"english", "spanish")
	})

	t.Run("should handle maximum nesting depth without exceeding limits", func(t *testing.T) {
		r := compile(t, NewFilter().And(
			NewFilter().Eq("level1", "value1"),
			NewFilter().Or(
				NewFilter().Eq("level2a", "value2a"),
				NewFilter().And(
					NewFilter().Eq("level3a", "value3a"),
					NewFilter().Or(
						NewFilter().Eq("level4a", "value4a"),
						NewFilter().And(
							NewFilter().Eq("level5a", "value5a"),
							NewFilter().Or(
								NewFilter().Eq("level6a", "value6a"),
								NewFilter().And(
									NewFilter().Eq("level7a", "value7a"),
									NewFilter().Eq("level8a", "value8a"),
								),
							),
						),
					),
				),
			),
		))
		wantText(t, r.Text(),
			`((("level1" = ?) AND (("level2a" = ?) OR (("level3a" = ?) AND (("level4a" = ?) OR (("level5a" = ?) AND (("level6a" = ?) OR (("level7a" = ?) AND ("level8a" = ?)))))))))`)
		wantValues(t, r.Values(),
			"value1", "value2a", "value3a", "value4a", "value5a", "value6a", "value7a", "value8a")
	})
}

func TestFilter_ErrorHandling(t *testing.T) {
	t.Run("should throw on unknown operators", func(t *testing.T) {
		compileErr(t, NewFilter().Where("age", NewOps().Set("unknown", 18)), "Unknown operator: unknown")
	})

	t.Run("should throw on empty AND array", func(t *testing.T) {
		compileErr(t, NewFilter().And(), "AND operator cannot be used with empty arrays")
	})

	t.Run("should throw on empty OR array", func(t *testing.T) {
		compileErr(t, NewFilter().Or(), "OR operator cannot be used with empty arrays")
	})

	t.Run("should throw on empty IN array", func(t *testing.T) {
		compileErr(t, NewFilter().Where("status", NewOps().In()), "IN operator cannot be used with empty arrays")
	})

	t.Run("should throw on empty NIN array", func(t *testing.T) {
		compileErr(t, NewFilter().Where("status", NewOps().Nin()), "NIN operator cannot be used with empty arrays")
	})

	t.Run("should throw on non-array IN value", func(t *testing.T) {
		compileErr(t, NewFilter().Where("status", NewOps().Set("in", "ACTIVE")), "IN operator requires an array")
	})

	t.Run("should throw on non-array NIN value", func(t *testing.T) {
		compileErr(t, NewFilter().Where("status", NewOps().Set("nin", "ACTIVE")), "NIN operator requires an array")
	})

	t.Run("should throw on non-string LIKE value", func(t *testing.T) {
		compileErr(t, NewFilter().Where("name", NewOps().Set("like", 123)), "LIKE operator requires a string pattern")
	})

	t.Run("should throw on non-string ILIKE value", func(t *testing.T) {
		compileErr(t, NewFilter().Where("name", NewOps().Set("ilike", 123)), "ILIKE operator requires a string pattern")
	})

	t.Run("should throw on non-string REGEX value", func(t *testing.T) {
		compileErr(t, NewFilter().Where("email", NewOps().Set("regex", 123)), "REGEX operator requires a string pattern")
	})

	t.Run("should throw on invalid JSON path", func(t *testing.T) {
		compileErr(t, NewFilter().Eq("field.", "value"), "Invalid JSON path: field.")
	})

	t.Run("should throw on array field values", func(t *testing.T) {
		compileErr(t, NewFilter().Set("status", []any{"ACTIVE", "INACTIVE"}),
			"Field 'status' cannot have array value")
	})

	t.Run("should throw on empty filter object", func(t *testing.T) {
		compileErr(t, NewFilter(), "Filter must contain at least one condition")
	})

	t.Run("should throw on null filter", func(t *testing.T) {
		compileErr(t, nil, "Filter must be an object")
	})

	// TYPE-ENFORCED. CompileFilter takes a *Filter; there is no non-object to
	// pass. The nil case above covers what remains of the reference's check.
	t.Run("should throw on non-object filter (TYPE-ENFORCED)", func(t *testing.T) {
		compileErr(t, nil, "Filter must be an object")
	})

	t.Run("should throw on operator object with no valid operators", func(t *testing.T) {
		compileErr(t, NewFilter().Where("age", NewOps().Set("invalid", true)), "Unknown operator: invalid")
	})

	t.Run("should throw on large IN arrays", func(t *testing.T) {
		large := make([]any, 1000)
		for i := range large {
			large[i] = "value"
		}
		compileErr(t, NewFilter().Where("status", NewOps().In(large...)),
			"IN operator cannot be used with arrays larger than 999 items")
	})

	t.Run("should throw on large NIN arrays", func(t *testing.T) {
		large := make([]any, 1000)
		for i := range large {
			large[i] = "value"
		}
		compileErr(t, NewFilter().Where("status", NewOps().Nin(large...)),
			"NIN operator cannot be used with arrays larger than 999 items")
	})

	t.Run("an empty operator object is rejected", func(t *testing.T) {
		compileErr(t, NewFilter().Where("a", NewOps()),
			"Operator object must contain at least one valid operator")
	})
}

func TestFilter_DoSProtection(t *testing.T) {
	t.Run("should throw on excessive nesting depth", func(t *testing.T) {
		f := NewFilter().Eq("status", "ACTIVE")
		for range 12 {
			f = NewFilter().And(f)
		}
		compileErr(t, f, "Nesting depth too deep (max: 10)")
	})

	t.Run("nine levels of nesting are accepted, ten are not", func(t *testing.T) {
		// Verified against the reference 2026-09-06.
		nest := func(n int) *Filter {
			f := NewFilter().Eq("status", "ACTIVE")
			for range n {
				f = NewFilter().And(f)
			}
			return f
		}
		compile(t, nest(9))
		compileErr(t, nest(10), "Nesting depth too deep")
	})

	t.Run("should throw on too many operators with smaller test", func(t *testing.T) {
		subs := make([]*Filter, 60)
		for i := range subs {
			subs[i] = NewFilter().Where("field"+strconvItoa(i), NewOps().Gt(i).Lt(i+10))
		}
		compileErr(t, NewFilter().And(subs...), "Too many operators (max: 100)")
	})

	t.Run("should count operators correctly across nested structures", func(t *testing.T) {
		compile(t, NewFilter().And(
			NewFilter().Where("field1", NewOps().Gt(1).Lt(10)),
			NewFilter().Where("field2", NewOps().In(1, 2, 3)),
			NewFilter().Or(
				NewFilter().Where("field3", NewOps().Like("%test%")),
				NewFilter().Where("field4", NewOps().Exists(true)),
			),
		))
	})

	t.Run("exactly 100 operators pass and 101 do not", func(t *testing.T) {
		// Verified against the reference 2026-09-06.
		build := func(n int) *Filter {
			subs := make([]*Filter, n)
			for i := range subs {
				subs[i] = NewFilter().Eq("f"+strconvItoa(i), 1)
			}
			return NewFilter().And(subs...)
		}
		compile(t, build(99))
		compileErr(t, build(100), "Too many operators (max: 100)")
	})
}

func TestFilter_ReturnValueFormat(t *testing.T) {
	// TYPE-ENFORCED: see the note on Fragment in sql_test.go.
	t.Run("should return frozen objects (TYPE-ENFORCED)", func(t *testing.T) {
		r := compile(t, NewFilter().Eq("status", "ACTIVE"))
		wantText(t, r.Text(), `(("status" = ?))`)
	})

	t.Run("should always wrap result in parentheses", func(t *testing.T) {
		r := compile(t, NewFilter().Eq("status", "ACTIVE"))
		if !strings.HasPrefix(r.Text(), "(") || !strings.HasSuffix(r.Text(), ")") {
			t.Errorf("text %q is not wrapped in parentheses", r.Text())
		}
	})

	t.Run("should have matching text placeholders and values length", func(t *testing.T) {
		r := compile(t, NewFilter().
			Where("name", NewOps().Like("John%")).
			Where("age", NewOps().Gte(18).Lt(65)).
			Where("role", NewOps().In("ADMIN", "USER")))
		if got, want := countByte(r.Text(), '?'), len(r.Values()); got != want {
			t.Errorf("placeholders = %d, values = %d", got, want)
		}
	})
}

func TestFilter_IdentifierValidation(t *testing.T) {
	t.Run("should handle valid simple identifiers", func(t *testing.T) {
		r := compile(t, NewFilter().Eq("user_name", "john"))
		wantText(t, r.Text(), `(("user_name" = ?))`)
		wantValues(t, r.Values(), "john")
	})

	t.Run("should handle valid qualified identifiers in JSON paths", func(t *testing.T) {
		r := compile(t, NewFilter().Eq("user_profile.contact_info.email", "test@example.com"))
		wantText(t, r.Text(), `((json_extract("user_profile", ?) = ?))`)
		wantValues(t, r.Values(), "$.contact_info.email", "test@example.com")
	})

	t.Run("should reject malicious field names through ident validation", func(t *testing.T) {
		compileErr(t, NewFilter().Eq("users; DROP TABLE users; --", "value"), "Invalid identifier")
	})
}

func TestFilter_AliasBlocks(t *testing.T) {
	t.Run("should handle simple alias block with equality", func(t *testing.T) {
		r := compile(t, NewFilter().
			Eq("status", "ACTIVE").
			Alias("t2", NewFilter().Where("column_in_table_2", NewOps().Gte(100))))
		wantText(t, r.Text(), `((("status" = ?) AND (t2."column_in_table_2" >= ?)))`)
		wantValues(t, r.Values(), "ACTIVE", 100)
	})

	t.Run("should handle multiple alias blocks", func(t *testing.T) {
		r := compile(t, NewFilter().
			Eq("status", "ACTIVE").
			Alias("t2", NewFilter().Where("column_in_table_2", NewOps().Gte(100))).
			Alias("orders", NewFilter().Where("amount", NewOps().Gt(500))))
		wantText(t, r.Text(),
			`((("status" = ?) AND (t2."column_in_table_2" >= ?) AND (orders."amount" > ?)))`)
		wantValues(t, r.Values(), "ACTIVE", 100, 500)
	})

	t.Run("should handle alias block with JSON path", func(t *testing.T) {
		r := compile(t, NewFilter().
			Eq("status", "ACTIVE").
			Alias("t2", NewFilter().Where("stats.avg", NewOps().Lt(10))))
		wantText(t, r.Text(), `((("status" = ?) AND (json_extract(t2."stats", ?) < ?)))`)
		wantValues(t, r.Values(), "ACTIVE", "$.avg", 10)
	})

	t.Run("should handle alias block with complex conditions", func(t *testing.T) {
		r := compile(t, NewFilter().Alias("users", NewFilter().
			Where("age", NewOps().Gte(18).Lt(65)).
			Where("country", NewOps().In("US", "CA")).
			Eq("profile.verified", true)))
		wantText(t, r.Text(),
			`(((users."age" >= ? AND users."age" < ?) AND (users."country" IN (?,?)) AND (json_extract(users."profile", ?) = ?)))`)
		wantValues(t, r.Values(), 18, 65, "US", "CA", "$.verified", true)
	})

	t.Run("should handle alias block with logical operators", func(t *testing.T) {
		r := compile(t, NewFilter().Alias("orders", NewFilter().Or(
			NewFilter().Eq("status", "pending"),
			NewFilter().Where("priority", NewOps().Gte(8)),
		)))
		wantText(t, r.Text(), `(((orders."status" = ?) OR (orders."priority" >= ?)))`)
		wantValues(t, r.Values(), "pending", 8)
	})

	t.Run("should handle nested logical operators within alias blocks", func(t *testing.T) {
		r := compile(t, NewFilter().
			Eq("status", "ACTIVE").
			Alias("products", NewFilter().And(
				NewFilter().Eq("category", "electronics"),
				NewFilter().Or(NewFilter().Eq("brand", "Apple"), NewFilter().Eq("brand", "Samsung")),
			)))
		wantText(t, r.Text(),
			`((("status" = ?) AND ((products."category" = ?) AND ((products."brand" = ?) OR (products."brand" = ?)))))`)
		wantValues(t, r.Values(), "ACTIVE", "electronics", "Apple", "Samsung")
	})

	t.Run("should handle example from specification", func(t *testing.T) {
		r := compile(t, NewFilter().
			Eq("status", "ACTIVE").
			Where("age", NewOps().Gte(18).Lt(65)).
			Alias("t2", NewFilter().
				Where("column_in_table_2", NewOps().Gte(100)).
				Where("stats.avg", NewOps().Lt(10))).
			Alias("orders", NewFilter().Where("amount", NewOps().Gt(500))))
		wantText(t, r.Text(),
			`((("status" = ?) AND ("age" >= ? AND "age" < ?) AND ((t2."column_in_table_2" >= ?) AND (json_extract(t2."stats", ?) < ?)) AND (orders."amount" > ?)))`)
		wantValues(t, r.Values(), "ACTIVE", 18, 65, 100, "$.avg", 10, 500)
	})

	t.Run("should handle alias with null values", func(t *testing.T) {
		r := compile(t, NewFilter().Alias("users", NewFilter().
			Eq("deleted_at", nil).
			Where("metadata.flag", NewOps().Exists(false))))
		wantText(t, r.Text(),
			`(((users."deleted_at" IS NULL) AND (json_extract(users."metadata", ?) IS NULL)))`)
		wantValues(t, r.Values(), "$.flag")
	})

	t.Run("should validate invalid alias identifiers", func(t *testing.T) {
		compileErr(t, NewFilter().Alias("123invalid", NewFilter().Eq("column", "value")),
			"Invalid alias identifier: 123invalid")
	})

	t.Run("should validate alias with special characters", func(t *testing.T) {
		compileErr(t, NewFilter().Alias("invalid-alias", NewFilter().Eq("column", "value")),
			"Invalid alias identifier: invalid-alias")
	})

	t.Run("should validate alias with dots", func(t *testing.T) {
		compileErr(t, NewFilter().Alias("table.alias", NewFilter().Eq("column", "value")),
			"Invalid alias identifier: table.alias")
	})

	t.Run("should allow valid alias with underscores", func(t *testing.T) {
		r := compile(t, NewFilter().Alias("table_alias_123", NewFilter().Eq("column", "value")))
		wantText(t, r.Text(), `((table_alias_123."column" = ?))`)
		wantValues(t, r.Values(), "value")
	})

	t.Run("should allow alias starting with underscore", func(t *testing.T) {
		r := compile(t, NewFilter().Alias("_table", NewFilter().Eq("column", "value")))
		wantText(t, r.Text(), `((_table."column" = ?))`)
		wantValues(t, r.Values(), "value")
	})

	t.Run("should ensure plain filters still work unchanged", func(t *testing.T) {
		r := compile(t, NewFilter().
			Eq("status", "ACTIVE").
			Where("age", NewOps().Gte(18).Lt(65)).
			Eq("profile.verified", true).
			Or(
				NewFilter().Where("priority", NewOps().Gte(8)),
				NewFilter().Eq("category", "urgent"),
			))
		wantText(t, r.Text(),
			`(((("priority" >= ?) OR ("category" = ?)) AND ("status" = ?) AND ("age" >= ? AND "age" < ?) AND (json_extract("profile", ?) = ?)))`)
		wantValues(t, r.Values(), 8, "urgent", "ACTIVE", 18, 65, "$.verified", true)
	})

	t.Run("should handle empty alias block gracefully", func(t *testing.T) {
		compileErr(t, NewFilter().Eq("status", "ACTIVE").Alias("empty", NewFilter()),
			"Filter must contain at least one condition")
	})

	t.Run("should ignore undefined alias blocks", func(t *testing.T) {
		r := compile(t, NewFilter().Eq("status", "ACTIVE").Set("$ignored", Undefined))
		wantText(t, r.Text(), `(("status" = ?))`)
		wantValues(t, r.Values(), "ACTIVE")
	})

	t.Run("should ignore non-object alias blocks", func(t *testing.T) {
		r := compile(t, NewFilter().Eq("status", "ACTIVE").Set("$invalid", "not-an-object"))
		wantText(t, r.Text(), `(("status" = ?))`)
		wantValues(t, r.Values(), "ACTIVE")
	})

	t.Run("a nil alias block is ignored", func(t *testing.T) {
		// {$x: null} — value !== null fails, so the key falls out of both lists.
		// Verified against the reference 2026-09-06.
		r := compile(t, NewFilter().Set("$x", nil).Eq("y", 1))
		wantText(t, r.Text(), `(("y" = ?))`)
		wantValues(t, r.Values(), 1)
	})
}

// ------------------------------------------------ preserved reference bug ---

// TestFilter_JSONPathMultiOperatorMisalignment pins a defect in the reference,
// reproduced deliberately.
//
// For a JSON-path field with two or more comparison operators, the path is
// pushed onto the value list ONCE (src/filter.ts:174-176) but json_extract(col,
// ?) is rendered into EVERY clause (src/filter.ts:169-171). The result has more
// placeholders than values.
//
// Standalone, compileFilter() returns that misaligned fragment. Composed into a
// query the integrity check catches it, so the failure mode is a hard error
// rather than a wrong row set — but a caller binding the fragment directly gets
// the wrong query. Reported separately; NOT fixed here.
func TestFilter_JSONPathMultiOperatorMisalignment(t *testing.T) {
	r := compile(t, NewFilter().Where("a.b", NewOps().Gt(1).Lt(2)))
	wantText(t, r.Text(), `((json_extract("a", ?) > ? AND json_extract("a", ?) < ?))`)
	wantValues(t, r.Values(), "$.b", 1, 2)
	if got, want := countByte(r.Text(), '?'), len(r.Values()); got == want {
		t.Fatalf("the reference bug was fixed while porting: %d placeholders, %d values", got, want)
	}

	_, err := Q("SELECT * FROM t WHERE ?", r)
	wantErr(t, err, "Placeholder count (4) does not match bound value count (3)")
}
