import { describe, expect, it, vi } from 'vitest';
import { sql } from './sql.js';

describe('sql tagged template', () => {
  describe('basic functionality', () => {
    it('should create simple queries', () => {
      const query = sql`SELECT * FROM users`;
      expect(query.text).toBe('SELECT * FROM users');
      expect(query.values).toEqual([]);
    });

    it('should handle string values', () => {
      const name = 'John';
      const query = sql`SELECT * FROM users WHERE name = ${name}`;
      expect(query.text).toBe('SELECT * FROM users WHERE name = ?');
      expect(query.values).toEqual(['John']);
    });

    it('should handle numeric values', () => {
      const id = 42;
      const query = sql`SELECT * FROM users WHERE id = ${id}`;
      expect(query.text).toBe('SELECT * FROM users WHERE id = ?');
      expect(query.values).toEqual([42]);
    });

    it('should handle boolean values', () => {
      const active = true;
      const query = sql`SELECT * FROM users WHERE active = ${active}`;
      expect(query.text).toBe('SELECT * FROM users WHERE active = ?');
      expect(query.values).toEqual([true]);
    });

    it('should handle null values', () => {
      const value = null;
      const query = sql`SELECT * FROM users WHERE deleted_at = ${value}`;
      expect(query.text).toBe('SELECT * FROM users WHERE deleted_at = ?');
      expect(query.values).toEqual([null]);
    });

    it('should handle undefined values as null', () => {
      const value = undefined;
      const query = sql`SELECT * FROM users WHERE deleted_at = ${value}`;
      expect(query.text).toBe('SELECT * FROM users WHERE deleted_at = ?');
      expect(query.values).toEqual([null]);
    });

    it('should handle Date values', () => {
      const date = new Date('2023-12-25T10:30:00');
      const query = sql`SELECT * FROM users WHERE created_at = ${date}`;
      expect(query.text).toBe('SELECT * FROM users WHERE created_at = ?');
      expect(query.values).toEqual(['2023-12-25 10:30:00']);
    });

    it('should handle multiple values', () => {
      const name = 'John';
      const age = 30;
      const active = true;
      const query = sql`SELECT * FROM users WHERE name = ${name} AND age = ${age} AND active = ${active}`;
      expect(query.text).toBe(
        'SELECT * FROM users WHERE name = ? AND age = ? AND active = ?'
      );
      expect(query.values).toEqual(['John', 30, true]);
    });

    it('should return frozen objects', () => {
      const query = sql`SELECT * FROM users`;
      expect(Object.isFrozen(query)).toBe(true);
      expect(Object.isFrozen(query.values)).toBe(true);
    });
  });

  describe('sql.ident()', () => {
    it('should quote valid identifiers', () => {
      const fragment = sql.ident('users');
      expect(fragment.text).toBe('"users"');
      expect(fragment.values).toEqual([]);
    });

    it('should handle identifiers with underscores', () => {
      const fragment = sql.ident('user_name');
      expect(fragment.text).toBe('"user_name"');
      expect(fragment.values).toEqual([]);
    });

    it('should handle identifiers starting with underscore', () => {
      const fragment = sql.ident('_private');
      expect(fragment.text).toBe('"_private"');
      expect(fragment.values).toEqual([]);
    });

    it('should work in queries', () => {
      const table = 'users';
      const query = sql`SELECT * FROM ${sql.ident(table)}`;
      expect(query.text).toBe('SELECT * FROM "users"');
      expect(query.values).toEqual([]);
    });

    it('should reject empty strings', () => {
      expect(() => sql.ident('')).toThrow(
        'Identifier must be a non-empty string'
      );
    });

    it('should reject non-strings', () => {
      // eslint-disable-next-line @typescript-eslint/no-unsafe-argument, @typescript-eslint/no-explicit-any
      expect(() => sql.ident(123 as any)).toThrow(
        'Identifier must be a non-empty string'
      );
    });

    it('should reject invalid identifiers', () => {
      expect(() => sql.ident('123invalid')).toThrow('Invalid identifier');
      expect(() => sql.ident('user-name')).toThrow('Invalid identifier');
      expect(() => sql.ident('user name')).toThrow('Invalid identifier');
      expect(() => sql.ident('user.name')).toThrow('Invalid identifier');
    });
  });

  describe('sql.in()', () => {
    it('should create IN clauses', () => {
      const ids = [1, 2, 3];
      const fragment = sql.in(ids);
      expect(fragment.text).toBe('(?,?,?)');
      expect(fragment.values).toEqual([1, 2, 3]);
    });

    it('should work in queries', () => {
      const ids = [1, 2, 3];
      const query = sql`SELECT * FROM users WHERE id IN ${sql.in(ids)}`;
      expect(query.text).toBe('SELECT * FROM users WHERE id IN (?,?,?)');
      expect(query.values).toEqual([1, 2, 3]);
    });

    it('should handle mixed types', () => {
      const values = ['a', 1, null, true];
      const fragment = sql.in(values);
      expect(fragment.text).toBe('(?,?,?,?)');
      expect(fragment.values).toEqual(['a', 1, null, true]);
    });

    it('should reject empty arrays', () => {
      expect(() => sql.in([])).toThrow(
        'sql.in() cannot be used with empty arrays'
      );
    });

    it('should reject non-arrays', () => {
      // eslint-disable-next-line @typescript-eslint/no-unsafe-argument, @typescript-eslint/no-explicit-any
      expect(() => sql.in('not an array' as any)).toThrow(
        'sql.in() requires an array'
      );
    });

    it('should warn for large arrays', () => {
      const consoleSpy = vi.spyOn(console, 'warn').mockImplementation(() => {});
      const largeArray = Array.from({ length: 1001 }, (_, i) => i);

      sql.in(largeArray);

      expect(consoleSpy).toHaveBeenCalledWith(
        'sql.in(): Large array with 1001 items. Consider using temporary tables for better performance.'
      );

      consoleSpy.mockRestore();
    });
  });

  describe('sql.raw()', () => {
    it('should create raw SQL fragments', () => {
      const fragment = sql.raw('NOW()');
      expect(fragment.text).toBe('NOW()');
      expect(fragment.values).toEqual([]);
    });

    it('should work in queries', () => {
      const query = sql`SELECT * FROM users WHERE created_at > ${sql.raw('NOW() - INTERVAL 1 DAY')}`;
      expect(query.text).toBe(
        'SELECT * FROM users WHERE created_at > NOW() - INTERVAL 1 DAY'
      );
      expect(query.values).toEqual([]);
    });

    it('should reject non-strings', () => {
      // eslint-disable-next-line @typescript-eslint/no-unsafe-argument, @typescript-eslint/no-explicit-any
      expect(() => sql.raw(123 as any)).toThrow('sql.raw() requires a string');
    });
  });

  describe('sql.blob()', () => {
    it('should create BLOB fragments from Buffer', () => {
      const buffer = Buffer.from('test data');
      const fragment = sql.blob(buffer);
      expect(fragment.text).toBe('?');
      expect(fragment.values).toEqual([buffer]);
    });

    it('should create BLOB fragments from Uint8Array', () => {
      const uint8Array = new Uint8Array([1, 2, 3, 4]);
      const fragment = sql.blob(uint8Array);
      expect(fragment.text).toBe('?');
      expect(fragment.values).toEqual([uint8Array]);
    });

    it('should work in queries with Buffer', () => {
      const buffer = Buffer.from('test data');
      const query = sql`INSERT INTO files (data) VALUES (${sql.blob(buffer)})`;
      expect(query.text).toBe('INSERT INTO files (data) VALUES (?)');
      expect(query.values).toEqual([buffer]);
    });

    it('should work in queries with Uint8Array', () => {
      const uint8Array = new Uint8Array([1, 2, 3, 4]);
      const query = sql`UPDATE files SET data = ${sql.blob(uint8Array)} WHERE id = ${1}`;
      expect(query.text).toBe('UPDATE files SET data = ? WHERE id = ?');
      expect(query.values).toEqual([uint8Array, 1]);
    });

    it('should reject non-binary data', () => {
      // eslint-disable-next-line @typescript-eslint/no-unsafe-argument, @typescript-eslint/no-explicit-any
      expect(() => sql.blob('string' as any)).toThrow(
        'sql.blob() requires a Buffer or Uint8Array'
      );
      // eslint-disable-next-line @typescript-eslint/no-unsafe-argument, @typescript-eslint/no-explicit-any
      expect(() => sql.blob(123 as any)).toThrow(
        'sql.blob() requires a Buffer or Uint8Array'
      );
      // eslint-disable-next-line @typescript-eslint/no-unsafe-argument, @typescript-eslint/no-explicit-any
      expect(() => sql.blob(null as any)).toThrow(
        'sql.blob() requires a Buffer or Uint8Array'
      );
    });

    it('should handle empty buffers', () => {
      const buffer = Buffer.alloc(0);
      const fragment = sql.blob(buffer);
      expect(fragment.text).toBe('?');
      expect(fragment.values).toEqual([buffer]);
    });

    it('should handle large binary data', () => {
      const largeBuffer = Buffer.alloc(10000, 0xaa);
      const fragment = sql.blob(largeBuffer);
      expect(fragment.text).toBe('?');
      expect(fragment.values).toEqual([largeBuffer]);
    });
  });

  describe('sql.join()', () => {
    it('should join fragments with default separator', () => {
      const fragments = [
        sql.ident('name'),
        sql.ident('email'),
        sql.ident('age'),
      ];
      const joined = sql.join(fragments);
      expect(joined.text).toBe('"name", "email", "age"');
      expect(joined.values).toEqual([]);
    });

    it('should join fragments with custom separator', () => {
      const fragments = [sql.ident('name'), sql.ident('email')];
      const joined = sql.join(fragments, ' AND ');
      expect(joined.text).toBe('"name" AND "email"');
      expect(joined.values).toEqual([]);
    });

    it('should handle fragments with values', () => {
      const name = 'John';
      const age = 30;
      const fragments = [sql`name = ${name}`, sql`age = ${age}`];
      const joined = sql.join(fragments, ' AND ');
      expect(joined.text).toBe('name = ? AND age = ?');
      expect(joined.values).toEqual([name, age]);
    });

    it('should work in queries', () => {
      const conditions = [sql`name = ${'John'}`, sql`age = ${30}`];
      const query = sql`SELECT * FROM users WHERE ${sql.join(conditions, ' AND ')}`;
      expect(query.text).toBe('SELECT * FROM users WHERE name = ? AND age = ?');
      expect(query.values).toEqual(['John', 30]);
    });

    it('should handle empty arrays', () => {
      const joined = sql.join([]);
      expect(joined.text).toBe('');
      expect(joined.values).toEqual([]);
    });

    it('should reject non-arrays', () => {
      // eslint-disable-next-line @typescript-eslint/no-unsafe-argument, @typescript-eslint/no-explicit-any
      expect(() => sql.join('not an array' as any)).toThrow(
        'sql.join() requires an array of fragments'
      );
    });
  });

  describe('security features', () => {
    it('should reject stacked queries', () => {
      expect(() => sql`SELECT * FROM users; DROP TABLE users;`).toThrow(
        'Stacked queries are not allowed'
      );
      expect(() => sql`SELECT * FROM users;\nDROP TABLE users;`).toThrow(
        'Stacked queries are not allowed'
      );
      expect(() => sql`SELECT * FROM users;\tDROP TABLE users;`).toThrow(
        'Stacked queries are not allowed'
      );
    });

    it('should allow semicolons in strings', () => {
      const text = 'Hello; World';
      const query = sql`SELECT * FROM users WHERE comment = ${text}`;
      expect(query.text).toBe('SELECT * FROM users WHERE comment = ?');
      expect(query.values).toEqual(['Hello; World']);
    });

    it('should reject queries that are too long', () => {
      const originalEnv = process.env.TRUTO_SQL_MAX_LENGTH;
      process.env.TRUTO_SQL_MAX_LENGTH = '100';

      try {
        const longQuery = 'SELECT * FROM users WHERE ' + 'a'.repeat(200);
        expect(() => sql`${sql.raw(longQuery)}`).toThrow('Query too long');
      } finally {
        process.env.TRUTO_SQL_MAX_LENGTH = originalEnv;
      }
    });

    it('should reject Buffer values in interpolation', () => {
      const buffer = Buffer.from('test');
      expect(() => sql`SELECT * FROM users WHERE data = ${buffer}`).toThrow(
        'Buffer/Uint8Array values must be used with sql.blob() for safe BLOB handling'
      );
    });

    it('should reject Uint8Array values in interpolation', () => {
      const uint8Array = new Uint8Array([1, 2, 3]);
      expect(() => sql`SELECT * FROM users WHERE data = ${uint8Array}`).toThrow(
        'Buffer/Uint8Array values must be used with sql.blob() for safe BLOB handling'
      );
    });

    it('should reject unsupported value types', () => {
      const symbol = Symbol('test');
      expect(() => sql`SELECT * FROM users WHERE data = ${symbol}`).toThrow(
        'Unsupported value type: symbol'
      );
    });
  });

  describe('SQL injection prevention', () => {
    it('should prevent basic SQL injection', () => {
      const maliciousInput = "'; DROP TABLE users; --";
      const query = sql`SELECT * FROM users WHERE name = ${maliciousInput}`;
      expect(query.text).toBe('SELECT * FROM users WHERE name = ?');
      expect(query.values).toEqual([maliciousInput]);
    });

    it('should prevent OR 1=1 injection', () => {
      const maliciousInput = "' OR 1=1 --";
      const query = sql`SELECT * FROM users WHERE name = ${maliciousInput}`;
      expect(query.text).toBe('SELECT * FROM users WHERE name = ?');
      expect(query.values).toEqual([maliciousInput]);
    });

    it('should prevent UNION injection', () => {
      const maliciousInput = "' UNION SELECT password FROM admin --";
      const query = sql`SELECT * FROM users WHERE name = ${maliciousInput}`;
      expect(query.text).toBe('SELECT * FROM users WHERE name = ?');
      expect(query.values).toEqual([maliciousInput]);
    });

    it('should handle special characters safely', () => {
      const specialChars = `'"\\%_`;
      const query = sql`SELECT * FROM users WHERE name = ${specialChars}`;
      expect(query.text).toBe('SELECT * FROM users WHERE name = ?');
      expect(query.values).toEqual([specialChars]);
    });
  });

  describe('edge cases', () => {
    it('should handle empty template strings', () => {
      const query = sql``;
      expect(query.text).toBe('');
      expect(query.values).toEqual([]);
    });

    it('should handle only interpolation', () => {
      const value = 'test';
      const query = sql`${value}`;
      expect(query.text).toBe('?');
      expect(query.values).toEqual(['test']);
    });

    it('should handle Date edge cases', () => {
      const date = new Date('2023-01-01T00:00:00');
      const query = sql`SELECT * FROM users WHERE created_at = ${date}`;
      expect(query.text).toBe('SELECT * FROM users WHERE created_at = ?');
      expect(query.values).toEqual(['2023-01-01 00:00:00']);
    });

    it('should handle nested template literals', () => {
      const table = 'users';
      const condition = sql`name = ${'John'}`;
      const query = sql`SELECT * FROM ${sql.ident(table)} WHERE ${condition}`;
      expect(query.text).toBe('SELECT * FROM "users" WHERE name = ?');
      expect(query.values).toEqual(['John']);
    });
  });

  describe('complex queries', () => {
    it('should handle INSERT queries', () => {
      const name = 'John Doe';
      const email = 'john@example.com';
      const age = 30;

      const query = sql`
        INSERT INTO users (name, email, age)
        VALUES (${name}, ${email}, ${age})
      `;

      expect(query.text.trim()).toBe(
        'INSERT INTO users (name, email, age)\n        VALUES (?, ?, ?)'
      );
      expect(query.values).toEqual([name, email, age]);
    });

    it('should handle UPDATE queries', () => {
      const name = 'Jane Doe';
      const age = 25;
      const id = 1;

      const query = sql`
        UPDATE users 
        SET name = ${name}, age = ${age}
        WHERE id = ${id}
      `;

      expect(query.text.trim()).toBe(
        'UPDATE users \n        SET name = ?, age = ?\n        WHERE id = ?'
      );
      expect(query.values).toEqual([name, age, id]);
    });

    it('should handle complex SELECT with JOINs', () => {
      const userId = 1;
      const status = 'active';

      const query = sql`
        SELECT u.name, p.title
        FROM ${sql.ident('users')} u
        JOIN ${sql.ident('posts')} p ON u.id = p.user_id
        WHERE u.id = ${userId} AND p.status = ${status}
      `;

      expect(query.text.trim()).toBe(
        'SELECT u.name, p.title\n        FROM "users" u\n        JOIN "posts" p ON u.id = p.user_id\n        WHERE u.id = ? AND p.status = ?'
      );
      expect(query.values).toEqual([userId, status]);
    });
  });
});
