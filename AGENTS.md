# AGENTS.md

## Project Overview

`kubectl-debug_pvc` is a kubectl plugin that creates ephemeral debug containers in running Kubernetes pods **with PVC volume access**. It works by patching the pod's `ephemeralcontainers` subresource directly via the Kubernetes API, including `volumeMounts` — something `kubectl debug` does not support natively.

The tool provides both an interactive Bubble Tea TUI and a non-interactive CLI flag mode.

## Architecture

```
main.go                         Entry point, calls cmd.Execute()
cmd/
  root.go                       Cobra CLI: flag parsing, routes to interactive or non-interactive mode
pkg/
  k8s/
    client.go                   Kubernetes client init from standard kubeconfig
    discovery.go                Cluster scanning: namespaces -> pods -> PVC volumes
    ephemeral.go                Core logic: PATCH ephemeral container with volumeMounts, wait, attach
  tui/
    app.go                      Bubble Tea model, step-based state machine, async commands
    styles.go                   Lip Gloss color/style definitions (theme)
    namespace.go                Namespace selection view (filtered to PVC-only)
    pod.go                      Pod selection view (filtered to PVC-only)
    volume.go                   PVC volume multi-select view
    config.go                   Image and mount prefix input view
    progress.go                 Creation progress, success, error, and done views
```

### Key Design Decisions

- **Ephemeral container PATCH**: The core mechanism calls the Kubernetes API directly to patch the pod's `ephemeralcontainers` subresource with a strategic merge patch that includes `volumeMounts`. There is no `kubectl` command that can do this: `kubectl debug` does not expose `volumeMounts` as a flag, and `kubectl patch --subresource` (added in KEP-2590, Kubernetes 1.33+) only supports `status`, `scale`, and `resize` — `ephemeralcontainers` is not in that list. The only alternative to this tool is a raw HTTP call to the API server (e.g. via `kubectl proxy`). The relevant code is in `pkg/k8s/ephemeral.go`.
- **PVC-only filtering**: Namespace and pod lists are pre-filtered to only show resources that have PVC-backed volumes. This is the primary value-add over a generic debug tool.
- **PVC-first discovery**: To avoid scanning all pods across all namespaces (which is extremely slow on large clusters), the discovery phase lists PVCs cluster-wide (a single API call) to determine which namespaces have PVCs. Pods are only listed within a single namespace after the user selects one. See `pkg/k8s/discovery.go`.
- **TUI state machine**: The Bubble Tea model in `pkg/tui/app.go` uses a `step` enum to progress through: Loading -> Namespace -> Pod -> Volume -> Config -> Progress -> Done.
- **Non-interactive mode**: When `--namespace`, `--pod`, and `--volume` flags are all provided, the tool skips the TUI and goes straight to ephemeral container creation (`cmd/root.go`).

## Build & Quality

### Commands

```bash
make build       # Build the binary
make check       # Run all checks (go vet + golangci-lint)
make lint        # Run golangci-lint only
make vet         # Run go vet only
make fmt         # Run go fmt
make install     # Build and install to /usr/local/bin
make deps        # Run go mod tidy
```

### Mandatory Checks

**Always run `golangci-lint run ./...` (or `make lint`) before considering any change complete.** The codebase must pass with zero issues. This is the primary quality gate.

Run `make check` to execute all checks at once (`go vet` + `golangci-lint`).

### Build Verification

After any code change, verify:
1. `go build -o kubectl-debug_pvc .` succeeds
2. `make check` passes with zero issues

## Dependencies

| Dependency | Purpose |
|---|---|
| `k8s.io/client-go` | Kubernetes API client (kubeconfig, clientset, REST) |
| `k8s.io/api` | Kubernetes API types (corev1.Pod, corev1.EphemeralContainer, etc.) |
| `k8s.io/apimachinery` | API machinery (metav1, types, json, wait) |
| `github.com/charmbracelet/bubbletea` | TUI framework (Elm architecture) |
| `github.com/charmbracelet/bubbles` | TUI components (spinner) |
| `github.com/charmbracelet/lipgloss` | TUI styling |
| `github.com/spf13/cobra` | CLI framework |

## Conventions

### Go Code

- Standard Go project layout: `cmd/` for CLI entry, `pkg/` for library code.
- The `pkg/k8s` package is the Kubernetes interaction layer — all K8s API calls live here.
- The `pkg/tui` package is the presentation layer — all Bubble Tea models and views live here.
- Views are split by step: one file per TUI step (`namespace.go`, `pod.go`, `volume.go`, etc.).
- The shared `model` struct lives in `app.go` and is referenced by all view files in the same package.
- Styles are centralized in `styles.go`. Use the defined style variables; do not hardcode colors elsewhere.

### Error Handling

- K8s API errors should be wrapped with `fmt.Errorf("context: %w", err)` for proper error chains.
- In the TUI, errors are displayed in the progress view with a retry option (press `r`).
- In non-interactive mode, errors are returned directly to Cobra which prints them and exits non-zero.

### Naming

- Binary name: `kubectl-debug_pvc` (underscore for the kubectl plugin discovery mechanism to map `kubectl debug-pvc`).
- The Go module is `github.com/zwindler/kubectl-debug-pvc`.

## Testing

There are no tests yet. When adding tests:

- K8s interaction tests should use `k8s.io/client-go/kubernetes/fake` for the clientset.
- TUI tests can use Bubble Tea's `tea.NewProgram` with test messages.
- Place tests adjacent to the code they test (`*_test.go` files).

## Usage

```bash
# Interactive TUI mode (walks through namespace -> pod -> volume -> config)
kubectl debug-pvc

# Non-interactive mode
kubectl debug-pvc -n my-namespace -p my-pod-0 -v volume-name:/debug/volume-name -i ubuntu:latest

# Multiple volumes
kubectl debug-pvc -n my-ns -p my-pod-0 -v data:/debug/data -v logs:/debug/logs
```
