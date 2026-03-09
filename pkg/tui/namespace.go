package tui

import (
	"fmt"
	"strings"

	"github.com/zwindler/kubectl-debug-pvc/pkg/k8s"
)

// namespaceView renders the namespace selection step.
func namespaceView(m model) string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("Select Namespace"))
	b.WriteString("\n")
	b.WriteString(subtitleStyle.Render("Showing namespaces with PVCs"))
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

	filtered := filterNamespaces(m.namespaces, m.filterText)

	if len(filtered) == 0 {
		b.WriteString(dimStyle.Render("  No matching namespaces found"))
		b.WriteString("\n")
	} else {
		for i, ns := range filtered {
			cursor := "  "
			if i == m.cursor {
				cursor = cursorStyle.Render("> ")
			}

			name := ns.Name
			if i == m.cursor {
				name = selectedStyle.Render(ns.Name)
			}

			detail := dimStyle.Render(fmt.Sprintf(" (%d PVCs)", ns.PVCCount))
			fmt.Fprintf(&b, "%s%s%s\n", cursor, name, detail)
		}
	}

	b.WriteString(helpStyle.Render("\n  j/k or arrows: navigate | enter: select | /: filter | q: quit"))

	return b.String()
}

func filterNamespaces(namespaces []k8s.NamespaceInfo, filter string) []k8s.NamespaceInfo {
	if filter == "" {
		return namespaces
	}
	filter = strings.ToLower(filter)
	var result []k8s.NamespaceInfo
	for _, ns := range namespaces {
		if strings.Contains(strings.ToLower(ns.Name), filter) {
			result = append(result, ns)
		}
	}
	return result
}
