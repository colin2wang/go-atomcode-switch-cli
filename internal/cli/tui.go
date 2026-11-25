package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"go-atomcode-switch-cli/internal/i18n"

	"golang.org/x/term"
)

const (
	ansiClear = "\033[2J\033[H"
	ansiBold  = "\033[1m"
	ansiDim   = "\033[2m"
	ansiReset = "\033[0m"
)

type menuKey int

const (
	keyUp menuKey = iota
	keyDown
	keyEnter
	keyQuit
	keyOther
)

type tty struct {
	fd     int
	isTTY  bool
	origin *term.State
}

func stdinIsTTY() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

func newTTY() (*tty, error) {
	fd := int(os.Stdin.Fd())
	t := &tty{fd: fd, isTTY: stdinIsTTY()}
	if t.isTTY {
		st, err := term.MakeRaw(fd)
		if err != nil {
			return nil, err
		}
		t.origin = st
		enableRawModeExtras()
	}
	return t, nil
}

func (t *tty) restore() {
	if t.isTTY && t.origin != nil {
		_ = term.Restore(t.fd, t.origin)
		t.origin = nil
	}
}

func (t *tty) canonical(fn func() error) error {
	t.restore()
	err := fn()
	if t.isTTY {
		st, rawErr := term.MakeRaw(t.fd)
		if rawErr != nil {
			return rawErr
		}
		t.origin = st
		enableRawModeExtras()
	}
	return err
}

func (t *tty) readKey() menuKey {
	buf := make([]byte, 3)
	n, err := os.Stdin.Read(buf)
	if err != nil || n == 0 {
		return keyQuit
	}
	switch {
	case n == 1 && (buf[0] == '\r' || buf[0] == '\n'):
		return keyEnter
	case n == 1 && (buf[0] == 'q' || buf[0] == 'Q' || buf[0] == 3 || buf[0] == 27):
		return keyQuit
	case n >= 3 && buf[0] == 0x1b && buf[1] == '[' && buf[2] == 'A':
		return keyUp
	case n >= 3 && buf[0] == 0x1b && buf[1] == '[' && buf[2] == 'B':
		return keyDown
	default:
		return keyOther
	}
}

func (t *tty) choose(title string, items []string, footer string) int {
	if len(items) == 0 {
		return -1
	}
	cursor := 0
	for {
		fmt.Print(ansiClear)
		fmt.Println(ansiBold + title + ansiReset)
		fmt.Println()
		for i, item := range items {
			if i == cursor {
				fmt.Printf("%s> %s%s\n", ansiBold, item, ansiReset)
			} else {
				fmt.Printf("  %s\n", item)
			}
		}
		fmt.Println()
		if footer != "" {
			fmt.Println(ansiDim + footer + ansiReset)
		}
		fmt.Println(ansiDim + i18n.T("menu_choose_hint") + ansiReset)
		switch t.readKey() {
		case keyUp:
			if cursor > 0 {
				cursor--
			}
		case keyDown:
			if cursor < len(items)-1 {
				cursor++
			}
		case keyEnter:
			return cursor
		case keyQuit:
			return -1
		}
	}
}

func (t *tty) confirm() bool {
	return t.choose(i18n.T("menu_confirm_title"),
		[]string{i18n.T("menu_yes"), i18n.T("menu_no")}, "") == 0
}

func (t *tty) pause() {
	fmt.Println()
	fmt.Println(ansiDim + i18n.T("menu_press_enter") + ansiReset)
	for {
		switch t.readKey() {
		case keyEnter, keyQuit:
			return
		}
	}
}

func (t *tty) readLine(prompt, defaultVal string) string {
	var line string
	_ = t.canonical(func() error {
		if defaultVal != "" {
			fmt.Printf("%s [%s]: ", prompt, defaultVal)
		} else {
			fmt.Print(prompt)
		}
		s, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		line = strings.TrimSpace(s)
		if line == "" {
			line = defaultVal
		}
		return nil
	})
	fmt.Println()
	return line
}
