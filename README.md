# 🏗️ truto-sqlite-builder

[![npm version](https://badge.fury.io/js/truto-sqlite-builder.svg)](https://badge.fury.io/js/truto-sqlite-builder)
[![CI](https://github.com/truto/truto-sqlite-builder/actions/workflows/ci.yml/badge.svg)](https://github.com/truto/truto-sqlite-builder/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/truto/truto-sqlite-builder/branch/main/graph/badge.svg)](https://codecov.io/gh/truto/truto-sqlite-builder)
[![TypeScript](https://img.shields.io/badge/TypeScript-Ready-blue.svg)](https://www.typescriptlang.org/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

**Safe, zero-dependency template-literal tag for SQLite queries in any JS environment.**

`truto-sqlite-builder` provides a secure and ergonomic way to build SQLite queries using tagged template literals. It prevents SQL injection attacks through parameterized queries while offering convenient helper functions for common SQL patterns.

## ✨ Features

- 🔒 **Injection-safe**: All values are parameterized, preventing SQL injection
- 🚫 **Defense in depth**: Multiple security layers including stacked query detection
- 🪶 **Zero dependencies**: Pure TypeScript/JavaScript with no runtime dependencies
- 🌍 **Universal**: Works in Node.js, Bun, Deno, and modern browsers
- 🎯 **TypeScript-first**: Full type safety with excellent IDE support
- 🔧 **Helper functions**: Built-in utilities for identifiers, IN clauses, and more
- ⚡ **Lightweight**: Minimal bundle size with tree-shaking support

## 📦 Installation

```bash
npm install truto-sqlite-builder
```

```bash
yarn add truto-sqlite-builder
```

```bash
pnpm add truto-sqlite-builder
```

```bash
bun add truto-sqlite-builder
```

## 🚀 Quick Start

```typescript
import sqlite3 from 'better-sqlite3';
import { sql } from 'truto-sqlite-builder';

const db = new sqlite3('database.db');

// Simple query
const name = 'Alice';
const { text, values } = sql`SELECT * FROM users WHERE name = ${name}`;
const users = db.prepare(text).all(...values);

// Complex query with helpers
const userIds = [1, 2, 3];
const query = sql`
  SELECT u.*, p.title 
  FROM ${sql.ident('users')} u
  JOIN ${sql.ident('posts')} p ON u.id = p.user_id
  WHERE u.id IN ${sql.in(userIds)}
    AND u.status = ${'active'}
`;

const results = db.prepare(query.text).all(...query.values);
```

## 📖 API Reference

### `sql` Tagged Template

The main function for building SQL queries.

```typescript
const query = sql`SELECT * FROM users WHERE id = ${userId}`;
// Returns: { text: "SELECT * FROM users WHERE id = ?", values: [userId] }
```

**Parameters:**

- Template strings and interpolated values
- Returns a frozen `SqlQuery` object with `text` and `values` properties

### `sql.ident(identifier: string)`

Safely quotes SQL identifiers (table names, column names, etc.).

```typescript
const table = 'users';
const query = sql`SELECT * FROM ${sql.ident(table)}`;
// Returns: { text: 'SELECT * FROM "users"', values: [] }
```

**Security:** Only accepts valid ANSI identifiers matching `/^[A-Za-z_][A-Za-z0-9_]*$/`

### `sql.in(array: readonly unknown[])`

Creates parameterized IN clauses from arrays.

```typescript
const ids = [1, 2, 3];
const query = sql`SELECT * FROM users WHERE id IN ${sql.in(ids)}`;
// Returns: { text: "SELECT * FROM users WHERE id IN (?,?,?)", values: [1, 2, 3] }
```

**Features:**

- Rejects empty arrays (would create invalid SQL)
- Warns for arrays with >1000 items (performance consideration)

### `sql.raw(rawSql: string)`

Embeds raw SQL without parameterization. **⚠️ Use with extreme caution!**

```typescript
const query = sql`SELECT * FROM users WHERE created_at > ${sql.raw('datetime("now", "-1 day")')}`;
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
];

const query = sql`SELECT * FROM users WHERE ${sql.join(conditions, ' AND ')}`;
// Returns: { text: "SELECT * FROM users WHERE name = ? AND age = ? AND active = ?", values: ['John', 30, true] }
```

## 🛡️ Security Model

### What's Protected

- **SQL Injection**: All interpolated values are parameterized
- **Stacked Queries**: Queries containing `;` followed by additional SQL are rejected
- **Identifier Safety**: `sql.ident()` validates against ANSI identifier rules
- **Length Limits**: Queries exceeding 100KB are rejected (configurable via `TRUTO_SQL_MAX_LENGTH`)

### What's Your Responsibility

- **Never use `sql.raw()` with user input**
- **Validate identifiers before using `sql.ident()`** (though it has built-in validation)
- **Use `sql.in()` instead of string concatenation** for arrays
- **Keep your SQLite driver updated**

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
`;

// ❌ Unsafe types (will throw TypeError)
sql`SELECT * FROM users WHERE data = ${Buffer.from('test')}`; // Use sql.raw() for buffers
sql`SELECT * FROM users WHERE id = ${Symbol('test')}`; // Unsupported type
```

## 🔍 Comparison with Other Libraries

| Feature                  | truto-sqlite-builder | sql-template-strings | slonik |
| ------------------------ | -------------------- | -------------------- | ------ |
| Zero dependencies        | ✅                   | ❌                   | ❌     |
| TypeScript-first         | ✅                   | ❌                   | ✅     |
| Browser support          | ✅                   | ✅                   | ❌     |
| SQLite-optimized         | ✅                   | ❌                   | ❌     |
| Stacked query protection | ✅                   | ❌                   | ✅     |
| Bundle size              | ~3KB                 | ~15KB                | ~200KB |

## 📋 Examples

### Basic CRUD Operations

```typescript
import { sql } from 'truto-sqlite-builder';

// CREATE
const insertUser = sql`
  INSERT INTO users (name, email, age)
  VALUES (${name}, ${email}, ${age})
`;

// READ
const getUser = sql`
  SELECT * FROM users 
  WHERE id = ${userId}
`;

// UPDATE
const updateUser = sql`
  UPDATE users 
  SET name = ${newName}, updated_at = ${new Date()}
  WHERE id = ${userId}
`;

// DELETE
const deleteUser = sql`
  DELETE FROM users 
  WHERE id = ${userId}
`;
```

### Dynamic Queries

```typescript
// Dynamic WHERE conditions
const filters = [];
if (name) filters.push(sql`name = ${name}`);
if (minAge) filters.push(sql`age >= ${minAge}`);
if (isActive !== undefined) filters.push(sql`active = ${isActive}`);

const whereClause =
  filters.length > 0 ? sql.join(filters, ' AND ') : sql.raw('1=1');

const query = sql`
  SELECT * FROM users 
  WHERE ${whereClause}
  ORDER BY created_at DESC
`;

// Dynamic column selection
const columns = ['id', 'name', 'email'];
const columnList = sql.join(
  columns.map((col) => sql.ident(col)),
  ', '
);

const selectQuery = sql`SELECT ${columnList} FROM users`;
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
`;
```

### Transactions

```typescript
// Works great with better-sqlite3 transactions
const insertUsers = db.transaction((users) => {
  const stmt = db.prepare(
    sql`
    INSERT INTO users (name, email) 
    VALUES (?, ?)
  `.text
  );

  for (const user of users) {
    const { values } = sql`${user.name}, ${user.email}`;
    stmt.run(...values);
  }
});

insertUsers([
  { name: 'Alice', email: 'alice@example.com' },
  { name: 'Bob', email: 'bob@example.com' },
]);
```

## ⚙️ Configuration

### Environment Variables

- `TRUTO_SQL_MAX_LENGTH`: Maximum query length in bytes (default: 102400 = 100KB)

```bash
# Increase query size limit to 1MB
export TRUTO_SQL_MAX_LENGTH=1048576
```

## 🧪 Testing

```bash
# Run tests
npm test

# Run tests in watch mode
npm run dev

# Run tests with coverage
npm run test:coverage

# Run tests with UI
npm run test:ui
```

## 🤝 Contributing

We welcome contributions! Please see our [Contributing Guide](CONTRIBUTING.md) for details.

### Development Setup

```bash
git clone https://github.com/truto/truto-sqlite-builder.git
cd truto-sqlite-builder
npm install
npm run dev  # Start tests in watch mode
```

### Release Process

We use [changesets](https://github.com/changesets/changesets) for version management:

```bash
# Add a changeset
npx changeset

# Release
npx changeset version
npm run build
git commit -am "Release"
git push --follow-tags
```

## 🔒 Security Policy

If you discover a security vulnerability, please email security@truto.com instead of creating a public issue.

## 📄 License

MIT © [Truto Team](https://github.com/truto)

## 💡 Inspiration

This library was inspired by:

- [sql-template-strings](https://github.com/felixfbecker/sql-template-strings)
- [slonik](https://github.com/gajus/slonik)
- [Postgres.js](https://github.com/porsager/postgres)

Built with ❤️ for the SQLite community.
