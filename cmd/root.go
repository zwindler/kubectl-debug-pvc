package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zwindler/kubectl-debug-pvc/pkg/k8s"
	"github.com/zwindler/kubectl-debug-pvc/pkg/tui"
)

var (
	kubeconfig  string
	kubeContext string
	namespace   string
	pod         string
	volumes     []string
	image       string
	mountBase   string
)

// rootCmd represents the base command.
var rootCmd = &cobra.Command{
	Use:   "kubectl-debug_pvc",
	Short: "Debug Kubernetes pods with PVC volume access via ephemeral containers",
	Long: `kubectl debug-pvc creates an ephemeral debug container in a running pod
with access to its PVC-mounted volumes.

Unlike 'kubectl debug', this tool includes volumeMounts in the ephemeral
container spec, allowing access to the pod's PVC-backed volumes. It patches
the pod's ephemeral containers subresource directly via the Kubernetes API.

When run without flags, an interactive TUI guides you through:
  1. Selecting a namespace (filtered to those with PVC-backed pods)
  2. Selecting a pod (filtered to those with PVC volumes)
  3. Choosing which PVC volumes to mount
  4. Configuring the debug container image and mount paths

You can also use flags for non-interactive / scripted usage.`,
	Example: `  # Interactive TUI mode
  kubectl debug-pvc

  # Non-interactive mode
  kubectl debug-pvc --namespace my-ns --pod my-pod-0 --volume data:/debug/data --image ubuntu:latest

  # Mount multiple volumes
  kubectl debug-pvc -n my-ns -p my-pod-0 -v data:/debug/data -v logs:/debug/logs`,
	RunE: runDebugPVC,
}

func init() {
	rootCmd.Flags().StringVar(&kubeconfig, "kubeconfig", "", "Path to kubeconfig file (default: standard resolution)")
	rootCmd.Flags().StringVar(&kubeContext, "context", "", "Kubeconfig context to use (default: current context)")
	rootCmd.Flags().StringVarP(&namespace, "namespace", "n", "", "Kubernetes namespace")
	rootCmd.Flags().StringVarP(&pod, "pod", "p", "", "Pod name")
	rootCmd.Flags().StringArrayVarP(&volumes, "volume", "v", nil, "Volume mounts in format name:mountpath (can be repeated)")
	rootCmd.Flags().StringVarP(&image, "image", "i", "ubuntu:latest", "Debug container image")
	rootCmd.Flags().StringVar(&mountBase, "mount-base", "/debug", "Base mount path (used in interactive mode)")
}

// Execute runs the root command with the given build version.
func Execute(version string) {
	rootCmd.Version = version
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func runDebugPVC(cmd *cobra.Command, args []string) error {
	// Preflight: ensure kubectl is available in PATH before doing anything.
	// We need kubectl for the attach step; check early so we don't create an
	// ephemeral container (which cannot be removed) and then fail to attach.
	if _, err := exec.LookPath("kubectl"); err != nil {
		return fmt.Errorf("kubectl not found in PATH: %w", err)
	}

	// Initialize K8s client
	client, err := k8s.NewClient(kubeconfig, kubeContext)
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	// Determine if we should use interactive or non-interactive mode
	if namespace != "" && pod != "" && len(volumes) > 0 {
		return runNonInteractive(client)
	}

	// Interactive TUI mode
	return runInteractive(client)
}

func runInteractive(client *k8s.Client) error {
	ns, podName, containerName, shouldAttach, err := tui.Run(client)
	if err != nil {
		return err
	}

	if shouldAttach && containerName != "" {
		fmt.Printf("\nAttaching to container %s...\n", containerName)
		return k8s.AttachToContainer(ns, podName, containerName)
	}

	return nil
}

func runNonInteractive(client *k8s.Client) error {
	ctx := context.Background()

	// Parse volume mounts
	var mounts []k8s.DebugVolumeMount
	for _, v := range volumes {
		parts := strings.SplitN(v, ":", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid volume format %q, expected name:mountpath", v)
		}
		volName := parts[0]
		mountPath := parts[1]
		if volName == "" {
			return fmt.Errorf("invalid volume format %q: volume name must not be empty", v)
		}
		if mountPath == "" {
			return fmt.Errorf("invalid volume format %q: mount path must not be empty", v)
		}
		if mountPath[0] != '/' {
			return fmt.Errorf("invalid volume format %q: mount path must be absolute (start with /)", v)
		}
		mounts = append(mounts, k8s.DebugVolumeMount{
			VolumeName: volName,
			MountPath:  mountPath,
		})
	}

	fmt.Printf("Creating ephemeral container in pod %s/%s...\n", namespace, pod)
	fmt.Printf("  Image: %s\n", image)
	for _, m := range mounts {
		fmt.Printf("  Volume: %s -> %s\n", m.VolumeName, m.MountPath)
	}

	containerName, warnings, err := client.CreateEphemeralContainer(ctx, k8s.EphemeralContainerOpts{
		PodName:      pod,
		Namespace:    namespace,
		Image:        image,
		VolumeMounts: mounts,
	})
	if err != nil {
		return fmt.Errorf("failed to create ephemeral container: %w", err)
	}

	fmt.Printf("Container '%s' created successfully!\n", containerName)
	for _, w := range warnings {
		fmt.Printf("Warning: %s\n", w)
	}
	fmt.Printf("Attaching...\n")

	return k8s.AttachToContainer(namespace, pod, containerName)
}
