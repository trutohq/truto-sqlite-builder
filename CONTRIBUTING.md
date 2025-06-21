# Contributing to truto-sqlite-builder

Thank you for your interest in contributing to `truto-sqlite-builder`! This document provides guidelines and information for contributors.

## 🚀 Getting Started

### Prerequisites

- Node.js 18 or higher
- npm, yarn, pnpm, or bun
- Git

### Development Setup

1. **Fork and clone the repository:**

   ```bash
   git clone https://github.com/YOUR_USERNAME/truto-sqlite-builder.git
   cd truto-sqlite-builder
   ```

2. **Install dependencies:**

   ```bash
   npm install
   ```

3. **Run tests to ensure everything works:**

   ```bash
   npm test
   ```

4. **Start development mode:**
   ```bash
   npm run dev  # Runs tests in watch mode
   ```

## 🏗️ Project Structure

```
truto-sqlite-builder/
├── src/                 # Source code
│   ├── index.ts        # Main entry point
│   ├── sql.ts          # Core SQL tagged template implementation
│   ├── types.ts        # TypeScript type definitions
│   └── sql.test.ts     # Test suite
├── bench/              # Benchmarking suite
├── examples/           # Usage examples
├── .github/            # GitHub Actions workflows
└── dist/              # Built output (generated)
```

## 🧪 Development Workflow

### Running Tests

```bash
# Run all tests
npm test

# Run tests in watch mode
npm run dev

# Run tests with coverage
npm run test:coverage

# Run tests with UI
npm run test:ui
```

### Code Quality

```bash
# Type checking
npm run typecheck

# Linting
npm run lint
npm run lint:fix

# Formatting
npm run format
npm run format:check
```

### Building

```bash
# Build the project
npm run build
```

## 📝 Writing Code

### Code Style

- We use **Prettier** for code formatting
- We use **ESLint** with security plugins for linting
- Follow existing code patterns and conventions
- Write clear, self-documenting code

### TypeScript Guidelines

- Use strict TypeScript settings
- Provide proper type annotations
- Avoid `any` types when possible
- Use readonly types for immutable data

### Security Considerations

This is a security-focused library. When contributing:

- **Never introduce SQL injection vulnerabilities**
- **Validate all user inputs**
- **Use parameterized queries exclusively**
- **Test security features thoroughly**
- **Consider edge cases and attack vectors**

## 🧪 Testing Guidelines

### Test Coverage

- We maintain **100% test coverage**
- All new code must include comprehensive tests
- Tests should cover both happy paths and error cases

### Test Structure

```typescript
describe('feature name', () => {
  describe('specific functionality', () => {
    it('should do something specific', () => {
      // Arrange
      const input = 'test input';

      // Act
      const result = sql`SELECT * FROM users WHERE name = ${input}`;

      // Assert
      expect(result.text).toBe('SELECT * FROM users WHERE name = ?');
      expect(result.values).toEqual(['test input']);
    });
  });
});
```

### Security Tests

Always include tests for:

- SQL injection attempts
- Edge cases with special characters
- Invalid inputs
- Boundary conditions

## 📋 Pull Request Process

### Before Submitting

1. **Create a branch** from `main`:

   ```bash
   git checkout -b feature/your-feature-name
   ```

2. **Make your changes** following the guidelines above

3. **Run the full test suite:**

   ```bash
   npm run typecheck
   npm run lint
   npm test
   npm run build
   ```

4. **Update documentation** if needed

5. **Add a changeset** for your changes:
   ```bash
   npx changeset
   ```

### PR Requirements

- [ ] All tests pass
- [ ] Code coverage remains at 100%
- [ ] No linting errors
- [ ] TypeScript compiles without errors
- [ ] Documentation updated (if applicable)
- [ ] Changeset added (for user-facing changes)

### PR Description Template

```markdown
## Description

Brief description of what this PR accomplishes.

## Type of Change

- [ ] Bug fix (non-breaking change that fixes an issue)
- [ ] New feature (non-breaking change that adds functionality)
- [ ] Breaking change (fix or feature that would cause existing functionality to change)
- [ ] Documentation update

## Testing

- [ ] I have added tests that prove my fix is effective or that my feature works
- [ ] New and existing unit tests pass locally with my changes
- [ ] I have tested edge cases and error conditions

## Security

- [ ] I have considered security implications of my changes
- [ ] I have not introduced any SQL injection vulnerabilities
- [ ] I have validated all user inputs appropriately

## Checklist

- [ ] My code follows the style guidelines of this project
- [ ] I have performed a self-review of my code
- [ ] I have commented my code, particularly in hard-to-understand areas
- [ ] I have made corresponding changes to the documentation
- [ ] My changes generate no new warnings
- [ ] I have added tests that prove my fix is effective or that my feature works
- [ ] New and existing unit tests pass locally with my changes
```

## 🐛 Reporting Issues

### Security Issues

**Do not create public issues for security vulnerabilities.** Instead, email `security@truto.com` with:

- Description of the vulnerability
- Steps to reproduce
- Potential impact
- Suggested fix (if you have one)

### Bug Reports

For non-security bugs, please create an issue with:

- **Clear title** describing the problem
- **Environment details** (Node.js version, OS, etc.)
- **Reproduction steps** with minimal code example
- **Expected vs actual behavior**
- **Any error messages or logs**

### Feature Requests

For new features:

- **Use case** description
- **Proposed API** or interface
- **Why this should be part of the core library**
- **Alternatives considered**

## 🎯 Areas for Contribution

We welcome contributions in these areas:

- **Performance optimizations**
- **Additional security features**
- **Better error messages**
- **Documentation improvements**
- **Example applications**
- **Integration guides**

## 📚 Resources

- [SQLite Documentation](https://sqlite.org/docs.html)
- [SQL Injection Prevention](https://owasp.org/www-community/attacks/SQL_Injection)
- [TypeScript Handbook](https://www.typescriptlang.org/docs/)
- [Vitest Documentation](https://vitest.dev/)

## 📄 License

By contributing, you agree that your contributions will be licensed under the MIT License.

## 🤝 Code of Conduct

This project follows a standard code of conduct:

- Be respectful and inclusive
- Focus on constructive feedback
- Help maintain a welcoming environment
- Report unacceptable behavior

## 💬 Getting Help

- **GitHub Issues**: For bugs and feature requests
- **GitHub Discussions**: For questions and general discussion
- **Email**: For security issues or private concerns

Thank you for contributing to `truto-sqlite-builder`! 🎉
