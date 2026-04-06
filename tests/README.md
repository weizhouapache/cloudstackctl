# Test Layout

The `tests/` directory holds black-box tests that only depend on exported package APIs.

Some tests intentionally remain next to the source packages under `cmd/`, `pkg/`, and `controller/` because they validate unexported helpers and internal controller methods. In Go, those white-box tests must stay in the same package directory unless the production code is refactored to export the tested internals.

Current split:
- `tests/`: exported API tests and package-level integration tests
- package-local `*_test.go`: white-box tests for unexported helpers and controller internals

Run all tests from the repository root with:

```bash
go test ./...
```

or:

```bash
make test
```

Run the same sequence used by GitHub Actions:

```bash
make ci
```

## PostgreSQL-backed controller tests

Controller tests under `tests/controller/` use PostgreSQL (matching production) and create an isolated schema per test.

By default, tests use:

```bash
host=localhost user=postgres password=secret dbname=cloudstackctl port=5432 sslmode=disable
```

Override with:

```bash
export TEST_DATABASE_DSN='host=localhost user=postgres password=secret dbname=cloudstackctl port=5432 sslmode=disable'
go test ./tests/controller/...
```

`TEST_DATABASE_DSN` takes precedence over `DATABASE_DSN`.
