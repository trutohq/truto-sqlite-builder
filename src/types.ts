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
