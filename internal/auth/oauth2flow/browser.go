package oauth2flow

import (
	"fmt"
	"os/exec"
	"runtime"
)

// openBrowser opens the given URL in the user's default browser.
func openBrowser(url string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		return fmt.Errorf("unsupported platform: %s — open this URL manually:\n%s", runtime.GOOS, url)
	}

	return cmd.Start()
}
