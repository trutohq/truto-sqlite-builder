# Changelog

## 2.0.0

### Major Changes

- Harden the builder against SQL injection and denial-of-service. This release
  tightens previously-unsafe behavior, so some patterns that used to "work" now
  throw:

  - **Unforgeable fragments**: only fragments minted by the library (`sql`,
    `sql.raw`, `sql.ident`, `sql.in`, `sql.blob`, `sql.join`, `compileFilter`) may
    contribute raw SQL. A plain `{ text, values }` object (e.g. from `JSON.parse`)
    is now treated as a value and throws, instead of being injected as raw SQL.
  - **`compileFilter` returns a branded fragment**: interpolate it directly into a
    `sql\`\`` template (`sql\`WHERE ${compileFilter(filter)}\``). The old
`sql.raw(compileFilter(filter).text)` pattern now throws a placeholder/value
    mismatch instead of silently producing a misaligned query.
  - **Placeholder integrity**: the `sql` tag rejects any query whose `?` count
    does not equal its bound-value count.
  - **Safe `sql.join` separators**: string separators are validated as pure
    connectors; `SqlFragment` separators are supported for parameterized joins.
  - **New limits**: identifier length (255), compiled-filter output size, and
    `like`/`ilike`/`regex` pattern length (1024).
  - Stacked-query detection is now string-literal and comment aware.

## 1.0.4

### Patch Changes

- make the compileFilter work for join tables by introducing $aliases

## 1.0.3

### Patch Changes

- fix the multi level identifiers

## 1.0.2

### Patch Changes

- Added compileFilter function which converts a mongodb like query object into SQL conditions which can be used with `WHERE`

## 1.0.1

### Patch Changes

- Update the README to use the latest npm package name

## 1.0.0

### Major Changes

- Birth of the project

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.0] - 2024-12-28

### Added

- Initial release of `truto-sqlite-builder`
- `sql` tagged template function for safe SQLite query building
- `sql.ident()` helper for safe identifier quoting
- `sql.in()` helper for safe IN clause generation
- `sql.raw()` helper for raw SQL fragments (use with caution)
- `sql.join()` helper for joining SQL fragments
- Defense-in-depth security measures:
  - Stacked query detection and prevention
  - Query length limits
  - Value type validation
- Zero dependencies (only dev dependencies)
- Full TypeScript support
- 100% test coverage
- Works in Node.js 18+ and modern browsers
