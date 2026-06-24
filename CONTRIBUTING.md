# Contributing to sqlex

Thank you for your interest in contributing to sqlex! We welcome contributions from everyone.

## How to Contribute

### Reporting Bugs

- Open an issue with a clear title and description
- Include Go version, OS, and database driver information
- Provide a minimal reproducible example when possible

### Before Committing

Run this single command before every commit — it auto-formats, then checks formatting and lint:

```bash
make prep
```

If you just want to see what's wrong without modifying files, use `make check`.

### Submitting Changes

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/my-change`)
3. Make your changes
4. Run `make prep` to fix formatting + run checks
5. Add or update tests as needed; run `go test -v -race -count=1 ./...`
6. Commit with a clear message
7. Submit a pull request

### Code Style

- Follow standard Go conventions ([Effective Go](https://go.dev/doc/effective_go))
- Run `make prep` before committing (covers `gofmt` + `go vet` + `staticcheck`)
- Doc comments should be in English
- Keep line width ≤ 180 columns

### Testing

- All new features must include tests
- Bug fixes should include a regression test
- Unit tests should not require a database
- Integration tests require a running database; use `SQLX_*_DSN=skip` to skip specific drivers

## Development Setup

```bash
go mod download
cp .env.test.example .env.test  # Edit with your database credentials
go test ./...
```

## License

By contributing, you agree that your contributions will be licensed under the MIT License.
