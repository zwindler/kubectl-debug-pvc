# kubectl debug-pvc

A kubectl plugin that creates ephemeral debug containers in running Kubernetes pods **with PVC volume access**.

`kubectl debug` can create ephemeral containers, but its CLI does not expose `volumeMounts` for them. There is no `kubectl` command that can patch the `ephemeralcontainers` subresource directly: Kubernetes 1.33+ added `--subresource` support to `kubectl patch` ([KEP-2590](https://github.com/kubernetes/enhancements/issues/2590)), but as of Kubernetes 1.35 only `status`, `scale`, and `resize` are accepted — `ephemeralcontainers` is not supported.

The only way to attach volume mounts to an ephemeral container is to call the Kubernetes API directly. This tool does exactly that, and wraps the entire workflow — PVC discovery, pod filtering, volume selection, patch construction, readiness wait, and attach — into a single command.

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
- PodSecurity compatible -- inherits the target container's `securityContext` so the debug container satisfies `restricted`, `baseline`, or any enforced policy

## Requirements

- Go 1.25+ (to build from source)
- `kubectl` on your PATH (used for `attach`)
- Kubernetes cluster with ephemeral containers enabled (v1.25+)
- Permissions to patch the `pods/ephemeralcontainers` subresource

## Installation

### Via Krew (recommended)

[Krew](https://krew.sigs.k8s.io/) is the kubectl plugin manager.

```bash
kubectl krew install debug-pvc
```

Once installed, update with:

```bash
kubectl krew upgrade debug-pvc
```

> **Note:** The first release of this plugin must be manually submitted to the
> [krew-index](https://github.com/kubernetes-sigs/krew-index). Subsequent
> releases are automated via [krew-release-bot](https://github.com/rajatjindal/krew-release-bot).

### From source

```bash
git clone https://github.com/zwindler/kubectl-debug-pvc.git
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
4. Applies a strategic merge patch to the pod's `ephemeralcontainers` subresource via the Kubernetes API
5. Waits for the ephemeral container to reach Running state
6. Executes `kubectl attach -it` to connect you to the debug container

The patch is a standard strategic merge patch against the `pods/ephemeralcontainers` subresource. There is no `kubectl` command that can issue it — `kubectl patch --subresource` does not support `ephemeralcontainers`. The equivalent raw API call (e.g. via `kubectl proxy`) looks like this:

```bash
curl http://localhost:8001/api/v1/namespaces/<namespace>/pods/<pod>/ephemeralcontainers \
  -X PATCH \
  -H 'Content-Type: application/strategic-merge-patch+json' \
  -d '{
    "spec": {
      "ephemeralContainers": [
        {
          "name": "debugger",
          "image": "ubuntu",
          "command": ["/bin/sh"],
          "targetContainerName": "<target-container>",
          "stdin": true,
          "tty": true,
          "volumeMounts": [
            {
              "name": "<volume-name>",
              "mountPath": "/debug/<volume-name>"
            }
          ]
        }
      ]
    }
  }'
```

This tool automates the discovery, patch construction, readiness wait, and attach steps so you don't have to craft this manually.

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

## Limitations

### Ephemeral containers cannot be removed

Ephemeral containers are append-only in the Kubernetes API. Once created, they cannot be deleted or modified -- this is a Kubernetes design constraint, not a limitation of this tool. Stopped debug containers remain as terminated entries in the pod spec. They consume no CPU or memory, but will show up in `kubectl describe pod` output.

The only way to clean them up is to restart the pod (e.g., `kubectl rollout restart deployment/...`).

## Releases

Releases are automated via [GoReleaser](https://goreleaser.com/) and a GitHub Actions workflow triggered by pushing a semver tag.

### Creating a release

```bash
# Tag the commit you want to release
git tag -s v1.2.3 -m "Release v1.2.3"
git push origin v1.2.3
```

The `release` workflow then:

1. Runs `make check` (go vet + golangci-lint) — the tag is rejected if checks fail
2. Builds binaries for Linux, macOS, and Windows (amd64 + arm64) with `-trimpath` and `CGO_ENABLED=0` for reproducible, statically linked binaries
3. Packages each binary as a `.tar.gz` (`.zip` on Windows) with the LICENSE and README
4. Produces a SHA-256 checksum file covering all archives
5. Signs the checksum file with [cosign](https://github.com/sigstore/cosign) using **keyless signing** — no private key is stored anywhere; the GitHub Actions OIDC token is used to obtain a short-lived certificate from [Sigstore Fulcio](https://github.com/sigstore/fulcio), and the signature is recorded in the [Rekor](https://github.com/sigstore/rekor) public transparency log
6. Generates a CycloneDX SBOM for each archive using [syft](https://github.com/anchore/syft)
7. Creates the GitHub Release with all assets attached

### Verifying a release

To verify the checksum file signature after downloading:

```bash
# Download the release assets
gh release download v1.2.3 --repo zwindler/kubectl-debug-pvc

# Verify the signature against the Rekor transparency log
# (no local key needed — cosign looks up the cert in Rekor)
cosign verify-blob \
  --certificate kubectl-debug_pvc_v1.2.3_checksums.txt.pem \
  --signature  kubectl-debug_pvc_v1.2.3_checksums.txt.sig \
  --certificate-identity-regexp "https://github.com/zwindler/kubectl-debug-pvc/.github/workflows/release.yml@refs/tags/v.*" \
  --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
  kubectl-debug_pvc_v1.2.3_checksums.txt

# Then verify your archive against the checksums
sha256sum --check --ignore-missing kubectl-debug_pvc_v1.2.3_checksums.txt
```

### GitHub Actions security posture

- All workflow permissions default to `{}` (none); each job grants only what it needs
- All third-party actions are pinned to immutable commit SHAs, not mutable version tags
- The runner is hardened with [step-security/harden-runner](https://github.com/step-security/harden-runner) in `audit` mode (egress monitoring)
- `id-token: write` is granted only to the release job, and only for the duration of that job
- `GITHUB_TOKEN` is the only secret used — no external credentials are stored in the repository

## License

See [LICENSE](LICENSE) for details.
