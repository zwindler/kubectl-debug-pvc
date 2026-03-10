package k8s

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"os"
	"os/exec"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/apimachinery/pkg/util/wait"
)

// EphemeralContainerOpts holds the options for creating an ephemeral debug container.
type EphemeralContainerOpts struct {
	PodName      string
	Namespace    string
	Image        string
	VolumeMounts []DebugVolumeMount
}

// DebugVolumeMount defines a volume to mount in the debug container.
type DebugVolumeMount struct {
	VolumeName string
	MountPath  string
}

// CreateEphemeralContainer patches the pod to add an ephemeral container with the
// specified volume mounts. This uses the same API that kubectl debug uses, but includes
// volumeMounts which kubectl debug doesn't expose as a CLI flag.
// Returns the container name, a slice of advisory warnings, and any error.
func (c *Client) CreateEphemeralContainer(ctx context.Context, opts EphemeralContainerOpts) (string, []string, error) {
	containerName := fmt.Sprintf("debugger-%s", randomSuffix(8))

	// Build volume mounts
	mounts := make([]corev1.VolumeMount, len(opts.VolumeMounts))
	for i, vm := range opts.VolumeMounts {
		mounts[i] = corev1.VolumeMount{
			Name:      vm.VolumeName,
			MountPath: vm.MountPath,
		}
	}

	// Determine target container (first container in the pod)
	pod, err := c.Clientset.CoreV1().Pods(opts.Namespace).Get(ctx, opts.PodName, metav1.GetOptions{})
	if err != nil {
		return "", nil, fmt.Errorf("failed to get pod: %w", err)
	}

	targetContainer := ""
	var targetSecurityContext *corev1.SecurityContext
	if len(pod.Spec.Containers) > 0 {
		targetContainer = pod.Spec.Containers[0].Name
		targetSecurityContext = pod.Spec.Containers[0].SecurityContext
	}

	// Collect advisory warnings about inherited security context settings
	// that may impede common debug operations.
	var warnings []string
	if targetSecurityContext != nil {
		if targetSecurityContext.ReadOnlyRootFilesystem != nil && *targetSecurityContext.ReadOnlyRootFilesystem {
			warnings = append(warnings, "readOnlyRootFilesystem is enabled: writing to the container's root filesystem (e.g. /tmp, package installs) will fail")
		}
		if targetSecurityContext.AllowPrivilegeEscalation != nil && !*targetSecurityContext.AllowPrivilegeEscalation {
			warnings = append(warnings, "allowPrivilegeEscalation is false: sudo and setuid binaries will not work")
		}
		if targetSecurityContext.RunAsNonRoot != nil && *targetSecurityContext.RunAsNonRoot {
			warnings = append(warnings, "runAsNonRoot is true: the debug container will not run as root")
		}
	}

	// Build the ephemeral container spec.
	// Copy securityContext from the target container so the debug container
	// satisfies the same PodSecurity policy the pod already passes (e.g. restricted).
	ephemeralContainer := corev1.EphemeralContainer{
		EphemeralContainerCommon: corev1.EphemeralContainerCommon{
			Name:            containerName,
			Image:           opts.Image,
			Command:         []string{"/bin/sh"},
			Stdin:           true,
			TTY:             true,
			VolumeMounts:    mounts,
			SecurityContext: targetSecurityContext,
		},
		TargetContainerName: targetContainer,
	}

	// Build the strategic merge patch
	type patchSpec struct {
		EphemeralContainers []corev1.EphemeralContainer `json:"ephemeralContainers"`
	}
	type patchBody struct {
		Spec patchSpec `json:"spec"`
	}

	patch := patchBody{
		Spec: patchSpec{
			EphemeralContainers: []corev1.EphemeralContainer{ephemeralContainer},
		},
	}

	patchBytes, err := json.Marshal(patch)
	if err != nil {
		return "", nil, fmt.Errorf("failed to marshal patch: %w", err)
	}

	// Apply the patch to the pod's ephemeralcontainers subresource
	_, err = c.Clientset.CoreV1().Pods(opts.Namespace).Patch(
		ctx,
		opts.PodName,
		types.StrategicMergePatchType,
		patchBytes,
		metav1.PatchOptions{},
		"ephemeralcontainers",
	)
	if err != nil {
		return "", nil, fmt.Errorf("failed to patch ephemeral containers: %w", err)
	}

	// Wait for the ephemeral container to be running
	err = c.waitForEphemeralContainer(ctx, opts.Namespace, opts.PodName, containerName)
	if err != nil {
		return containerName, warnings, fmt.Errorf("ephemeral container created but not yet running: %w", err)
	}

	return containerName, warnings, nil
}

// waitForEphemeralContainer waits until the ephemeral container is running.
func (c *Client) waitForEphemeralContainer(ctx context.Context, namespace, podName, containerName string) error {
	return wait.PollUntilContextTimeout(ctx, 1*time.Second, 60*time.Second, true, func(ctx context.Context) (bool, error) {
		pod, err := c.Clientset.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
		if err != nil {
			// Fatal errors: stop polling immediately
			if apierrors.IsNotFound(err) || apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err) {
				return false, fmt.Errorf("cannot get pod: %w", err)
			}
			// Transient errors (network glitches, etc.): retry
			return false, nil
		}

		for _, status := range pod.Status.EphemeralContainerStatuses {
			if status.Name == containerName {
				if status.State.Running != nil {
					return true, nil
				}
				if status.State.Terminated != nil {
					return false, fmt.Errorf("container terminated: %s", status.State.Terminated.Reason)
				}
			}
		}

		return false, nil
	})
}

// AttachToContainer execs kubectl attach to connect to the ephemeral container.
func AttachToContainer(namespace, podName, containerName string) error {
	cmd := exec.Command("kubectl", "attach",
		"-n", namespace,
		"-c", containerName,
		"-it",
		"--", podName,
	)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

func randomSuffix(length int) string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, length)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
		if err != nil {
			// Fallback: use current time nanoseconds for entropy (extremely unlikely path)
			n = big.NewInt(int64(time.Now().UnixNano() % int64(len(chars))))
		}
		b[i] = chars[n.Int64()]
	}
	return string(b)
}
