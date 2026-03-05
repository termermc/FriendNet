package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

var checkedBins bool
var hasKdialog bool
var hasZenity bool
var hasYad bool
var hasXmessage bool

func hasBin(bin string) bool {
	// Check for binary in PATH.
	_, err := exec.LookPath(bin)
	return err == nil
}

func kdialog(title string, content string) {
	// kdialog --title "..." --msgbox "..."
	cmd := exec.Command("kdialog", "--title", title, "--msgbox", content)
	_ = cmd.Run()
}

func zenity(title string, content string) {
	// zenity --info --title="..." --text="..."
	cmd := exec.Command("zenity", "--info", "--title="+title, "--text="+content)
	_ = cmd.Run()
}

func yad(title string, content string) {
	// yad --title="..." --text="..." --button=OK
	cmd := exec.Command("yad", "--title="+title, "--text="+content, "--button=OK")
	_ = cmd.Run()
}

func xmessage(title string, content string) {
	// xmessage -title "..." "..."
	cmd := exec.Command("xmessage", "-title", title, content)
	_ = cmd.Run()
}

func ttyEnter(title string, content string) {
	// Println title and content, then wait for enter to be pressed.
	if title != "" {
		fmt.Println(title)
		fmt.Println(strings.Repeat("-", len([]rune(title))))
	}
	if content != "" {
		fmt.Println(content)
	}
	fmt.Print("\nPress Enter to continue...")
	_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
}

func InfoBox(title string, content string) {
	if !checkedBins {
		checkedBins = true

		// Check existence of binaries. Stop at first found one.
		switch {
		case hasBin("kdialog"):
			hasKdialog = true
		case hasBin("zenity"):
			hasZenity = true
		case hasBin("yad"):
			hasYad = true
		case hasBin("xmessage"):
			hasXmessage = true
		}
	}

	// Choose which function to call based on existence of binaries.
	// If none, call ttyEnter.
	switch {
	case hasKdialog:
		kdialog(title, content)
	case hasZenity:
		zenity(title, content)
	case hasYad:
		yad(title, content)
	case hasXmessage:
		xmessage(title, content)
	default:
		ttyEnter(title, content)
	}
}
