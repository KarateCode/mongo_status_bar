package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"image/color"
	"os"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type model struct {
	tunnelActive bool
	width        int
	height       int
	rowStatus    string
}

type tickMsg time.Time
type stdinMsg string

var (
	yellow = color.RGBA{0xFF, 0xCC, 0x00, 0xFF}
	red    = color.RGBA{0xFF, 0x00, 0x00, 0xFF}

	localStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF")).
			Padding(0, 1)
)

func gradientTunnelActive() string {
	text := "   TUNNEL ACTIVE   "
	runes := []rune(text)
	n := len(runes) + 2 // +2 for the cap characters
	var out strings.Builder
	darkGray := lipgloss.Color("#333333")

	// Leading cap
	t := float64(0) / float64(n-1)
	rC := uint8(float64(yellow.R)*(1-t) + float64(red.R)*t)
	gC := uint8(float64(yellow.G)*(1-t) + float64(red.G)*t)
	bC := uint8(float64(yellow.B)*(1-t) + float64(red.B)*t)
	col := lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", rC, gC, bC))
	out.WriteString(lipgloss.NewStyle().Foreground(col).Render(""))

	// Main text
	for i, r := range runes {
		t := float64(i+1) / float64(n-1)
		rC := uint8(float64(yellow.R)*(1-t) + float64(red.R)*t)
		gC := uint8(float64(yellow.G)*(1-t) + float64(red.G)*t)
		bC := uint8(float64(yellow.B)*(1-t) + float64(red.B)*t)
		col := lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", rC, gC, bC))

		out.WriteString(lipgloss.NewStyle().Foreground(darkGray).Background(col).Render(string(r)))
	}

	// Trailing cap
	t = float64(n-1) / float64(n-1)
	rC = uint8(float64(yellow.R)*(1-t) + float64(red.R)*t)
	gC = uint8(float64(yellow.G)*(1-t) + float64(red.G)*t)
	bC = uint8(float64(yellow.B)*(1-t) + float64(red.B)*t)
	col = lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", rC, gC, bC))
	out.WriteString(lipgloss.NewStyle().Foreground(col).Render(""))

	return out.String()
}

func checkTunnel() bool {
	cmd := exec.Command("lsof", "-i", ":27018")
	err := cmd.Run()
	return err == nil
}

func getDatabaseName() string {
	cmd := exec.Command("bash", "-c", `tmux show-environment -s DATABASE_NAME | cut -d '"' -f 2`)
	output, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(output))
}

func tickCmd() tea.Cmd {
	return tea.Tick(10*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func waitForStdin(reader *bufio.Reader) tea.Cmd {
	return func() tea.Msg {
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil
		}
		return stdinMsg(line)
	}
}

func parseRowCount(content string) string {
	if content == "" {
		return ""
	}

	var arr []any
	if err := json.Unmarshal([]byte(content), &arr); err == nil {
		if len(arr) == 1 {
			return "1 row returned"
		}
		return fmt.Sprintf("%d rows returned", len(arr))
	}

	var obj any
	if err := json.Unmarshal([]byte(content), &obj); err == nil {
		return "1 row returned"
	}

	return ""
}

func (m model) Init() tea.Cmd {
	return tickCmd()
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tickMsg:
		m.tunnelActive = checkTunnel()
		return m, tickCmd()
	case stdinMsg:
		m.rowStatus = parseRowCount(string(msg))
		return m, nil
	}
	return m, nil
}

func (m model) View() string {
	var tunnelBox string
	if m.tunnelActive {
		tunnelBox = gradientTunnelActive()
	} else {
		dbName := getDatabaseName()
		tunnelBox = localStyle.Render(fmt.Sprintf("Local connection: %s", dbName))
	}

	var content string
	if m.rowStatus != "" {
		statusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
		content = lipgloss.JoinVertical(lipgloss.Center, tunnelBox, statusStyle.Render(m.rowStatus))
	} else {
		content = tunnelBox
	}

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
}

func main() {
	m := model{
		tunnelActive: checkTunnel(),
	}

	reader := bufio.NewReader(os.Stdin)

	// Open /dev/tty for keyboard input since stdin is used for piped data
	tty, err := os.Open("/dev/tty")
	if err != nil {
		fmt.Printf("Error opening tty: %v\n", err)
		os.Exit(1)
	}
	defer tty.Close()

	p := tea.NewProgram(m, tea.WithInput(tty))

	// Read stdin in a goroutine and send messages to the program
	go func() {
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			p.Send(stdinMsg(line))
		}
	}()

	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}
