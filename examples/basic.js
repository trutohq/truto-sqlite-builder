import Database from 'better-sqlite3';
import { sql } from 'truto-sqlite-builder';

console.log('🚀 Basic truto-sqlite-builder Example\n');

// Create an in-memory database
const db = new Database(':memory:');

// Create a users table
db.exec(`
  CREATE TABLE users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    email TEXT UNIQUE NOT NULL,
    age INTEGER,
    active BOOLEAN DEFAULT 1,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
  )
`);

console.log('✅ Created users table');

// Insert some sample data
const insertUsers = [
  { name: 'Alice Johnson', email: 'alice@example.com', age: 28 },
  { name: 'Bob Smith', email: 'bob@example.com', age: 34 },
  { name: 'Charlie Brown', email: 'charlie@example.com', age: 22 },
  { name: 'Diana Prince', email: 'diana@example.com', age: 31 },
];

console.log('\n📝 Inserting users...');

insertUsers.forEach((user) => {
  const query = sql`
    INSERT INTO users (name, email, age)
    VALUES (${user.name}, ${user.email}, ${user.age})
  `;

  console.log(`Query: ${query.text}`);
  console.log(`Values: ${JSON.stringify(query.values)}`);

  const result = db.prepare(query.text).run(...query.values);
  console.log(`✅ Inserted user with ID: ${result.lastInsertRowid}\n`);
});

// Query all users
console.log('📋 Querying all users:');
const allUsersQuery = sql`SELECT * FROM users ORDER BY created_at`;
const allUsers = db.prepare(allUsersQuery.text).all(...allUsersQuery.values);

allUsers.forEach((user) => {
  console.log(`- ${user.name} (${user.email}) - Age: ${user.age}`);
});

// Query with WHERE condition
console.log('\n🔍 Finding users older than 25:');
const minAge = 25;
const olderUsersQuery = sql`
  SELECT name, email, age 
  FROM users 
  WHERE age > ${minAge}
  ORDER BY age DESC
`;

console.log(`Query: ${olderUsersQuery.text}`);
console.log(`Values: ${JSON.stringify(olderUsersQuery.values)}`);

const olderUsers = db
  .prepare(olderUsersQuery.text)
  .all(...olderUsersQuery.values);
olderUsers.forEach((user) => {
  console.log(`- ${user.name} (${user.age} years old)`);
});

// Query with IN clause
console.log('\n📊 Finding specific users by ID:');
const userIds = [1, 3, 4];
const specificUsersQuery = sql`
  SELECT * FROM users 
  WHERE id IN ${sql.in(userIds)}
`;

console.log(`Query: ${specificUsersQuery.text}`);
console.log(`Values: ${JSON.stringify(specificUsersQuery.values)}`);

const specificUsers = db
  .prepare(specificUsersQuery.text)
  .all(...specificUsersQuery.values);
specificUsers.forEach((user) => {
  console.log(`- ID ${user.id}: ${user.name}`);
});

// Dynamic query building
console.log('\n🎯 Dynamic query with optional filters:');

const filters = [];
const name = 'Alice';
const minAgeFilter = 20;

if (name) {
  filters.push(sql`name LIKE ${`%${name}%`}`);
}
if (minAgeFilter) {
  filters.push(sql`age >= ${minAgeFilter}`);
}

const whereClause =
  filters.length > 0 ? sql.join(filters, ' AND ') : sql.raw('1=1');

const dynamicQuery = sql`
  SELECT * FROM users 
  WHERE ${whereClause}
`;

console.log(`Query: ${dynamicQuery.text}`);
console.log(`Values: ${JSON.stringify(dynamicQuery.values)}`);

const filteredUsers = db.prepare(dynamicQuery.text).all(...dynamicQuery.values);
filteredUsers.forEach((user) => {
  console.log(`- ${user.name} matches the filters`);
});

// Update a user
console.log('\n✏️  Updating a user:');
const userId = 1;
const newEmail = 'alice.johnson@example.com';

const updateQuery = sql`
  UPDATE users 
  SET email = ${newEmail}
  WHERE id = ${userId}
`;

console.log(`Query: ${updateQuery.text}`);
console.log(`Values: ${JSON.stringify(updateQuery.values)}`);

const updateResult = db.prepare(updateQuery.text).run(...updateQuery.values);
console.log(`✅ Updated ${updateResult.changes} user(s)`);

// Safe identifier usage
console.log('\n🔒 Safe identifier usage:');
const tableName = 'users';
const columnName = 'name';

const safeQuery = sql`
  SELECT ${sql.ident(columnName)} 
  FROM ${sql.ident(tableName)} 
  LIMIT 1
`;

console.log(`Query: ${safeQuery.text}`);
console.log(`Values: ${JSON.stringify(safeQuery.values)}`);

const safeResult = db.prepare(safeQuery.text).get(...safeQuery.values);
console.log(`✅ Safe query result: ${JSON.stringify(safeResult)}`);

// Clean up
db.close();
console.log('\n🧹 Database connection closed');
console.log('\n✨ Example completed successfully!');
