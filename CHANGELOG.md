# Changelog

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
