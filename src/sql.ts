import type { SqlFragment, SqlQuery, SqlValue } from './types'

// Maximum query length (100KB)
const MAX_QUERY_LENGTH = 102400

// Regex to detect stacked queries (semicolon followed by non-whitespace)
const STACKED_QUERY_REGEX = /;[\s\S]*\S/

// ANSI identifier validation - supports qualified identifiers (e.g., table.column)
const QUALIFIED_IDENTIFIER_REGEX =
  /^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)*$/

/**
 * Format a date for SQLite (YYYY-MM-DD HH:MM:SS)
 */
function formatDate(date: Date): string {
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  const hours = String(date.getHours()).padStart(2, '0')
  const minutes = String(date.getMinutes()).padStart(2, '0')
  const seconds = String(date.getSeconds()).padStart(2, '0')

  return `${year}-${month}-${day} ${hours}:${minutes}:${seconds}`
}

/**
 * Convert a value to its SQLite representation
 */
function sqlValue(value: SqlValue): unknown {
  if (value === null || value === undefined) {
    return null
  }

  if (typeof value === 'string') {
    return value
  }

  if (typeof value === 'number' || typeof value === 'boolean') {
    return value
  }

  if (value instanceof Date) {
    return formatDate(value)
  }

  if (value instanceof Buffer || value instanceof Uint8Array) {
    throw new TypeError(
      'Buffer/Uint8Array values must be used with sql.blob() for safe BLOB handling',
    )
  }

  throw new TypeError(`Unsupported value type: ${typeof value}`)
}

/**
 * Validate and quote a SQL identifier or array of identifiers/fragments
 */
function sqlIdent(
  identifier: string | readonly (string | SqlFragment)[],
): SqlFragment {
  // Handle array of identifiers and fragments
  if (Array.isArray(identifier)) {
    if (identifier.length === 0) {
      throw new TypeError('Identifier array cannot be empty')
    }

    const fragments: SqlFragment[] = []

    for (const item of identifier) {
      // Handle SqlFragment objects (like sql.raw())
      if (
        item &&
        typeof item === 'object' &&
        'text' in item &&
        'values' in item &&
        typeof (item as Record<string, unknown>).text === 'string' &&
        Array.isArray((item as Record<string, unknown>).values)
      ) {
        fragments.push(item as SqlFragment)
      } else if (typeof item === 'string') {
        // Handle string identifiers
        if (!item) {
          throw new TypeError('All identifiers must be non-empty strings')
        }

        if (!QUALIFIED_IDENTIFIER_REGEX.test(item)) {
          throw new TypeError(
            `Invalid identifier: ${item}. Must be a valid identifier or qualified identifier (e.g., table.column)`,
          )
        }

        fragments.push({
          text: '"' + item + '"',
          values: [],
        })
      } else {
        throw new TypeError('Array items must be strings or SQL fragments')
      }
    }

    // Join all fragments
    const text = fragments.map((f) => f.text).join(', ')
    const values = fragments.flatMap((f) => [...f.values])

    return {
      text,
      values,
    }
  }

  // Handle single identifier (existing behavior)
  if (!identifier || typeof identifier !== 'string') {
    throw new TypeError('Identifier must be a non-empty string')
  }

  if (!QUALIFIED_IDENTIFIER_REGEX.test(identifier)) {
    throw new TypeError(
      `Invalid identifier: ${identifier}. Must be a valid identifier or qualified identifier (e.g., table.column)`,
    )
  }

  return {
    text: '"' + identifier + '"',
    values: [],
  }
}

/**
 * Create SQL IN clause from array
 */
function sqlIn(array: readonly unknown[]): SqlFragment {
  if (!Array.isArray(array)) {
    throw new TypeError('sql.in() requires an array')
  }

  if (array.length === 0) {
    throw new TypeError('sql.in() cannot be used with empty arrays')
  }

  // Soft warning for large arrays
  if (array.length > 1000) {
    console.warn(
      `sql.in(): Large array with ${array.length} items. Consider using temporary tables for better performance.`,
    )
  }

  const placeholders = array.map(() => '?').join(',')
  const values = array.map(sqlValue)

  return {
    text: `(${placeholders})`,
    values,
  }
}

/**
 * Create raw SQL fragment (DANGEROUS - must not contain user input)
 */
function sqlRaw(rawSql: string): SqlFragment {
  if (typeof rawSql !== 'string') {
    throw new TypeError('sql.raw() requires a string')
  }

  return {
    text: rawSql,
    values: [],
  }
}

/**
 * Create SQL fragment for BLOB data (for validated binary data)
 */
function sqlBlob(data: Buffer | Uint8Array): SqlFragment {
  if (!(data instanceof Buffer) && !(data instanceof Uint8Array)) {
    throw new TypeError('sql.blob() requires a Buffer or Uint8Array')
  }

  return {
    text: '?',
    values: [data],
  }
}

/**
 * Join SQL fragments with a separator
 */
function sqlJoin(
  fragments: readonly SqlFragment[],
  separator = ', ',
): SqlFragment {
  if (!Array.isArray(fragments)) {
    throw new TypeError('sql.join() requires an array of fragments')
  }

  if (fragments.length === 0) {
    return { text: '', values: [] }
  }

  const text = fragments.map((f: SqlFragment) => f.text).join(separator)
  const values = fragments.flatMap((f: SqlFragment) => [...f.values])

  return { text, values }
}

/**
 * Main SQL tagged template function
 */
function sql(strings: TemplateStringsArray, ...values: unknown[]): SqlQuery {
  // Build the query text and collect values
  let text = strings[0] || ''
  const queryValues: unknown[] = []

  for (let i = 0; i < values.length; i++) {
    const value = values[i]

    // Handle SqlFragment objects (from helper functions)
    if (
      value &&
      typeof value === 'object' &&
      'text' in value &&
      'values' in value &&
      typeof (value as Record<string, unknown>).text === 'string' &&
      Array.isArray((value as Record<string, unknown>).values)
    ) {
      const fragment = value as SqlFragment
      text += fragment.text
      queryValues.push(...fragment.values)
    } else {
      // Regular value - add placeholder and collect value
      text += '?'
      queryValues.push(sqlValue(value as SqlValue))
    }

    text += strings[i + 1] || ''
  }

  // Security checks
  if (text.length > MAX_QUERY_LENGTH) {
    throw new Error(
      `Query too long: ${text.length} bytes (max: ${MAX_QUERY_LENGTH})`,
    )
  }

  if (STACKED_QUERY_REGEX.test(text)) {
    throw new Error('Stacked queries are not allowed')
  }

  // Return frozen result
  return Object.freeze({
    text,
    values: Object.freeze([...queryValues]),
  })
}

// Attach helper functions to sql
sql.value = sqlValue
sql.ident = sqlIdent
sql.in = sqlIn
sql.raw = sqlRaw
sql.blob = sqlBlob
sql.join = sqlJoin

export { sql }
