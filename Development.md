# cloudstackctl — Development & Setup Guide

For the full CLI reference, YAML examples, and complete environment variable list see [README.md](README.md).

## Prerequisites

| Tool | Required for |
|---|---|
| Go 1.21+ | Building binaries (Options A and B) |
| Docker | Building the container image (Options B and C) |
| `kubectl` + `kind` | Option C only |

---

## Step 1 — Build

### Binaries

Required for Option A; also needed if you want to run the CLI locally with Options B or C.

```bash
# Build binary for CLI
go build -o cloudstackctl main.go

# Build binary for Controller (if run locally)
go build -o cloudstackctl-controller cmd/controller/main.go

# Install to PATH
sudo mv cloudstackctl /usr/local/bin/
```

## Build container image

Required for Options B and C. Build once before running either:

```bash
sudo docker build -t cloudstackctl:local .
```

---

## Step 2 — Credentials

Copy the provided example env files and fill in your values:

```bash
cp .env.cloudstack.example .env.cloudstack
cp .env.database.example .env.database
# edit both files with real values
```

`.env.cloudstack` template:

```env
CLOUDSTACK_ENDPOINT=http://10.20.30.40:8080/client/api
CLOUDSTACK_API_KEY=
CLOUDSTACK_SECRET_KEY=
VERIFY_SSL=false
```

These files are ignored by git. The CLI auto-loads `.env.cloudstack` when present, or accepts `-c <file>` to specify a different path. For the full list of supported environment variables see the [Environment Variables Reference](README.md#environment-variables-reference) in README.md.

---

## Step 3 — Choose a deployment option

| Option | Best for |
|---|---|
| [A — Local binaries](#option-a--local-binaries) | Simplest setup; no Docker required |
| [B — Docker / docker-compose](#option-b--docker--docker-compose) | Fast local setup using containers |
| [C — kind / Kubernetes](#option-c--kind-kubernetes) | Test Kubernetes behaviours or deploy to a production cluster |

---

## Option A — Local binaries

Run PostgreSQL and the controller directly as local processes.

**1. Start PostgreSQL**

Use an existing local PostgreSQL instance, or run one in a container:

```bash
docker run -d --name cloudstackctl-postgres -p 5432:5432 \
  -e POSTGRES_PASSWORD=secret -e POSTGRES_DB=cloudstackctl postgres:15
```

**2. Export credentials**

```bash
# CloudStack
source .env.cloudstack

# Database
export DATABASE_DSN="host=localhost user=postgres password=secret dbname=cloudstackctl port=5432 sslmode=disable"
```

**3. Start the controller**

```bash
./cloudstackctl-controller
```

The controller creates/migrates its database tables on startup and listens on `:65426`.

**4. Use the CLI**

```bash
./cloudstackctl apply -f application.yaml
./cloudstackctl get Application

# Get/describe CloudStack resources (standalone or controller mode)
./cloudstackctl get Project                          # list all projects
./cloudstackctl get VirtualMachine -p my-project     # VMs in a project
./cloudstackctl get VirtualMachine -P                # VMs across all projects
./cloudstackctl get VirtualMachine -A                # all VMs incl. unmanaged (controller only)
./cloudstackctl describe Network my-net -p my-project
```

---

## Option B — Docker / docker-compose

**Quick start**

```bash
sudo docker-compose up
```

This starts `postgres` and `cloudstackctl-controller` using `docker-compose.yml` at the project root. The image contains both the `cloudstackctl` (CLI) and `cloudstackctl-controller` binaries.

**Run containers individually**

```bash
# Postgres
sudo docker run -d --name cloudstackctl-postgres -p 5432:5432 \
  -e POSTGRES_PASSWORD=secret -e POSTGRES_DB=cloudstackctl postgres:15

# Controller
sudo docker run -d --name cloudstackctl-controller \
  --link cloudstackctl-postgres:postgres -p 65426:65426 \
  -e DATABASE_DSN='host=postgres user=postgres password=secret dbname=cloudstackctl port=5432 sslmode=disable' \
  -e CLOUDSTACK_ENDPOINT='https://your-cloudstack-api.com/client/api' \
  -e CLOUDSTACK_API_KEY=your-api-key \
  -e CLOUDSTACK_SECRET_KEY=your-secret-key \
  cloudstackctl:local /cloudstackctl-controller
```

**Using env files**

```bash
sudo docker run -d --name cloudstackctl-postgres -p 5432:5432 \
  -e POSTGRES_PASSWORD=secret -e POSTGRES_DB=cloudstackctl postgres:15

sudo docker run -d --name cloudstackctl-controller \
  --link cloudstackctl-postgres:postgres -p 65426:65426 \
  --env-file .env.cloudstack --env-file .env.database \
  cloudstackctl:local /cloudstackctl-controller
```

Notes:
- Use bind mounts or Docker secrets to avoid exposing secrets in env vars.
- The CLI connects to `http://localhost:65426` by default; override with `CONTROLLER_ENDPOINT`.
- The controller logs to `/var/log/cloudstackctl-controller.log` by default; override with `CONTROLLER_LOG_FILE`.

---

## Option C — kind (Kubernetes)

**Install kind and kubectl** (if not already present)

```bash
# kind (Linux x86_64)
curl -Lo ./kind https://kind.sigs.k8s.io/dl/latest/kind-linux-amd64
chmod +x ./kind && sudo mv ./kind /usr/local/bin/kind

# kubectl
curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"
chmod +x kubectl && sudo mv kubectl /usr/local/bin/
```

**Create a cluster and load the image**

```bash
kind create cluster --name cloudstackctl
kubectl cluster-info --context kind-cloudstackctl
kind load docker-image cloudstackctl:local --name cloudstackctl
```

**Create Secrets**

```bash
kubectl create secret generic cloudstack-secret --from-env-file=.env.cloudstack
kubectl create secret generic database-secret --from-env-file=.env.database
```

**Deploy**

Example manifests are in `examples/k8s/`:

| Manifest | Purpose |
|---|---|
| `cloudstack-secret.yaml` | CloudStack credentials (`CLOUDSTACK_API_KEY`, `CLOUDSTACK_SECRET_KEY`, `CLOUDSTACK_ENDPOINT`, `VERIFY_SSL`) |
| `database-secret.yaml` | Database credentials (`DATABASE_DSN` or PG* values) |
| `postgres-deployment.yaml` | PostgreSQL Deployment + Service |
| `controller-deployment.yaml` | Controller Deployment |

```bash
kubectl apply -f examples/k8s/cloudstack-secret.yaml
kubectl apply -f examples/k8s/database-secret.yaml
kubectl apply -f examples/k8s/postgres-deployment.yaml
kubectl apply -f examples/k8s/controller-deployment.yaml
```

**Verify**

```bash
kubectl get pods -n default
kubectl logs deployment/cloudstackctl-controller -n default
```

**Inject credentials via `envFrom`** (recommended)

All settings are sensitive — use Kubernetes `Secret`s rather than `ConfigMap`s:

```yaml
containers:
  - name: cloudstackctl-controller
    image: cloudstackctl:local
    envFrom:
      - secretRef:
          name: cloudstack-secret
      - secretRef:
          name: database-secret
```

The controller also supports loading credentials directly from the Kubernetes API (looks up the Secret named by `CLOUDSTACK_SECRET_NAME` in `CLOUDSTACK_SECRET_NAMESPACE`). Use whichever approach fits your environment.

**Cleanup**

```bash
kind delete cluster --name cloudstackctl
```

---

## Controller-managed tables and migration

The controller creates and migrates its database tables on startup (idempotent, via GORM `AutoMigrate`):

| Table | Stores |
|---|---|
| `applications` | `Application` resources |
| `components` | `Component` resources |
| `vm_specs` | `VirtualMachineSpec` resources |
| `virtual_machines` | `VirtualMachine` desired/observed records |

CloudStack-managed resources (`Network`, `Volume`, `SSHKey`, `SecurityGroup`, `AffinityGroup`, `UserData`) are **not** persisted in the database — they are managed directly via the CloudStack API.

If explicit migration/version control is needed, add a migration tool alongside GORM's `AutoMigrate`.
