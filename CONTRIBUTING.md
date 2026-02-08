# Contributing to TapeBackarr

Thank you for your interest in contributing to TapeBackarr! This document provides guidelines and information for contributors.

## Table of Contents

1. [Code of Conduct](#code-of-conduct)
2. [Getting Started](#getting-started)
3. [Development Setup](#development-setup)
4. [How to Contribute](#how-to-contribute)
5. [Coding Standards](#coding-standards)
6. [Testing](#testing)
7. [Submitting Changes](#submitting-changes)
8. [Reporting Issues](#reporting-issues)

## Code of Conduct

By participating in this project, you agree to maintain a respectful and inclusive environment. Please:

- Be respectful and considerate in all communications
- Accept constructive criticism gracefully
- Focus on what is best for the community
- Show empathy towards other community members

## Getting Started

### Prerequisites

- **Go 1.24+**: Backend development
- **Node.js 18+**: Frontend development
- **Git**: Version control
- **Make**: Build automation

### Repository Structure

```
TapeBackarr/
├── cmd/tapebackarr/    # Main application entry point
├── internal/           # Internal packages
│   ├── api/            # REST API handlers
│   ├── auth/           # Authentication
│   ├── backup/         # Backup service
│   ├── config/         # Configuration
│   ├── database/       # Database layer
│   ├── encryption/     # Encryption service
│   ├── logging/        # Logging utilities
│   ├── models/         # Data models
│   ├── notifications/  # Notification services
│   ├── proxmox/        # Proxmox integration
│   ├── restore/        # Restore service
│   ├── scheduler/      # Job scheduler
│   └── tape/           # Tape I/O layer
├── web/frontend/       # SvelteKit frontend
├── deploy/             # Deployment scripts and configs
├── docs/               # Documentation
└── Makefile            # Build automation
```

## Development Setup

### 1. Clone the Repository

```bash
git clone https://github.com/RoseOO/TapeBackarr.git
cd TapeBackarr
```

### 2. Install Dependencies

```bash
# Backend dependencies
go mod download

# Frontend dependencies
cd web/frontend
npm install
cd ../..
```

### 3. Development Configuration

Create a development configuration:

```bash
cp deploy/config.example.json config.json
# Edit config.json with your development settings
```

### 4. Running in Development Mode

```bash
# Run backend (in one terminal)
make dev-backend

# Run frontend (in another terminal)
make dev-frontend
```

The frontend will be available at `http://localhost:5173` and the API at `http://localhost:8080`.

### 5. Building for Production

```bash
make build
```

This creates the `tapebackarr` binary and builds the frontend.

## How to Contribute

### Types of Contributions

1. **Bug Fixes**: Fix issues and bugs
2. **Features**: Add new functionality
3. **Documentation**: Improve or add documentation
4. **Tests**: Add or improve test coverage
5. **Performance**: Optimize existing code
6. **Refactoring**: Improve code quality without changing behavior

### Contribution Workflow

1. **Fork** the repository
2. **Create a branch** for your feature/fix:
   ```bash
   git checkout -b feature/your-feature-name
   # or
   git checkout -b fix/your-bug-fix
   ```
3. **Make your changes** following our coding standards
4. **Write/update tests** as needed
5. **Run tests** to ensure they pass
6. **Commit** with clear, descriptive messages
7. **Push** to your fork
8. **Create a Pull Request**

## Coding Standards

### Go Code

- Follow the [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)
- Use `gofmt` for formatting
- Use `go vet` for static analysis
- Write doc comments for exported functions, types, and packages
- Handle errors explicitly (no silent swallowing)
- Use meaningful variable and function names

**Example:**

```go
// BackupSet represents a single backup operation
type BackupSet struct {
    ID        int64     `json:"id"`
    JobID     int64     `json:"job_id"`
    StartTime time.Time `json:"start_time"`
    // ...
}

// GetBackupSet retrieves a backup set by ID.
// Returns nil and an error if the backup set is not found.
func (s *Service) GetBackupSet(id int64) (*BackupSet, error) {
    if id <= 0 {
        return nil, ErrInvalidID
    }
    // ...
}
```

### Frontend Code (Svelte/TypeScript)

- Use TypeScript for type safety
- Follow the existing component structure
- Use meaningful component and variable names
- Keep components focused and reusable

### Commit Messages

Write clear, concise commit messages:

```
type(scope): short description

Longer description if needed.

Fixes #123
```

Types:
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation only
- `style`: Formatting, no code change
- `refactor`: Code restructuring
- `test`: Adding tests
- `chore`: Maintenance tasks

**Examples:**
```
feat(backup): add incremental backup support
fix(tape): handle tape full condition correctly
docs(api): update restore endpoint documentation
```

## Testing

### Running Tests

```bash
# Run all tests
make test

# Run tests with coverage
make test-coverage

# Run specific package tests
go test -v ./internal/backup/...
```

### Writing Tests

- Write tests for new functionality
- Update tests when modifying existing code
- Use table-driven tests where appropriate
- Mock external dependencies (tape devices, network)

**Example test:**

```go
func TestBackupService_CreateBackupSet(t *testing.T) {
    tests := []struct {
        name    string
        input   CreateBackupSetInput
        wantErr bool
    }{
        {
            name:    "valid input",
            input:   CreateBackupSetInput{JobID: 1, Type: "full"},
            wantErr: false,
        },
        {
            name:    "invalid job ID",
            input:   CreateBackupSetInput{JobID: 0, Type: "full"},
            wantErr: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Test implementation
        })
    }
}
```

## Submitting Changes

### Pull Request Guidelines

1. **Title**: Clear, descriptive title
2. **Description**: Explain what changes you made and why
3. **Testing**: Describe how you tested the changes
4. **Screenshots**: Include screenshots for UI changes
5. **Documentation**: Update documentation if needed
6. **Breaking Changes**: Clearly note any breaking changes

### Pull Request Template

```markdown
## Description
Brief description of changes.

## Type of Change
- [ ] Bug fix
- [ ] New feature
- [ ] Documentation update
- [ ] Refactoring

## Testing
- [ ] Unit tests pass
- [ ] Manual testing completed
- [ ] New tests added (if applicable)

## Checklist
- [ ] Code follows project style guidelines
- [ ] Documentation updated
- [ ] No new warnings
```

### Review Process

1. Maintainers will review your PR
2. Address any requested changes
3. Once approved, your PR will be merged

## Reporting Issues

### Bug Reports

When reporting bugs, include:

1. **Title**: Clear, specific title
2. **Environment**: OS, Go version, TapeBackarr version
3. **Steps to Reproduce**: Detailed steps to reproduce the issue
4. **Expected Behavior**: What you expected to happen
5. **Actual Behavior**: What actually happened
6. **Logs**: Relevant log output (sanitize sensitive data)
7. **Screenshots**: If applicable

### Feature Requests

When requesting features:

1. **Title**: Clear description of the feature
2. **Use Case**: Why this feature would be useful
3. **Proposed Solution**: Your ideas for implementation
4. **Alternatives**: Other solutions you've considered

### Security Issues

For security vulnerabilities, please see our [Security Policy](SECURITY.md). Do not report security issues in public GitHub issues.

## Development Tips

### Testing Without Tape Hardware

For development without physical tape hardware:

1. Use the mock tape device (coming soon)
2. Focus on API and database testing
3. Use integration tests with file-based simulation

### Debugging

```bash
# Run with debug logging
./tapebackarr -config config.json -log-level debug

# View logs
tail -f /var/log/tapebackarr/tapebackarr.log | jq
```

### Common Development Tasks

```bash
# Format code
make lint

# Run a specific test
go test -v -run TestBackupService ./internal/backup/

# Check for race conditions
go test -race ./...
```

## Questions?

If you have questions about contributing:

1. Check existing documentation
2. Search existing issues
3. Open a new issue with the "question" label

Thank you for contributing to TapeBackarr! Your help makes this project better for everyone.
