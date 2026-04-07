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

## Real CloudStack E2E tests

E2E tests live in `tests/e2e/` and are split into separate files:

- `unmanaged_resources_e2e_test.go`: real CloudStack unmanaged resources
- `managed_resources_e2e_test.go`: managed resources through controller endpoint

Fixtures:

- `tests/e2e/fixtures/application-full.yaml`: default managed mutation fixture

All e2e tests are skipped by default unless explicitly enabled.

Run non-destructive CloudStack smoke e2e tests:

```bash
E2E_CLOUDSTACK=true go test -v ./tests/e2e -count=1
```

Run managed-resource e2e tests against a controller endpoint:

```bash
E2E_MANAGED=true E2E_CONTROLLER_ENDPOINT=http://localhost:65426 go test -v ./tests/e2e -count=1
```

Run optional mutation tests (create/delete project):

```bash
E2E_CLOUDSTACK=true E2E_ALLOW_MUTATION=true go test -v ./tests/e2e -count=1
```

You can also use:

```bash
make e2e
```

Additional convenience targets:

```bash
# Unmanaged-only e2e tests
make e2e-unmanaged

# Managed-only e2e tests (against controller endpoint)
E2E_CONTROLLER_ENDPOINT=http://localhost:65426 make e2e-managed

# Run unmanaged + managed smoke suites together
E2E_CONTROLLER_ENDPOINT=http://localhost:65426 make e2e-all

# Run unmanaged + managed suites with mutation tests enabled
E2E_CONTROLLER_ENDPOINT=http://localhost:65426 make e2e-all-mutation
```

If you want to load CloudStack credentials from a specific env file path, set:

```bash
export E2E_CLOUDSTACK_CONFIG=/path/to/.env.cloudstack
```

Notes:

- `E2E_ALLOW_MUTATION=true` enables create/delete tests in both suites.
- Managed mutation tests create/delete `VirtualMachineSpec`, `Component`, and `Application` resources through the controller API.
- Unmanaged mutation tests include `Project`, `SecurityGroup`, `AffinityGroup`, and `UserData` flows.
- Set `E2E_PROJECT=<project-name>` to scope unmanaged mutations to a project when needed.
- Set `E2E_AFFINITY_TYPE` to override affinity group type (default: `host anti-affinity`).
- Set `E2E_MANAGED_FIXTURE` to use a custom managed YAML fixture path instead of `tests/e2e/fixtures/application-full.yaml`.
