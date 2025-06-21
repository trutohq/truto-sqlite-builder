/// <reference types="node" />

import { performance } from 'perf_hooks';
import { sql } from '../src/sql.js';

// Benchmark configuration
const ITERATIONS = 100_000;
const USERS = Array.from({ length: 1000 }, (_, i) => ({
  id: i + 1,
  name: `User ${i + 1}`,
  email: `user${i + 1}@example.com`,
  age: Math.floor(Math.random() * 80) + 18,
}));

// Utility function to measure performance
function benchmark(
  name: string,
  fn: () => void,
  iterations: number = ITERATIONS
) {
  // Warm up
  for (let i = 0; i < 1000; i++) {
    fn();
  }

  const start = performance.now();
  for (let i = 0; i < iterations; i++) {
    fn();
  }
  const end = performance.now();

  const duration = end - start;
  const opsPerSecond = Math.floor((iterations / duration) * 1000);

  console.log(
    `${name}: ${duration.toFixed(2)}ms (${opsPerSecond.toLocaleString()} ops/sec)`
  );
  return { duration, opsPerSecond };
}

console.log('🚀 truto-sqlite-builder Benchmark\n');
console.log(`Iterations: ${ITERATIONS.toLocaleString()}\n`);

// Benchmark 1: Simple SELECT query
console.log('📊 Simple SELECT Query:');

const results1 = {
  tagged: benchmark('Tagged template', () => {
    const userId = Math.floor(Math.random() * 1000) + 1;
    const query = sql`SELECT * FROM users WHERE id = ${userId}`;
    return query;
  }),

  concat: benchmark('String concatenation', () => {
    const userId = Math.floor(Math.random() * 1000) + 1;
    return {
      text: 'SELECT * FROM users WHERE id = ?',
      values: [userId],
    };
  }),
};

console.log(
  `Tagged template is ${(results1.concat.opsPerSecond / results1.tagged.opsPerSecond).toFixed(2)}x slower than string concat\n`
);

// Benchmark 2: Complex query with multiple values
console.log('📊 Complex Query with Multiple Values:');

const results2 = {
  tagged: benchmark('Tagged template', () => {
    const user = USERS[Math.floor(Math.random() * USERS.length)];
    const query = sql`
      INSERT INTO users (name, email, age, created_at)
      VALUES (${user.name}, ${user.email}, ${user.age}, ${new Date()})
    `;
    return query;
  }),

  concat: benchmark('String concatenation', () => {
    const user = USERS[Math.floor(Math.random() * USERS.length)];
    return {
      text: 'INSERT INTO users (name, email, age, created_at) VALUES (?, ?, ?, ?)',
      values: [user.name, user.email, user.age, new Date()],
    };
  }),
};

console.log(
  `Tagged template is ${(results2.concat.opsPerSecond / results2.tagged.opsPerSecond).toFixed(2)}x slower than string concat\n`
);

// Benchmark 3: IN clause with arrays
console.log('📊 IN Clause with Arrays:');

const results3 = {
  tagged: benchmark(
    'Tagged template with sql.in()',
    () => {
      const ids = Array.from(
        { length: 10 },
        () => Math.floor(Math.random() * 1000) + 1
      );
      const query = sql`SELECT * FROM users WHERE id IN ${sql.in(ids)}`;
      return query;
    },
    10_000
  ), // Fewer iterations for this more complex operation

  concat: benchmark(
    'Manual placeholders',
    () => {
      const ids = Array.from(
        { length: 10 },
        () => Math.floor(Math.random() * 1000) + 1
      );
      const placeholders = ids.map(() => '?').join(',');
      return {
        text: `SELECT * FROM users WHERE id IN (${placeholders})`,
        values: ids,
      };
    },
    10_000
  ),
};

console.log(
  `Tagged template is ${(results3.concat.opsPerSecond / results3.tagged.opsPerSecond).toFixed(2)}x slower than manual placeholders\n`
);

// Benchmark 4: Identifier quoting
console.log('📊 Identifier Quoting:');

const tables = ['users', 'posts', 'comments', 'categories', 'tags'];

const results4 = {
  tagged: benchmark('sql.ident()', () => {
    const table = tables[Math.floor(Math.random() * tables.length)];
    const fragment = sql.ident(table);
    return fragment;
  }),

  manual: benchmark('Manual quoting', () => {
    const table = tables[Math.floor(Math.random() * tables.length)];
    return {
      text: `"${table}"`,
      values: [],
    };
  }),
};

console.log(
  `sql.ident() is ${(results4.manual.opsPerSecond / results4.tagged.opsPerSecond).toFixed(2)}x slower than manual quoting\n`
);

// Summary
console.log('📈 Summary:');
console.log(
  '- Tagged templates provide safety and convenience at a moderate performance cost'
);
console.log(
  '- For high-performance scenarios, consider caching parsed templates'
);
console.log(
  '- The safety benefits typically outweigh the performance overhead'
);
console.log(
  '- Performance difference is negligible for most real-world applications'
);
