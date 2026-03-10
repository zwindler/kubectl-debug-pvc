package tui

import (
	"fmt"
	"strings"

	"github.com/zwindler/kubectl-debug-pvc/pkg/k8s"
)

// podView renders the pod selection step.
func podView(m model) string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("Select Pod"))
	b.WriteString("\n")
	b.WriteString(subtitleStyle.Render(fmt.Sprintf("Namespace: %s", m.selectedNamespace)))
	b.WriteString("\n")

	// Filter input
	if m.filterMode {
		b.WriteString(filterPromptStyle.Render("Filter: "))
		b.WriteString(filterTextStyle.Render(m.filterText))
		b.WriteString(cursorStyle.Render("|"))
		b.WriteString("\n\n")
	} else {
		b.WriteString(dimStyle.Render("Press / to filter"))
		b.WriteString("\n\n")
	}

	filtered := filterPods(m.pods, m.filterText)

	if len(filtered) == 0 {
		b.WriteString(dimStyle.Render("  No matching pods found"))
		b.WriteString("\n")
	} else {
		const headerLines = 5
		visible := m.height - headerLines
		if visible < 1 {
			visible = len(filtered)
		}
		end := m.viewportOffset + visible
		if end > len(filtered) {
			end = len(filtered)
		}
		for i := m.viewportOffset; i < end; i++ {
			pod := filtered[i]
			cursor := "  "
			if i == m.cursor {
				cursor = cursorStyle.Render("> ")
			}

			name := pod.Name
			if i == m.cursor {
				name = selectedStyle.Render(pod.Name)
			}

			status := dimStyle.Render(fmt.Sprintf(" (%s", pod.Status))

			// Show PVC names
			var pvcNames []string
			for _, vol := range pod.PVCVolumes {
				pvcNames = append(pvcNames, vol.ClaimName)
			}
			pvcs := dimStyle.Render(fmt.Sprintf(", PVCs: %s)", strings.Join(pvcNames, ", ")))

			fmt.Fprintf(&b, "%s%s%s%s\n", cursor, name, status, pvcs)
		}
		if len(filtered) > visible {
			shown := fmt.Sprintf("  %d-%d of %d", m.viewportOffset+1, end, len(filtered))
			b.WriteString(dimStyle.Render(shown))
			b.WriteString("\n")
		}
	}

	b.WriteString(helpStyle.Render("\n  j/k or arrows: navigate | enter: select | /: filter | esc: back | q: quit"))

	return b.String()
}

func filterPods(pods []k8s.PodInfo, filter string) []k8s.PodInfo {
	if filter == "" {
		return pods
	}
	filter = strings.ToLower(filter)
	var result []k8s.PodInfo
	for _, pod := range pods {
		if strings.Contains(strings.ToLower(pod.Name), filter) {
			result = append(result, pod)
		}
	}
	return result
}
