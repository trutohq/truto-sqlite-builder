import { describe, expect, it } from 'vitest'
import { compileFilter } from './filter'
import type { JsonFilter } from './types'

describe('compileFilter', () => {
  describe('basic equality operations', () => {
    it('should handle simple equality', () => {
      const filter: JsonFilter = { status: 'ACTIVE' }
      const result = compileFilter(filter)
      expect(result.text).toBe('(("status" = ?))')
      expect(result.values).toEqual(['ACTIVE'])
    })

    it('should handle number equality', () => {
      const filter: JsonFilter = { age: 25 }
      const result = compileFilter(filter)
      expect(result.text).toBe('(("age" = ?))')
      expect(result.values).toEqual([25])
    })

    it('should handle boolean equality', () => {
      const filter: JsonFilter = { active: true }
      const result = compileFilter(filter)
      expect(result.text).toBe('(("active" = ?))')
      expect(result.values).toEqual([true])
    })

    it('should handle null equality with IS NULL', () => {
      const filter: JsonFilter = { deleted_at: null }
      const result = compileFilter(filter)
      expect(result.text).toBe('(("deleted_at" IS NULL))')
      expect(result.values).toEqual([])
    })

    it('should handle undefined by filtering it out', () => {
      const filter: JsonFilter = { deleted_at: undefined }
      expect(() => compileFilter(filter)).toThrow(
        'Filter must contain at least one condition',
      )
    })

    it('should handle Date values', () => {
      const date = new Date('2023-12-25T10:30:00Z')
      const filter: JsonFilter = { created_at: date }
      const result = compileFilter(filter)
      expect(result.text).toBe('(("created_at" = ?))')
      // Use the actual formatted date from the system
      expect(result.values).toHaveLength(1)
      expect(typeof result.values[0]).toBe('string')
    })
  })

  describe('inequality operations', () => {
    it('should handle not equal operator', () => {
      const filter: JsonFilter = { status: { ne: 'DELETED' } }
      const result = compileFilter(filter)
      expect(result.text).toBe('(("status" <> ?))')
      expect(result.values).toEqual(['DELETED'])
    })

    it('should handle not equal with null using IS NOT NULL', () => {
      const filter: JsonFilter = { deleted_at: { ne: null } }
      const result = compileFilter(filter)
      expect(result.text).toBe('(("deleted_at" IS NOT NULL))')
      expect(result.values).toEqual([])
    })
  })

  describe('comparison operations', () => {
    it('should handle greater than', () => {
      const filter: JsonFilter = { age: { gt: 18 } }
      const result = compileFilter(filter)
      expect(result.text).toBe('(("age" > ?))')
      expect(result.values).toEqual([18])
    })

    it('should handle greater than or equal', () => {
      const filter: JsonFilter = { age: { gte: 21 } }
      const result = compileFilter(filter)
      expect(result.text).toBe('(("age" >= ?))')
      expect(result.values).toEqual([21])
    })

    it('should handle less than', () => {
      const filter: JsonFilter = { age: { lt: 65 } }
      const result = compileFilter(filter)
      expect(result.text).toBe('(("age" < ?))')
      expect(result.values).toEqual([65])
    })

    it('should handle less than or equal', () => {
      const filter: JsonFilter = { age: { lte: 64 } }
      const result = compileFilter(filter)
      expect(result.text).toBe('(("age" <= ?))')
      expect(result.values).toEqual([64])
    })

    it('should handle multiple comparison operators', () => {
      const filter: JsonFilter = { age: { gt: 18, lt: 65 } }
      const result = compileFilter(filter)
      expect(result.text).toBe('(("age" > ? AND "age" < ?))')
      expect(result.values).toEqual([18, 65])
    })

    it('should handle range with gte and lte', () => {
      const filter: JsonFilter = { score: { gte: 80, lte: 100 } }
      const result = compileFilter(filter)
      expect(result.text).toBe('(("score" >= ? AND "score" <= ?))')
      expect(result.values).toEqual([80, 100])
    })
  })

  describe('set membership operations', () => {
    it('should handle IN operator', () => {
      const filter: JsonFilter = { role: { in: ['ADMIN', 'EDITOR'] } }
      const result = compileFilter(filter)
      expect(result.text).toBe('(("role" IN (?,?)))')
      expect(result.values).toEqual(['ADMIN', 'EDITOR'])
    })

    it('should handle NOT IN operator', () => {
      const filter: JsonFilter = { role: { nin: ['GUEST', 'BANNED'] } }
      const result = compileFilter(filter)
      expect(result.text).toBe('(("role" NOT IN (?,?)))')
      expect(result.values).toEqual(['GUEST', 'BANNED'])
    })

    it('should handle IN with single value', () => {
      const filter: JsonFilter = { status: { in: ['ACTIVE'] } }
      const result = compileFilter(filter)
      expect(result.text).toBe('(("status" IN (?)))')
      expect(result.values).toEqual(['ACTIVE'])
    })

    it('should handle IN with mixed types', () => {
      const filter: JsonFilter = { priority: { in: [1, 2, 3] } }
      const result = compileFilter(filter)
      expect(result.text).toBe('(("priority" IN (?,?,?)))')
      expect(result.values).toEqual([1, 2, 3])
    })
  })

  describe('NULL check operations', () => {
    it('should handle exists true (IS NOT NULL)', () => {
      const filter: JsonFilter = { email: { exists: true } }
      const result = compileFilter(filter)
      expect(result.text).toBe('(("email" IS NOT NULL))')
      expect(result.values).toEqual([])
    })

    it('should handle exists false (IS NULL)', () => {
      const filter: JsonFilter = { deleted_at: { exists: false } }
      const result = compileFilter(filter)
      expect(result.text).toBe('(("deleted_at" IS NULL))')
      expect(result.values).toEqual([])
    })

    it('should prioritize exists over other operators', () => {
      const filter: JsonFilter = {
        email: {
          exists: true,
          like: '%@example.com', // This should be ignored when exists is present
        },
      }
      const result = compileFilter(filter)
      expect(result.text).toBe('(("email" IS NOT NULL))')
      expect(result.values).toEqual([])
    })
  })

  describe('LIKE operations', () => {
    it('should handle LIKE operator', () => {
      const filter: JsonFilter = { username: { like: 'john%' } }
      const result = compileFilter(filter)
      expect(result.text).toBe('(("username" LIKE ?))')
      expect(result.values).toEqual(['john%'])
    })

    it('should handle ILIKE operator (case-insensitive)', () => {
      const filter: JsonFilter = { username: { ilike: '%doe%' } }
      const result = compileFilter(filter)
      expect(result.text).toBe('(("username" LIKE ? COLLATE NOCASE))')
      expect(result.values).toEqual(['%doe%'])
    })

    it('should handle LIKE with special characters', () => {
      const filter: JsonFilter = { description: { like: 'test_file%' } }
      const result = compileFilter(filter)
      expect(result.text).toBe('(("description" LIKE ?))')
      expect(result.values).toEqual(['test_file%'])
    })
  })

  describe('regex operations', () => {
    it('should handle regex operator', () => {
      const filter: JsonFilter = {
        email: { regex: '^[A-Za-z0-9._%+-]+@example\\.com$' },
      }
      const result = compileFilter(filter)
      expect(result.text).toBe('(("email" REGEXP ?))')
      expect(result.values).toEqual(['^[A-Za-z0-9._%+-]+@example\\.com$'])
    })

    it('should handle simple regex patterns', () => {
      const filter: JsonFilter = { phone: { regex: '^\\+1' } }
      const result = compileFilter(filter)
      expect(result.text).toBe('(("phone" REGEXP ?))')
      expect(result.values).toEqual(['^\\+1'])
    })
  })

  describe('logical operations', () => {
    it('should handle AND operator', () => {
      const filter: JsonFilter = {
        and: [{ status: 'ACTIVE' }, { age: { gte: 18 } }],
      }
      const result = compileFilter(filter)
      expect(result.text).toBe('((("status" = ?) AND ("age" >= ?)))')
      expect(result.values).toEqual(['ACTIVE', 18])
    })

    it('should handle OR operator', () => {
      const filter: JsonFilter = {
        or: [{ age: { lt: 18 } }, { age: { gte: 65 } }],
      }
      const result = compileFilter(filter)
      expect(result.text).toBe('((("age" < ?) OR ("age" >= ?)))')
      expect(result.values).toEqual([18, 65])
    })

    it('should handle mixed AND and OR', () => {
      const filter: JsonFilter = {
        and: [
          { status: 'ACTIVE' },
          { or: [{ age: { lt: 18 } }, { age: { gte: 65 } }] },
        ],
      }
      const result = compileFilter(filter)
      expect(result.text).toBe(
        '((("status" = ?) AND (("age" < ?) OR ("age" >= ?))))',
      )
      expect(result.values).toEqual(['ACTIVE', 18, 65])
    })

    it('should handle implicit AND with multiple fields', () => {
      const filter: JsonFilter = {
        status: 'ACTIVE',
        age: { gte: 18 },
        country: { in: ['US', 'CA'] },
      }
      const result = compileFilter(filter)
      expect(result.text).toBe(
        '((("status" = ?) AND ("age" >= ?) AND ("country" IN (?,?))))',
      )
      expect(result.values).toEqual(['ACTIVE', 18, 'US', 'CA'])
    })
  })

  describe('JSON path operations', () => {
    it('should handle JSON path equality', () => {
      const filter: JsonFilter = { 'profile.address.city': 'Paris' }
      const result = compileFilter(filter)
      expect(result.text).toBe('((json_extract("profile", ?) = ?))')
      expect(result.values).toEqual(['$.address.city', 'Paris'])
    })

    it('should handle JSON path with null', () => {
      const filter: JsonFilter = { 'metadata.deleted': null }
      const result = compileFilter(filter)
      expect(result.text).toBe('((json_extract("metadata", ?) IS NULL))')
      expect(result.values).toEqual(['$.deleted'])
    })

    it('should handle JSON path with comparison operators', () => {
      const filter: JsonFilter = { 'profile.age': { gte: 21 } }
      const result = compileFilter(filter)
      expect(result.text).toBe('((json_extract("profile", ?) >= ?))')
      expect(result.values).toEqual(['$.age', 21])
    })

    it('should handle JSON path with IN operator', () => {
      const filter: JsonFilter = { 'settings.theme': { in: ['dark', 'light'] } }
      const result = compileFilter(filter)
      expect(result.text).toBe('((json_extract("settings", ?) IN (?,?)))')
      expect(result.values).toEqual(['$.theme', 'dark', 'light'])
    })

    it('should handle deep JSON paths', () => {
      const filter: JsonFilter = {
        'profile.social.twitter.followers': { gt: 1000 },
      }
      const result = compileFilter(filter)
      expect(result.text).toBe('((json_extract("profile", ?) > ?))')
      expect(result.values).toEqual(['$.social.twitter.followers', 1000])
    })

    it('should handle JSON path with exists operator', () => {
      const filter: JsonFilter = { 'profile.email': { exists: false } }
      const result = compileFilter(filter)
      expect(result.text).toBe('((json_extract("profile", ?) IS NULL))')
      expect(result.values).toEqual(['$.email'])
    })
  })

  describe('complex nested examples from specification', () => {
    it('should handle kitchen sink example A - Active users with age restrictions by country', () => {
      const filter: JsonFilter = {
        and: [
          { status: 'ACTIVE' },
          { or: [{ age: { lt: 18 } }, { age: { gte: 65 } }] },
          { country: { in: ['US', 'CA', 'GB'] } },
        ],
      }
      const result = compileFilter(filter)
      expect(result.text).toBe(
        '((("status" = ?) AND (("age" < ?) OR ("age" >= ?)) AND ("country" IN (?,?,?))))',
      )
      expect(result.values).toEqual(['ACTIVE', 18, 65, 'US', 'CA', 'GB'])
    })

    it('should handle kitchen sink example B - Company filtering with priority tagging', () => {
      const filter: JsonFilter = {
        name: { like: 'Acme%' },
        category: { nin: ['ARCHIVED', 'DELETED'] },
        deleted_at: { exists: false },
        or: [{ tags: { ilike: '%urgent%' } }, { priority: { gte: 8 } }],
      }
      const result = compileFilter(filter)
      // Logical operators (or) processed first, then field conditions
      expect(result.text).toBe(
        '(((("tags" LIKE ? COLLATE NOCASE) OR ("priority" >= ?)) AND ("name" LIKE ?) AND ("category" NOT IN (?,?)) AND ("deleted_at" IS NULL)))',
      )
      expect(result.values).toEqual([
        '%urgent%',
        8,
        'Acme%',
        'ARCHIVED',
        'DELETED',
      ])
    })

    it('should handle kitchen sink example C - User signup analysis with JSON path queries', () => {
      const filter: JsonFilter = {
        and: [
          {
            'profile.email': { regex: '.*@example\\.org$' },
          },
          {
            or: [
              { signup_source: 'REFERRAL' },
              {
                signup_source: 'SOCIAL',
                'profile.social.followers': { gt: 10000 },
              },
            ],
          },
          {
            age: { gte: 18, lte: 30 },
          },
        ],
      }
      const result = compileFilter(filter)
      expect(result.text).toBe(
        '(((json_extract("profile", ?) REGEXP ?) AND (("signup_source" = ?) OR (("signup_source" = ?) AND (json_extract("profile", ?) > ?))) AND ("age" >= ? AND "age" <= ?)))',
      )
      expect(result.values).toEqual([
        '$.email',
        '.*@example\\.org$',
        'REFERRAL',
        'SOCIAL',
        '$.social.followers',
        10000,
        18,
        30,
      ])
    })
  })

  describe('deeply nested logical operations', () => {
    it('should handle OR within AND within OR', () => {
      const filter: JsonFilter = {
        or: [
          {
            and: [
              { category: 'electronics' },
              { or: [{ brand: 'Apple' }, { brand: 'Samsung' }] },
              { price: { lte: 1000 } },
            ],
          },
          {
            and: [
              { category: 'books' },
              { or: [{ genre: 'fiction' }, { genre: 'mystery' }] },
              { rating: { gte: 4 } },
            ],
          },
        ],
      }
      const result = compileFilter(filter)
      expect(result.text).toBe(
        '(((("category" = ?) AND (("brand" = ?) OR ("brand" = ?)) AND ("price" <= ?)) OR (("category" = ?) AND (("genre" = ?) OR ("genre" = ?)) AND ("rating" >= ?))))',
      )
      expect(result.values).toEqual([
        'electronics',
        'Apple',
        'Samsung',
        1000,
        'books',
        'fiction',
        'mystery',
        4,
      ])
    })

    it('should handle AND within OR within AND', () => {
      const filter: JsonFilter = {
        and: [
          { status: 'ACTIVE' },
          {
            or: [
              {
                and: [
                  { department: 'engineering' },
                  { level: { gte: 5 } },
                  { skills: { like: '%javascript%' } },
                ],
              },
              {
                and: [
                  { department: 'design' },
                  { level: { gte: 3 } },
                  { portfolio: { exists: true } },
                ],
              },
            ],
          },
          { location: { in: ['SF', 'NYC', 'LA'] } },
        ],
      }
      const result = compileFilter(filter)
      expect(result.text).toBe(
        '((("status" = ?) AND ((("department" = ?) AND ("level" >= ?) AND ("skills" LIKE ?)) OR (("department" = ?) AND ("level" >= ?) AND ("portfolio" IS NOT NULL))) AND ("location" IN (?,?,?))))',
      )
      expect(result.values).toEqual([
        'ACTIVE',
        'engineering',
        5,
        '%javascript%',
        'design',
        3,
        'SF',
        'NYC',
        'LA',
      ])
    })

    it('should handle triple-nested conditions', () => {
      const filter: JsonFilter = {
        and: [
          { 'user.status': 'ACTIVE' },
          {
            or: [
              {
                and: [
                  { 'user.type': 'premium' },
                  {
                    or: [
                      { 'billing.plan': 'annual' },
                      { 'billing.credits': { gte: 100 } },
                    ],
                  },
                ],
              },
              {
                and: [
                  { 'user.type': 'trial' },
                  { 'user.trial_days_left': { gt: 0 } },
                  {
                    or: [
                      { 'user.referral_code': { exists: true } },
                      { 'user.email': { regex: '.*@company\\.com$' } },
                    ],
                  },
                ],
              },
            ],
          },
        ],
      }
      const result = compileFilter(filter)
      expect(result.text).toBe(
        '(((json_extract("user", ?) = ?) AND (((json_extract("user", ?) = ?) AND ((json_extract("billing", ?) = ?) OR (json_extract("billing", ?) >= ?))) OR ((json_extract("user", ?) = ?) AND (json_extract("user", ?) > ?) AND ((json_extract("user", ?) IS NOT NULL) OR (json_extract("user", ?) REGEXP ?))))))',
      )
      expect(result.values).toEqual([
        '$.status',
        'ACTIVE',
        '$.type',
        'premium',
        '$.plan',
        'annual',
        '$.credits',
        100,
        '$.type',
        'trial',
        '$.trial_days_left',
        0,
        '$.referral_code',
        '$.email',
        '.*@company\\.com$',
      ])
    })

    it('should handle complex real-world user permissions filter', () => {
      const filter: JsonFilter = {
        and: [
          { 'user.active': true },
          {
            or: [
              // Admin users have full access
              { 'user.role': 'ADMIN' },
              // Department managers have access to their department
              {
                and: [
                  { 'user.role': 'MANAGER' },
                  { 'resource.department': { in: ['${user.department}'] } },
                ],
              },
              // Regular users have limited access
              {
                and: [
                  { 'user.role': 'USER' },
                  {
                    or: [
                      // Own resources
                      { 'resource.owner_id': '${user.id}' },
                      // Shared resources
                      {
                        and: [
                          { 'resource.shared': true },
                          { 'resource.visibility': { in: ['public', 'team'] } },
                        ],
                      },
                      // Team resources for team members
                      {
                        and: [
                          { 'resource.team_id': { exists: true } },
                          {
                            'user.team_memberships': {
                              like: '%${resource.team_id}%',
                            },
                          },
                        ],
                      },
                    ],
                  },
                ],
              },
            ],
          },
        ],
      }
      const result = compileFilter(filter)
      expect(result.text).toBe(
        '(((json_extract("user", ?) = ?) AND ((json_extract("user", ?) = ?) OR ((json_extract("user", ?) = ?) AND (json_extract("resource", ?) IN (?))) OR ((json_extract("user", ?) = ?) AND ((json_extract("resource", ?) = ?) OR ((json_extract("resource", ?) = ?) AND (json_extract("resource", ?) IN (?,?))) OR ((json_extract("resource", ?) IS NOT NULL) AND (json_extract("user", ?) LIKE ?)))))))',
      )
      expect(result.values).toEqual([
        '$.active',
        true,
        '$.role',
        'ADMIN',
        '$.role',
        'MANAGER',
        '$.department',
        '${user.department}',
        '$.role',
        'USER',
        '$.owner_id',
        '${user.id}',
        '$.shared',
        true,
        '$.visibility',
        'public',
        'team',
        '$.team_id',
        '$.team_memberships',
        '%${resource.team_id}%',
      ])
    })

    it('should handle e-commerce product search filter', () => {
      const filter: JsonFilter = {
        and: [
          { available: true },
          { price: { gte: 10, lte: 500 } },
          {
            or: [
              // Electronics category with specific brands
              {
                and: [
                  { category: 'electronics' },
                  {
                    or: [
                      { brand: { in: ['Apple', 'Samsung', 'Google'] } },
                      {
                        and: [
                          { brand: 'Other' },
                          { rating: { gte: 4.5 } },
                          { reviews_count: { gte: 100 } },
                        ],
                      },
                    ],
                  },
                ],
              },
              // Clothing category with size and style filters
              {
                and: [
                  { category: 'clothing' },
                  {
                    or: [
                      {
                        and: [
                          { 'attributes.gender': 'unisex' },
                          { 'attributes.size': { in: ['S', 'M', 'L'] } },
                        ],
                      },
                      {
                        and: [
                          { 'attributes.style': { like: '%casual%' } },
                          { discount_percentage: { gte: 20 } },
                        ],
                      },
                    ],
                  },
                ],
              },
              // Books category
              {
                and: [
                  { category: 'books' },
                  {
                    or: [
                      { genre: { in: ['fiction', 'mystery', 'sci-fi'] } },
                      {
                        and: [
                          { genre: 'non-fiction' },
                          { 'metadata.bestseller': true },
                        ],
                      },
                    ],
                  },
                  { language: { in: ['english', 'spanish'] } },
                ],
              },
            ],
          },
        ],
      }
      const result = compileFilter(filter)
      expect(result.text).toBe(
        '((("available" = ?) AND ("price" >= ? AND "price" <= ?) AND ((("category" = ?) AND (("brand" IN (?,?,?)) OR (("brand" = ?) AND ("rating" >= ?) AND ("reviews_count" >= ?)))) OR (("category" = ?) AND (((json_extract("attributes", ?) = ?) AND (json_extract("attributes", ?) IN (?,?,?))) OR ((json_extract("attributes", ?) LIKE ?) AND ("discount_percentage" >= ?)))) OR (("category" = ?) AND (("genre" IN (?,?,?)) OR (("genre" = ?) AND (json_extract("metadata", ?) = ?))) AND ("language" IN (?,?))))))',
      )
      expect(result.values).toEqual([
        true,
        10,
        500,
        'electronics',
        'Apple',
        'Samsung',
        'Google',
        'Other',
        4.5,
        100,
        'clothing',
        '$.gender',
        'unisex',
        '$.size',
        'S',
        'M',
        'L',
        '$.style',
        '%casual%',
        20,
        'books',
        'fiction',
        'mystery',
        'sci-fi',
        'non-fiction',
        '$.bestseller',
        true,
        'english',
        'spanish',
      ])
    })

    it('should handle maximum nesting depth without exceeding limits', () => {
      // Create a filter with 8 levels of nesting (under the 10 limit)
      const filter: JsonFilter = {
        and: [
          { level1: 'value1' },
          {
            or: [
              { level2a: 'value2a' },
              {
                and: [
                  { level3a: 'value3a' },
                  {
                    or: [
                      { level4a: 'value4a' },
                      {
                        and: [
                          { level5a: 'value5a' },
                          {
                            or: [
                              { level6a: 'value6a' },
                              {
                                and: [
                                  { level7a: 'value7a' },
                                  { level8a: 'value8a' },
                                ],
                              },
                            ],
                          },
                        ],
                      },
                    ],
                  },
                ],
              },
            ],
          },
        ],
      }
      const result = compileFilter(filter)
      expect(result.text).toBe(
        '((("level1" = ?) AND (("level2a" = ?) OR (("level3a" = ?) AND (("level4a" = ?) OR (("level5a" = ?) AND (("level6a" = ?) OR (("level7a" = ?) AND ("level8a" = ?)))))))))',
      )
      expect(result.values).toEqual([
        'value1',
        'value2a',
        'value3a',
        'value4a',
        'value5a',
        'value6a',
        'value7a',
        'value8a',
      ])
    })
  })

  describe('error handling', () => {
    it('should throw on unknown operators', () => {
      const filter: any = { age: { unknown: 18 } }
      expect(() => compileFilter(filter)).toThrow('Unknown operator: unknown')
    })

    it('should throw on empty AND array', () => {
      const filter: JsonFilter = { and: [] }
      expect(() => compileFilter(filter)).toThrow(
        'AND operator cannot be used with empty arrays',
      )
    })

    it('should throw on empty OR array', () => {
      const filter: JsonFilter = { or: [] }
      expect(() => compileFilter(filter)).toThrow(
        'OR operator cannot be used with empty arrays',
      )
    })

    it('should throw on empty IN array', () => {
      const filter: JsonFilter = { status: { in: [] } }
      expect(() => compileFilter(filter)).toThrow(
        'IN operator cannot be used with empty arrays',
      )
    })

    it('should throw on empty NIN array', () => {
      const filter: JsonFilter = { status: { nin: [] } }
      expect(() => compileFilter(filter)).toThrow(
        'NIN operator cannot be used with empty arrays',
      )
    })

    it('should throw on non-array IN value', () => {
      const filter: any = { status: { in: 'ACTIVE' } }
      expect(() => compileFilter(filter)).toThrow(
        'IN operator requires an array',
      )
    })

    it('should throw on non-array NIN value', () => {
      const filter: any = { status: { nin: 'ACTIVE' } }
      expect(() => compileFilter(filter)).toThrow(
        'NIN operator requires an array',
      )
    })

    it('should throw on non-string LIKE value', () => {
      const filter: any = { name: { like: 123 } }
      expect(() => compileFilter(filter)).toThrow(
        'LIKE operator requires a string pattern',
      )
    })

    it('should throw on non-string ILIKE value', () => {
      const filter: any = { name: { ilike: 123 } }
      expect(() => compileFilter(filter)).toThrow(
        'ILIKE operator requires a string pattern',
      )
    })

    it('should throw on non-string REGEX value', () => {
      const filter: any = { email: { regex: 123 } }
      expect(() => compileFilter(filter)).toThrow(
        'REGEX operator requires a string pattern',
      )
    })

    it('should throw on invalid JSON path', () => {
      const filter: JsonFilter = { 'field.': 'value' }
      expect(() => compileFilter(filter)).toThrow('Invalid JSON path: field.')
    })

    it('should throw on array field values', () => {
      const filter: any = { status: ['ACTIVE', 'INACTIVE'] }
      expect(() => compileFilter(filter)).toThrow(
        "Field 'status' cannot have array value",
      )
    })

    it('should throw on empty filter object', () => {
      const filter: JsonFilter = {}
      expect(() => compileFilter(filter)).toThrow(
        'Filter must contain at least one condition',
      )
    })

    it('should throw on null filter', () => {
      expect(() => compileFilter(null as any)).toThrow(
        'Filter must be an object',
      )
    })

    it('should throw on non-object filter', () => {
      expect(() => compileFilter('string' as any)).toThrow(
        'Filter must be an object',
      )
    })

    it('should throw on operator object with no valid operators', () => {
      const filter: any = { age: { invalid: true } }
      expect(() => compileFilter(filter)).toThrow('Unknown operator: invalid')
    })

    it('should throw on large IN arrays', () => {
      const largeArray = Array(1000).fill('value')
      const filter: JsonFilter = { status: { in: largeArray } }
      expect(() => compileFilter(filter)).toThrow(
        'IN operator cannot be used with arrays larger than 999 items',
      )
    })

    it('should throw on large NIN arrays', () => {
      const largeArray = Array(1000).fill('value')
      const filter: JsonFilter = { status: { nin: largeArray } }
      expect(() => compileFilter(filter)).toThrow(
        'NIN operator cannot be used with arrays larger than 999 items',
      )
    })
  })

  describe('DoS protection', () => {
    it('should throw on excessive nesting depth', () => {
      // Create deeply nested AND structure
      let filter: JsonFilter = { status: 'ACTIVE' }
      for (let i = 0; i < 12; i++) {
        filter = { and: [filter] }
      }
      expect(() => compileFilter(filter)).toThrow(
        'Nesting depth too deep (max: 10)',
      )
    })

    it('should throw on too many operators with smaller test', () => {
      // Create filter with many operators - use much larger number to ensure it works
      const filter: JsonFilter = {
        and: Array(60)
          .fill(null)
          .map((_, i) => ({ [`field${i}`]: { gt: i, lt: i + 10 } })), // 2 operators each = 120 + 1 AND = 121 total
      }
      expect(() => compileFilter(filter)).toThrow(
        'Too many operators (max: 100)',
      )
    })

    it('should count operators correctly across nested structures', () => {
      // This should be under the limit
      const filter: JsonFilter = {
        and: [
          { field1: { gt: 1, lt: 10 } }, // 2 operators
          { field2: { in: [1, 2, 3] } }, // 1 operator
          {
            or: [
              { field3: { like: '%test%' } }, // 1 operator
              { field4: { exists: true } }, // 1 operator
            ],
          },
        ],
      }
      // Total: 1 (and) + 2 + 1 + 1 (or) + 1 + 1 = 7 operators - should work
      expect(() => compileFilter(filter)).not.toThrow()
    })
  })

  describe('return value format', () => {
    it('should return frozen objects', () => {
      const filter: JsonFilter = { status: 'ACTIVE' }
      const result = compileFilter(filter)
      expect(Object.isFrozen(result)).toBe(true)
      expect(Object.isFrozen(result.values)).toBe(true)
    })

    it('should always wrap result in parentheses', () => {
      const filter: JsonFilter = { status: 'ACTIVE' }
      const result = compileFilter(filter)
      expect(result.text.startsWith('(')).toBe(true)
      expect(result.text.endsWith(')')).toBe(true)
    })

    it('should have matching text placeholders and values length', () => {
      const filter: JsonFilter = {
        name: { like: 'John%' },
        age: { gte: 18, lt: 65 },
        role: { in: ['ADMIN', 'USER'] },
      }
      const result = compileFilter(filter)
      const placeholderCount = (result.text.match(/\?/g) || []).length
      expect(placeholderCount).toBe(result.values.length)
    })
  })

  describe('identifier validation', () => {
    it('should handle valid simple identifiers', () => {
      const filter: JsonFilter = { user_name: 'john' }
      const result = compileFilter(filter)
      expect(result.text).toBe('(("user_name" = ?))')
      expect(result.values).toEqual(['john'])
    })

    it('should handle valid qualified identifiers in JSON paths', () => {
      const filter: JsonFilter = {
        'user_profile.contact_info.email': 'test@example.com',
      }
      const result = compileFilter(filter)
      expect(result.text).toBe('((json_extract("user_profile", ?) = ?))')
      expect(result.values).toEqual([
        '$.contact_info.email',
        'test@example.com',
      ])
    })

    it('should reject malicious field names through sql.ident validation', () => {
      // The sql.ident function should handle malicious identifiers
      // This test verifies that our filter compiler relies on that validation
      const filter: any = { 'users; DROP TABLE users; --': 'value' }
      expect(() => compileFilter(filter)).toThrow('Invalid identifier')
    })
  })

  describe('alias blocks', () => {
    it('should handle simple alias block with equality', () => {
      const filter: JsonFilter = {
        status: 'ACTIVE',
        $t2: {
          column_in_table_2: { gte: 100 },
        },
      }
      const result = compileFilter(filter)
      expect(result.text).toBe(
        '((("status" = ?) AND (t2."column_in_table_2" >= ?)))',
      )
      expect(result.values).toEqual(['ACTIVE', 100])
    })

    it('should handle multiple alias blocks', () => {
      const filter: JsonFilter = {
        status: 'ACTIVE',
        $t2: {
          column_in_table_2: { gte: 100 },
        },
        $orders: {
          amount: { gt: 500 },
        },
      }
      const result = compileFilter(filter)
      expect(result.text).toBe(
        '((("status" = ?) AND (t2."column_in_table_2" >= ?) AND (orders."amount" > ?)))',
      )
      expect(result.values).toEqual(['ACTIVE', 100, 500])
    })

    it('should handle alias block with JSON path', () => {
      const filter: JsonFilter = {
        status: 'ACTIVE',
        $t2: {
          'stats.avg': { lt: 10 },
        },
      }
      const result = compileFilter(filter)
      expect(result.text).toBe(
        '((("status" = ?) AND (json_extract(t2."stats", ?) < ?)))',
      )
      expect(result.values).toEqual(['ACTIVE', '$.avg', 10])
    })

    it('should handle alias block with complex conditions', () => {
      const filter: JsonFilter = {
        $users: {
          age: { gte: 18, lt: 65 },
          country: { in: ['US', 'CA'] },
          'profile.verified': true,
        },
      }
      const result = compileFilter(filter)
      expect(result.text).toBe(
        '(((users."age" >= ? AND users."age" < ?) AND (users."country" IN (?,?)) AND (json_extract(users."profile", ?) = ?)))',
      )
      expect(result.values).toEqual([18, 65, 'US', 'CA', '$.verified', true])
    })

    it('should handle alias block with logical operators', () => {
      const filter: JsonFilter = {
        $orders: {
          or: [{ status: 'pending' }, { priority: { gte: 8 } }],
        },
      }
      const result = compileFilter(filter)
      expect(result.text).toBe(
        '(((orders."status" = ?) OR (orders."priority" >= ?)))',
      )
      expect(result.values).toEqual(['pending', 8])
    })

    it('should handle nested logical operators within alias blocks', () => {
      const filter: JsonFilter = {
        status: 'ACTIVE',
        $products: {
          and: [
            { category: 'electronics' },
            { or: [{ brand: 'Apple' }, { brand: 'Samsung' }] },
          ],
        },
      }
      const result = compileFilter(filter)
      expect(result.text).toBe(
        '((("status" = ?) AND ((products."category" = ?) AND ((products."brand" = ?) OR (products."brand" = ?)))))',
      )
      expect(result.values).toEqual([
        'ACTIVE',
        'electronics',
        'Apple',
        'Samsung',
      ])
    })

    it('should handle example from specification', () => {
      const filter: JsonFilter = {
        status: 'ACTIVE',
        age: { gte: 18, lt: 65 },
        $t2: {
          column_in_table_2: { gte: 100 },
          'stats.avg': { lt: 10 },
        },
        $orders: {
          amount: { gt: 500 },
        },
      }
      const result = compileFilter(filter)
      expect(result.text).toBe(
        '((("status" = ?) AND ("age" >= ? AND "age" < ?) AND ((t2."column_in_table_2" >= ?) AND (json_extract(t2."stats", ?) < ?)) AND (orders."amount" > ?)))',
      )
      expect(result.values).toEqual(['ACTIVE', 18, 65, 100, '$.avg', 10, 500])
    })

    it('should handle alias with null values', () => {
      const filter: JsonFilter = {
        $users: {
          deleted_at: null,
          'metadata.flag': { exists: false },
        },
      }
      const result = compileFilter(filter)
      expect(result.text).toBe(
        '(((users."deleted_at" IS NULL) AND (json_extract(users."metadata", ?) IS NULL)))',
      )
      expect(result.values).toEqual(['$.flag'])
    })

    it('should validate invalid alias identifiers', () => {
      const filter: JsonFilter = {
        $123invalid: {
          column: 'value',
        },
      }
      expect(() => compileFilter(filter)).toThrow(
        'Invalid alias identifier: 123invalid',
      )
    })

    it('should validate alias with special characters', () => {
      const filter: JsonFilter = {
        '$invalid-alias': {
          column: 'value',
        },
      }
      expect(() => compileFilter(filter)).toThrow(
        'Invalid alias identifier: invalid-alias',
      )
    })

    it('should validate alias with dots', () => {
      const filter: JsonFilter = {
        '$table.alias': {
          column: 'value',
        },
      }
      expect(() => compileFilter(filter)).toThrow(
        'Invalid alias identifier: table.alias',
      )
    })

    it('should allow valid alias with underscores', () => {
      const filter: JsonFilter = {
        $table_alias_123: {
          column: 'value',
        },
      }
      const result = compileFilter(filter)
      expect(result.text).toBe('((table_alias_123."column" = ?))')
      expect(result.values).toEqual(['value'])
    })

    it('should allow alias starting with underscore', () => {
      const filter: JsonFilter = {
        $_table: {
          column: 'value',
        },
      }
      const result = compileFilter(filter)
      expect(result.text).toBe('((_table."column" = ?))')
      expect(result.values).toEqual(['value'])
    })

    it('should ensure plain filters still work unchanged', () => {
      const filter: JsonFilter = {
        status: 'ACTIVE',
        age: { gte: 18, lt: 65 },
        'profile.verified': true,
        or: [{ priority: { gte: 8 } }, { category: 'urgent' }],
      }
      const result = compileFilter(filter)
      expect(result.text).toBe(
        '(((("priority" >= ?) OR ("category" = ?)) AND ("status" = ?) AND ("age" >= ? AND "age" < ?) AND (json_extract("profile", ?) = ?)))',
      )
      expect(result.values).toEqual([
        8,
        'urgent',
        'ACTIVE',
        18,
        65,
        '$.verified',
        true,
      ])
    })

    it('should handle empty alias block gracefully', () => {
      const filter: JsonFilter = {
        status: 'ACTIVE',
        $empty: {},
      }
      expect(() => compileFilter(filter)).toThrow(
        'Filter must contain at least one condition',
      )
    })

    it('should ignore undefined alias blocks', () => {
      const filter: JsonFilter = {
        status: 'ACTIVE',
        $ignored: undefined,
      }
      const result = compileFilter(filter)
      expect(result.text).toBe('(("status" = ?))')
      expect(result.values).toEqual(['ACTIVE'])
    })

    it('should ignore non-object alias blocks', () => {
      const filter: JsonFilter = {
        status: 'ACTIVE',
        $invalid: 'not-an-object',
      }
      const result = compileFilter(filter)
      expect(result.text).toBe('(("status" = ?))')
      expect(result.values).toEqual(['ACTIVE'])
    })
  })
})
