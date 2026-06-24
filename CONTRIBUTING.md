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
6. Update [CHANGELOG.md](CHANGELOG.md) with a brief note under the next version
7. Commit with a clear message
8. Submit a pull request

### Code Style

- Follow standard Go conventions ([Effective Go](https://go.dev/doc/effective_go))
- Run `make prep` before committing (covers `gofmt` + `go vet` + `staticcheck`)
- Doc comments should be in English
- Keep line width ≤ 180 columns

### Testing

All new features must include tests. Bug fixes should include a regression test.

#### Quick commands

| Command | What it does |
|---------|-------------|
| `make test` | Full test suite with race detection |
| `make test-cover` | Full test + coverage HTML report in `cover/` |
| `FUNC=TestName make test-func` | Run a single test function |

> See all available commands with `make help`.

#### Unit Tests (no database)

Root-level `*_test.go` files test isolated logic (lexer, bind, hook, named parameters). No database required:

```bash
go test -v -count=1 .
```

#### Integration Tests (requires database)

Tests under `tests/cross_db/` exercise real database drivers. Set up credentials first:

```bash
cp .env.test.example .env.test   # edit with your credentials
go test -v -count=1 ./tests/cross_db/
```

To skip a driver you don't have running, set `skip` in `.env.test`:

```bash
SQLX_POSTGRES_DSN=skip
SQLX_SQLSERVER_DSN=skip
```

Or inline:

```bash
SQLX_POSTGRES_DSN=skip SQLX_SQLSERVER_DSN=skip go test -v -count=1 ./tests/cross_db/
```

#### Race Detection

Always run race tests before submitting:

```bash
make test
```

#### Running a Single Test

```bash
FUNC=TestDB_Queryx make test-func
```

#### Coverage

```bash
make test-cover
open cover/cover.html
```

## Development Setup

```bash
go mod download
cp .env.test.example .env.test  # Edit with your database credentials
go test ./...
```

## License

By contributing, you agree that your contributions will be licensed under the MIT License.
