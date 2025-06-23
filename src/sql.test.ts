import { describe, expect, it, vi } from 'vitest'
import { sql } from './sql.js'

describe('sql tagged template', () => {
  describe('basic functionality', () => {
    it('should create simple queries', () => {
      const query = sql`SELECT * FROM users`
      expect(query.text).toBe('SELECT * FROM users')
      expect(query.values).toEqual([])
    })

    it('should handle string values', () => {
      const name = 'John'
      const query = sql`SELECT * FROM users WHERE name = ${name}`
      expect(query.text).toBe('SELECT * FROM users WHERE name = ?')
      expect(query.values).toEqual(['John'])
    })

    it('should handle numeric values', () => {
      const id = 42
      const query = sql`SELECT * FROM users WHERE id = ${id}`
      expect(query.text).toBe('SELECT * FROM users WHERE id = ?')
      expect(query.values).toEqual([42])
    })

    it('should handle boolean values', () => {
      const active = true
      const query = sql`SELECT * FROM users WHERE active = ${active}`
      expect(query.text).toBe('SELECT * FROM users WHERE active = ?')
      expect(query.values).toEqual([true])
    })

    it('should handle null values', () => {
      const value = null
      const query = sql`SELECT * FROM users WHERE deleted_at = ${value}`
      expect(query.text).toBe('SELECT * FROM users WHERE deleted_at = ?')
      expect(query.values).toEqual([null])
    })

    it('should handle undefined values as null', () => {
      const value = undefined
      const query = sql`SELECT * FROM users WHERE deleted_at = ${value}`
      expect(query.text).toBe('SELECT * FROM users WHERE deleted_at = ?')
      expect(query.values).toEqual([null])
    })

    it('should handle Date values', () => {
      const date = new Date('2023-12-25T10:30:00')
      const query = sql`SELECT * FROM users WHERE created_at = ${date}`
      expect(query.text).toBe('SELECT * FROM users WHERE created_at = ?')
      expect(query.values).toEqual(['2023-12-25 10:30:00'])
    })

    it('should handle multiple values', () => {
      const name = 'John'
      const age = 30
      const active = true
      const query = sql`SELECT * FROM users WHERE name = ${name} AND age = ${age} AND active = ${active}`
      expect(query.text).toBe(
        'SELECT * FROM users WHERE name = ? AND age = ? AND active = ?',
      )
      expect(query.values).toEqual(['John', 30, true])
    })

    it('should return frozen objects', () => {
      const query = sql`SELECT * FROM users`
      expect(Object.isFrozen(query)).toBe(true)
      expect(Object.isFrozen(query.values)).toBe(true)
    })
  })

  describe('sql.ident()', () => {
    it('should quote valid identifiers', () => {
      const fragment = sql.ident('users')
      expect(fragment.text).toBe('"users"')
      expect(fragment.values).toEqual([])
    })

    it('should handle identifiers with underscores', () => {
      const fragment = sql.ident('user_name')
      expect(fragment.text).toBe('"user_name"')
      expect(fragment.values).toEqual([])
    })

    it('should handle identifiers starting with underscore', () => {
      const fragment = sql.ident('_private')
      expect(fragment.text).toBe('"_private"')
      expect(fragment.values).toEqual([])
    })

    it('should work in queries', () => {
      const table = 'users'
      const query = sql`SELECT * FROM ${sql.ident(table)}`
      expect(query.text).toBe('SELECT * FROM "users"')
      expect(query.values).toEqual([])
    })

    it('should reject empty strings', () => {
      expect(() => sql.ident('')).toThrow(
        'Identifier must be a non-empty string',
      )
    })

    it('should reject non-strings', () => {
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      expect(() => sql.ident(123 as any)).toThrow(
        'Identifier must be a non-empty string',
      )
    })

    it('should handle qualified identifiers', () => {
      const fragment = sql.ident('u.name')
      expect(fragment.text).toBe('"u.name"')
      expect(fragment.values).toEqual([])
    })

    it('should handle multi-level qualified identifiers', () => {
      const fragment = sql.ident('schema.table.column')
      expect(fragment.text).toBe('"schema.table.column"')
      expect(fragment.values).toEqual([])
    })

    it('should work with qualified identifiers in queries', () => {
      const query = sql`SELECT ${sql.ident('u.name')}, ${sql.ident('p.title')} FROM users u JOIN posts p ON u.id = p.user_id`
      expect(query.text).toBe(
        'SELECT "u.name", "p.title" FROM users u JOIN posts p ON u.id = p.user_id',
      )
      expect(query.values).toEqual([])
    })

    it('should handle qualified identifiers in arrays', () => {
      const columns = ['u.id', 'u.name', 'p.title', 'p.created_at']
      const fragment = sql.ident(columns)
      expect(fragment.text).toBe('"u.id", "u.name", "p.title", "p.created_at"')
      expect(fragment.values).toEqual([])
    })

    it('should handle mixed simple and qualified identifiers in arrays', () => {
      const columns = ['name', 'u.email', 'created_at']
      const fragment = sql.ident(columns)
      expect(fragment.text).toBe('"name", "u.email", "created_at"')
      expect(fragment.values).toEqual([])
    })

    it('should reject invalid identifiers', () => {
      expect(() => sql.ident('123invalid')).toThrow('Invalid identifier')
      expect(() => sql.ident('user-name')).toThrow('Invalid identifier')
      expect(() => sql.ident('user name')).toThrow('Invalid identifier')
      expect(() => sql.ident('user.')).toThrow('Invalid identifier')
      expect(() => sql.ident('.name')).toThrow('Invalid identifier')
      expect(() => sql.ident('user..name')).toThrow('Invalid identifier')
      expect(() => sql.ident('user.123invalid')).toThrow('Invalid identifier')
    })

    // Array of identifiers tests
    it('should quote arrays of valid identifiers', () => {
      const fragment = sql.ident(['name', 'email', 'age'])
      expect(fragment.text).toBe('"name", "email", "age"')
      expect(fragment.values).toEqual([])
    })

    it('should handle arrays with single identifier', () => {
      const fragment = sql.ident(['users'])
      expect(fragment.text).toBe('"users"')
      expect(fragment.values).toEqual([])
    })

    it('should handle arrays with identifiers containing underscores', () => {
      const fragment = sql.ident(['user_name', 'email_address', '_private'])
      expect(fragment.text).toBe('"user_name", "email_address", "_private"')
      expect(fragment.values).toEqual([])
    })

    it('should work with arrays in SELECT queries', () => {
      const columns = ['name', 'email', 'created_at']
      const query = sql`SELECT ${sql.ident(columns)} FROM users`
      expect(query.text).toBe('SELECT "name", "email", "created_at" FROM users')
      expect(query.values).toEqual([])
    })

    it('should work with arrays in INSERT queries', () => {
      const columns = ['name', 'email', 'age']
      const values = ['John', 'john@example.com', 30]
      const query = sql`INSERT INTO users (${sql.ident(columns)}) VALUES (${values[0]}, ${values[1]}, ${values[2]})`
      expect(query.text).toBe(
        'INSERT INTO users ("name", "email", "age") VALUES (?, ?, ?)',
      )
      expect(query.values).toEqual(['John', 'john@example.com', 30])
    })

    it('should reject empty arrays', () => {
      expect(() => sql.ident([])).toThrow('Identifier array cannot be empty')
    })

    it('should reject arrays with invalid identifiers', () => {
      expect(() => sql.ident(['valid', '123invalid'])).toThrow(
        'Invalid identifier: 123invalid',
      )
      expect(() => sql.ident(['name', 'user-name'])).toThrow(
        'Invalid identifier: user-name',
      )
      expect(() => sql.ident(['email', 'user name'])).toThrow(
        'Invalid identifier: user name',
      )
      expect(() => sql.ident(['valid', 'user.'])).toThrow(
        'Invalid identifier: user.',
      )
    })

    it('should reject arrays with invalid non-string/non-fragment elements', () => {
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      expect(() => sql.ident(['name', 123 as any])).toThrow(
        'Array items must be strings or SQL fragments',
      )
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      expect(() => sql.ident(['name', null as any])).toThrow(
        'Array items must be strings or SQL fragments',
      )
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      expect(() => sql.ident(['name', '' as any])).toThrow(
        'All identifiers must be non-empty strings',
      )
    })

    // Mixed arrays with SQL fragments tests
    it('should handle mixed arrays with strings and SQL fragments', () => {
      const fragment = sql.ident([
        'id',
        'name',
        sql.raw('UPPER(email) as email_upper'),
      ])
      expect(fragment.text).toBe('"id", "name", UPPER(email) as email_upper')
      expect(fragment.values).toEqual([])
    })

    it('should handle mixed arrays with fragments that have values', () => {
      const userRole = 'admin'
      const fragment = sql.ident([
        'id',
        'name',
        sql`CASE WHEN role = ${userRole} THEN 'Admin User' ELSE name END as display_name`,
      ])
      expect(fragment.text).toBe(
        '"id", "name", CASE WHEN role = ? THEN \'Admin User\' ELSE name END as display_name',
      )
      expect(fragment.values).toEqual(['admin'])
    })

    it('should work with mixed arrays in SELECT queries', () => {
      const columns = ['id', 'name', sql.raw('COUNT(*) as total')]
      const query = sql`SELECT ${sql.ident(columns)} FROM users GROUP BY id, name`
      expect(query.text).toBe(
        'SELECT "id", "name", COUNT(*) as total FROM users GROUP BY id, name',
      )
      expect(query.values).toEqual([])
    })

    it('should handle complex mixed arrays', () => {
      const status = 'active'
      const fragment = sql.ident([
        'id',
        'name',
        sql.raw('u.email'),
        sql`CASE WHEN u.status = ${status} THEN 'Active' ELSE 'Inactive' END as status_label`,
        sql.raw('COUNT(p.id) as post_count'),
      ])
      expect(fragment.text).toBe(
        '"id", "name", u.email, CASE WHEN u.status = ? THEN \'Active\' ELSE \'Inactive\' END as status_label, COUNT(p.id) as post_count',
      )
      expect(fragment.values).toEqual(['active'])
    })

    it('should validate string identifiers in mixed arrays', () => {
      expect(() =>
        sql.ident(['valid', sql.raw('COUNT(*)'), '123invalid']),
      ).toThrow('Invalid identifier: 123invalid')
      expect(() =>
        sql.ident(['u.valid', sql.raw('COUNT(*)'), 'user.']),
      ).toThrow('Invalid identifier: user.')
    })

    // SQL Injection Prevention Tests for sql.ident()
    describe('SQL injection prevention', () => {
      it('should allow SQL keywords as quoted identifiers (valid behavior)', () => {
        // SQL keywords are valid identifiers when properly quoted
        // This is correct behavior - databases allow reserved words as identifiers when quoted
        const sqlKeywords = [
          'SELECT',
          'INSERT',
          'UPDATE',
          'DELETE',
          'DROP',
          'CREATE',
          'ALTER',
          'TRUNCATE',
          'EXEC',
          'UNION',
          'WHERE',
          'FROM',
          'JOIN',
          'ORDER',
          'GROUP',
          'HAVING',
        ]

        sqlKeywords.forEach((keyword) => {
          const fragment = sql.ident(keyword)
          expect(fragment.text).toBe(`"${keyword}"`)
          expect(fragment.values).toEqual([])

          // Test lowercase as well
          const lowerFragment = sql.ident(keyword.toLowerCase())
          expect(lowerFragment.text).toBe(`"${keyword.toLowerCase()}"`)
          expect(lowerFragment.values).toEqual([])
        })
      })

      it('should reject identifiers with SQL comments', () => {
        const maliciousIdentifiers = [
          'name--',
          'name-- comment',
          'name/*comment*/',
          'name/*',
          '*/name',
          'na--me',
          'na/*me*/',
          'table-- DROP TABLE users',
          'column/* malicious comment */',
        ]

        maliciousIdentifiers.forEach((identifier) => {
          expect(() => sql.ident(identifier)).toThrow('Invalid identifier')
        })
      })

      it('should reject identifiers with semicolons', () => {
        const maliciousIdentifiers = [
          'name;',
          'name; DROP TABLE users',
          'name;DROP TABLE users',
          ';name',
          'na;me',
          'table; DELETE FROM users',
        ]

        maliciousIdentifiers.forEach((identifier) => {
          expect(() => sql.ident(identifier)).toThrow('Invalid identifier')
        })
      })

      it('should reject identifiers with quotes', () => {
        const maliciousIdentifiers = [
          "name'",
          "name' OR 1=1",
          "name' UNION SELECT",
          "'name",
          "na'me",
          'name"',
          'name" OR 1=1',
          '"name',
          'na"me',
          "name'; DROP TABLE users; --",
        ]

        maliciousIdentifiers.forEach((identifier) => {
          expect(() => sql.ident(identifier)).toThrow('Invalid identifier')
        })
      })

      it('should reject identifiers with parentheses and function calls', () => {
        const maliciousIdentifiers = [
          'name()',
          'DROP()',
          'EXEC()',
          'name(1)',
          'name)',
          '(name',
          'na(me',
          'name)me',
          'SUBSTRING(password',
          'CONCAT(name, password)',
        ]

        maliciousIdentifiers.forEach((identifier) => {
          expect(() => sql.ident(identifier)).toThrow('Invalid identifier')
        })
      })

      it('should reject identifiers with arithmetic and comparison operators', () => {
        const maliciousIdentifiers = [
          'name+1',
          'name-1',
          'name*1',
          'name/1',
          'name=1',
          'name<>1',
          'name!=1',
          'name>1',
          'name<1',
          'name>=1',
          'name<=1',
          'name%1',
          'name^1',
          'name&1',
          'name|1',
        ]

        maliciousIdentifiers.forEach((identifier) => {
          expect(() => sql.ident(identifier)).toThrow('Invalid identifier')
        })
      })

      it('should reject identifiers with whitespace characters', () => {
        const maliciousIdentifiers = [
          'name ',
          ' name',
          'na me',
          'name\t',
          '\tname',
          'na\tme',
          'name\n',
          '\nname',
          'na\nme',
          'name\r',
          '\rname',
          'na\rme',
        ]

        maliciousIdentifiers.forEach((identifier) => {
          expect(() => sql.ident(identifier)).toThrow('Invalid identifier')
        })
      })

      it('should reject identifiers with special characters', () => {
        const maliciousIdentifiers = [
          'name@',
          'name#',
          'name$',
          'name%',
          'name&',
          'name*',
          'name+',
          'name-',
          'name=',
          'name[',
          'name]',
          'name{',
          'name}',
          'name|',
          'name\\',
          'name/',
          'name?',
          'name!',
          'name~',
          'name`',
        ]

        maliciousIdentifiers.forEach((identifier) => {
          expect(() => sql.ident(identifier)).toThrow('Invalid identifier')
        })
      })

      it('should reject qualified identifiers with SQL injection attempts', () => {
        const maliciousQualifiedIdentifiers = [
          'table.name; DROP TABLE users',
          'table.name-- comment',
          'table.name/*comment*/',
          "table.name' OR 1=1",
          'table.name" OR 1=1',
          'table.name()',
          'table.name=1',
          'table.name OR 1=1',
          'table.name UNION SELECT',
          'table.name; EXEC',
          "2' AND ORD(MID((SELECT DISTINCT(IFNULL(CAST(schema_name AS NCHAR),0x20)) FROM INFORMATION_SCHEMA.SCHEMATA LIMIT 5,1),5,1))>1 AND 'vFAF'='vFAF",
        ]

        maliciousQualifiedIdentifiers.forEach((identifier) => {
          expect(() => sql.ident(identifier)).toThrow('Invalid identifier')
        })
      })

      it('should allow valid qualified identifiers with SQL keywords', () => {
        // These are valid qualified identifiers even if they contain SQL keywords
        const validQualifiedIdentifiers = [
          'DROP.TABLE.users', // Valid: DROP is a table name, TABLE is a column name
          'SELECT.FROM.users', // Valid: SELECT is a table name, FROM is a column name
          'ORDER.BY.name', // Valid: ORDER is a table name, BY is a column name
        ]

        validQualifiedIdentifiers.forEach((identifier) => {
          const fragment = sql.ident(identifier)
          expect(fragment.text).toBe(`"${identifier}"`)
          expect(fragment.values).toEqual([])
        })
      })

      it('should reject arrays containing SQL injection attempts', () => {
        const maliciousArrays = [
          ['valid', 'name; DROP TABLE users'],
          ['valid', "name' OR 1=1"],
          ['valid', 'name-- comment'],
          ['valid', 'name/*comment*/'],
          ['valid', 'name()'],
          ['valid', 'name=1'],
          ['table.valid', 'table.name-- comment'],
          ['table.valid', "table.name' OR 1=1"],
        ]

        maliciousArrays.forEach((identifiers) => {
          expect(() => sql.ident(identifiers)).toThrow('Invalid identifier')
        })
      })

      it('should reject Unicode and encoded injection attempts', () => {
        const maliciousIdentifiers = [
          'name\u0000',
          'name\u0027', // Unicode single quote
          'name\u0022', // Unicode double quote
          'name\u003B', // Unicode semicolon
          'name\u002D\u002D', // Unicode double dash
          'name\uFEFF', // Zero-width no-break space
          'name\u200B', // Zero-width space
        ]

        maliciousIdentifiers.forEach((identifier) => {
          expect(() => sql.ident(identifier)).toThrow('Invalid identifier')
        })
      })

      it('should reject identifiers that could break out of quotes', () => {
        const maliciousIdentifiers = [
          'name" + "',
          "name' + '",
          'name" || "',
          "name' || '",
          'name" CONCAT "',
          "name' CONCAT '",
          'name"+"',
          "name'+'",
        ]

        maliciousIdentifiers.forEach((identifier) => {
          expect(() => sql.ident(identifier)).toThrow('Invalid identifier')
        })
      })

      it('should properly quote valid identifiers to prevent misinterpretation', () => {
        // These are valid identifiers that should be properly quoted
        const validIdentifiers = [
          'user_id',
          'table_name',
          'column_name',
          '_private',
          'u',
          'user123',
          'User_Name_123',
          'u.name',
          'users.id',
          'schema.table.column',
        ]

        validIdentifiers.forEach((identifier) => {
          const fragment = sql.ident(identifier)
          expect(fragment.text).toBe(`"${identifier}"`)
          expect(fragment.values).toEqual([])

          // Ensure the identifier is properly quoted in a query context
          const query = sql`SELECT ${sql.ident(identifier)} FROM users`
          expect(query.text).toBe(`SELECT "${identifier}" FROM users`)
          expect(query.values).toEqual([])
        })
      })

      it('should maintain security when used in real query contexts', () => {
        // Test that even if someone tries to use malicious input, it gets rejected
        const maliciousTable = 'users; DROP TABLE users; --'
        const maliciousColumn = "name' OR 1=1 --"

        expect(() => {
          return sql`SELECT * FROM ${sql.ident(maliciousTable)}`
        }).toThrow('Invalid identifier')

        expect(() => {
          return sql`SELECT ${sql.ident(maliciousColumn)} FROM users`
        }).toThrow('Invalid identifier')

        expect(() => {
          return sql`SELECT ${sql.ident([maliciousColumn, 'email'])} FROM users`
        }).toThrow('Invalid identifier')
      })

      it('should demonstrate comprehensive injection protection in realistic scenarios', () => {
        // Simulate user input that could come from untrusted sources
        const maliciousInputs = [
          "'; DROP TABLE users; --",
          '" OR 1=1 --',
          'admin/**/AND/**/password',
          'users; EXEC xp_cmdshell("format c:")',
          "users' UNION SELECT password FROM admin --",
          'table/*comment*/name',
          'column_name--',
          'name`OR`1=1',
          'users WHERE 1=1',
          'name\\x00\\x27',
        ]

        // Test in various SQL contexts where sql.ident() might be used
        maliciousInputs.forEach((maliciousInput) => {
          // SELECT statements
          expect(() => {
            return sql`SELECT ${sql.ident(maliciousInput)} FROM users`
          }).toThrow('Invalid identifier')

          // INSERT statements
          expect(() => {
            return sql`INSERT INTO ${sql.ident(maliciousInput)} (name) VALUES (${'test'})`
          }).toThrow('Invalid identifier')

          // UPDATE statements
          expect(() => {
            return sql`UPDATE users SET ${sql.ident(maliciousInput)} = ${'value'}`
          }).toThrow('Invalid identifier')

          // Arrays (common in SELECT column lists)
          expect(() => {
            return sql`SELECT ${sql.ident(['id', maliciousInput, 'email'])} FROM users`
          }).toThrow('Invalid identifier')

          // Qualified identifiers
          expect(() => {
            return sql`SELECT ${sql.ident(`table.${maliciousInput}`)} FROM users`
          }).toThrow('Invalid identifier')
        })
      })

      it('should allow legitimate identifiers that might look suspicious but are valid', () => {
        // These are valid identifiers that should be allowed
        const legitimateIdentifiers = [
          'SELECT', // SQL keyword but valid as quoted identifier
          'user_id_123',
          '_internal_column',
          'Column_With_Underscores',
          'u.user_name',
          'schema.table.column',
          'UserTable',
          'ORDER', // SQL keyword but valid as quoted identifier
        ]

        legitimateIdentifiers.forEach((identifier) => {
          const fragment = sql.ident(identifier)
          expect(fragment.text).toBe(`"${identifier}"`)
          expect(fragment.values).toEqual([])

          // Should work in actual queries
          const query = sql`SELECT ${sql.ident(identifier)} FROM users`
          expect(query.text).toBe(`SELECT "${identifier}" FROM users`)
          expect(query.values).toEqual([])
        })
      })
    })
  })

  describe('sql.in()', () => {
    it('should create IN clauses', () => {
      const ids = [1, 2, 3]
      const fragment = sql.in(ids)
      expect(fragment.text).toBe('(?,?,?)')
      expect(fragment.values).toEqual([1, 2, 3])
    })

    it('should work in queries', () => {
      const ids = [1, 2, 3]
      const query = sql`SELECT * FROM users WHERE id IN ${sql.in(ids)}`
      expect(query.text).toBe('SELECT * FROM users WHERE id IN (?,?,?)')
      expect(query.values).toEqual([1, 2, 3])
    })

    it('should handle mixed types', () => {
      const values = ['a', 1, null, true]
      const fragment = sql.in(values)
      expect(fragment.text).toBe('(?,?,?,?)')
      expect(fragment.values).toEqual(['a', 1, null, true])
    })

    it('should reject empty arrays', () => {
      expect(() => sql.in([])).toThrow(
        'sql.in() cannot be used with empty arrays',
      )
    })

    it('should reject non-arrays', () => {
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      expect(() => sql.in('not an array' as any)).toThrow(
        'sql.in() requires an array',
      )
    })

    it('should warn for large arrays', () => {
      const consoleSpy = vi.spyOn(console, 'warn').mockImplementation(() => {})
      const largeArray = Array.from({ length: 1001 }, (_, i) => i)

      sql.in(largeArray)

      expect(consoleSpy).toHaveBeenCalledWith(
        'sql.in(): Large array with 1001 items. Consider using temporary tables for better performance.',
      )

      consoleSpy.mockRestore()
    })
  })

  describe('sql.raw()', () => {
    it('should create raw SQL fragments', () => {
      const fragment = sql.raw('NOW()')
      expect(fragment.text).toBe('NOW()')
      expect(fragment.values).toEqual([])
    })

    it('should work in queries', () => {
      const query = sql`SELECT * FROM users WHERE created_at > ${sql.raw('NOW() - INTERVAL 1 DAY')}`
      expect(query.text).toBe(
        'SELECT * FROM users WHERE created_at > NOW() - INTERVAL 1 DAY',
      )
      expect(query.values).toEqual([])
    })

    it('should reject non-strings', () => {
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      expect(() => sql.raw(123 as any)).toThrow('sql.raw() requires a string')
    })
  })

  describe('sql.blob()', () => {
    it('should create BLOB fragments from Buffer', () => {
      const buffer = Buffer.from('test data')
      const fragment = sql.blob(buffer)
      expect(fragment.text).toBe('?')
      expect(fragment.values).toEqual([buffer])
    })

    it('should create BLOB fragments from Uint8Array', () => {
      const uint8Array = new Uint8Array([1, 2, 3, 4])
      const fragment = sql.blob(uint8Array)
      expect(fragment.text).toBe('?')
      expect(fragment.values).toEqual([uint8Array])
    })

    it('should work in queries with Buffer', () => {
      const buffer = Buffer.from('test data')
      const query = sql`INSERT INTO files (data) VALUES (${sql.blob(buffer)})`
      expect(query.text).toBe('INSERT INTO files (data) VALUES (?)')
      expect(query.values).toEqual([buffer])
    })

    it('should work in queries with Uint8Array', () => {
      const uint8Array = new Uint8Array([1, 2, 3, 4])
      const query = sql`UPDATE files SET data = ${sql.blob(uint8Array)} WHERE id = ${1}`
      expect(query.text).toBe('UPDATE files SET data = ? WHERE id = ?')
      expect(query.values).toEqual([uint8Array, 1])
    })

    it('should reject non-binary data', () => {
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      expect(() => sql.blob('string' as any)).toThrow(
        'sql.blob() requires a Buffer or Uint8Array',
      )
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      expect(() => sql.blob(123 as any)).toThrow(
        'sql.blob() requires a Buffer or Uint8Array',
      )
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      expect(() => sql.blob(null as any)).toThrow(
        'sql.blob() requires a Buffer or Uint8Array',
      )
    })

    it('should handle empty buffers', () => {
      const buffer = Buffer.alloc(0)
      const fragment = sql.blob(buffer)
      expect(fragment.text).toBe('?')
      expect(fragment.values).toEqual([buffer])
    })

    it('should handle large binary data', () => {
      const largeBuffer = Buffer.alloc(10000, 0xaa)
      const fragment = sql.blob(largeBuffer)
      expect(fragment.text).toBe('?')
      expect(fragment.values).toEqual([largeBuffer])
    })
  })

  describe('sql.join()', () => {
    it('should join fragments with default separator', () => {
      const fragments = [
        sql.ident('name'),
        sql.ident('email'),
        sql.ident('age'),
      ]
      const joined = sql.join(fragments)
      expect(joined.text).toBe('"name", "email", "age"')
      expect(joined.values).toEqual([])
    })

    it('should join fragments with custom separator', () => {
      const fragments = [sql.ident('name'), sql.ident('email')]
      const joined = sql.join(fragments, ' AND ')
      expect(joined.text).toBe('"name" AND "email"')
      expect(joined.values).toEqual([])
    })

    it('should handle fragments with values', () => {
      const name = 'John'
      const age = 30
      const fragments = [sql`name = ${name}`, sql`age = ${age}`]
      const joined = sql.join(fragments, ' AND ')
      expect(joined.text).toBe('name = ? AND age = ?')
      expect(joined.values).toEqual([name, age])
    })

    it('should work in queries', () => {
      const conditions = [sql`name = ${'John'}`, sql`age = ${30}`]
      const query = sql`SELECT * FROM users WHERE ${sql.join(conditions, ' AND ')}`
      expect(query.text).toBe('SELECT * FROM users WHERE name = ? AND age = ?')
      expect(query.values).toEqual(['John', 30])
    })

    it('should handle empty arrays', () => {
      const joined = sql.join([])
      expect(joined.text).toBe('')
      expect(joined.values).toEqual([])
    })

    it('should reject non-arrays', () => {
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      expect(() => sql.join('not an array' as any)).toThrow(
        'sql.join() requires an array of fragments',
      )
    })
  })

  describe('security features', () => {
    it('should reject stacked queries', () => {
      expect(() => sql`SELECT * FROM users; DROP TABLE users;`).toThrow(
        'Stacked queries are not allowed',
      )
      expect(() => sql`SELECT * FROM users;\nDROP TABLE users;`).toThrow(
        'Stacked queries are not allowed',
      )
      expect(() => sql`SELECT * FROM users;\tDROP TABLE users;`).toThrow(
        'Stacked queries are not allowed',
      )
    })

    it('should allow semicolons in strings', () => {
      const text = 'Hello; World'
      const query = sql`SELECT * FROM users WHERE comment = ${text}`
      expect(query.text).toBe('SELECT * FROM users WHERE comment = ?')
      expect(query.values).toEqual(['Hello; World'])
    })

    it('should reject queries that are too long', () => {
      // Test with a query that exceeds the 100KB limit
      const longQuery = 'SELECT * FROM users WHERE ' + 'a'.repeat(102500)
      expect(() => sql`${sql.raw(longQuery)}`).toThrow(
        /Query too long: \d+ bytes \(max: 102400\)/,
      )
    })

    it('should reject Buffer values in interpolation', () => {
      const buffer = Buffer.from('test')
      expect(() => sql`SELECT * FROM users WHERE data = ${buffer}`).toThrow(
        'Buffer/Uint8Array values must be used with sql.blob() for safe BLOB handling',
      )
    })

    it('should reject Uint8Array values in interpolation', () => {
      const uint8Array = new Uint8Array([1, 2, 3])
      expect(() => sql`SELECT * FROM users WHERE data = ${uint8Array}`).toThrow(
        'Buffer/Uint8Array values must be used with sql.blob() for safe BLOB handling',
      )
    })

    it('should reject unsupported value types', () => {
      const symbol = Symbol('test')
      expect(() => sql`SELECT * FROM users WHERE data = ${symbol}`).toThrow(
        'Unsupported value type: symbol',
      )
    })
  })

  describe('SQL injection prevention', () => {
    it('should prevent basic SQL injection', () => {
      const maliciousInput = "'; DROP TABLE users; --"
      const query = sql`SELECT * FROM users WHERE name = ${maliciousInput}`
      expect(query.text).toBe('SELECT * FROM users WHERE name = ?')
      expect(query.values).toEqual([maliciousInput])
    })

    it('should prevent OR 1=1 injection', () => {
      const maliciousInput = "' OR 1=1 --"
      const query = sql`SELECT * FROM users WHERE name = ${maliciousInput}`
      expect(query.text).toBe('SELECT * FROM users WHERE name = ?')
      expect(query.values).toEqual([maliciousInput])
    })

    it('should prevent UNION injection', () => {
      const maliciousInput = "' UNION SELECT password FROM admin --"
      const query = sql`SELECT * FROM users WHERE name = ${maliciousInput}`
      expect(query.text).toBe('SELECT * FROM users WHERE name = ?')
      expect(query.values).toEqual([maliciousInput])
    })

    it('should handle special characters safely', () => {
      const specialChars = `'"\\%_`
      const query = sql`SELECT * FROM users WHERE name = ${specialChars}`
      expect(query.text).toBe('SELECT * FROM users WHERE name = ?')
      expect(query.values).toEqual([specialChars])
    })

    it('should prevent comment-based injections', () => {
      const commentInjections = [
        "admin'--",
        "admin'/*comment*/",
        "admin' -- comment",
        "admin'/* multi\nline\ncomment */",
        "admin'#comment",
        "admin';-- comment",
        "'; /* comment */ SELECT * FROM admin --",
      ]

      commentInjections.forEach((maliciousInput) => {
        const query = sql`SELECT * FROM users WHERE name = ${maliciousInput}`
        expect(query.text).toBe('SELECT * FROM users WHERE name = ?')
        expect(query.values).toEqual([maliciousInput])
      })
    })

    it('should prevent various UNION-based injections', () => {
      const unionInjections = [
        "' UNION SELECT NULL, username, password FROM admin --",
        "' UNION ALL SELECT 1,2,3,4 --",
        "' UNION SELECT database(), user(), version() --",
        "' UNION SELECT 1,table_name,3 FROM information_schema.tables --",
        "' UNION SELECT CONCAT(username,':',password) FROM admin --",
        "' OR 1=1 UNION SELECT NULL, password FROM users --",
        "' UNION SELECT * FROM (SELECT password FROM admin) AS t --",
      ]

      unionInjections.forEach((maliciousInput) => {
        const query = sql`SELECT * FROM users WHERE name = ${maliciousInput}`
        expect(query.text).toBe('SELECT * FROM users WHERE name = ?')
        expect(query.values).toEqual([maliciousInput])
      })
    })

    it('should prevent boolean-based blind injections', () => {
      const blindInjections = [
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
      ]

      blindInjections.forEach((maliciousInput) => {
        const query = sql`SELECT * FROM users WHERE name = ${maliciousInput}`
        expect(query.text).toBe('SELECT * FROM users WHERE name = ?')
        expect(query.values).toEqual([maliciousInput])
      })
    })

    it('should prevent time-based blind injections', () => {
      const timeBasedInjections = [
        "'; WAITFOR DELAY '00:00:05' --",
        "' AND (SELECT SLEEP(5)) --",
        "' OR (SELECT pg_sleep(5)) --",
        "'; SELECT pg_sleep(5); --",
        "' AND IF(1=1, SLEEP(5), 0) --",
        "' UNION SELECT SLEEP(5) --",
        "'; IF(1=1) WAITFOR DELAY '0:0:5' --",
      ]

      timeBasedInjections.forEach((maliciousInput) => {
        const query = sql`SELECT * FROM users WHERE name = ${maliciousInput}`
        expect(query.text).toBe('SELECT * FROM users WHERE name = ?')
        expect(query.values).toEqual([maliciousInput])
      })
    })

    it('should prevent error-based injections', () => {
      const errorBasedInjections = [
        "' AND (SELECT * FROM (SELECT COUNT(*),CONCAT(version(),FLOOR(RAND(0)*2))x FROM information_schema.tables GROUP BY x)a) --",
        "' AND EXP(~(SELECT * FROM (SELECT USER())a)) --",
        "' AND GTID_SUBSET(CONCAT(0x7e,(SELECT database()),0x7e),1) --",
        "' AND (SELECT * FROM information_schema.tables FOR XML PATH('')) --",
        "' UNION SELECT 1/0 --",
        "' AND (SELECT password FROM admin WHERE username='admin' AND 1=CAST('a' AS INTEGER)) --",
      ]

      errorBasedInjections.forEach((maliciousInput) => {
        const query = sql`SELECT * FROM users WHERE name = ${maliciousInput}`
        expect(query.text).toBe('SELECT * FROM users WHERE name = ?')
        expect(query.values).toEqual([maliciousInput])
      })
    })

    it('should prevent authentication bypass attempts', () => {
      const bypassAttempts = [
        "admin'--",
        "admin'/*",
        "' OR 1=1#",
        "' OR 1=1--",
        "' OR '1'='1",
        "' OR '1'='1'--",
        "' OR '1'='1'/*",
        "') OR ('1'='1",
        "') OR ('1'='1'--",
        "admin') --",
        "admin') #",
        "') OR 1=1 --",
        "' OR 'x'='x",
        "' OR a=a--",
        "' or true--",
        "' OR 'something' like 'some%'",
        "' OR 2 > 1--",
        "' OR 'text' = N'text'",
        "' OR 'something' = 'something'",
        "' OR 'text' like 'text'",
        "' OR 1=1 LIMIT 1--",
      ]

      bypassAttempts.forEach((maliciousInput) => {
        const query = sql`SELECT * FROM users WHERE username = ${maliciousInput}`
        expect(query.text).toBe('SELECT * FROM users WHERE username = ?')
        expect(query.values).toEqual([maliciousInput])
      })
    })

    it('should prevent subquery-based injections', () => {
      const subqueryInjections = [
        "' AND (SELECT COUNT(*) FROM admin) > 0 --",
        "' AND (SELECT password FROM admin WHERE username='admin') LIKE 'a%' --",
        "' OR (SELECT user FROM mysql.user WHERE user='root') = 'root' --",
        "' AND (SELECT table_name FROM information_schema.tables WHERE table_schema=database() LIMIT 1) = 'admin' --",
        "' OR EXISTS(SELECT 1 FROM admin WHERE username='admin' AND password='pass') --",
        "' UNION SELECT (SELECT password FROM admin WHERE username='admin') --",
        "' AND 1=(SELECT COUNT(*) FROM tabname); --",
      ]

      subqueryInjections.forEach((maliciousInput) => {
        const query = sql`SELECT * FROM users WHERE name = ${maliciousInput}`
        expect(query.text).toBe('SELECT * FROM users WHERE name = ?')
        expect(query.values).toEqual([maliciousInput])
      })
    })

    it('should prevent function-based injections', () => {
      const functionInjections = [
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
      ]

      functionInjections.forEach((maliciousInput) => {
        const query = sql`SELECT * FROM users WHERE name = ${maliciousInput}`
        expect(query.text).toBe('SELECT * FROM users WHERE name = ?')
        expect(query.values).toEqual([maliciousInput])
      })
    })

    it('should prevent second-order injections', () => {
      // Second-order injections involve storing malicious input that gets executed later
      const secondOrderPayloads = [
        "admin'; DROP TABLE users; SELECT 'data",
        "test'; INSERT INTO admin (username, password) VALUES ('hacker', 'pass'); SELECT 'data",
        "user'; UPDATE admin SET password='newpass' WHERE username='admin'; SELECT 'data",
        "data'; TRUNCATE TABLE logs; SELECT 'normal",
      ]

      secondOrderPayloads.forEach((maliciousInput) => {
        // Insert operation that would store the payload
        const insertQuery = sql`INSERT INTO user_data (content) VALUES (${maliciousInput})`
        expect(insertQuery.text).toBe(
          'INSERT INTO user_data (content) VALUES (?)',
        )
        expect(insertQuery.values).toEqual([maliciousInput])

        // Select operation that would retrieve and potentially execute the payload
        const selectQuery = sql`SELECT * FROM user_data WHERE content = ${maliciousInput}`
        expect(selectQuery.text).toBe(
          'SELECT * FROM user_data WHERE content = ?',
        )
        expect(selectQuery.values).toEqual([maliciousInput])
      })
    })

    it('should prevent Unicode and encoding-based injections', () => {
      const unicodeInjections = [
        'admin\u0027 OR 1=1--', // Unicode single quote
        'admin\u0022 OR 1=1--', // Unicode double quote
        'admin\u003B DROP TABLE users--', // Unicode semicolon
        'admin\u002D\u002D', // Unicode double dash
        'admin\uFF07 OR 1=1--', // Fullwidth apostrophe
        'admin\u2019 OR 1=1--', // Right single quotation mark
        'admin%27 OR 1=1--', // URL encoded single quote
        'admin%3B DROP TABLE users--', // URL encoded semicolon
        'admin\x27 OR 1=1--', // Hex encoded single quote
        'admin\x00', // NULL byte
        'admin\x00\x27 OR 1=1--', // NULL byte with injection
      ]

      unicodeInjections.forEach((maliciousInput) => {
        const query = sql`SELECT * FROM users WHERE name = ${maliciousInput}`
        expect(query.text).toBe('SELECT * FROM users WHERE name = ?')
        expect(query.values).toEqual([maliciousInput])
      })
    })

    it('should prevent database-specific injections', () => {
      const dbSpecificInjections = [
        // MySQL specific
        "' OR 1=1 INTO OUTFILE '/tmp/file.txt' --",
        "' UNION SELECT @@version, @@datadir --",
        "'; SET @sql = CONCAT('DROP TABLE ', 'users'); PREPARE stmt FROM @sql; EXECUTE stmt; --",

        // PostgreSQL specific
        "'; COPY users TO '/tmp/users.csv' --",
        "' OR 1=1; CREATE OR REPLACE FUNCTION evil() RETURNS void AS $$ BEGIN RAISE NOTICE 'pwned'; END; $$ LANGUAGE plpgsql --",

        // SQLite specific
        "' UNION SELECT sql FROM sqlite_master --",
        "'; ATTACH DATABASE '/tmp/evil.db' AS evil; --",

        // SQL Server specific
        "'; EXEC xp_cmdshell('net user hacker pass123 /add') --",
        "'; EXEC sp_configure 'show advanced options', 1 --",
        "' UNION SELECT @@servername, @@version --",

        // Oracle specific
        "' UNION SELECT banner FROM v$version --",
        "'; GRANT DBA TO PUBLIC --",
      ]

      dbSpecificInjections.forEach((maliciousInput) => {
        const query = sql`SELECT * FROM users WHERE name = ${maliciousInput}`
        expect(query.text).toBe('SELECT * FROM users WHERE name = ?')
        expect(query.values).toEqual([maliciousInput])
      })
    })

    it('should prevent advanced evasion techniques', () => {
      const evasionTechniques = [
        // Case variation
        "' Or 1=1 --",
        "' oR 1=1 --",
        "' UnIoN sElEcT --",

        // Whitespace variations
        "'\t\tOR\t\t1=1--",
        "'\n\nOR\n\n1=1--",
        "'\r\rOR\r\r1=1--",
        "' /**/OR/**/1=1--",
        "'/*comment*/OR/*comment*/1=1--",

        // String concatenation
        "' + 'OR' + ' 1=1 --",
        "' || 'OR' || ' 1=1 --",
        "' CONCAT('OR', ' 1=1') --",

        // Function calls to bypass filters
        "' OR ASCII('A')=65 --",
        "' OR CHAR(65)='A' --",
        "' OR HEX('41')='A' --",

        // Conditional comments (MySQL)
        "'/*!32302 OR*/ 1=1 --",
        "'/*!50000 UNION SELECT*/ 1,2,3 --",
      ]

      evasionTechniques.forEach((maliciousInput) => {
        const query = sql`SELECT * FROM users WHERE name = ${maliciousInput}`
        expect(query.text).toBe('SELECT * FROM users WHERE name = ?')
        expect(query.values).toEqual([maliciousInput])
      })
    })

    it('should prevent multiple statement attacks', () => {
      // These should be caught by the stacked query detection
      const multiStatementAttacks = [
        "admin'; DROP TABLE users; SELECT 'done",
        "admin'; INSERT INTO admin VALUES ('hacker', 'pass'); SELECT 'done",
        "admin'; UPDATE users SET admin=1 WHERE username='hacker'; SELECT 'done",
        "admin'; DELETE FROM logs; SELECT 'done",
        "admin'; TRUNCATE TABLE sessions; SELECT 'done",
        "admin'; CREATE TABLE backdoor (id INT); SELECT 'done",
        "admin'; ALTER TABLE users ADD COLUMN backdoor TEXT; SELECT 'done",
      ]

      multiStatementAttacks.forEach((maliciousInput) => {
        const query = sql`SELECT * FROM users WHERE name = ${maliciousInput}`
        expect(query.text).toBe('SELECT * FROM users WHERE name = ?')
        expect(query.values).toEqual([maliciousInput])
      })
    })

    it('should safely handle complex legitimate data that might look suspicious', () => {
      const legitimateData = [
        "O'Reilly", // Common Irish name with apostrophe
        "Jean-Luc O'Connor", // Name with hyphen and apostrophe
        'SELECT * FROM my_table', // SQL-like text as data
        "UPDATE SET name = 'value'", // SQL command as content
        'User input: DROP TABLE users', // SQL injection as quoted text
        'Email: user@domain.com; Note: important', // Semicolon in legitimate context
        'Comment: /* This is a comment */', // SQL comment syntax in text
        'Formula: a = b AND c = d', // Logical operators in data
        'Path: C:\\Users\\admin\\file.txt', // File path with 'admin'
        'Version: 1.0.0 UNION compatible', // Version string with SQL keyword
      ]

      legitimateData.forEach((legitimateInput) => {
        const query = sql`SELECT * FROM users WHERE description = ${legitimateInput}`
        expect(query.text).toBe('SELECT * FROM users WHERE description = ?')
        expect(query.values).toEqual([legitimateInput])

        // Also test in INSERT context
        const insertQuery = sql`INSERT INTO posts (content) VALUES (${legitimateInput})`
        expect(insertQuery.text).toBe('INSERT INTO posts (content) VALUES (?)')
        expect(insertQuery.values).toEqual([legitimateInput])
      })
    })

    it('should demonstrate protection across different query types', () => {
      const maliciousInput = "'; DROP TABLE users; --"

      // SELECT queries
      const selectQuery = sql`SELECT * FROM users WHERE name = ${maliciousInput}`
      expect(selectQuery.text).toBe('SELECT * FROM users WHERE name = ?')
      expect(selectQuery.values).toEqual([maliciousInput])

      // INSERT queries
      const insertQuery = sql`INSERT INTO users (name) VALUES (${maliciousInput})`
      expect(insertQuery.text).toBe('INSERT INTO users (name) VALUES (?)')
      expect(insertQuery.values).toEqual([maliciousInput])

      // UPDATE queries
      const updateQuery = sql`UPDATE users SET name = ${maliciousInput} WHERE id = ${1}`
      expect(updateQuery.text).toBe('UPDATE users SET name = ? WHERE id = ?')
      expect(updateQuery.values).toEqual([maliciousInput, 1])

      // DELETE queries
      const deleteQuery = sql`DELETE FROM users WHERE name = ${maliciousInput}`
      expect(deleteQuery.text).toBe('DELETE FROM users WHERE name = ?')
      expect(deleteQuery.values).toEqual([maliciousInput])

      // Complex JOIN queries
      const joinQuery = sql`
        SELECT u.name, p.title 
        FROM users u 
        JOIN posts p ON u.id = p.user_id 
        WHERE u.name = ${maliciousInput} AND p.status = ${'active'}
      `
      expect(joinQuery.text.replace(/\s+/g, ' ').trim()).toBe(
        'SELECT u.name, p.title FROM users u JOIN posts p ON u.id = p.user_id WHERE u.name = ? AND p.status = ?',
      )
      expect(joinQuery.values).toEqual([maliciousInput, 'active'])
    })
  })

  describe('edge cases', () => {
    it('should handle empty template strings', () => {
      const query = sql``
      expect(query.text).toBe('')
      expect(query.values).toEqual([])
    })

    it('should handle only interpolation', () => {
      const value = 'test'
      const query = sql`${value}`
      expect(query.text).toBe('?')
      expect(query.values).toEqual(['test'])
    })

    it('should handle Date edge cases', () => {
      const date = new Date('2023-01-01T00:00:00')
      const query = sql`SELECT * FROM users WHERE created_at = ${date}`
      expect(query.text).toBe('SELECT * FROM users WHERE created_at = ?')
      expect(query.values).toEqual(['2023-01-01 00:00:00'])
    })

    it('should handle nested template literals', () => {
      const table = 'users'
      const condition = sql`name = ${'John'}`
      const query = sql`SELECT * FROM ${sql.ident(table)} WHERE ${condition}`
      expect(query.text).toBe('SELECT * FROM "users" WHERE name = ?')
      expect(query.values).toEqual(['John'])
    })
  })

  describe('complex queries', () => {
    it('should handle INSERT queries', () => {
      const name = 'John Doe'
      const email = 'john@example.com'
      const age = 30

      const query = sql`
        INSERT INTO users (name, email, age)
        VALUES (${name}, ${email}, ${age})
      `

      expect(query.text.trim()).toBe(
        'INSERT INTO users (name, email, age)\n        VALUES (?, ?, ?)',
      )
      expect(query.values).toEqual([name, email, age])
    })

    it('should handle UPDATE queries', () => {
      const name = 'Jane Doe'
      const age = 25
      const id = 1

      const query = sql`
        UPDATE users 
        SET name = ${name}, age = ${age}
        WHERE id = ${id}
      `

      expect(query.text.trim()).toBe(
        'UPDATE users \n        SET name = ?, age = ?\n        WHERE id = ?',
      )
      expect(query.values).toEqual([name, age, id])
    })

    it('should handle complex SELECT with JOINs', () => {
      const userId = 1
      const status = 'active'

      const query = sql`
        SELECT u.name, p.title
        FROM ${sql.ident('users')} u
        JOIN ${sql.ident('posts')} p ON u.id = p.user_id
        WHERE u.id = ${userId} AND p.status = ${status}
      `

      expect(query.text.trim()).toBe(
        'SELECT u.name, p.title\n        FROM "users" u\n        JOIN "posts" p ON u.id = p.user_id\n        WHERE u.id = ? AND p.status = ?',
      )
      expect(query.values).toEqual([userId, status])
    })
  })
})
