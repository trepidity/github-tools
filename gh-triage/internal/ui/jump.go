package ui

import (
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// editJumpDigits accepts a numeric prefix without intercepting other list keys.
func (m *Model) editJumpDigits(k tea.KeyMsg) bool {
	s := k.String()
	if k.Type == tea.KeyRunes && !k.Alt && s != "" && strings.Trim(s, "0123456789") == "" {
		// Bound the displayed input; startJump also checks integer overflow.
		if len(m.jumpDigits)+len(s) <= 20 {
			m.jumpDigits += s
		}
		return true
	}
	if s == "backspace" && m.jumpDigits != "" {
		m.jumpDigits = m.jumpDigits[:len(m.jumpDigits)-1]
		return true
	}
	return false
}

func (m *Model) keyJump(k tea.KeyMsg) tea.Cmd {
	if m.editJumpDigits(k) {
		return nil
	}
	switch k.String() {
	case "enter":
		return m.startJump()
	case "esc":
		m.mode, m.jumpDigits, m.status = modeNone, "", ""
	}
	return nil
}

func (m *Model) startJump() tea.Cmd {
	n, err := strconv.Atoi(m.jumpDigits)
	m.mode, m.jumpDigits, m.resume = modeNone, "", nil
	if err != nil || n <= 0 {
		m.status = "enter a positive issue number"
		return nil
	}
	m.jumpTarget = n
	return m.seekJump()
}

// seekJump selects the first matching issue in the current filtered queue. Search
// results may span repositories, so numbers cannot be used to infer page order.
func (m *Model) seekJump() tea.Cmd {
	for vi, i := range m.visible {
		if m.issues[i].Number == m.jumpTarget {
			m.cursor, m.jumpTarget, m.status = vi, 0, ""
			return nil
		}
	}
	if m.loadingPage || m.hasMore {
		m.status = fmt.Sprintf("looking for #%d… esc cancels", m.jumpTarget)
		if !m.loadingPage {
			m.loadingPage = true
			return m.fetchPage()
		}
		return nil
	}
	m.status = fmt.Sprintf("#%d is not in the current list", m.jumpTarget)
	m.jumpTarget = 0
	return nil
}
