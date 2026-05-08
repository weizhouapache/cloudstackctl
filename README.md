# cloudstackctl (csctl)

Kubernetes-style declarative orchestration tool for Apache CloudStack

For local development configuration see [Development.md](Development.md).

---

# Architecture Overview

* Single `cloudstackctl` binary provides the CLI.
* Single `cloudstackctl-controller` binary provides the API server and controller runtime.
* Controller sends async requests to CloudStack API, writes desired & observed state, manages health checks and dependency graph. It exposes a small health endpoint on port `65426` (`/health`).
* CLI operates locally and uses the CloudStack client to perform direct actions when appropriate; credentials are loaded from a config file (see `-c`) or environment variables (`.env.cloudstack` is used by default if present).

There are two supported mode:

- **Standalone mode (`-s` / `--standalone`):** CLI-only mode that talks directly to CloudStack APIs and does not read or write the database. Use this for quick ad-hoc operations without running the controller. Controller mode requires running the controller and Postgres (see Development.md).

- **Controller mode (default):** `cloudstackctl` operates with a PostgreSQL backing store and a controller process that reconciles desired state with CloudStack. Resources are managed via the database/controller.

## Standalone mode

<img src="Architecture-standalone.png" width="50%" alt="Architecture of Standalone mode" />

## Controller mode

<img src="Architecture.png" width="50%" alt="Architecture of Controller mode" />


---

## Two Modes With YAML Support

| Feature | Standalone Mode | Controller mode |
|---|---|---|
| Purpose | Direct CloudStack resource management using YAML | Declarative orchestration with controller and DB |
| Architecture | CLI → CloudStack API | CLI → API Server → PostgreSQL → Controller → CloudStack API |
| Controller | No | Yes |
| Database | No | Yes (PostgreSQL stores desired state) |
| Execution | CLI parses YAML and performs API calls immediately | CLI posts to API server; controller reconciles desired state |
| Typical YAML kinds | VirtualMachine, Network, Volume, SecurityGroup, AffinityGroup, SSHKey, UserData | Application, Component, VirtualMachineSpec, VirtualMachine, Network, Volume, SecurityGroup, AffinityGroup, SSHKey, UserData |

### Resource Support Matrix

| Resource | Standalone Mode | Controller mode |
|---|:---:|:---:|
| VirtualMachine | ✅ | ✅ |
| Network | ✅ | ✅ |
| Volume | ✅ | ✅ |
| SecurityGroup | ✅ | ✅ |
| AffinityGroup | ✅ | ✅ |
| SSHKey | ✅ | ✅ |
| UserData | ✅ | ✅ |
| Project | ✅ | ✅ |
| Application | ❌ | ✅ |
| Component | ❌ | ✅ |
| VirtualMachineSpec | ❌ | ✅ |

### Apply Behavior by Kind

| Kind | Project Supported | Same Name on Apply |
|---|---|---|
| VirtualMachine | Yes (`metadata.project` / `spec.project`; top-level `project` accepted) | Rejected as already exists when found in same project; if `spec.networks` is set, existence must also match one of those networks |
| Network | Yes (`metadata.project`; top-level `project` accepted) | Rejected as already exists in same project |
| Volume | Yes (`metadata.project`; top-level `project` accepted) | Rejected as already exists in same project |
| SSHKey | Yes (`metadata.project`; top-level `project` accepted) | Rejected as already exists in same project |
| SecurityGroup | Yes (`metadata.project`; top-level `project` accepted) | Rejected as already exists in same project |
| AffinityGroup | Yes (`metadata.project`; top-level `project` accepted) | Rejected as already exists in same project |
| UserData | Yes (`metadata.project`; top-level `project` accepted) | Rejected as already exists in same project |
| Project | N/A (global resource) | Rejected as already exists by project name |
| Application (controller mode) | Yes (`metadata.project` / `spec.project`) | Upsert in DB by name (existing record is updated) |
| Component (controller mode) | Partial (stored in `metadata.project`; inherited from `Application` when managed by app) | Upsert in DB by name; ownership conflicts are rejected |
| VirtualMachineSpec (controller mode) | Yes (`metadata.project` / `spec.project`) | Create-only by name; identical re-apply is allowed, spec changes are rejected |



# Supported Resource Types and Actions

| Kind                   | Shortnames                               | Description                             | Actions Supported                                                                                         | Kubernetes Equivalent                    |
| ---------------------- | ---------------------------------------- | --------------------------------------- | --------------------------------------------------------------------------------------------------------- | ---------------------------------------- |
| **Application**        | app, apps                                | Full application or service stack       | create, update, delete, get, describe                                                                     | Namespace / Application CRD              |
| **Component**          | comp, comps                              | Set of VMs for a specific role          | create, update, delete, get, describe                                                                     | Deployment / StatefulSet                 |
| **VirtualMachine**     | vm, vms                                  | Individual VM instance                  | create, update, delete, get, describe                                                                     | Pod                                      |
| **VirtualMachineSpec** | vmspec, vmspecs                          | VM specifications for Components        | create, update, delete, get, describe                                                                     | PodTemplateSpec                          |
| **Network**            | net, nets, network, networks             | CloudStack network                      | create, update, delete, get, describe                                                                     | NetworkPolicy / Service                  |
| **Volume**             | vol, vols, volume, volumes               | Disk attached to VMs                    | create, update, delete, get, describe                                                                     | PersistentVolume / PersistentVolumeClaim |
| **SSHKey**             | key, keys, sshkey, sshkeys               | Key pair for VM access                  | create, update, delete, get, describe                                                                     | Secret                                   |
| **UserData**           | userdata, ud, uds                        | User data scripts for VM initialization | create, update, delete, get, describe                                                                     | ConfigMap / Secret                       |
| **AffinityGroup**      | ag, affinitygroup, affinitygroups        | Host/VM affinity or anti-affinity       | create, update, delete, get, describe                                                                     | PodAffinity / PodAntiAffinity            |
| **SecurityGroup**      | sg, sgs, securitygroup, securitygroups   | Firewall rules for VMs                  | create, update, delete, get, describe                                                                     | NetworkPolicy                            |
| **Project**            | proj, projs, project, projects           | CloudStack project (resource boundary)  | create, delete, get, describe                                                                             | Namespace                                |

---

## Resource Field Reference

Quick reference for the key fields users will commonly set for higher-level resources.

**Application**

| Field | Type | Description |
|---|---|---|
| `metadata.name` | string | Application name |
| `spec.project` | string | CloudStack project UUID or name |
| `spec.components` | list | Ordered list of component references (name, vmspec, replicas) |

**Component**

| Field | Type | Description |
|---|---|---|
| `metadata.name` | string | Component name (used to derive VM names) |
| `spec.virtualMachineSpec` | string | Name of reusable `VirtualMachineSpec` to use |
| `spec.replicas` | int | Number of VM replicas to create |
| `spec.overrides.userDataRefs` | list | Optional: references to `UserData` to apply to this component's VMs |
| `spec.overrides.sshKeys` | list | Optional additional SSH keys to merge into the VM spec |

**VirtualMachineSpec**

| Field | Type | Description |
|---|---|---|
| `metadata.name` | string | Reusable VM spec identifier |
| `spec.template` | string | Template name or ID |
| `spec.serviceOffering` | string | Service offering (size) |
| `spec.networks` | list | Network IDs to attach |
| `spec.sshKeys` | list | SSH key names to inject |
| `spec.userDataRefs` | list | Optional references to `UserData` resources |
---

# YAML Examples

### VirtualMachineSpec

```yaml
apiVersion: cloudstackctl/v1
kind: VirtualMachineSpec
metadata:
  name: basic-vm-spec
spec:
  zone: zone-1
  template: ubuntu-22.04
  serviceOffering: medium
  networks:
    - existing-network-1-id
  sshKeys:
    - platform-admin
  volumes:
    - name: data-disk
      diskOffering: standard-hdd
      size: 50
```

### Component Reusing VirtualMachineSpec

```yaml
apiVersion: cloudstackctl/v1
kind: Application
metadata:
  name: app-with-reused-vmspec
spec:
  project: project-2
  components:
    - name: frontend
      virtualMachineSpec: basic-vm-spec
      overrides:
        sshKeys:
          - web
      replicas: 2
      healthChecks:
        - type: ping
          interval: 10s
          timeout: 5s
```

### Application with Multiple Components

```yaml
apiVersion: cloudstackctl/v1
kind: Application
metadata:
  name: simple-app
spec:
  project: project-2
  components:
    - name: frontend
      virtualMachineSpec: basic-vm-spec
      replicas: 2
    - name: backend
      virtualMachineSpec: basic-vm-spec
      replicas: 1
```

### Standalone VirtualMachine

```yaml
apiVersion: cloudstackctl/v1
kind: VirtualMachine
metadata:
  name: standalone-vm
spec:
  zone: zone-1
  project: project-2
  template: ubuntu-22.04
  serviceOffering: medium
  networks:
    - existing-network-1-id
  sshKeys:
    - platform-admin
  volumes:
    - name: data-disk
      diskOffering: standard-hdd
      size: 50
  affinityGroups:
    - vm-spread
  securityGroups:
    - default-sec
  parameters:
    bootMode: SECURE
    bootType: UEFI
```

Usage notes:

- To run the CLI in standalone mode (no DB/controller): `./cloudstackctl -s get VirtualMachine` or `./cloudstackctl -s apply -f vm.yaml`.
- To run in controller mode (default), ensure the controller and Postgres are running; `apply` will send resources to the controller which persists them to the DB and reconciles via CloudStack.

### `get` command flags

| Flag | Short | Description |
|---|---|---|
| `--all-vms` | `-A` | (Controller mode, VirtualMachine only) List all VMs from CloudStack including unmanaged ones. Without this flag only DB-managed VMs are returned. |
| `--all-projects` | `-P` | List resources across all CloudStack projects (and resources not in any project). |
| `--project` | `-p` | Filter results to a specific CloudStack project by name. |
| `--application` | `-a` | Filter results by application name. Applies to `Application`, `Component`, and `VirtualMachine` resources in controller mode. |

Examples:

```bash
# List only managed VMs
./cloudstackctl get VirtualMachine

# List all VMs from CloudStack (including unmanaged)
./cloudstackctl get VirtualMachine -A

# List VMs in a specific project
./cloudstackctl get VirtualMachine -p my-project

# List VMs across all projects
./cloudstackctl get VirtualMachine -P

# List all projects
./cloudstackctl get Project

# List components belonging to a specific application
./cloudstackctl get Component -a my-app
```

### `describe` command flags

| Flag | Short | Description |
|---|---|---|
| `--all-vms` | `-A` | (Controller mode, VirtualMachine only) Describe a VM directly from CloudStack rather than from the DB. |
| `--project` | `-p` | Look up the resource within a specific CloudStack project by name. |

Examples:

```bash
# Describe a managed VM from the DB
./cloudstackctl describe VirtualMachine my-vm

# Describe a VM directly from CloudStack
./cloudstackctl describe VirtualMachine my-vm -A

# Describe a network inside a project
./cloudstackctl describe Network my-net -p my-project

# Describe a project
./cloudstackctl describe Project my-project
```

### `reconcile` command (TODO)

Trigger on-demand reconciliation for a resource in controller mode:

```bash
cloudstackctl reconcile <resource-type> <name>
```

| Flag | Default | Description |
|---|---|---|
| `--wait` | false | Poll until the resource reaches a ready/healthy state |
| `--timeout` | (seconds) | Maximum time to wait when `--wait` is set |
| `--interval` | (seconds) | Polling interval when `--wait` is set |

Examples:

```bash
# Trigger reconciliation for a component
./cloudstackctl reconcile Component my-backend

# Trigger and wait for the application to become healthy
./cloudstackctl reconcile Application my-app --wait --timeout 120 --interval 10
```

---

# Logging and Troubleshooting

| Component  | Logs / Errors                                                   | Access                                                                     |
| ---------- | --------------------------------------------------------------- | -------------------------------------------------------------------------- |
| CLI        | Command output, validation, API responses                       | Console (standard output/error)                                            |
| Controller | Reconciliation, CloudStack API, health checks, dependency graph | File set by `CONTROLLER_LOG_FILE` (default `/var/log/cloudstackctl-controller.log`), or `kubectl logs <controller-pod>` in Kubernetes |
| PostgreSQL | Connection or transaction errors                                | Standard PostgreSQL logs                                                   |

**Debugging Tips:**

* CLI: Check stderr output for error details
* Controller: Set `CONTROLLER_LOG_FILE` to redirect logs; falls back to stdout if the file cannot be opened
* Check PostgreSQL for DB connection errors

---

# Environment Variables Reference

## CloudStack credentials

| Variable | Required | Description |
|---|---|---|
| `CLOUDSTACK_ENDPOINT` | Yes | CloudStack API URL |
| `CLOUDSTACK_API_KEY` | Yes | CloudStack API key |
| `CLOUDSTACK_SECRET_KEY` | Yes | CloudStack secret key |
| `VERIFY_SSL` | No (default: `true`) | Set to `false` to skip TLS verification |
| `CLOUDSTACK_SECRET_NAME` | No (default: `cloudstack-credentials`) | Kubernetes Secret name for in-cluster credential loading |
| `CLOUDSTACK_SECRET_NAMESPACE` | No (default: `default`) | Kubernetes namespace for the CloudStack Secret |

## Controller

| Variable | Default | Description |
|---|---|---|
| `CONTROLLER_ENDPOINT` | `http://localhost:65426` | URL the CLI uses to reach the controller. Override when the controller is not on localhost. |
| `CONTROLLER_LOG_FILE` | `/var/log/cloudstackctl-controller.log` | File path for controller log output. Falls back to stdout if the file cannot be opened. |

---

# Database

* **PostgreSQL** only
* Stores desired and observed state
* ACID transactions, JSONB support for YAML specs

Configuration: the server reads the DB connection from `DATABASE_DSN` if set,
or assembles one from `PGHOST`, `PGUSER`, `PGPASSWORD`, `PGDATABASE`, `PGPORT`,
and `PGSSLMODE`. Example env vars:

```bash
# preferred: provide a single DSN
export DATABASE_DSN="host=localhost user=postgres password=secret dbname=cloudstackctl port=5432 sslmode=disable"

# or set individual PG* variables
export PGHOST=localhost
export PGUSER=postgres
export PGPASSWORD=secret
export PGDATABASE=cloudstackctl
export PGPORT=5432
export PGSSLMODE=disable
```

---

# Deployment

Controller mode requires two running services alongside the CLI:

| Service | Role |
|---|---|
| PostgreSQL | Stores desired and observed state for managed resources |
| `cloudstackctl-controller` | Reconciles desired state with CloudStack; exposes the HTTP API on port `65426` |

Choose the option that fits your environment — full step-by-step instructions are in [Development.md](Development.md):

| Option | Best for |
|---|---|
| [A — Local binaries](Development.md#option-a--local-binaries) | Simplest setup; PostgreSQL and controller run as local processes, no Docker required |
| [B — Docker / docker-compose](Development.md#option-b--docker--docker-compose) | Fast local setup using containers |
| [C — kind / Kubernetes](Development.md#option-c--kind-kubernetes) | Test Kubernetes behaviours, or deploy to a production cluster |

## Production notes

* Run the controller as a single replica; it is stateful (holds per-application reconcile tickers).
* PostgreSQL should use a persistent volume and be backed up regularly — it is the source of truth for desired state.
* The controller image contains both binaries; the controller entrypoint is `/cloudstackctl-controller`.
* Point the CLI at a remote controller with `CONTROLLER_ENDPOINT=http://<host>:65426`.

---

# Future Enhancements

* CLI: Support resource update via YAML file
* CLI: Support Rolling updates
* CLI: Multi-zone deployments
* CLI: Security group improvements
* CLI: Support UserData details
* CLI/Controller: Support reconciling resources
* Controller: Support network services of Isolated network
* Controller: Advanced health checks
* Controller: Dependency graph visualization
* Controller: Self-healing of VMs and components
* Controller: Scaling of components
* Controller: Configurable timeout settings

---

# License

Apache License 2.0
