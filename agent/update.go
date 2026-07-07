package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
)

const releasesLatest = "https://api.github.com/repos/adsbiq/adsbiq-airport/releases/latest"

type ghRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

// assetSuffix is what a release asset for THIS platform is named with, e.g.
// "adsbiq-feed-agent-windows-amd64.exe".
func assetSuffix() string {
	s := fmt.Sprintf("%s-%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		return s + ".exe"
	}
	return s
}

// checkUpdate returns (newerTag, assetURL) if the latest release is newer than
// current and ships a binary for this platform; empty strings mean up to date.
func checkUpdate(current string) (string, string, error) {
	req, _ := http.NewRequest(http.MethodGet, releasesLatest, nil)
	req.Header.Set("User-Agent", "adsbiq-feed-agent/"+current)
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", "", fmt.Errorf("releases: %d", resp.StatusCode)
	}
	var rel ghRelease
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&rel); err != nil {
		return "", "", err
	}
	if !semverNewer(current, rel.TagName) {
		return "", "", nil
	}
	suf := assetSuffix()
	for _, a := range rel.Assets {
		if strings.HasSuffix(a.Name, suf) {
			return rel.TagName, a.URL, nil
		}
	}
	return "", "", fmt.Errorf("release %s has no asset for %s", rel.TagName, suf)
}

// semverNewer reports whether tag (e.g. "v0.3.0") is a higher version than
// current (e.g. "0.2.0"). Non-numeric parts are ignored conservatively.
func semverNewer(current, tag string) bool {
	c := parseVer(current)
	t := parseVer(tag)
	for i := 0; i < 3; i++ {
		if t[i] != c[i] {
			return t[i] > c[i]
		}
	}
	return false
}

func parseVer(s string) [3]int {
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	var v [3]int
	for i, p := range strings.SplitN(s, ".", 3) {
		if i > 2 {
			break
		}
		n, _ := strconv.Atoi(strings.TrimFunc(p, func(r rune) bool { return r < '0' || r > '9' }))
		v[i] = n
	}
	return v
}

// selfUpdate downloads assetURL and swaps it in for the running executable.
// Windows permits renaming a running .exe, so we rename current -> .old, drop
// the new binary in place, and let the service restart into it.
func selfUpdate(assetURL string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	newPath := exe + ".new"
	if err := download(assetURL, newPath); err != nil {
		return err
	}
	_ = os.Chmod(newPath, 0o755)
	oldPath := exe + ".old"
	_ = os.Remove(oldPath)
	if err := os.Rename(exe, oldPath); err != nil {
		os.Remove(newPath)
		return fmt.Errorf("move running exe aside: %w", err)
	}
	if err := os.Rename(newPath, exe); err != nil {
		_ = os.Rename(oldPath, exe) // roll back
		return fmt.Errorf("install new exe: %w", err)
	}
	return nil
}

func download(url, dst string) error {
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("User-Agent", "adsbiq-feed-agent/"+Version)
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("download %s: %d", url, resp.StatusCode)
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}
