//go:build !windows && !linux && !darwin

package main

func InfoBox(title string, content string) {
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
