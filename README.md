# cloudstackctl

`cloudstackctl` is a Kubernetes-style declarative orchestration tool for Apache CloudStack. It provides a way to manage virtual machines, networking, and storage via YAML, supporting **create, update, delete**, health checks, dependency graphs, and drift detection.

---

# Features

* Declarative management of Applications, Components, VirtualMachines, and other CloudStack resources
* Reusable **VirtualMachineSpec** for consistent VM definitions
* Health checks and dependency graph enforcement
* Drift detection and reconciliation
* PostgreSQL as the single source of truth
* CLI, API server, and Controller manager architecture
* Logs and debugging support for CLI, API server, and Controller

---

# CloudStack Credentials

CloudStack credentials can be defined using a Secret:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: cloudstack-credentials
type: Opaque
stringData:
  apiKey: <YOUR_API_KEY>
  secretKey: <YOUR_SECRET_KEY>
  endpoint: https://<CLOUDSTACK_API_URL>
```

---

# Architecture Overview

![Architecture](Architecture.png)

* API server validates requests and returns immediate acknowledgment
* Controller sends async requests to CloudStack API, writes observed state, manages health checks and dependency graph
* CLI polls API server for near real-time status

---

# Supported Resource Types, Actions, and Kubernetes Equivalents

| Kind                   | Description                             | Actions Supported                                                                                         | Kubernetes Equivalent                    |
| ---------------------- | --------------------------------------- | --------------------------------------------------------------------------------------------------------- | ---------------------------------------- |
| **Application**        | Full application or service stack       | create, update, delete, get, describe                                                                     | Namespace / Application CRD              |
| **Component**          | Set of VMs for a specific role          | create, update, delete, get, describe, scale                                                              | Deployment / StatefulSet                 |
| **VirtualMachine**     | Individual VM instance                  | create, update, delete, get, describe                                                                     | Pod                                      |
| **VirtualMachineSpec** | Reusable VM template for Components     | create, update, delete, get, describe                                                                     | PodTemplateSpec                          |
| **Network**            | CloudStack network                      | create, update, delete, get, describe (mostly references existing networks; future: declarative creation) | NetworkPolicy / Service                  |
| **Volume**             | Disk attached to VMs                    | create, update, delete, get, describe                                                                     | PersistentVolume / PersistentVolumeClaim |
| **SSHKey**             | Key pair for VM access                  | create, update, delete, get, describe                                                                     | Secret                                   |
| **UserDataRef**        | User data scripts for VM initialization | create, update, delete, get, describe                                                                     | ConfigMap / Secret                       |
| **AffinityGroup**      | Host/VM affinity or anti-affinity       | create, update, delete, get, describe                                                                     | PodAffinity / PodAntiAffinity            |
| **SecurityGroup**      | Firewall rules for VMs                  | create, update, delete, get, describe                                                                     | NetworkPolicy                            |

---

# YAML Examples

### VirtualMachineSpec

```yaml
apiVersion: cloudstackctl/v1
kind: VirtualMachineSpec
metadata:
  name: basic-vm-spec
spec:
  template: ubuntu-22.04
  serviceOffering: medium
  networkIds:
    - existing-network-1-id
  sshKeys:
    - platform-admin
  volumes:
    - name: data-disk
      diskOffering: standard-hdd
      size: 50GB
```

### Component Reusing VirtualMachineSpec

```yaml
apiVersion: cloudstackctl/v1
kind: Application
metadata:
  name: app-with-reused-vmspec
spec:
  projectId: 987e6543-e21b-12d3-a456-426655440000
  components:
    - name: frontend
      virtualMachineSpec: basic-vm-spec
      replicas: 2
      healthChecks:
        - type: ping
          interval: 10s
          timeout: 5s
```

### Standalone VirtualMachine

```yaml
apiVersion: cloudstackctl/v1
kind: VirtualMachine
metadata:
  name: standalone-vm
spec:
  projectId: 987e6543-e21b-12d3-a456-426655440000
  template: ubuntu-22.04
  serviceOffering: medium
  networkIds:
    - existing-network-1-id
  sshKeys:
    - platform-admin
  volumes:
    - name: data-disk
      diskOffering: standard-hdd
      size: 50GB
  affinityGroups:
    - type: hostAntiAffinity
      name: vm-spread
  securityGroups:
    - default-sec
  parameters:
    bootMode: SECURE
    bootType: UEFI
  healthChecks:
    - type: ping
      interval: 10s
      timeout: 5s
```

---

# Logging and Troubleshooting

| Component  | Logs / Errors                                                   | Access                                                                     |
| ---------- | --------------------------------------------------------------- | -------------------------------------------------------------------------- |
| CLI        | Command output, validation, API responses                       | Console (`--verbose` or `--debug`)                                         |
| API Server | Validation, forwarding, errors                                  | `kubectl logs <api-server-pod>` or `/var/log/cloudstackctl-api.log`        |
| Controller | Reconciliation, CloudStack API, health checks, dependency graph | `kubectl logs <controller-pod>` or `/var/log/cloudstackctl-controller.log` |
| PostgreSQL | Connection or transaction errors                                | Standard PostgreSQL logs                                                   |

**Debugging Tips:**

* CLI: `--verbose` / `--debug` for step-by-step and API payload logs
* Controller: `DEBUG=true` for full CloudStack API logging
* Check PostgreSQL for DB connection errors

---

# Database

* **PostgreSQL** only
* Stores desired and observed state
* ACID transactions, JSONB support for YAML specs

---

# Deployment

* **Local:** `kind` cluster for CLI, API server, controller, PostgreSQL containers
* **Production:** Kubernetes or VM cluster, CloudStack manages actual VM provisioning

---

# Future Enhancements

* Rolling updates
* Automatic load balancer creation
* Advanced health checks
* Dependency graph visualization
* Drift detection and auto-healing
* Multi-zone deployments
* Self-healing of VMs and components

---

# License

Apache License 2.0