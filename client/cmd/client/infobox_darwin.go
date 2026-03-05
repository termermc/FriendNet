package main

import (
	"fmt"
	"os/exec"
)

func InfoBox(title string, content string) {
	script := fmt.Sprintf(`display dialog %q with title %q buttons {"OK"} default button 1`, content, title)
	cmd := exec.Command("osascript", "-e", script)
	_ = cmd.Run()
}
