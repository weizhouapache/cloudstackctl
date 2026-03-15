# cloudstackctl
Kubernetes-style declarative orchestration tool for Apache CloudStack

## Build & Install

```bash
# Build binary for CLI
go build -o cloudstackctl main.go

# Build binary for Controller (if run locally)
go build -o cloudstackctl-controller cmd/controller

# Install to PATH
sudo mv cloudstackctl /usr/local/bin/
```

## Build container image

The Docker image is used by both the local `docker-compose` flow and the
`kind`-based Option 2. Build the local image once before running either option:

```bash
# Build local Docker image (used by Option 1 and Option 2)
sudo docker build -t cloudstackctl:local .
```

## Controller-managed tables and migration

The controller is responsible for creating and migrating the database tables used
for persisted, reconciled resources. These tables are managed by the controller
at startup and use explicit, stable table names to make queries predictable.

- `applications`: stores `Application` resources
- `components`: stores `Component` resources
- `vm_specs`: stores reusable `VirtualMachineSpec` resources (`VirtualMachineSpecResource`)
- `virtual_machines`: stores `VirtualMachine` desired/observed records

Note: CloudStack-managed resources such as `Network`, `Volume`, `SSHKey`,
`SecurityGroup`, `AffinityGroup`, and `UserData` are *not* persisted in the
database — they are managed directly via the CloudStack API/SDK. The controller
creates/migrates the application/component/vmspec/vm tables on startup (idempotent).

If you need explicit migration/version control, add migration tooling or
database migration files; the current approach relies on GORM's `AutoMigrate`.

## CloudStack Credentials

cloudstackctl expects CloudStack credentials in environment variables or (when
running in Kubernetes) as a Secret. Canonical environment variables used by
the code are:

- `CLOUDSTACK_ENDPOINT` — CloudStack API URL (e.g. https://cloudstack.example.com/client/api)
- `CLOUDSTACK_API_KEY`
- `CLOUDSTACK_SECRET_KEY`
- `VERIFY_SSL` (optional, default: true)

You can set these directly, or copy the provided example env files and edit values when running with Docker or `docker-compose`.

CloudStack env example file (copy to `.env.cloudstack` and fill values):

```env
# CloudStack env (example). Copy to .env.cloudstack and fill values.
CLOUDSTACK_ENDPOINT=http://10.20.30.40:8080/client/api
CLOUDSTACK_API_KEY=
CLOUDSTACK_SECRET_KEY=
VERIFY_SSL=false
```

Notes:
- When running the controller in-cluster the controller will try to load the
	Secret named by `CLOUDSTACK_SECRET_NAME` (default `cloudstack-credentials`)
	in the namespace set by `CLOUDSTACK_SECRET_NAMESPACE` (default `default`).
- For local `docker-compose` development, copy and edit the example files:

```bash
cp .env.cloudstack.example .env.cloudstack
cp .env.database.example .env.database
# edit .env.cloudstack and .env.database with your real values
```

These files are ignored by git.

Tip: the CLI supports `-c / --cloudstack-config <file>` to load CloudStack credentials from a KEY=VALUE file; when not provided the code will use `.env.cloudstack` if it exists.

Modes and CLI behavior
---------------------

- `--standalone | -s` : Run the CLI in standalone mode. In this mode the CLI will not read from or write to the PostgreSQL database and will interact directly with CloudStack using the SDK. Standalone mode is useful for quick ad-hoc operations or environments where running the controller/postgres is inconvenient.

	- Unsupported in standalone: `Application`, `Component`, and `VirtualMachineSpec`. These higher-level constructs require the controller and DB.
	- Supported in standalone: direct CloudStack resource kinds such as `VirtualMachine`, `Network`, `Volume`, `SSHKey`, `SecurityGroup`, `AffinityGroup`, and `UserData` (subject to SDK coverage).

- Default behavior (controller mode): when `-s` is not specified the CLI assumes controller mode and operations are intended to be persisted to the database and reconciled by the controller. Resources are managed via the database/controller.

Examples:

```bash
# Standalone: list VMs directly from CloudStack
./cloudstackctl -s get VirtualMachine

# Standalone: deploy a VM from YAML directly via CloudStack SDK
./cloudstackctl -s apply -f standalone-vm.yaml

# Cluster mode (default): apply a resource that will be persisted and reconciled
./cloudstackctl apply -f application.yaml
```

CLI flags: `--all / -A`
------------------------

Two commands support an `--all` (short `-A`) flag in controller mode to query CloudStack directly rather than the controller DB:

- `get VirtualMachine -A` — list all VMs from CloudStack (including unmanaged VMs); without `-A` the controller returns only VMs persisted in the DB (managed by cloudstackctl).
- `describe <Kind> <name> -A` — describe the named resource by querying CloudStack directly rather than using controller-managed state.

Examples:

```bash
# Cluster mode: list only managed VMs (default)
./cloudstackctl get VirtualMachine

# Cluster mode: list all VMs from CloudStack (include unmanaged)
./cloudstackctl get VirtualMachine -A

# Describe a VM from CloudStack directly
./cloudstackctl describe VirtualMachine my-vm -A
```

## PostgreSQL environment variables

`cloudstackctl` reads the database connection from `DATABASE_DSN` if provided, or
it assembles a DSN from the following environment variables (with defaults shown):

- `PGHOST` (default: `localhost`)
- `PGUSER` (default: `postgres`)
- `PGPASSWORD` (default: `secret`)
- `PGDATABASE` (default: `cloudstackctl`)
- `PGPORT` (default: `5432`)
- `PGSSLMODE` (default: `disable`)

Examples:

```bash
# preferred: single DSN
export DATABASE_DSN="host=localhost user=postgres password=secret dbname=cloudstackctl port=5432 sslmode=disable"

# or set individual PG* variables
export PGHOST=localhost
export PGUSER=postgres
export PGPASSWORD=secret
export PGDATABASE=cloudstackctl
export PGPORT=5432
export PGSSLMODE=disable
```


## Option 1 — All in containers (Docker)

You can run `cloudstackctl` components as standalone Docker containers. This is
fast for local development and does not require Kubernetes.

1) Run with `docker-compose` (example `docker-compose.yml` at project root):

```bash
sudo docker-compose up
```

This starts `postgres` and `cloudstackctl-controller` services. The image runs the
`cloudstackctl` binary; the controller process runs as `cloudstackctl-controller`.
Environment variables are passed via the compose file; secrets can be provided via env or mounted files.

2) Alternatively, run containers individually:

```bash
# Postgres
sudo docker run -d --name cloudstackctl-postgres -p 5432:5432 \
	-e POSTGRES_PASSWORD=secret -e POSTGRES_DB=cloudstackctl postgres:15

# Controller (example)
sudo docker run -d --name cloudstackctl-controller --link cloudstackctl-postgres:postgres \
	-e DATABASE_DSN='host=postgres user=postgres password=secret dbname=cloudstackctl port=5432 sslmode=disable' \
	-e CLOUDSTACK_ENDPOINT='https://your-cloudstack-api.com/client/api' -e CLOUDSTACK_API_KEY=your-api-key -e CLOUDSTACK_SECRET_KEY=your-secret-key \
	cloudstackctl:local /cloudstackctl-controller
```


3) Run containers using env files (`.env.cloudstack` and `.env.database`)

Copy the examples and edit values first:

```bash
cp .env.cloudstack.example .env.cloudstack
cp .env.database.example .env.database
# edit .env.cloudstack and .env.database with your real values
```

Then run Postgres and Controller using the env files:

```bash
# Postgres
sudo docker run -d --name cloudstackctl-postgres -p 5432:5432 \
	-e POSTGRES_PASSWORD=secret -e POSTGRES_DB=cloudstackctl postgres:15

# Controller using .env files
sudo docker run -d --name cloudstackctl-controller --link cloudstackctl-postgres:postgres -p 65426:65426 \
	--env-file .env.cloudstack --env-file .env.database cloudstackctl:local /cloudstackctl-controller
```

Notes:
- Use bind mounts or Docker secrets if you prefer not to expose secrets in env vars.

## Option 2 — Use `kind` (Kubernetes)

If you want to test Kubernetes behaviors (Secret mounts, Services, pod networking),
deploy into a `kind` cluster. The commands below install `kind` and `kubectl`
(if missing) and create a cluster.

```bash
# install kind (Linux x86_64) - download the latest release
curl -Lo ./kind https://kind.sigs.k8s.io/dl/latest/kind-linux-amd64
chmod +x ./kind
sudo mv ./kind /usr/local/bin/kind

# install kubectl (if not already installed)
curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"
chmod +x kubectl
sudo mv kubectl /usr/local/bin/

# create a kind cluster
kind create cluster --name cloudstackctl

# verify cluster
kubectl cluster-info --context kind-cloudstackctl

# Create Kubernetes Secrets from the example env files (Option 2)
# Copy the example files, edit them with real values, then create Secrets:
```
Copy example files to editable filenames
```bash
cp .env.cloudstack.example .env.cloudstack
cp .env.database.example .env.database

# Edit `.env.cloudstack` and `.env.database` with real values, then create Secrets
kubectl create secret generic cloudstack-secret --from-env-file=.env.cloudstack
kubectl create secret generic database-secret --from-env-file=.env.database
```

Notes:
- Ensure Docker is running locally before creating the `kind` cluster.
- You can delete the cluster with `kind delete cluster --name cloudstackctl`.

Example manifests are in `examples/k8s/`:

- `examples/k8s/cloudstack-secret.yaml` — CloudStack Secret (stringData keys: `CLOUDSTACK_API_KEY`, `CLOUDSTACK_SECRET_KEY`, `CLOUDSTACK_ENDPOINT`, `VERIFY_SSL`)
- `examples/k8s/database-secret.yaml` — Database Secret (stringData keys: `DATABASE_DSN` or PG* values)
- `examples/k8s/postgres-deployment.yaml` — Postgres Deployment + Service
-- `examples/k8s/api-deployment.yaml` — API Deployment + Service (removed)
- `examples/k8s/controller-deployment.yaml` — Controller Deployment (controller reads Secret via Kubernetes API or via `envFrom`)

Deploy with:

```bash
kubectl apply -f examples/k8s/cloudstack-secret.yaml
kubectl apply -f examples/k8s/database-secret.yaml
kubectl apply -f examples/k8s/postgres-deployment.yaml
kubectl apply -f examples/k8s/controller-deployment.yaml
```

Verify pods and logs:

```bash
kubectl get pods -n default
kubectl logs deployment/cloudstackctl-controller -n default
```


Notes:
- All settings are treated as sensitive in this project — use Kubernetes `Secret`s rather than `ConfigMap`s.
- The recommended approach is to inject values as environment variables using `envFrom` from one or more Secrets. Example:

```yaml
containers:
	- name: cloudstackctl-controller
		image: cloudstackctl:local
		args: ["controller"]
		envFrom:
			- secretRef:
					name: cloudstack-secret
			- secretRef:
					name: database-secret
```

- The controller can also read credentials directly from the Kubernetes API (it looks up the Secret named by `CLOUDSTACK_SECRET_NAME` in `CLOUDSTACK_SECRET_NAMESPACE`). Use whichever approach fits your environment.
- Adjust resource requests/limits and replica counts for your environment.
