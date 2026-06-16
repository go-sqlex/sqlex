# Contributing to sqlex

Thank you for your interest in contributing to sqlex! We welcome contributions from everyone.

## How to Contribute

### Reporting Bugs

- Open an issue with a clear title and description
- Include Go version, OS, and database driver information
- Provide a minimal reproducible example when possible

### Submitting Changes

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/my-change`)
3. Make your changes with clear commit messages
4. Add or update tests as needed
5. Ensure all tests pass: `go test -v -race -count=1 ./...`
6. Run linter: `make lint`
7. Run formatter: `make fmt`
8. Submit a pull request

### Code Style

- Follow standard Go conventions ([Effective Go](https://go.dev/doc/effective_go))
- Run `gofmt` and `goimports` before committing
- Doc comments should be in English
- Keep line width ≤ 180 columns

### Testing

- All new features must include tests
- Bug fixes should include a regression test
- Integration tests require a running database; use `SQLX_*_DSN=skip` to skip specific drivers

## Development Setup

```bash
go mod download
cp .env.test.example .env.test  # Edit with your database credentials
go test ./...
```

## License

By contributing, you agree that your contributions will be licensed under the MIT License.
