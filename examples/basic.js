import { Database } from 'bun:sqlite'
import { sql } from '../src/index'

console.log('🚀 Basic truto-sqlite-builder Example\n')

// Create an in-memory database
const db = new Database(':memory:')

// Create a users table
db.run(`
  CREATE TABLE users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    email TEXT UNIQUE NOT NULL,
    age INTEGER,
    active BOOLEAN DEFAULT 1,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
  )
`)

console.log('✅ Created users table')

// Insert some sample data
const insertUsers = [
  { name: 'Alice Johnson', email: 'alice@example.com', age: 28 },
  { name: 'Bob Smith', email: 'bob@example.com', age: 34 },
  { name: 'Charlie Brown', email: 'charlie@example.com', age: 22 },
  { name: 'Diana Prince', email: 'diana@example.com', age: 31 },
]

console.log('\n📝 Inserting users...')

insertUsers.forEach((user) => {
  const query = sql`
    INSERT INTO users (name, email, age)
    VALUES (${user.name}, ${user.email}, ${user.age})
  `

  console.log(`Query: ${query.text}`)
  console.log(`Values: ${JSON.stringify(query.values)}`)

  const result = db.query(query.text).run(...query.values)
  console.log(`✅ Inserted user with ID: ${result.lastInsertRowid}\n`)
})

// Query all users
console.log('📋 Querying all users:')
const allUsersQuery = sql`SELECT * FROM users ORDER BY created_at`
const allUsers = db.query(allUsersQuery.text).all(...allUsersQuery.values)

allUsers.forEach((user) => {
  console.log(`- ${user.name} (${user.email}) - Age: ${user.age}`)
})

// Query with WHERE condition
console.log('\n🔍 Finding users older than 25:')
const minAge = 25
const olderUsersQuery = sql`
  SELECT name, email, age 
  FROM users 
  WHERE age > ${minAge}
  ORDER BY age DESC
`

console.log(`Query: ${olderUsersQuery.text}`)
console.log(`Values: ${JSON.stringify(olderUsersQuery.values)}`)

const olderUsers = db
  .query(olderUsersQuery.text)
  .all(...olderUsersQuery.values)
olderUsers.forEach((user) => {
  console.log(`- ${user.name} (${user.age} years old)`)
})

// Query with IN clause
console.log('\n📊 Finding specific users by ID:')
const userIds = [1, 3, 4]
const specificUsersQuery = sql`
  SELECT * FROM users 
  WHERE id IN ${sql.in(userIds)}
`

console.log(`Query: ${specificUsersQuery.text}`)
console.log(`Values: ${JSON.stringify(specificUsersQuery.values)}`)

const specificUsers = db
  .query(specificUsersQuery.text)
  .all(...specificUsersQuery.values)
specificUsers.forEach((user) => {
  console.log(`- ID ${user.id}: ${user.name}`)
})

// Dynamic query building
console.log('\n🎯 Dynamic query with optional filters:')

const filters = []
const name = 'Alice'
const minAgeFilter = 20

if (name) {
  filters.push(sql`name LIKE ${`%${name}%`}`)
}
if (minAgeFilter) {
  filters.push(sql`age >= ${minAgeFilter}`)
}

const whereClause =
  filters.length > 0 ? sql.join(filters, ' AND ') : sql.raw('1=1')

const dynamicQuery = sql`
  SELECT * FROM users 
  WHERE ${whereClause}
`

console.log(`Query: ${dynamicQuery.text}`)
console.log(`Values: ${JSON.stringify(dynamicQuery.values)}`)

const filteredUsers = db.query(dynamicQuery.text).all(...dynamicQuery.values)
filteredUsers.forEach((user) => {
  console.log(`- ${user.name} matches the filters`)
})

// Update a user
console.log('\n✏️  Updating a user:')
const userId = 1
const newEmail = 'alice.johnson@example.com'

const updateQuery = sql`
  UPDATE users 
  SET email = ${newEmail}
  WHERE id = ${userId}
`

console.log(`Query: ${updateQuery.text}`)
console.log(`Values: ${JSON.stringify(updateQuery.values)}`)

const updateResult = db.query(updateQuery.text).run(...updateQuery.values)
console.log(`✅ Updated ${updateResult.changes} user(s)`)

// Safe identifier usage
console.log('\n🔒 Safe identifier usage:')
const tableName = 'users'
const columnName = 'name'

const safeQuery = sql`
  SELECT ${sql.ident(columnName)} 
  FROM ${sql.ident(tableName)} 
  LIMIT 1
`

console.log(`Query: ${safeQuery.text}`)
console.log(`Values: ${JSON.stringify(safeQuery.values)}`)

const safeResult = db.query(safeQuery.text).get(...safeQuery.values)
console.log(`✅ Safe query result: ${JSON.stringify(safeResult)}`)

// Array of identifiers example
const columns = ['name', 'email', 'created_at']
const selectQuery = sql`SELECT ${sql.ident(columns)} FROM users WHERE active = ${true}`
console.log('SELECT with array identifiers:')
console.log('Query:', selectQuery.text)
console.log('Values:', selectQuery.values)
console.log()

const insertColumns = ['name', 'email', 'age']
const insertQuery = sql`
  INSERT INTO users (${sql.ident(insertColumns)}) 
  VALUES (${'John Doe'}, ${'john@example.com'}, ${30})
`
console.log('INSERT with array identifiers:')
console.log('Query:', insertQuery.text.trim())
console.log('Values:', insertQuery.values)

// Mixed array with strings and SQL fragments
const mixedColumns = ['id', 'name', sql.raw('UPPER(email) as email_upper'), sql.raw('COUNT(*) as total')]
const mixedQuery = sql`SELECT ${sql.ident(mixedColumns)} FROM users GROUP BY id, name, email`
console.log('Mixed identifiers and SQL fragments:')
console.log('Query:', mixedQuery.text)
console.log('Values:', mixedQuery.values)

// Dynamic mixed array with parameterized fragments
const status = 'premium'
const dynamicMixed = [
  'id', 
  'name',
  sql`CASE WHEN status = ${status} THEN 'Premium User' ELSE 'Regular User' END as user_type`,
  sql.raw('created_at')
]
const dynamicMixedQuery = sql`SELECT ${sql.ident(dynamicMixed)} FROM users WHERE active = ${true}`
console.log('Dynamic mixed with parameters:')
console.log('Query:', dynamicMixedQuery.text)
console.log('Values:', dynamicMixedQuery.values)

// Qualified identifiers for table aliases
const aliasQuery = sql`
  SELECT ${sql.ident('u.name')}, ${sql.ident('u.email')}, ${sql.ident('p.title')} 
  FROM users u 
  JOIN posts p ON ${sql.ident('u.id')} = ${sql.ident('p.user_id')}
  WHERE ${sql.ident('u.active')} = ${true}
`
console.log('Qualified identifiers with table aliases:')
console.log('Query:', aliasQuery.text)
console.log('Values:', aliasQuery.values)

// Mixed qualified and simple identifiers in arrays
const mixedIdentifiers = ['u.id', 'name', 'u.email', 'created_at', 'p.title']
const qualifiedMixedQuery = sql`SELECT ${sql.ident(mixedIdentifiers)} FROM users u LEFT JOIN posts p ON u.id = p.user_id`
console.log('Mixed qualified and simple identifiers:')
console.log('Query:', qualifiedMixedQuery.text)
console.log('Values:', qualifiedMixedQuery.values)

// Clean up
db.close()
console.log('\n🧹 Database connection closed')
console.log('\n✨ Example completed successfully!')
