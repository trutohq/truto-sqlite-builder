# 🏗️ truto-sqlite-builder

[![npm version](https://badge.fury.io/js/%40truto%2Fsqlite-builder.svg)](https://badge.fury.io/js/%40truto%2Fsqlite-builder)
[![CI](https://github.com/trutohq/truto-sqlite-builder/actions/workflows/ci.yml/badge.svg)](https://github.com/trutohq/truto-sqlite-builder/actions/workflows/ci.yml)
[![TypeScript](https://img.shields.io/badge/TypeScript-Ready-blue.svg)](https://www.typescriptlang.org/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

**Safe, zero-dependency template-literal tag for SQLite queries in any JS environment.**

`@truto/sqlite-builder` provides a secure and ergonomic way to build SQLite queries using tagged template literals. It prevents SQL injection attacks through parameterized queries while offering convenient helper functions for common SQL patterns.

## ✨ Features

- 🔒 **Injection-safe**: All values are parameterized, preventing SQL injection
- 🚫 **Defense in depth**: Multiple security layers including stacked query detection
- 🪶 **Zero dependencies**: Pure TypeScript/JavaScript with no runtime dependencies
- 🌍 **Universal**: Works in Bun, Node.js, Deno, and modern browsers
- 🎯 **TypeScript-first**: Full type safety with excellent IDE support
- 🔧 **Helper functions**: Built-in utilities for identifiers, IN clauses, and more
- 🔍 **JSON Filter Language**: MongoDB-style JSON filters for WHERE clauses
- 🔗 **Qualified Filters**: Table/alias scoping for complex JOINs using `$alias` blocks
- ⚡ **Lightweight**: Minimal bundle size with tree-shaking support

## 📦 Installation

```bash
bun add @truto/sqlite-builder
```

```bash
npm install @truto/sqlite-builder
```

```bash
yarn add @truto/sqlite-builder
```

```bash
pnpm add @truto/sqlite-builder
```

## 🚀 Quick Start

```typescript
import sqlite3 from 'better-sqlite3'
import { sql, compileFilter } from '@truto/sqlite-builder'

const db = new sqlite3('database.db')

// Simple query
const name = 'Alice'
const { text, values } = sql`SELECT * FROM users WHERE name = ${name}`
const users = db.prepare(text).all(...values)

// JSON Filter queries
const filter = {
  name: { like: 'John%' },
  age: { gte: 18, lt: 65 },
  or: [{ email: { regex: '.*@example.com$' } }, { phone: { exists: false } }],
}

const { text: whereText, values: whereValues } = compileFilter(filter)
const query = sql`
  SELECT * FROM users
  WHERE ${sql.raw(whereText)}
`

const results = db.prepare(query.text).all(...query.values)

// Qualified filters for JOINs with alias blocks
const joinFilter = {
  status: 'ACTIVE', // Main table
  $profiles: {
    // Profile table alias
    verified: true,
    'settings.theme': 'dark', // JSON path in profile
  },
  $orders: {
    // Orders table alias
    total: { gt: 100 },
  },
}

const joinWhere = compileFilter(joinFilter)
const joinQuery = sql`
  SELECT u.name, p.verified, o.total
  FROM users u
  JOIN profiles p ON u.id = p.user_id
  JOIN orders o ON u.id = o.user_id  
  WHERE ${sql.raw(joinWhere.text)}
`
```

## 📖 API Reference

### `sql` Tagged Template

The main function for building SQL queries.

```typescript
const query = sql`SELECT * FROM users WHERE id = ${userId}`
// Returns: { text: "SELECT * FROM users WHERE id = ?", values: [userId] }
```

**Parameters:**

- Template strings and interpolated values
- Returns a frozen `SqlQuery` object with `text` and `values` properties

### `sql.ident(identifier: string | readonly (string | SqlFragment)[])`

Safely quotes SQL identifiers (table names, column names, etc.). Accepts single identifiers, arrays of identifiers, or mixed arrays containing both identifiers and SQL fragments.

```typescript
// Single identifier
const table = 'users'
const query = sql`SELECT * FROM ${sql.ident(table)}`
// Returns: { text: 'SELECT * FROM "users"', values: [] }

// Qualified identifiers (table.column)
const qualifiedQuery = sql`SELECT ${sql.ident('u.name')}, ${sql.ident('u.email')} FROM users u`
// Returns: { text: 'SELECT "u"."name", "u"."email" FROM users u', values: [] }

// Array of identifiers (useful for column lists)
const columns = ['name', 'email', 'created_at']
const selectQuery = sql`SELECT ${sql.ident(columns)} FROM users`
// Returns: { text: 'SELECT "name", "email", "created_at" FROM users', values: [] }

// Mixed arrays with qualified and simple identifiers
const mixedColumns = ['u.id', 'name', 'u.email', 'p.title']
const joinQuery = sql`SELECT ${sql.ident(mixedColumns)} FROM users u JOIN posts p ON u.id = p.user_id`
// Returns: { text: 'SELECT "u"."id", "name", "u"."email", "p"."title" FROM users u JOIN posts p ON u.id = p.user_id', values: [] }

// Mixed arrays with identifiers and SQL fragments
const mixedColumns = ['id', 'name', sql.raw('UPPER(email) as email_upper')]
const mixedQuery = sql`SELECT ${sql.ident(mixedColumns)} FROM users`
// Returns: { text: 'SELECT "id", "name", UPPER(email) as email_upper FROM users', values: [] }

// Mixed arrays with parameterized fragments
const status = 'premium'
const dynamicColumns = [
  'id',
  'name',
  sql`CASE WHEN status = ${status} THEN 'Premium' ELSE 'Regular' END as user_type`,
]
const dynamicQuery = sql`SELECT ${sql.ident(dynamicColumns)} FROM users`
// Returns: { text: 'SELECT "id", "name", CASE WHEN status = ? THEN \'Premium\' ELSE \'Regular\' END as user_type FROM users', values: ['premium'] }

// In INSERT statements
const insertColumns = ['name', 'email', 'age']
const insertQuery = sql`
  INSERT INTO users (${sql.ident(insertColumns)}) 
  VALUES (${name}, ${email}, ${age})
`
// Returns: { text: 'INSERT INTO users ("name", "email", "age") VALUES (?, ?, ?)', values: [name, email, age] }
```

**Security:** Only accepts valid ANSI identifiers (simple: `name`, qualified: `table.column`) for string elements. SQL fragments are passed through as-is.

### `sql.in(array: readonly unknown[])`

Creates parameterized IN clauses from arrays.

```typescript
const ids = [1, 2, 3]
const query = sql`SELECT * FROM users WHERE id IN ${sql.in(ids)}`
// Returns: { text: "SELECT * FROM users WHERE id IN (?,?,?)", values: [1, 2, 3] }
```

**Features:**

- Rejects empty arrays (would create invalid SQL)
- Warns for arrays with >1000 items (performance consideration)

### `sql.raw(rawSql: string)`

Embeds raw SQL without parameterization. **⚠️ Use with extreme caution!**

```typescript
const query = sql`SELECT * FROM users WHERE created_at > ${sql.raw('datetime("now", "-1 day")')}`
// Returns: { text: 'SELECT * FROM users WHERE created_at > datetime("now", "-1 day")', values: [] }
```

**⚠️ Warning:** Never use `sql.raw()` with user input. Only use with trusted, static SQL fragments.

### `sql.join(fragments: SqlFragment[], separator?: string)`

Joins multiple SQL fragments with a separator.

```typescript
const conditions = [
  sql`name = ${'John'}`,
  sql`age = ${30}`,
  sql`active = ${true}`,
]

const query = sql`SELECT * FROM users WHERE ${sql.join(conditions, ' AND ')}`
// Returns: { text: "SELECT * FROM users WHERE name = ? AND age = ? AND active = ?", values: ['John', 30, true] }
```

## 🔍 JSON Filter Language

Build complex WHERE clauses using MongoDB-style JSON filters. Perfect for APIs and dynamic queries.

### `compileFilter(filter: JsonFilter): FilterResult`

Compiles a JSON filter object into a parameterized SQL WHERE clause.

```typescript
import { compileFilter } from '@truto/sqlite-builder'

const filter = {
  status: 'ACTIVE',
  age: { gte: 18, lt: 65 },
}

const result = compileFilter(filter)
// Returns: { text: '(("status" = ? AND "age" >= ? AND "age" < ?))', values: ['ACTIVE', 18, 65] }

// Use with the main sql template
const query = sql`
  SELECT * FROM users
  WHERE ${sql.raw(result.text)}
`
```

### Supported Operators

| Operator Family           | JSON Form                             | SQL Fragment                             | Description                         |
| ------------------------- | ------------------------------------- | ---------------------------------------- | ----------------------------------- |
| **Equality**              | `"field": value`                      | `"field" = ?`                            | Direct value comparison             |
| **Inequality**            | `"field": { "ne": value }`            | `"field" <> ?`                           | Not equal comparison                |
| **Comparison**            | `"field": { "gt": value }`            | `"field" > ?`                            | Greater than, gte, lt, lte          |
| **Set Membership**        | `"field": { "in": [1, 2, 3] }`        | `"field" IN (?,?,?)`                     | Value in array                      |
| **Negative Set**          | `"field": { "nin": [1, 2] }`          | `"field" NOT IN (?,?)`                   | Value not in array                  |
| **NULL Checks**           | `"field": { "exists": false }`        | `"field" IS NULL`                        | Check for NULL/NOT NULL             |
| **LIKE Patterns**         | `"field": { "like": "john%" }`        | `"field" LIKE ?`                         | Pattern matching                    |
| **Case-insensitive LIKE** | `"field": { "ilike": "%DOE%" }`       | `"field" LIKE ? COLLATE NOCASE`          | Case-insensitive patterns           |
| **Regular Expressions**   | `"field": { "regex": "^[A-Z]+" }`     | `"field" REGEXP ?`                       | Regex patterns (requires extension) |
| **Logical AND**           | `"and": [filter1, filter2]`           | `(filter1 AND filter2)`                  | All conditions must match           |
| **Logical OR**            | `"or": [filter1, filter2]`            | `(filter1 OR filter2)`                   | Any condition must match            |
| **JSON Path**             | `"profile.email": "test@example.com"` | `json_extract("profile", '$.email') = ?` | Query JSON column fields            |
| **Alias Blocks**          | `"$alias": { "field": value }`        | `alias."field" = ?`                      | Table/alias qualified fields        |

### Filter Examples

#### Basic Operations

```typescript
// Equality and comparison
const filter1 = {
  status: 'ACTIVE',
  age: { gte: 18, lt: 65 },
  score: { gt: 80, lte: 100 },
}
// SQL: (("status" = ? AND "age" >= ? AND "age" < ? AND "score" > ? AND "score" <= ?))

// Set membership
const filter2 = {
  role: { in: ['ADMIN', 'EDITOR'] },
  department: { nin: ['ARCHIVED', 'DELETED'] },
}
// SQL: (("role" IN (?,?) AND "department" NOT IN (?,?)))

// NULL checks
const filter3 = {
  email: { exists: true }, // IS NOT NULL
  deleted_at: { exists: false }, // IS NULL
}
// SQL: (("email" IS NOT NULL AND "deleted_at" IS NULL))
```

#### Pattern Matching

```typescript
// LIKE patterns
const filter4 = {
  username: { like: 'john%' }, // Starts with 'john'
  email: { ilike: '%@EXAMPLE.COM%' }, // Case-insensitive contains
}
// SQL: (("username" LIKE ? AND "email" LIKE ? COLLATE NOCASE))

// Regular expressions (requires REGEXP extension)
const filter5 = {
  email: { regex: '^[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\\.[A-Za-z]{2,}$' },
  phone: { regex: '^\\+1[0-9]{10}$' },
}
// SQL: (("email" REGEXP ? AND "phone" REGEXP ?))
```

#### Logical Operators

```typescript
// AND operator (explicit)
const filter6 = {
  and: [
    { status: 'ACTIVE' },
    { age: { gte: 18 } },
    { country: { in: ['US', 'CA', 'GB'] } },
  ],
}
// SQL: ((("status" = ?) AND ("age" >= ?) AND ("country" IN (?,?,?))))

// OR operator
const filter7 = {
  or: [
    { age: { lt: 18 } }, // Minors
    { age: { gte: 65 } }, // Seniors
  ],
}
// SQL: ((("age" < ?) OR ("age" >= ?)))

// Mixed AND/OR logic
const filter8 = {
  status: 'ACTIVE', // Implicit AND
  or: [{ tags: { ilike: '%urgent%' } }, { priority: { gte: 8 } }],
}
// SQL: (((("tags" LIKE ? COLLATE NOCASE) OR ("priority" >= ?)) AND ("status" = ?)))
```

#### JSON Path Querying

For SQLite JSON columns, use dot notation to query nested fields:

```typescript
// Query JSON fields
const filter9 = {
  'profile.email': { regex: '.*@example\\.org$' },
  'profile.age': { gte: 21 },
  'settings.theme': { in: ['dark', 'light'] },
  'metadata.tags': { exists: true },
}
// SQL: ((json_extract("profile", '$.email') REGEXP ? AND
//        json_extract("profile", '$.age') >= ? AND
//        json_extract("settings", '$.theme') IN (?,?) AND
//        json_extract("metadata", '$.tags') IS NOT NULL))

// Complex nested JSON query
const filter10 = {
  and: [
    { 'user.profile.email': { regex: '.*@company\\.com$' } },
    { or: [{ 'user.role': 'ADMIN' }, { 'user.permissions.canEdit': true }] },
  ],
}
```

#### Qualified Filters for JOINs

For complex queries involving multiple tables or aliases, use **alias blocks** with keys starting with `$`:

```typescript
// Basic alias usage
const joinFilter = {
  // Main table fields (no prefix)
  status: 'ACTIVE',
  age: { gte: 18, lt: 65 },

  // Table alias 't2'
  $t2: {
    column_in_table_2: { gte: 100 },
    'stats.avg': { lt: 10 }, // JSON path in aliased table
  },

  // Table alias 'orders'
  $orders: {
    amount: { gt: 500 },
    status: { in: ['completed', 'shipped'] },
  },
}

const result = compileFilter(joinFilter)
// SQL: ((("status" = ?) AND ("age" >= ? AND "age" < ?) AND
//        ((t2."column_in_table_2" >= ?) AND (json_extract(t2."stats", '$.avg') < ?)) AND
//        ((orders."amount" > ?) AND (orders."status" IN (?,?)))))
```

**Alias Block Rules:**

- **Alias names**: Must be valid SQL identifiers (`[A-Za-z_][A-Za-z0-9_]*`)
- **Prefixing**: All fields in alias blocks get prefixed with `alias.`
- **JSON paths**: Work seamlessly with aliases: `json_extract(alias."column", '$.path')`
- **Combination**: Alias blocks are AND-combined with root fields and each other
- **Order**: Regular fields processed first, then alias blocks

```typescript
// Complex alias example with logical operators
const complexJoinFilter = {
  // Primary table conditions
  user_status: 'ACTIVE',

  // User profile table
  $profile: {
    verified: true,
    'preferences.notifications': { ne: false },
    or: [{ subscription_type: 'premium' }, { credits: { gte: 100 } }],
  },

  // Orders table with complex conditions
  $orders: {
    and: [
      { created_at: { gte: '2024-01-01' } },
      { or: [{ total_amount: { gt: 1000 } }, { item_count: { gte: 5 } }] },
    ],
  },
}

// Use in JOIN queries
const whereClause = compileFilter(complexJoinFilter)
const query = sql`
  SELECT u.id, u.name, p.subscription_type, o.total_amount
  FROM users u
  JOIN profiles p ON u.id = p.user_id  
  JOIN orders o ON u.id = o.user_id
  WHERE ${sql.raw(whereClause.text)}
`
```

**Security & Validation:**

```typescript
// ✅ Valid alias identifiers
$users: { name: 'John' }        // Simple identifier
$user_profiles: { age: 25 }     // Underscore allowed
$_temp: { status: 'active' }    // Starting underscore allowed

// ❌ Invalid alias identifiers (will throw SyntaxError)
$123invalid: { ... }            // Cannot start with number
$'invalid-alias': { ... }       // Hyphens not allowed
$'table.alias': { ... }         // Dots not allowed in alias name
```

**Integration with Complex Queries:**

```typescript
// Real-world JOIN example
const userOrderFilter = {
  // Users table
  active: true,
  email: { exists: true },

  // User profiles
  $profiles: {
    'settings.email_notifications': true,
    verified_at: { exists: true },
  },

  // Recent orders
  $recent_orders: {
    created_at: { gte: '2024-01-01' },
    status: { in: ['completed', 'shipped'] },
    total: { gt: 50 },
  },
}

const whereClause = compileFilter(userOrderFilter)

const complexQuery = sql`
  SELECT 
    u.id,
    u.name,
    u.email,
    p.verified_at,
    COUNT(ro.id) as recent_order_count,
    SUM(ro.total) as recent_order_total
  FROM users u
  JOIN profiles p ON u.id = p.user_id
  JOIN orders ro ON u.id = ro.user_id 
  WHERE ${sql.raw(whereClause.text)}
  GROUP BY u.id, u.name, u.email, p.verified_at
  HAVING recent_order_count > 0
  ORDER BY recent_order_total DESC
`

const results = db.prepare(complexQuery.text).all(...complexQuery.values)
```

#### Kitchen Sink Examples

Real-world complex filters:

```typescript
// Active users in specific regions, either minors/seniors or VIP
const complexFilter = {
  and: [
    { status: 'ACTIVE' },
    { or: [{ age: { lt: 18 } }, { age: { gte: 65 } }, { membership: 'VIP' }] },
    { country: { in: ['US', 'CA', 'GB'] } },
    { email: { exists: true } },
    { 'profile.verified': true },
  ],
}

// Content filtering with multiple criteria
const contentFilter = {
  name: { like: 'Project%' },
  category: { nin: ['ARCHIVED', 'DELETED', 'SPAM'] },
  created_at: { exists: true },
  or: [
    { tags: { ilike: '%important%' } },
    { priority: { gte: 8 } },
    { 'metadata.featured': true },
  ],
}
```

### Integration with SQL Template

```typescript
import { sql, compileFilter } from '@truto/sqlite-builder'

// Build the WHERE clause
const filter = {
  status: 'ACTIVE',
  age: { gte: 18 },
  role: { in: ['USER', 'ADMIN'] },
}

const whereClause = compileFilter(filter)

// Use in complete query
const query = sql`
  SELECT id, name, email, created_at
  FROM users
  WHERE ${sql.raw(whereClause.text)}
  ORDER BY created_at DESC
  LIMIT ${limit}
`

// Execute with driver
const results = db.prepare(query.text).all(...query.values)
```

### Security & Validation

The JSON filter compiler includes comprehensive security measures:

- **Operator validation**: Only known operators are allowed
- **Identifier safety**: Field names are validated using the same rules as `sql.ident()`
- **Array limits**: IN/NIN arrays limited to 999 items (SQLite limitation)
- **DoS protection**: Nesting depth ≤ 10, total operators ≤ 100
- **Type validation**: Strict type checking for all operator values
- **SQL injection prevention**: All values are parameterized

```typescript
// ❌ These will throw errors
compileFilter({ age: { unknown: 18 } }) // Unknown operator
compileFilter({ 'user; DROP TABLE': 'value' }) // Invalid identifier
compileFilter({ role: { in: [] } }) // Empty array
compileFilter({ role: { in: new Array(1000).fill('x') } }) // Too large array

// ✅ These are safe and valid
compileFilter({ age: { gte: 18, lte: 65 } }) // Multiple operators
compileFilter({ 'profile.email': { exists: true } }) // JSON path
compileFilter({ or: [{ x: 1 }, { y: 2 }] }) // Logical operators
```

### REGEXP Extension

To use the `regex` operator, you need to load a REGEXP extension in SQLite:

```typescript
// With better-sqlite3
import sqlite3 from 'better-sqlite3'

const db = new sqlite3('database.db')

// Load REGEXP extension (varies by implementation)
// This is implementation-specific - check your SQLite setup
db.loadExtension('regexp') // Example - actual method may vary

// Now regex filters work
const filter = {
  email: { regex: '^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\\.[a-zA-Z]{2,}$' },
}
```

## 🛡️ Security Model

### What's Protected

- **SQL Injection**: All interpolated values are parameterized
- **Stacked Queries**: Queries containing `;` followed by additional SQL are rejected
- **Identifier Safety**: `sql.ident()` validates against ANSI identifier rules
- **Length Limits**: Queries exceeding 100KB are rejected
- **Filter Security**: JSON filters validate operators, identifiers, and enforce limits

### What's Your Responsibility

- **Never use `sql.raw()` with user input**
- **Validate identifiers before using `sql.ident()`** (though it has built-in validation)
- **Use `sql.in()` instead of string concatenation** for arrays
- **Keep your SQLite driver updated**
- **Load REGEXP extension safely** if using regex filters

### Supported Value Types

```typescript
// ✅ Safe types (automatically parameterized)
const query = sql`
  INSERT INTO users (name, age, active, created_at, data, deleted_at)
  VALUES (
    ${'John'},           // string
    ${30},               // number  
    ${true},             // boolean
    ${new Date()},       // Date → 'YYYY-MM-DD HH:MM:SS'
    ${null},             // null
    ${undefined}         // undefined → null
  )
`

// ❌ Unsafe types (will throw TypeError)
sql`SELECT * FROM users WHERE data = ${Buffer.from('test')}` // Use sql.raw() for buffers
sql`SELECT * FROM users WHERE id = ${Symbol('test')}` // Unsupported type
```

## 📋 Examples

### Basic CRUD Operations

```typescript
import { sql } from '@truto/sqlite-builder'

// CREATE with array identifiers
const insertColumns = ['name', 'email', 'age']
const insertUser = sql`
  INSERT INTO users (${sql.ident(insertColumns)})
  VALUES (${name}, ${email}, ${age})
`

// READ with specific columns
const selectColumns = ['id', 'name', 'email', 'created_at']
const getUser = sql`
  SELECT ${sql.ident(selectColumns)} FROM users 
  WHERE id = ${userId}
`

// UPDATE
const updateUser = sql`
  UPDATE users 
  SET name = ${newName}, updated_at = ${new Date()}
  WHERE id = ${userId}
`

// DELETE
const deleteUser = sql`
  DELETE FROM users 
  WHERE id = ${userId}
`
```

### Dynamic Queries

```typescript
// Dynamic WHERE conditions
const filters = []
if (name) filters.push(sql`name = ${name}`)
if (minAge) filters.push(sql`age >= ${minAge}`)
if (isActive !== undefined) filters.push(sql`active = ${isActive}`)

const whereClause =
  filters.length > 0 ? sql.join(filters, ' AND ') : sql.raw('1=1')

const query = sql`
  SELECT * FROM users 
  WHERE ${whereClause}
  ORDER BY created_at DESC
`

// Dynamic column selection (simplified with array support)
const columns = ['id', 'name', 'email']
const selectQuery = sql`SELECT ${sql.ident(columns)} FROM users`

// Alternative approach for more complex column expressions
const complexColumns = [
  sql.ident('id'),
  sql.ident('name'),
  sql.raw('UPPER(email) as email_upper'),
]
const complexQuery = sql`SELECT ${sql.join(complexColumns)} FROM users`
```

### Dynamic Queries with JSON Filters

```typescript
import { sql, compileFilter } from '@truto/sqlite-builder'

// API endpoint that accepts JSON filter
app.get('/api/users', (req, res) => {
  // User sends filter as JSON
  const filter = req.body.filter || {}

  // Safely compile to SQL
  const whereClause = compileFilter(filter)

  const query = sql`
    SELECT id, name, email, created_at
    FROM users
    WHERE ${sql.raw(whereClause.text)}
    ORDER BY created_at DESC
    LIMIT ${req.query.limit || 20}
  `

  const users = db.prepare(query.text).all(...query.values)
  res.json(users)
})

// Example API calls:
// POST /api/users { "filter": { "status": "ACTIVE", "age": { "gte": 18 } } }
// POST /api/users { "filter": { "or": [{ "role": "ADMIN" }, { "verified": true }] } }
```

### Array Identifiers & Qualified Identifiers

The `sql.ident()` function supports simple identifiers, qualified identifiers (table.column), arrays, and mixed arrays with SQL fragments:

```typescript
// ✅ Simple identifiers
const table = 'users'
const column = 'name'
const simpleQuery = sql`SELECT ${sql.ident(column)} FROM ${sql.ident(table)}`
// Result: SELECT "name" FROM "users"

// ✅ Qualified identifiers (table.column)
const qualifiedQuery = sql`SELECT ${sql.ident('u.name')}, ${sql.ident('p.title')} FROM users u JOIN posts p ON u.id = p.user_id`
// Result: SELECT "u"."name", "p"."title" FROM users u JOIN posts p ON u.id = p.user_id

// ✅ Pure identifier arrays (clean and concise)
const columns = ['id', 'name', 'email', 'created_at']
const arrayQuery = sql`SELECT ${sql.ident(columns)} FROM users`
// Result: SELECT "id", "name", "email", "created_at" FROM users

// ✅ Mixed qualified and simple identifiers in arrays
const mixedColumns = ['u.id', 'name', 'u.email', 'p.title', 'created_at']
const mixedQuery = sql`SELECT ${sql.ident(mixedColumns)} FROM users u LEFT JOIN posts p ON u.id = p.user_id`
// Result: SELECT "u"."id", "name", "u"."email", "p"."title", "created_at" FROM users u LEFT JOIN posts p ON u.id = p.user_id

// ✅ NEW: Mixed arrays with identifiers and SQL fragments
const mixedColumns = [
  'id',
  'name',
  sql.raw('UPPER(email) as email_upper'),
  sql.raw('COUNT(*) as total'),
]
const mixedQuery = sql`SELECT ${sql.ident(mixedColumns)} FROM users GROUP BY id, name, email`
// Result: SELECT "id", "name", UPPER(email) as email_upper, COUNT(*) as total FROM users GROUP BY id, name, email

// ✅ Mixed arrays with parameterized fragments
const status = 'premium'
const dynamicColumns = [
  'id',
  'name',
  sql`CASE WHEN status = ${status} THEN 'Premium User' ELSE 'Regular User' END as user_type`,
  sql.raw('created_at'),
]
const parameterizedQuery = sql`SELECT ${sql.ident(dynamicColumns)} FROM users WHERE active = ${true}`
// Combines identifiers, raw SQL, and parameterized values seamlessly

// ✅ Works great for INSERT statements
const insertData = { name: 'John', email: 'john@example.com', age: 30 }
const insertColumns = Object.keys(insertData)
const insertValues = Object.values(insertData)
const insertQuery = sql`
  INSERT INTO users (${sql.ident(insertColumns)}) 
  VALUES (${insertValues[0]}, ${insertValues[1]}, ${insertValues[2]})
`

// ✅ Dynamic column selection
const userFields = ['name', 'email']
const includeTimestamps = true
if (includeTimestamps) {
  userFields.push('created_at', 'updated_at')
}
const dynamicQuery = sql`SELECT ${sql.ident(userFields)} FROM users`

// 🆚 OLD: Manual joining approach (still works, but much more verbose)
const oldWay = sql`SELECT ${sql.join([
  sql.ident('id'),
  sql.ident('name'),
  sql.raw('UPPER(email) as email_upper'),
])} FROM users`
```

### Complex Joins

```typescript
const getUsersWithPosts = sql`
  SELECT 
    ${sql.ident('u')}.id,
    ${sql.ident('u')}.name,
    COUNT(${sql.ident('p')}.id) as post_count
  FROM ${sql.ident('users')} u
  LEFT JOIN ${sql.ident('posts')} p ON u.id = p.user_id
  WHERE u.created_at > ${startDate}
    AND u.status IN ${sql.in(['active', 'premium'])}
  GROUP BY u.id
  HAVING post_count > ${minPosts}
  ORDER BY post_count DESC
  LIMIT ${limit}
`

// For simpler cases, you can use array identifiers directly
const getUsers = sql`
  SELECT ${sql.ident(['id', 'name', 'email', 'created_at'])}
  FROM ${sql.ident('users')}
  WHERE status = ${'active'}
`
```

### Transactions

```typescript
// Works great with better-sqlite3 transactions
const insertUsers = db.transaction((users) => {
  const stmt = db.prepare(
    sql`
    INSERT INTO users (name, email) 
    VALUES (?, ?)
  `.text,
  )

  for (const user of users) {
    const { values } = sql`${user.name}, ${user.email}`
    stmt.run(...values)
  }
})

insertUsers([
  { name: 'Alice', email: 'alice@example.com' },
  { name: 'Bob', email: 'bob@example.com' },
])
```

## 🧪 Testing

```bash
# Run tests
bun run test

# Run tests in watch mode
bun run dev

# Run tests with coverage
bun run test:coverage

# Run tests with UI
bun run test:ui
```

## 🤝 Contributing

We welcome contributions! Please see our [Contributing Guide](CONTRIBUTING.md) for details.

### Development Setup

```bash
git clone https://github.com/truto/truto-sqlite-builder.git
cd truto-sqlite-builder
bun install
bun run dev  # Start tests in watch mode
```

### Release Process

We use [changesets](https://github.com/changesets/changesets) for version management:

```bash
# Add a changeset
bunx changeset

# Release
bunx changeset version
bun run build
git commit -am "Release"
git push --follow-tags
```

## 🔒 Security Policy

If you discover a security vulnerability, please email eng@truto.one or create an issue.

## 📄 License

MIT © [Truto](https://github.com/trutohq)

## 💡 Inspiration

This library was inspired by:

- [sql-template-strings](https://github.com/felixfbecker/sql-template-strings)
- [slonik](https://github.com/gajus/slonik)
- [Postgres.js](https://github.com/porsager/postgres)

Built with ❤️ for the SQLite community.
