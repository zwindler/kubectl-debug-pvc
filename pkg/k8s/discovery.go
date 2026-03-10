package k8s

import (
	"context"
	"fmt"
	"sort"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PVCVolumeInfo holds information about a PVC volume mount on a pod.
type PVCVolumeInfo struct {
	VolumeName   string // Name of the volume in the pod spec
	ClaimName    string // PVC claim name
	MountPath    string // Where it's mounted in the container
	Size         string // Capacity (e.g., "50Gi")
	AccessModes  string // e.g., "RWO"
	StorageClass string // Storage class name
	ReadOnly     bool
}

// PodInfo holds information about a pod with PVC volumes.
type PodInfo struct {
	Name       string
	Namespace  string
	Status     string
	PVCVolumes []PVCVolumeInfo
}

// NamespaceInfo holds information about a namespace with PVCs.
type NamespaceInfo struct {
	Name     string
	PVCCount int // Number of PVCs in the namespace
}

// DiscoverNamespacesWithPVCs returns namespaces that have at least one PVC.
// This uses a single cluster-wide PVC list call instead of scanning pods in
// every namespace, making it orders of magnitude faster on large clusters.
func (c *Client) DiscoverNamespacesWithPVCs(ctx context.Context) ([]NamespaceInfo, error) {
	// Single API call: list all PVCs across all namespaces
	pvcs, err := c.Clientset.CoreV1().PersistentVolumeClaims("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list PVCs: %w", err)
	}

	// Count PVCs per namespace — only Bound PVCs represent usable storage
	nsPVCCount := make(map[string]int)
	for i := range pvcs.Items {
		if pvcs.Items[i].Status.Phase == corev1.ClaimBound {
			nsPVCCount[pvcs.Items[i].Namespace]++
		}
	}

	result := make([]NamespaceInfo, 0, len(nsPVCCount))
	for ns, count := range nsPVCCount {
		result = append(result, NamespaceInfo{
			Name:     ns,
			PVCCount: count,
		})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})

	return result, nil
}

// DiscoverPodsWithPVCs returns pods in the given namespace that have PVC volumes.
// It starts by listing PVCs (typically a small set), then finds pods that reference
// them. This is much faster than scanning all pods on clusters with many pods.
func (c *Client) DiscoverPodsWithPVCs(ctx context.Context, namespace string) ([]PodInfo, error) {
	// List PVCs first — this is the small set we care about
	pvcs, err := c.Clientset.CoreV1().PersistentVolumeClaims(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list PVCs in namespace %s: %w", namespace, err)
	}
	if len(pvcs.Items) == 0 {
		return nil, nil
	}

	pvcMap := make(map[string]*corev1.PersistentVolumeClaim)
	for i := range pvcs.Items {
		if pvcs.Items[i].Status.Phase == corev1.ClaimBound {
			pvcMap[pvcs.Items[i].Name] = &pvcs.Items[i]
		}
	}

	// If no bound PVCs exist, no pods can have usable mounts
	if len(pvcMap) == 0 {
		return nil, nil
	}

	// Now list pods — we still need to list all pods in the namespace, but this
	// is a single API call scoped to one namespace (not all namespaces).
	pods, err := c.Clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods in namespace %s: %w", namespace, err)
	}

	var result []PodInfo

	for i := range pods.Items {
		pod := &pods.Items[i]
		if !hasPVCVolumes(pod) {
			continue
		}

		podInfo := PodInfo{
			Name:      pod.Name,
			Namespace: pod.Namespace,
			Status:    string(pod.Status.Phase),
		}

		// Build a map of volume name -> PVC claim name
		volumeToClaim := make(map[string]string)
		for _, vol := range pod.Spec.Volumes {
			if vol.PersistentVolumeClaim != nil {
				volumeToClaim[vol.Name] = vol.PersistentVolumeClaim.ClaimName
			}
		}

		// Extract volume mount info from all containers
		mountPaths := make(map[string]string)
		mountReadOnly := make(map[string]bool)
		for _, container := range pod.Spec.Containers {
			for _, mount := range container.VolumeMounts {
				if _, isPVC := volumeToClaim[mount.Name]; isPVC {
					mountPaths[mount.Name] = mount.MountPath
					mountReadOnly[mount.Name] = mount.ReadOnly
				}
			}
		}

		for volName, claimName := range volumeToClaim {
			info := PVCVolumeInfo{
				VolumeName: volName,
				ClaimName:  claimName,
				MountPath:  mountPaths[volName],
				ReadOnly:   mountReadOnly[volName],
			}

			// Enrich with PVC details
			if pvc, ok := pvcMap[claimName]; ok {
				if storage, ok := pvc.Status.Capacity[corev1.ResourceStorage]; ok {
					info.Size = storage.String()
				}
				if len(pvc.Status.AccessModes) > 0 {
					info.AccessModes = formatAccessModes(pvc.Status.AccessModes)
				}
				if pvc.Spec.StorageClassName != nil {
					info.StorageClass = *pvc.Spec.StorageClassName
				}
			}

			podInfo.PVCVolumes = append(podInfo.PVCVolumes, info)
		}

		sort.Slice(podInfo.PVCVolumes, func(i, j int) bool {
			return podInfo.PVCVolumes[i].VolumeName < podInfo.PVCVolumes[j].VolumeName
		})

		result = append(result, podInfo)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})

	return result, nil
}

func hasPVCVolumes(pod *corev1.Pod) bool {
	for _, vol := range pod.Spec.Volumes {
		if vol.PersistentVolumeClaim != nil {
			return true
		}
	}
	return false
}

func formatAccessModes(modes []corev1.PersistentVolumeAccessMode) string {
	modeMap := map[corev1.PersistentVolumeAccessMode]string{
		corev1.ReadWriteOnce:    "RWO",
		corev1.ReadOnlyMany:     "ROX",
		corev1.ReadWriteMany:    "RWX",
		corev1.ReadWriteOncePod: "RWOP",
	}
	result := ""
	for i, mode := range modes {
		if i > 0 {
			result += ","
		}
		if short, ok := modeMap[mode]; ok {
			result += short
		} else {
			result += string(mode)
		}
	}
	return result
}
