/**
 * Security regression tests.
 *
 * Each block corresponds to a finding from the security audit and asserts that
 * the fix holds. These tests run under vitest (node) and assert on the builder
 * output / thrown errors rather than executing SQL, so they are environment
 * independent. A separate `security-pocs/run-pocs.ts` script confirms the same
 * behavior against a real SQLite database under Bun.
 */
import { describe, expect, it } from 'vitest'
import { compileFilter, sql } from './index'

describe('security regressions', () => {
  describe('VULN-01/09: fragments are unforgeable (no structural duck typing)', () => {
    it('rejects a forged { text, values } object in a sql interpolation', () => {
      const forged = { text: '1 OR 1=1', values: [] as unknown[] }
      expect(() => sql`SELECT * FROM users WHERE id = ${forged}`).toThrow(
        'Unsupported value type: object',
      )
    })

    it('rejects a JSON.parse payload that mimics a fragment', () => {
      const payload = JSON.parse(
        '{"text":"1 UNION SELECT secret FROM users","values":[]}',
      )
      expect(() => sql`SELECT name FROM users WHERE id = ${payload}`).toThrow(
        'Unsupported value type: object',
      )
    })

    it('still composes genuine, library-minted fragments safely', () => {
      const condition = sql`active = ${true} AND name LIKE ${'a%'}`
      const query = sql`SELECT * FROM users WHERE ${condition}`
      expect(query.text).toBe(
        'SELECT * FROM users WHERE active = ? AND name LIKE ?',
      )
      expect(query.values).toEqual([true, 'a%'])
    })

    it('treats a frozen look-alike object as a value, not SQL', () => {
      const forged = Object.freeze({
        text: 'DROP TABLE users',
        values: Object.freeze([]),
      })
      expect(() => sql`${forged}`).toThrow('Unsupported value type: object')
    })
  })

  describe('VULN-02: sql.join() handles arbitrary separators safely', () => {
    it('supports any structural connector', () => {
      const frags = [sql`a = ${1}`, sql`b = ${2}`, sql`c = ${3}`]
      for (const sep of [
        ', ',
        ' AND ',
        ' OR ',
        ' UNION ALL ',
        ' AND (1=1) AND ',
      ]) {
        const joined = sql.join(frags, sep)
        expect(joined.values).toEqual([1, 2, 3])
        expect(joined.text.split(sep)).toHaveLength(3)
      }
    })

    it('rejects separators containing string/identifier literal delimiters', () => {
      const frags = [sql`a = ${1}`, sql`b = ${2}`]
      for (const sep of [" OR '", ' OR "x"', ' OR `x`', ' OR [x]']) {
        expect(() => sql.join(frags, sep)).toThrow(
          'Unsafe sql.join() separator',
        )
      }
    })

    it('rejects separators containing comments, terminators, NUL or backslash', () => {
      const frags = [sql`a = ${1}`, sql`b = ${2}`]
      for (const sep of [' -- ', ' /* x */ ', '; ', ' \\ ', ' \0 ', ' */ ']) {
        expect(() => sql.join(frags, sep)).toThrow(
          'Unsafe sql.join() separator',
        )
      }
    })

    it('rejects separators with unbalanced parentheses', () => {
      const frags = [sql`a = ${1}`, sql`b = ${2}`]
      expect(() => sql.join(frags, ') OR (1=1) -- ')).toThrow(
        'Unsafe sql.join() separator',
      )
      expect(() => sql.join(frags, ' OR (')).toThrow('unbalanced parentheses')
      expect(() => sql.join(frags, ' )')).toThrow('unbalanced parentheses')
    })

    it('supports a SqlFragment separator with parameterized values', () => {
      const frags = [sql`a = ${1}`, sql`b = ${2}`]
      const joined = sql.join(frags, sql` OR flag = ${true} OR `)
      expect(joined.text).toBe('a = ? OR flag = ? OR b = ?')
      expect(joined.values).toEqual([1, true, 2])
    })

    it('rejects non-fragment array items', () => {
      expect(() => sql.join([{ text: 'x', values: [] } as never])).toThrow(
        'sql.join() requires SQL fragments',
      )
    })

    it('rejects a separator that is neither string nor fragment', () => {
      const frags = [sql`a = ${1}`, sql`b = ${2}`]
      expect(() => sql.join(frags, 123 as never)).toThrow(
        'separator must be a string or a SQL fragment',
      )
    })
  })

  describe('VULN-03/04: placeholder integrity catches raw-fragment smuggling', () => {
    it('throws when a raw fragment smuggles a stray placeholder', () => {
      expect(
        () => sql`SELECT * FROM users WHERE name = ${sql.raw('? OR 1=1 --')}`,
      ).toThrow(
        /Placeholder count \(1\) does not match bound value count \(0\)/,
      )
    })

    it('throws when raw SQL contains placeholders without supplying values', () => {
      expect(
        () => sql`WHERE a = ${sql.raw('?')} AND b = ${sql.raw('?')}`,
      ).toThrow(/Placeholder count/)
    })

    it('still allows legitimate raw SQL without placeholders', () => {
      const query = sql`SELECT * FROM users WHERE created_at > ${sql.raw('CURRENT_TIMESTAMP')}`
      expect(query.text).toBe(
        'SELECT * FROM users WHERE created_at > CURRENT_TIMESTAMP',
      )
      expect(query.values).toEqual([])
    })
  })

  describe('VULN-05: sql.ident() cannot be tricked by forged fragments', () => {
    it('rejects a forged fragment inside an identifier array', () => {
      const forged = { text: 'secret AS name', values: [] as unknown[] }
      expect(() => sql.ident(['id', forged as never])).toThrow(
        'Array items must be strings or SQL fragments',
      )
    })

    it('still allows explicit, developer-authored sql.raw expressions', () => {
      const fragment = sql.ident(['id', sql.raw('COUNT(*) as total')])
      expect(fragment.text).toBe('"id", COUNT(*) as total')
      expect(fragment.values).toEqual([])
    })
  })

  describe('VULN-06: identifier-length and output-size DoS limits', () => {
    it('rejects an oversized identifier in sql.ident()', () => {
      expect(() => sql.ident('a'.repeat(256))).toThrow(
        /Identifier part too long/,
      )
    })

    it('rejects an oversized field name in compileFilter()', () => {
      expect(() => compileFilter({ ['a'.repeat(256)]: 1 })).toThrow(
        /Identifier part too long/,
      )
    })

    it('rejects an oversized alias in compileFilter()', () => {
      expect(() =>
        compileFilter({ ['$' + 'a'.repeat(256)]: { x: 1 } }),
      ).toThrow('Invalid alias identifier')
    })

    it('enforces a maximum compiled-filter length (backstop)', () => {
      const inValues = Array.from({ length: 999 }, (_, i) => String(i))
      const subFilters = Array.from({ length: 60 }, (_, i) => ({
        [`field_${i}`]: { in: inValues },
      }))
      expect(() => compileFilter({ and: subFilters })).toThrow(
        /Compiled filter too long/,
      )
    })
  })

  describe('VULN-07: compileFilter result interpolates safely without sql.raw', () => {
    it('returns a branded fragment usable directly in a sql template', () => {
      const where = compileFilter({ name: 'alice', age: { gte: 18 } })
      const query = sql`SELECT * FROM users WHERE ${where}`
      expect(query.text).toBe(
        'SELECT * FROM users WHERE ((("name" = ?) AND ("age" >= ?)))',
      )
      // Values now travel with the fragment and are correctly aligned.
      expect(query.values).toEqual(['alice', 18])
      const placeholders = (query.text.match(/\?/g) ?? []).length
      expect(placeholders).toBe(query.values.length)
    })

    it('the old footgun (sql.raw of the filter text) now fails closed', () => {
      const where = compileFilter({ name: 'alice' })
      // Dropping the values via sql.raw is now caught by the integrity check
      // instead of silently producing a query with an unbound placeholder.
      expect(
        () => sql`SELECT * FROM users WHERE ${sql.raw(where.text)}`,
      ).toThrow(/Placeholder count/)
    })

    it('composes with additional interpolated values in the right order', () => {
      const where = compileFilter({ status: 'active' })
      const query = sql`SELECT * FROM users WHERE ${where} LIMIT ${10}`
      expect(query.text).toBe(
        'SELECT * FROM users WHERE (("status" = ?)) LIMIT ?',
      )
      expect(query.values).toEqual(['active', 10])
    })
  })

  describe('VULN-08: stacked-query detection is literal/comment aware', () => {
    it('still rejects genuine multi-statement queries', () => {
      expect(() => sql`SELECT 1; DROP TABLE users`).toThrow(
        'Stacked queries are not allowed',
      )
    })

    it('does not flag a semicolon inside a string literal', () => {
      const query = sql`SELECT ${sql.raw("';'")} AS s`
      expect(query.text).toBe("SELECT ';' AS s")
      expect(query.values).toEqual([])
    })

    it('does not flag a semicolon hidden inside a comment', () => {
      const query = sql`SELECT 1 ${sql.raw('/* ; DROP TABLE x */')}`
      expect(query.text).toBe('SELECT 1 /* ; DROP TABLE x */')
      expect(query.values).toEqual([])
    })

    it('allows a single trailing semicolon (single statement)', () => {
      const query = sql`SELECT 1;`
      expect(query.text).toBe('SELECT 1;')
    })
  })

  describe('VULN-10: LIKE/ILIKE/REGEXP pattern length is bounded', () => {
    it('rejects oversized patterns for each operator', () => {
      const huge = 'a'.repeat(1025)
      expect(() => compileFilter({ name: { like: huge } })).toThrow(
        /LIKE pattern too long/,
      )
      expect(() => compileFilter({ name: { ilike: huge } })).toThrow(
        /ILIKE pattern too long/,
      )
      expect(() => compileFilter({ name: { regex: huge } })).toThrow(
        /REGEX pattern too long/,
      )
    })

    it('allows patterns within the limit', () => {
      const ok = 'a'.repeat(1024)
      expect(() => compileFilter({ name: { like: ok } })).not.toThrow()
    })
  })

  describe('scanSql literal/comment handling', () => {
    it('skips placeholders and structure inside all literal kinds', () => {
      // single-quote with doubled-escape, backtick and bracket identifiers,
      // and a line comment: none contribute placeholders.
      const query = sql`SELECT ${sql.raw("'a''b?'")}, ${sql.raw('`c?`')}, ${sql.raw('[d?]')} ${sql.raw('-- ? trailing')}`
      expect(query.values).toEqual([])
      expect(query.text).toContain("'a''b?'")
    })
  })
})
