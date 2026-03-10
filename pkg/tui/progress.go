package tui

import (
	"fmt"
	"strings"
)

// progressView renders the progress/status view during ephemeral container creation.
func progressView(m model) string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("Creating Debug Container"))
	b.WriteString("\n\n")

	// Show configuration summary
	b.WriteString(dimStyle.Render("  Namespace:  "))
	b.WriteString(m.selectedNamespace)
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("  Pod:        "))
	b.WriteString(m.selectedPod.Name)
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("  Image:      "))
	b.WriteString(m.imageInput)
	b.WriteString("\n")

	b.WriteString(dimStyle.Render("  Volumes:"))
	b.WriteString("\n")
	for i, vol := range m.selectedPod.PVCVolumes {
		if m.volumeSelected[i] {
			mountPath := fmt.Sprintf("%s/%s", m.mountPrefix, vol.VolumeName)
			fmt.Fprintf(&b, "    %s -> %s\n", vol.VolumeName, mountPath)
		}
	}
	b.WriteString("\n")

	// Show status
	if m.creating {
		fmt.Fprintf(&b, "  %s Creating ephemeral container...\n", m.spinner.View())
	} else if m.err != nil {
		b.WriteString(errorStyle.Render(fmt.Sprintf("  Error: %s\n", m.err.Error())))
		b.WriteString(helpStyle.Render("\n  Press r to retry | esc: back | q: quit"))
	} else if m.containerName != "" {
		b.WriteString(successStyle.Render(fmt.Sprintf("  Container '%s' created successfully!\n", m.containerName)))
		b.WriteString("\n")

		// Show warnings about inherited security context restrictions
		if len(m.warnings) > 0 {
			b.WriteString(warningStyle.Render("  Security context warnings:"))
			b.WriteString("\n")
			for _, w := range m.warnings {
				b.WriteString(warningStyle.Render(fmt.Sprintf("  ! %s", w)))
				b.WriteString("\n")
			}
			b.WriteString("\n")
		}

		// Show manual attach command
		b.WriteString(dimStyle.Render("  To attach manually:\n"))
		attachCmd := fmt.Sprintf("  kubectl attach -n %s %s -c %s -it",
			m.selectedNamespace, m.selectedPod.Name, m.containerName)
		b.WriteString(successStyle.Render(attachCmd))
		b.WriteString("\n")

		b.WriteString(helpStyle.Render("\n  Press enter to attach now | q: quit"))
	}

	return b.String()
}

// doneView renders the final view after attach completes.
func doneView(m model) string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("Debug Session Complete"))
	b.WriteString("\n\n")

	if m.containerName != "" {
		b.WriteString(dimStyle.Render("  Note: Ephemeral containers cannot be removed from a pod.\n"))
		b.WriteString(dimStyle.Render("  The container will remain until the pod is deleted.\n"))
		b.WriteString("\n")
		b.WriteString(dimStyle.Render("  To re-attach:\n"))
		attachCmd := fmt.Sprintf("  kubectl attach -n %s %s -c %s -it",
			m.selectedNamespace, m.selectedPod.Name, m.containerName)
		b.WriteString(successStyle.Render(attachCmd))
		b.WriteString("\n")
	}

	return b.String()
}

// loadingView renders a loading spinner with a message.
func loadingView(m model) string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("kubectl debug-pvc"))
	b.WriteString("\n\n")
	fmt.Fprintf(&b, "  %s %s", m.spinner.View(), m.loadingMsg)
	b.WriteString("\n")

	return b.String()
}

// errorView renders an error message.
func errorView(errMsg string) string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("kubectl debug-pvc"))
	b.WriteString("\n\n")
	b.WriteString(errorStyle.Render(fmt.Sprintf("  Error: %s", errMsg)))
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("\n  Press q to quit"))

	return b.String()
}
