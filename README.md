# kubectl debug-pvc

A kubectl plugin that creates ephemeral debug containers in running Kubernetes pods **with PVC volume access**.

`kubectl debug` can create ephemeral containers, but it cannot mount volumes into them. This tool patches the pod's `ephemeralcontainers` subresource directly via the Kubernetes API, including `volumeMounts` -- something `kubectl debug` does not support.

## Why

When a pod holds an exclusive (RWO) lock on a PVC, you can't simply spin up another pod to inspect the data. You need to get into the running pod's context with access to those volumes. `kubectl debug` gets you an ephemeral container but without volume mounts. This tool bridges that gap.

## Features

- Mounts PVC volumes into ephemeral debug containers
- Interactive TUI with namespace/pod/volume selection
- Smart filtering: only shows namespaces and pods that have PVC-backed volumes
- Non-interactive mode for scripted usage
- Vim-style navigation (`j`/`k`) and fuzzy filtering (`/`) in the TUI
- Multi-volume selection -- mount one or more PVCs in a single debug session
- Automatic `kubectl attach` after container creation

## Requirements

- Go 1.25+ (to build from source)
- `kubectl` on your PATH (used for `attach`)
- Kubernetes cluster with ephemeral containers enabled (v1.25+)
- Permissions to patch the `pods/ephemeralcontainers` subresource

## Installation

### From source

```bash
git clone https://github.com/dgermain/kubectl-debug-pvc.git
cd kubectl-debug-pvc
make install
```

This builds the binary and installs it to `/usr/local/bin/kubectl-debug_pvc`. kubectl automatically discovers plugins by name, so `kubectl debug-pvc` will work immediately.

### To GOPATH

```bash
make install-gobin
```

### Build only

```bash
make build
# Binary: ./kubectl-debug_pvc
```

## Usage

### Interactive mode (TUI)

```bash
kubectl debug-pvc
```

The TUI walks you through:

1. **Namespace** -- select from namespaces that have PVC-backed pods
2. **Pod** -- select a pod with PVC volumes
3. **Volumes** -- multi-select which PVC volumes to mount
4. **Config** -- set the debug container image and mount path prefix
5. **Attach** -- the ephemeral container is created and you're attached automatically

### Non-interactive mode

Provide `--namespace`, `--pod`, and at least one `--volume` flag:

```bash
# Single volume
kubectl debug-pvc -n my-namespace -p my-pod-0 -v data:/debug/data

# Multiple volumes
kubectl debug-pvc -n my-namespace -p my-pod-0 -v data:/debug/data -v logs:/debug/logs

# Custom image
kubectl debug-pvc -n my-namespace -p my-pod-0 -v data:/debug/data -i alpine:latest
```

### Flags

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--namespace` | `-n` | | Kubernetes namespace |
| `--pod` | `-p` | | Pod name |
| `--volume` | `-v` | | Volume mount as `name:mountpath` (repeatable) |
| `--image` | `-i` | `ubuntu:latest` | Debug container image |
| `--mount-base` | | `/debug` | Base mount path (interactive mode) |
| `--kubeconfig` | | standard resolution | Path to kubeconfig file |

## TUI keyboard shortcuts

### Namespace and Pod selection

| Key | Action |
|-----|--------|
| `j` / `Down` | Move cursor down |
| `k` / `Up` | Move cursor up |
| `Enter` | Select item |
| `/` | Start filtering (type to search) |
| `Esc` | Exit filter / go back |
| `q` | Quit |

### Volume selection

| Key | Action |
|-----|--------|
| `j` / `Down` | Move cursor down |
| `k` / `Up` | Move cursor up |
| `Space` | Toggle volume selection |
| `Enter` | Confirm (at least one must be selected) |
| `Esc` | Go back |
| `q` | Quit |

### Config

| Key | Action |
|-----|--------|
| `Tab` | Switch between image and mount prefix fields |
| `Enter` | Confirm (empty fields use defaults) |
| `Esc` | Go back |

### Progress / Error

| Key | Action |
|-----|--------|
| `Enter` | Attach to container (on success) |
| `r` | Retry (on error) |
| `Esc` | Go back (on error) |
| `q` | Quit |

## How it works

1. Lists PVCs cluster-wide in a single API call to identify which namespaces have PVC-backed storage
2. Lists pods only in the selected namespace, filtered to those with PVC volumes
3. Builds an ephemeral container spec with `volumeMounts` referencing the pod's existing volumes
4. Applies a strategic merge patch to the pod's `ephemeralcontainers` subresource
5. Waits for the ephemeral container to reach Running state
6. Executes `kubectl attach -it` to connect you to the debug container

This uses the same Kubernetes API that `kubectl debug` uses. The key difference is that `volumeMounts` are included in the ephemeral container spec, which the `kubectl debug` CLI does not expose.

## RBAC

The tool requires the following permissions:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: debug-pvc
rules:
  # Discover namespaces with PVCs
  - apiGroups: [""]
    resources: ["persistentvolumeclaims"]
    verbs: ["list"]
  # List and read pods
  - apiGroups: [""]
    resources: ["pods"]
    verbs: ["get", "list"]
  # Create ephemeral debug containers
  - apiGroups: [""]
    resources: ["pods/ephemeralcontainers"]
    verbs: ["patch"]
  # kubectl attach
  - apiGroups: [""]
    resources: ["pods/attach"]
    verbs: ["create"]
```

## License

See [LICENSE](LICENSE) for details.
