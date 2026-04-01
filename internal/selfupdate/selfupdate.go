package selfupdate

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const repo = "kashportsa/kp-gruuk"

// CheckAndUpdate checks GitHub for a newer release and atomically replaces the
// binary if one is found. Prints progress to stdout and returns true if the
// binary was replaced — the caller must re-exec to run the new version.
// Silently no-ops on any error, network timeout, or if already up to date.
func CheckAndUpdate(currentVersion string) bool {
	// Skip dev builds and dirty/ahead builds (e.g. "v0.0.4-3-gabcdef").
	if currentVersion == "" || currentVersion == "dev" || strings.Contains(currentVersion, "-") {
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	latest, downloadURL, err := latestRelease(ctx)
	if err != nil || latest == currentVersion || !isNewer(latest, currentVersion) {
		return false
	}

	fmt.Printf("  Updating gruuk %s -> %s...\n", currentVersion, latest)

	if err := replace(ctx, downloadURL); err != nil {
		fmt.Printf("  Update failed, continuing with current version: %v\n", err)
		return false
	}

	fmt.Printf("  Updated! Restarting...\n\n")
	return true
}

// Reexec replaces the current process with the updated binary using the same
// arguments. On success this call never returns. Returns an error only if
// os.Executable or syscall.Exec fail.
func Reexec() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return syscall.Exec(exe, os.Args, os.Environ())
}

// --- internal ----------------------------------------------------------------

type githubRelease struct {
	TagName string        `json:"tag_name"`
	Assets  []githubAsset `json:"assets"`
}

type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

func latestRelease(ctx context.Context) (tag, downloadURL string, err error) {
	url := "https://api.github.com/repos/" + repo + "/releases/latest"
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("github API: status %d", resp.StatusCode)
	}

	var r githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return "", "", err
	}

	wantAsset := "gruuk-" + runtime.GOOS + "-" + runtime.GOARCH
	for _, a := range r.Assets {
		if a.Name == wantAsset {
			return r.TagName, a.BrowserDownloadURL, nil
		}
	}

	return "", "", fmt.Errorf("no asset %q in release %s", wantAsset, r.TagName)
}

// replace downloads the binary and atomically overwrites the current executable.
func replace(ctx context.Context, url string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download: status %d", resp.StatusCode)
	}

	exe, err := os.Executable()
	if err != nil {
		return err
	}

	tmp := exe + ".update"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		return fmt.Errorf("cannot write update (try reinstalling with sudo): %w", err)
	}

	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}

	if err := os.Rename(tmp, exe); err != nil {
		os.Remove(tmp)
		return err
	}

	return nil
}

// isNewer returns true if vNext is strictly greater than vCurrent in semver.
// Both should be "vMAJOR.MINOR.PATCH"; malformed parts are treated as 0.
func isNewer(vNext, vCurrent string) bool {
	a := parseSemver(vNext)
	b := parseSemver(vCurrent)
	for i := range a {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}
	return false
}

func parseSemver(v string) [3]int {
	v = strings.TrimPrefix(v, "v")
	parts := strings.SplitN(v, ".", 3)
	var out [3]int
	for i := 0; i < 3 && i < len(parts); i++ {
		out[i], _ = strconv.Atoi(parts[i])
	}
	return out
}
