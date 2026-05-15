// Package pdf renders a URL to a PDF file using a headless Chrome binary.
//
// We shell out to chrome --headless=new --print-to-pdf rather than embed
// a DevTools client: zero Go dependencies, identical output, and the
// binary is already on most dev machines.
package pdf

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// Render prints url to outPath using the system Chrome/Chromium.
// outPath is created (or overwritten). url should be reachable from the
// machine running the command (typically http://localhost:8080/vN).
func Render(url, outPath string) error {
	bin, err := findChrome()
	if err != nil {
		return err
	}
	abs, err := filepath.Abs(outPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return err
	}

	cmd := exec.Command(bin,
		"--headless=new",
		"--disable-gpu",
		"--no-sandbox",
		"--hide-scrollbars",
		"--virtual-time-budget=10000",
		"--run-all-compositor-stages-before-draw",
		"--print-to-pdf-no-header",
		"--print-to-pdf="+abs,
		url,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("chrome: %w\n%s", err, string(out))
	}
	if _, err := os.Stat(abs); err != nil {
		return fmt.Errorf("chrome ran but produced no PDF at %s: %w\n%s", abs, err, string(out))
	}
	return nil
}

func findChrome() (string, error) {
	if env := os.Getenv("CHROME_BIN"); env != "" {
		if _, err := os.Stat(env); err == nil {
			return env, nil
		}
	}
	candidates := []string{
		"google-chrome",
		"google-chrome-stable",
		"chromium",
		"chromium-browser",
		"chrome",
	}
	for _, c := range candidates {
		if p, err := exec.LookPath(c); err == nil {
			return p, nil
		}
	}
	if runtime.GOOS == "darwin" {
		mac := []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
			"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
		}
		for _, p := range mac {
			if _, err := os.Stat(p); err == nil {
				return p, nil
			}
		}
	}
	return "", errors.New("Chrome/Chromium not found — install Google Chrome or set CHROME_BIN")
}
