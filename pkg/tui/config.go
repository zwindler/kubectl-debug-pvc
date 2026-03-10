package tui

import (
	"fmt"
	"strings"
)

// configView renders the configuration input step (image and mount prefix).
func configView(m model) string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("Configure Debug Container"))
	b.WriteString("\n")
	b.WriteString(subtitleStyle.Render(fmt.Sprintf("Pod: %s/%s", m.selectedNamespace, m.selectedPod.Name)))
	b.WriteString("\n\n")

	// Show selected volumes summary
	b.WriteString(dimStyle.Render("  Volumes to mount:"))
	b.WriteString("\n")
	for i, vol := range m.selectedPod.PVCVolumes {
		if m.volumeSelected[i] {
			fmt.Fprintf(&b, "    %s %s\n",
				successStyle.Render("*"),
				vol.VolumeName)
		}
	}
	b.WriteString("\n")

	// Image input
	imageLabel := "  Image: "
	mountLabel := "  Mount prefix: "

	if m.configField == 0 {
		b.WriteString(cursorStyle.Render("> "))
		b.WriteString(filterPromptStyle.Render("Image: "))
		b.WriteString(filterTextStyle.Render(m.imageInput))
		b.WriteString(cursorStyle.Render("|"))
		b.WriteString("\n")
		fmt.Fprintf(&b, "%s%s\n", dimStyle.Render(mountLabel), dimStyle.Render(m.mountPrefix))
	} else {
		fmt.Fprintf(&b, "  %s%s\n", dimStyle.Render(imageLabel[2:]), m.imageInput)
		b.WriteString(cursorStyle.Render("> "))
		b.WriteString(filterPromptStyle.Render("Mount prefix: "))
		b.WriteString(filterTextStyle.Render(m.mountPrefix))
		b.WriteString(cursorStyle.Render("|"))
		b.WriteString("\n")
	}

	// Show resulting mounts preview
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("  Mount preview:"))
	b.WriteString("\n")
	for i, vol := range m.selectedPod.PVCVolumes {
		if m.volumeSelected[i] {
			mountPath := fmt.Sprintf("%s/%s", m.mountPrefix, vol.VolumeName)
			fmt.Fprintf(&b, "    %s -> %s\n",
				vol.VolumeName,
				subtitleStyle.Render(mountPath))
		}
	}

	b.WriteString(helpStyle.Render("\n  tab: next field | enter: create debug container | esc: back | ctrl+c: quit"))

	return b.String()
}
