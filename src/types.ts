/**
 * The result of a SQL tagged template
 */
export interface SqlQuery {
  readonly text: string
  readonly values: readonly unknown[]
}

/**
 * Valid SQL value types that can be safely interpolated
 */
export type SqlValue =
  | string
  | number
  | boolean
  | null
  | undefined
  | Date
  | Buffer
  | Uint8Array

/**
 * A SQL fragment that can be joined with other fragments
 */
export interface SqlFragment {
  readonly text: string
  readonly values: readonly unknown[]
}

/**
 * Comparison operators for JSON filters
 */
export interface ComparisonOperators {
  /** Greater than */
  gt?: SqlValue
  /** Greater than or equal */
  gte?: SqlValue
  /** Less than */
  lt?: SqlValue
  /** Less than or equal */
  lte?: SqlValue
  /** Not equal */
  ne?: SqlValue
  /** IN array */
  in?: readonly SqlValue[]
  /** NOT IN array */
  nin?: readonly SqlValue[]
  /** LIKE pattern */
  like?: string
  /** Case-insensitive LIKE */
  ilike?: string
  /** Regular expression pattern */
  regex?: string
  /** Field exists check (true = IS NOT NULL, false = IS NULL) */
  exists?: boolean
}

/**
 * A field condition can be a direct value or an object with operators
 */
export type FieldCondition = SqlValue | ComparisonOperators

/**
 * Logical operators for combining filters
 */
export interface LogicalOperators {
  /** All conditions must match */
  and?: readonly JsonFilter[]
  /** Any condition must match */
  or?: readonly JsonFilter[]
}

/**
 * A JSON filter for compiling to SQL WHERE clauses
 * Can be a field-to-condition mapping with implicit AND, explicit logical operators,
 * or alias blocks (keys starting with $) containing nested filters
 */
export type JsonFilter = LogicalOperators & {
  [field: string]:
    | FieldCondition
    | readonly JsonFilter[]
    | JsonFilter
    | undefined
}

/**
 * Result of compiling a JSON filter to SQL
 */
export interface FilterResult {
  readonly text: string
  readonly values: readonly unknown[]
}
