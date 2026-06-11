/**
 * Common regex patterns and limits used across the codebase
 */

// Maximum query length (100KB)
export const MAX_QUERY_LENGTH = 102400

// Maximum length of a single identifier part (table/column/alias name).
// Bounds the amount of attacker-influenced text that can be quoted into a
// query, preventing identifier-based denial-of-service (huge field names).
export const MAX_IDENTIFIER_LENGTH = 255

// Maximum length of a LIKE/ILIKE/REGEXP pattern. These patterns are evaluated
// by the underlying SQLite engine at query time; bounding their size limits
// pathological matching cost (e.g. catastrophic backtracking in a REGEXP
// extension, or quadratic LIKE scans).
export const MAX_PATTERN_LENGTH = 1024

// Regex to detect stacked queries (semicolon followed by non-whitespace).
// Applied to a "code-only" view of the query (string literals and comments
// stripped) so that semicolons inside literals/comments do not cause false
// positives, and commented-out semicolons cannot smuggle a second statement.
export const STACKED_QUERY_REGEX = /;[\s\S]*\S/

// ANSI identifier validation - supports qualified identifiers (e.g., table.column)
export const QUALIFIED_IDENTIFIER_REGEX =
  /^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)*$/

// Simple identifier validation (for individual parts)
export const SIMPLE_IDENTIFIER_REGEX = /^[A-Za-z_][A-Za-z0-9_]*$/
