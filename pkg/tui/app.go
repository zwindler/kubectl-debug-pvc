package tui

import (
	"context"
	"fmt"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/zwindler/kubectl-debug-pvc/pkg/k8s"
)

// step represents the current step in the TUI wizard.
type step int

const (
	stepLoading   step = iota
	stepNamespace      // Select namespace
	stepPod            // Select pod
	stepVolume         // Select volumes
	stepConfig         // Configure image and mount prefix
	stepProgress       // Creating container / showing result
	stepDone           // Complete
	stepError          // Fatal error
)

// Messages for async operations
type namespacesLoadedMsg struct {
	namespaces []k8s.NamespaceInfo
	err        error
}

type podsLoadedMsg struct {
	pods []k8s.PodInfo
	err  error
}

type containerCreatedMsg struct {
	containerName string
	warnings      []string
	err           error
}

// model is the Bubble Tea model for the TUI application.
type model struct {
	k8sClient *k8s.Client

	// State machine
	currentStep step

	// Loading
	spinner    spinner.Model
	loadingMsg string

	// Namespace step
	namespaces        []k8s.NamespaceInfo
	selectedNamespace string

	// Pod step
	pods        []k8s.PodInfo
	selectedPod k8s.PodInfo

	// Volume step
	volumeSelected []bool

	// Config step
	imageInput  string
	mountPrefix string
	configField int // 0 = image, 1 = mount prefix

	// Progress step
	creating      bool
	containerName string
	warnings      []string

	// Common
	cursor         int
	viewportOffset int // top index of visible window for list views
	filterMode     bool
	filterText     string
	err            error
	width          int
	height         int
}

// NewModel creates a new TUI model.
func NewModel(client *k8s.Client) model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(primaryColor)

	return model{
		k8sClient:   client,
		currentStep: stepLoading,
		spinner:     s,
		loadingMsg:  "Discovering namespaces with PVCs...",
		imageInput:  "ubuntu:latest",
		mountPrefix: "/debug",
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		m.loadNamespaces(),
	)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case namespacesLoadedMsg:
		if msg.err != nil {
			m.currentStep = stepError
			m.err = msg.err
			return m, nil
		}
		if len(msg.namespaces) == 0 {
			m.currentStep = stepError
			m.err = fmt.Errorf("no namespaces with PVCs found in the cluster")
			return m, nil
		}
		m.namespaces = msg.namespaces
		m.currentStep = stepNamespace
		m.cursor = 0
		m.viewportOffset = 0
		return m, nil

	case podsLoadedMsg:
		if msg.err != nil {
			m.currentStep = stepError
			m.err = msg.err
			return m, nil
		}
		m.pods = msg.pods
		m.currentStep = stepPod
		m.cursor = 0
		m.viewportOffset = 0
		m.filterText = ""
		m.filterMode = false
		return m, nil

	case containerCreatedMsg:
		m.creating = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.containerName = msg.containerName
		m.warnings = msg.warnings
		return m, nil

	case tea.KeyMsg:
		return m.handleKeyPress(msg)
	}

	return m, nil
}

func (m model) handleKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Global quit
	if msg.Type == tea.KeyCtrlC {
		return m, tea.Quit
	}

	switch m.currentStep {
	case stepNamespace:
		return m.handleNamespaceKey(msg)
	case stepPod:
		return m.handlePodKey(msg)
	case stepVolume:
		return m.handleVolumeKey(msg)
	case stepConfig:
		return m.handleConfigKey(msg)
	case stepProgress:
		return m.handleProgressKey(msg)
	case stepError:
		if msg.String() == "q" {
			return m, tea.Quit
		}
	}

	return m, nil
}

func (m model) handleNamespaceKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	filtered := filterNamespaces(m.namespaces, m.filterText)

	if m.filterMode {
		switch msg.Type {
		case tea.KeyEscape:
			m.filterMode = false
			m.filterText = ""
			m.cursor = 0
			m.viewportOffset = 0
			return m, nil
		case tea.KeyBackspace:
			if len(m.filterText) > 0 {
				m.filterText = m.filterText[:len(m.filterText)-1]
				m.cursor = 0
				m.viewportOffset = 0
			}
			return m, nil
		case tea.KeyEnter:
			if len(filtered) > 0 && m.cursor < len(filtered) {
				m.selectedNamespace = filtered[m.cursor].Name
				m.filterText = ""
				m.filterMode = false
				m.currentStep = stepLoading
				m.loadingMsg = fmt.Sprintf("Loading pods in %s...", m.selectedNamespace)
				return m, tea.Batch(m.spinner.Tick, m.loadPods())
			}
			return m, nil
		default:
			if msg.Type == tea.KeyRunes {
				m.filterText += string(msg.Runes)
				m.cursor = 0
				m.viewportOffset = 0
			}
			return m, nil
		}
	}

	switch msg.String() {
	case "q":
		return m, tea.Quit
	case "/":
		m.filterMode = true
		m.filterText = ""
		return m, nil
	case "j", "down":
		if m.cursor < len(filtered)-1 {
			m.cursor++
			m.viewportOffset = clampViewport(m.cursor, m.viewportOffset, m.height)
		}
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
			m.viewportOffset = clampViewport(m.cursor, m.viewportOffset, m.height)
		}
	case "enter":
		if len(filtered) > 0 && m.cursor < len(filtered) {
			m.selectedNamespace = filtered[m.cursor].Name
			m.currentStep = stepLoading
			m.loadingMsg = fmt.Sprintf("Loading pods in %s...", m.selectedNamespace)
			return m, tea.Batch(m.spinner.Tick, m.loadPods())
		}
	}
	return m, nil
}

func (m model) handlePodKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	filtered := filterPods(m.pods, m.filterText)

	if m.filterMode {
		switch msg.Type {
		case tea.KeyEscape:
			m.filterMode = false
			m.filterText = ""
			m.cursor = 0
			m.viewportOffset = 0
			return m, nil
		case tea.KeyBackspace:
			if len(m.filterText) > 0 {
				m.filterText = m.filterText[:len(m.filterText)-1]
				m.cursor = 0
				m.viewportOffset = 0
			}
			return m, nil
		case tea.KeyEnter:
			if len(filtered) > 0 && m.cursor < len(filtered) {
				m.selectedPod = filtered[m.cursor]
				m.volumeSelected = make([]bool, len(m.selectedPod.PVCVolumes))
				m.filterText = ""
				m.filterMode = false
				m.currentStep = stepVolume
				m.cursor = 0
				m.viewportOffset = 0
			}
			return m, nil
		default:
			if msg.Type == tea.KeyRunes {
				m.filterText += string(msg.Runes)
				m.cursor = 0
				m.viewportOffset = 0
			}
			return m, nil
		}
	}

	switch msg.String() {
	case "q":
		return m, tea.Quit
	case "/":
		m.filterMode = true
		m.filterText = ""
		return m, nil
	case "j", "down":
		if m.cursor < len(filtered)-1 {
			m.cursor++
			m.viewportOffset = clampViewport(m.cursor, m.viewportOffset, m.height)
		}
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
			m.viewportOffset = clampViewport(m.cursor, m.viewportOffset, m.height)
		}
	case "enter":
		if len(filtered) > 0 && m.cursor < len(filtered) {
			m.selectedPod = filtered[m.cursor]
			m.volumeSelected = make([]bool, len(m.selectedPod.PVCVolumes))
			m.currentStep = stepVolume
			m.cursor = 0
			m.viewportOffset = 0
		}
	case "esc":
		m.currentStep = stepNamespace
		m.cursor = 0
		m.viewportOffset = 0
		m.filterText = ""
		m.filterMode = false
	}
	return m, nil
}

func (m model) handleVolumeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		return m, tea.Quit
	case "j", "down":
		if m.cursor < len(m.selectedPod.PVCVolumes)-1 {
			m.cursor++
		}
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}
	case " ":
		if m.cursor < len(m.volumeSelected) {
			m.volumeSelected[m.cursor] = !m.volumeSelected[m.cursor]
		}
	case "enter":
		// Check at least one volume is selected
		hasSelection := false
		for _, v := range m.volumeSelected {
			if v {
				hasSelection = true
				break
			}
		}
		if hasSelection {
			m.currentStep = stepConfig
			m.configField = 0
		}
	case "esc":
		m.currentStep = stepPod
		m.cursor = 0
		m.filterText = ""
		m.filterMode = false
	}
	return m, nil
}

func (m model) handleConfigKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyTab:
		m.configField = (m.configField + 1) % 2
		return m, nil
	case tea.KeyEscape:
		m.currentStep = stepVolume
		m.cursor = 0
		return m, nil
	case tea.KeyBackspace:
		if m.configField == 0 && len(m.imageInput) > 0 {
			m.imageInput = m.imageInput[:len(m.imageInput)-1]
		} else if m.configField == 1 && len(m.mountPrefix) > 0 {
			m.mountPrefix = m.mountPrefix[:len(m.mountPrefix)-1]
		}
		return m, nil
	case tea.KeyEnter:
		if m.imageInput == "" {
			m.imageInput = "ubuntu:latest"
		}
		if m.mountPrefix == "" {
			m.mountPrefix = "/debug"
		}
		m.currentStep = stepProgress
		m.creating = true
		m.err = nil
		return m, tea.Batch(m.spinner.Tick, m.createEphemeralContainer())
	case tea.KeyRunes:
		if m.configField == 0 {
			m.imageInput += string(msg.Runes)
		} else {
			m.mountPrefix += string(msg.Runes)
		}
		return m, nil
	}
	return m, nil
}

func (m model) handleProgressKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		return m, tea.Quit
	case "r":
		if m.err != nil {
			m.creating = true
			m.err = nil
			return m, tea.Batch(m.spinner.Tick, m.createEphemeralContainer())
		}
	case "enter":
		if m.containerName != "" && !m.creating {
			// Quit TUI and attach
			m.currentStep = stepDone
			return m, tea.Quit
		}
	case "esc":
		if m.err != nil {
			m.currentStep = stepConfig
			m.err = nil
		}
	}
	return m, nil
}

func (m model) View() string {
	switch m.currentStep {
	case stepLoading:
		return loadingView(m)
	case stepNamespace:
		return namespaceView(m)
	case stepPod:
		return podView(m)
	case stepVolume:
		return volumeView(m)
	case stepConfig:
		return configView(m)
	case stepProgress:
		return progressView(m)
	case stepDone:
		return doneView(m)
	case stepError:
		errMsg := "unknown error"
		if m.err != nil {
			errMsg = m.err.Error()
		}
		return errorView(errMsg)
	}
	return ""
}

// Async commands

func (m model) loadNamespaces() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		namespaces, err := m.k8sClient.DiscoverNamespacesWithPVCs(ctx)
		return namespacesLoadedMsg{namespaces: namespaces, err: err}
	}
}

func (m model) loadPods() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		pods, err := m.k8sClient.DiscoverPodsWithPVCs(ctx, m.selectedNamespace)
		return podsLoadedMsg{pods: pods, err: err}
	}
}

func (m model) createEphemeralContainer() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()

		var mounts []k8s.DebugVolumeMount
		for i, vol := range m.selectedPod.PVCVolumes {
			if m.volumeSelected[i] {
				mounts = append(mounts, k8s.DebugVolumeMount{
					VolumeName: vol.VolumeName,
					MountPath:  fmt.Sprintf("%s/%s", m.mountPrefix, vol.VolumeName),
				})
			}
		}

		containerName, warnings, err := m.k8sClient.CreateEphemeralContainer(ctx, k8s.EphemeralContainerOpts{
			PodName:      m.selectedPod.Name,
			Namespace:    m.selectedNamespace,
			Image:        m.imageInput,
			VolumeMounts: mounts,
		})

		return containerCreatedMsg{containerName: containerName, warnings: warnings, err: err}
	}
}

// GetAttachInfo returns the information needed to attach to the debug container.
// This is used after the TUI exits to perform the kubectl attach.
func (m model) GetAttachInfo() (namespace, pod, container string, shouldAttach bool) {
	return m.selectedNamespace, m.selectedPod.Name, m.containerName, m.currentStep == stepDone && m.containerName != ""
}

// Run starts the TUI application and returns attach info when done.
func Run(client *k8s.Client) (namespace, pod, container string, shouldAttach bool, err error) {
	m := NewModel(client)
	p := tea.NewProgram(m, tea.WithAltScreen())

	finalModel, err := p.Run()
	if err != nil {
		return "", "", "", false, fmt.Errorf("TUI error: %w", err)
	}

	fm := finalModel.(model)
	ns, podName, containerName, attach := fm.GetAttachInfo()
	return ns, podName, containerName, attach, nil
}

// clampViewport adjusts the viewport offset so the cursor stays within the
// visible window. headerLines accounts for the title/filter rows above the list.
func clampViewport(cursor, offset, termHeight int) int {
	const headerLines = 5 // title + subtitle + filter row + blank line + help
	visible := termHeight - headerLines
	if visible < 1 {
		visible = 1
	}
	if cursor < offset {
		return cursor
	}
	if cursor >= offset+visible {
		return cursor - visible + 1
	}
	return offset
}
