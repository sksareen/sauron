package daemon

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// GetFrontmostApp returns the current frontmost application name and bundle ID.
// Window title is always empty: reading it goes through the Accessibility API
// and makes macOS prompt for permission. App identity comes from lsappinfo,
// which does not.
func GetFrontmostApp() (appName, bundleID, windowTitle string, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	frontCmd := exec.CommandContext(ctx, "lsappinfo", "front")
	frontOut, err := frontCmd.Output()
	if err != nil {
		return "", "", "", fmt.Errorf("lsappinfo front: %w", err)
	}
	asn := strings.TrimSpace(string(frontOut))
	if asn == "" {
		return "", "", "", fmt.Errorf("lsappinfo front: empty")
	}

	infoCmd := exec.CommandContext(ctx, "lsappinfo", "info", "-only", "name,bundleid", asn)
	infoOut, infoErr := infoCmd.Output()
	if infoErr == nil {
		appName, bundleID = parseLSAppInfo(string(infoOut))
	}
	if appName == "" {
		appName = parseFrontAppName(asn)
	}
	if appName == "" {
		if infoErr != nil {
			return "", "", "", fmt.Errorf("lsappinfo info: %w", infoErr)
		}
		return "", "", "", fmt.Errorf("lsappinfo: no display name")
	}
	return appName, bundleID, "", nil
}

// parseLSAppInfo extracts LSDisplayName (or CFBundleName) and CFBundleIdentifier
// from `lsappinfo info` output.
func parseLSAppInfo(out string) (name, bundleID string) {
	name = quotedInfoValue(out, "LSDisplayName")
	if name == "" {
		name = quotedInfoValue(out, "CFBundleName")
	}
	bundleID = quotedInfoValue(out, "CFBundleIdentifier")
	return name, bundleID
}

// parseFrontAppName reads a trailing quoted name from `lsappinfo front`
// when the info subcommand does not return one.
func parseFrontAppName(front string) string {
	i := strings.LastIndex(front, `"`)
	if i <= 0 {
		return ""
	}
	rest := front[:i]
	j := strings.LastIndex(rest, `"`)
	if j < 0 || j == i {
		return ""
	}
	return rest[j+1:]
}

func quotedInfoValue(out, key string) string {
	token := `"` + key + `"`
	i := strings.Index(out, token)
	if i < 0 {
		return ""
	}
	rest := strings.TrimLeft(out[i+len(token):], " \t=")
	if !strings.HasPrefix(rest, `"`) {
		return ""
	}
	rest = rest[1:]
	j := strings.Index(rest, `"`)
	if j < 0 {
		return ""
	}
	return rest[:j]
}

// GetClipboardContent returns the current clipboard text content.
func GetClipboardContent() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "pbpaste")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("pbpaste: %w", err)
	}
	return out.String(), nil
}
