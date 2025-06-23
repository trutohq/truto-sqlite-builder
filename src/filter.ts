import { SIMPLE_IDENTIFIER_REGEX } from './constants'
import { sql } from './sql'
import type {
  ComparisonOperators,
  FieldCondition,
  FilterResult,
  JsonFilter,
} from './types'

// Limits to prevent DoS attacks
const MAX_NESTING_DEPTH = 10
const MAX_OPERATORS = 100

// Valid operators for validation
const VALID_OPERATORS = new Set([
  'gt',
  'gte',
  'lt',
  'lte',
  'ne',
  'in',
  'nin',
  'like',
  'ilike',
  'regex',
  'exists',
  'and',
  'or',
])

/**
 * Context for tracking compilation state
 */
interface CompileContext {
  depth: number
  operatorCount: number
  values: unknown[]
}

/**
 * Check if a field name represents a JSON path (contains dots)
 */
function isJsonPath(field: string): boolean {
  return field.includes('.')
}

/**
 * Convert a JSON path field to json_extract expression
 */
function compileJsonPath(field: string): {
  columnName: string
  jsonPath: string
} {
  const parts = field.split('.')
  const columnName = parts[0]
  const jsonPath = parts.slice(1)

  if (!columnName || jsonPath.length === 0 || jsonPath.some((part) => !part)) {
    throw new SyntaxError(`Invalid JSON path: ${field}`)
  }

  return {
    columnName,
    jsonPath: '$.' + jsonPath.join('.'),
  }
}

/**
 * Validate if a string is a valid SQL identifier
 */
function isValidSqlIdentifier(identifier: string): boolean {
  return SIMPLE_IDENTIFIER_REGEX.test(identifier)
}

/**
 * Compile a single field condition to SQL
 */
function compileFieldCondition(
  field: string,
  condition: FieldCondition,
  context: CompileContext,
  alias?: string,
): string {
  // Handle direct value (equality)
  if (
    condition === null ||
    condition === undefined ||
    typeof condition === 'string' ||
    typeof condition === 'number' ||
    typeof condition === 'boolean' ||
    condition instanceof Date
  ) {
    context.operatorCount++
    if (context.operatorCount > MAX_OPERATORS) {
      throw new RangeError(`Too many operators (max: ${MAX_OPERATORS})`)
    }

    if (isJsonPath(field)) {
      const { columnName, jsonPath } = compileJsonPath(field)
      const identFragment = sql.ident(columnName)
      const fullFieldExpr = alias
        ? `${alias}.${identFragment.text}`
        : identFragment.text

      if (condition === null || condition === undefined) {
        context.values.push(jsonPath)
        return `(json_extract(${fullFieldExpr}, ?) IS NULL)`
      } else {
        context.values.push(jsonPath, sql.value(condition))
        return `(json_extract(${fullFieldExpr}, ?) = ?)`
      }
    } else {
      const identFragment = sql.ident(field)
      const fullFieldExpr = alias
        ? `${alias}.${identFragment.text}`
        : identFragment.text

      if (condition === null || condition === undefined) {
        return `(${fullFieldExpr} IS NULL)`
      } else {
        context.values.push(sql.value(condition))
        return `(${fullFieldExpr} = ?)`
      }
    }
  }

  // Handle operator object
  if (typeof condition !== 'object' || condition === null) {
    throw new TypeError('Condition must be a value or operator object')
  }

  const operators = condition as ComparisonOperators
  const clauses: string[] = []

  // Validate all operators are known
  for (const op of Object.keys(operators)) {
    if (!VALID_OPERATORS.has(op)) {
      throw new SyntaxError(`Unknown operator: ${op}`)
    }
  }

  const identFragment = sql.ident(
    isJsonPath(field) ? compileJsonPath(field).columnName : field,
  )
  const baseFieldExpr = alias
    ? `${alias}.${identFragment.text}`
    : identFragment.text
  const fieldExpr = isJsonPath(field)
    ? `json_extract(${baseFieldExpr}, ?)`
    : baseFieldExpr

  // Add JSON path to values if needed
  if (isJsonPath(field)) {
    context.values.push(compileJsonPath(field).jsonPath)
  }

  // Handle exists operator first (it overrides other operators)
  if ('exists' in operators) {
    context.operatorCount++
    if (context.operatorCount > MAX_OPERATORS) {
      throw new RangeError(`Too many operators (max: ${MAX_OPERATORS})`)
    }

    return operators.exists
      ? `(${fieldExpr} IS NOT NULL)`
      : `(${fieldExpr} IS NULL)`
  }

  // Handle comparison operators
  for (const [op, value] of Object.entries(operators)) {
    context.operatorCount++
    if (context.operatorCount > MAX_OPERATORS) {
      throw new RangeError(`Too many operators (max: ${MAX_OPERATORS})`)
    }

    switch (op) {
      case 'gt':
        context.values.push(sql.value(value))
        clauses.push(`${fieldExpr} > ?`)
        break
      case 'gte':
        context.values.push(sql.value(value))
        clauses.push(`${fieldExpr} >= ?`)
        break
      case 'lt':
        context.values.push(sql.value(value))
        clauses.push(`${fieldExpr} < ?`)
        break
      case 'lte':
        context.values.push(sql.value(value))
        clauses.push(`${fieldExpr} <= ?`)
        break
      case 'ne':
        if (value === null || value === undefined) {
          clauses.push(`${fieldExpr} IS NOT NULL`)
        } else {
          context.values.push(sql.value(value))
          clauses.push(`${fieldExpr} <> ?`)
        }
        break
      case 'in': {
        if (!Array.isArray(value)) {
          throw new TypeError('IN operator requires an array')
        }
        if (value.length === 0) {
          throw new TypeError('IN operator cannot be used with empty arrays')
        }
        if (value.length > 999) {
          throw new RangeError(
            'IN operator cannot be used with arrays larger than 999 items',
          )
        }

        const inFragment = sql.in(value)
        context.values.push(...inFragment.values)
        clauses.push(`${fieldExpr} IN ${inFragment.text}`)
        break
      }
      case 'nin': {
        if (!Array.isArray(value)) {
          throw new TypeError('NIN operator requires an array')
        }
        if (value.length === 0) {
          throw new TypeError('NIN operator cannot be used with empty arrays')
        }
        if (value.length > 999) {
          throw new RangeError(
            'NIN operator cannot be used with arrays larger than 999 items',
          )
        }

        const ninFragment = sql.in(value)
        context.values.push(...ninFragment.values)
        clauses.push(`${fieldExpr} NOT IN ${ninFragment.text}`)
        break
      }
      case 'like':
        if (typeof value !== 'string') {
          throw new TypeError('LIKE operator requires a string pattern')
        }
        context.values.push(value)
        clauses.push(`${fieldExpr} LIKE ?`)
        break
      case 'ilike':
        if (typeof value !== 'string') {
          throw new TypeError('ILIKE operator requires a string pattern')
        }
        context.values.push(value)
        clauses.push(`${fieldExpr} LIKE ? COLLATE NOCASE`)
        break
      case 'regex':
        if (typeof value !== 'string') {
          throw new TypeError('REGEX operator requires a string pattern')
        }
        context.values.push(value)
        clauses.push(`${fieldExpr} REGEXP ?`)
        break
    }
  }

  if (clauses.length === 0) {
    throw new SyntaxError(
      'Operator object must contain at least one valid operator',
    )
  }

  return clauses.length === 1 ? `(${clauses[0]})` : `(${clauses.join(' AND ')})`
}

/**
 * Compile a JSON filter to SQL recursively
 */
function compileFilterRecursive(
  filter: JsonFilter,
  context: CompileContext,
  alias?: string,
): string {
  if (context.depth >= MAX_NESTING_DEPTH) {
    throw new RangeError(`Nesting depth too deep (max: ${MAX_NESTING_DEPTH})`)
  }

  if (typeof filter !== 'object' || filter === null) {
    throw new TypeError('Filter must be an object')
  }

  context.depth++
  const clauses: string[] = []

  // Handle logical operators first
  if ('and' in filter && filter.and) {
    context.operatorCount++
    if (context.operatorCount > MAX_OPERATORS) {
      throw new RangeError(`Too many operators (max: ${MAX_OPERATORS})`)
    }

    if (!Array.isArray(filter.and)) {
      throw new TypeError('AND operator must be an array')
    }
    if (filter.and.length === 0) {
      throw new TypeError('AND operator cannot be used with empty arrays')
    }

    const andClauses = filter.and.map((subFilter) =>
      compileFilterRecursive(subFilter, context, alias),
    )
    clauses.push(`(${andClauses.join(' AND ')})`)
  }

  if ('or' in filter && filter.or) {
    context.operatorCount++
    if (context.operatorCount > MAX_OPERATORS) {
      throw new RangeError(`Too many operators (max: ${MAX_OPERATORS})`)
    }

    if (!Array.isArray(filter.or)) {
      throw new TypeError('OR operator must be an array')
    }
    if (filter.or.length === 0) {
      throw new TypeError('OR operator cannot be used with empty arrays')
    }

    const orClauses = filter.or.map((subFilter) =>
      compileFilterRecursive(subFilter, context, alias),
    )
    clauses.push(`(${orClauses.join(' OR ')})`)
  }

  // Handle field conditions (implicit AND) first - preserve original order
  const fieldEntries = Object.entries(filter).filter(
    ([field, condition]) =>
      field !== 'and' &&
      field !== 'or' &&
      !field.startsWith('$') &&
      condition !== undefined,
  )

  for (const [field, condition] of fieldEntries) {
    // Handle array conditions (for 'and'/'or' at field level)
    if (Array.isArray(condition)) {
      throw new SyntaxError(
        `Field '${field}' cannot have array value. Use logical operators 'and'/'or' instead.`,
      )
    }

    clauses.push(
      compileFieldCondition(field, condition as FieldCondition, context, alias),
    )
  }

  // Handle alias blocks (keys starting with $) after regular fields
  const aliasEntries = Object.entries(filter).filter(
    ([key, value]) =>
      key.startsWith('$') &&
      value !== undefined &&
      typeof value === 'object' &&
      value !== null,
  )

  for (const [aliasKey, aliasFilter] of aliasEntries) {
    const aliasName = aliasKey.slice(1) // Remove the $ prefix

    // Validate alias name
    if (!isValidSqlIdentifier(aliasName)) {
      throw new SyntaxError(`Invalid alias identifier: ${aliasName}`)
    }

    // Recursively compile the alias block with the alias context
    const aliasClause = compileFilterRecursive(
      aliasFilter as JsonFilter,
      context,
      aliasName,
    )
    clauses.push(aliasClause)
  }

  context.depth--

  if (clauses.length === 0) {
    throw new SyntaxError('Filter must contain at least one condition')
  }

  // Fix: ensure we have at least one clause before accessing clauses[0]
  if (clauses.length === 1) {
    return clauses[0]!
  } else {
    return `(${clauses.join(' AND ')})`
  }
}

/**
 * Compile a JSON filter to a SQL WHERE clause
 */
export function compileFilter(filter: JsonFilter): FilterResult {
  const context: CompileContext = {
    depth: 0,
    operatorCount: 0,
    values: [],
  }

  const text = compileFilterRecursive(filter, context)

  return Object.freeze({
    text: `(${text})`,
    values: Object.freeze([...context.values]),
  })
}
