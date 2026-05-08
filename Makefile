GO ?= go

.PHONY: build build-cli build-controller test ci e2e e2e-unmanaged e2e-managed e2e-all e2e-all-mutation

build: build-cli build-controller

build-cli:
	$(GO) build -o cloudstackctl main.go

build-controller:
	$(GO) build -o cloudstackctl-controller cmd/controller/main.go

test:
	$(GO) test ./...

ci: build test

e2e:
	@set -a; \
	[ -f .env.cloudstack ] && . ./.env.cloudstack || true; \
	[ -f .env.database ] && . ./.env.database || true; \
	set +a; \
	E2E_CLOUDSTACK=true go test -v ./tests/e2e -count=1

e2e-unmanaged:
	@set -a; \
	[ -f .env.cloudstack ] && . ./.env.cloudstack || true; \
	[ -f .env.database ] && . ./.env.database || true; \
	set +a; \
	E2E_CLOUDSTACK=true go test -v ./tests/e2e -run Unmanaged -count=1

e2e-managed:
	@set -a; \
	[ -f .env.cloudstack ] && . ./.env.cloudstack || true; \
	[ -f .env.database ] && . ./.env.database || true; \
	set +a; \
	E2E_MANAGED=true E2E_CONTROLLER_ENDPOINT=$${E2E_CONTROLLER_ENDPOINT:-http://localhost:65426} go test -v ./tests/e2e -run Managed -count=1

e2e-all:
	@set -a; \
	[ -f .env.cloudstack ] && . ./.env.cloudstack || true; \
	[ -f .env.database ] && . ./.env.database || true; \
	set +a; \
	E2E_CLOUDSTACK=true E2E_MANAGED=true E2E_CONTROLLER_ENDPOINT=$${E2E_CONTROLLER_ENDPOINT:-http://localhost:65426} go test -v ./tests/e2e -count=1

e2e-all-mutation:
	@set -a; \
	[ -f .env.cloudstack ] && . ./.env.cloudstack || true; \
	[ -f .env.database ] && . ./.env.database || true; \
	set +a; \
	E2E_CLOUDSTACK=true E2E_MANAGED=true E2E_ALLOW_MUTATION=true E2E_CONTROLLER_ENDPOINT=$${E2E_CONTROLLER_ENDPOINT:-http://localhost:65426} go test -v ./tests/e2e -count=1