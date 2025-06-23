/**
 * Common regex patterns used across the codebase
 */

// Maximum query length (100KB)
export const MAX_QUERY_LENGTH = 102400

// Regex to detect stacked queries (semicolon followed by non-whitespace)
export const STACKED_QUERY_REGEX = /;[\s\S]*\S/

// ANSI identifier validation - supports qualified identifiers (e.g., table.column)
export const QUALIFIED_IDENTIFIER_REGEX =
  /^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)*$/

// Simple identifier validation (for individual parts)
export const SIMPLE_IDENTIFIER_REGEX = /^[A-Za-z_][A-Za-z0-9_]*$/
