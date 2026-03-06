package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/spf13/cobra"
)

const (
	githubReleasesURL = "https://api.github.com/repos/Luen/ffsync/releases/latest"
)

type githubRelease struct {
	TagName string        `json:"tag_name"`
	Assets  []githubAsset `json:"assets"`
}

type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// SelfupdateCmd returns the self-update command (download latest from GitHub and replace binary).
func SelfupdateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "selfupdate",
		Short: "Update ffsync to the latest release from GitHub",
		Long:  "Downloads the latest release from GitHub (Luen/ffsync) for the current OS and architecture, then replaces the running binary.",
		RunE:  runSelfupdate,
	}
}

func runSelfupdate(cmd *cobra.Command, args []string) error {
	assetName := currentAssetName()
	if assetName == "" {
		return fmt.Errorf("unsupported platform: %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	// Fetch latest release
	req, err := http.NewRequest(http.MethodGet, githubReleasesURL, nil)
	if err != nil {
		return fmt.Errorf("request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetching release: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("release API returned %s: %s", resp.Status, string(body))
	}

	var rel githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return fmt.Errorf("parsing release: %w", err)
	}

	var downloadURL string
	for _, a := range rel.Assets {
		if a.Name == assetName {
			downloadURL = a.BrowserDownloadURL
			break
		}
	}
	if downloadURL == "" {
		return fmt.Errorf("no asset %q in release %s (assets: %v)", assetName, rel.TagName, assetNames(rel.Assets))
	}

	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("executable path: %w", err)
	}
	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		return fmt.Errorf("resolving executable: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Downloading %s (%s) from %s ...\n", rel.TagName, assetName, downloadURL)

	// Download to same dir as current exe, with .new suffix
	dir := filepath.Dir(exePath)
	newPath := filepath.Join(dir, assetName+".new")
	if runtime.GOOS == "windows" {
		newPath = exePath + ".new"
	}

	if err := downloadFile(downloadURL, newPath); err != nil {
		return fmt.Errorf("download: %w", err)
	}

	if runtime.GOOS != "windows" {
		if err := os.Chmod(newPath, 0755); err != nil {
			os.Remove(newPath)
			return fmt.Errorf("chmod: %w", err)
		}
		if err := os.Rename(newPath, exePath); err != nil {
			os.Remove(newPath)
			return fmt.Errorf("replace binary: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Updated to %s. Run 'ffsync version' to confirm.\n", rel.TagName)
		return nil
	}

	// Windows: cannot replace running exe. Spawn a batch that waits for this process to exit, then replaces the binary.
	batchPath := filepath.Join(dir, "ffsync-update.bat")
	// Batch receives our PID as %1; loops until that process is gone, then replaces the exe (no fixed delay)
	batchContent := fmt.Sprintf("@echo off\n:wait\ntasklist /fi \"pid eq %%1\" 2>nul | find /i \"%%1\" >nul\nif not errorlevel 1 (ping -n 1 127.0.0.1 >nul & goto wait)\ndel %q\nren %q %s\nexit\n",
		exePath, newPath, filepath.Base(exePath))
	if err := os.WriteFile(batchPath, []byte(batchContent), 0600); err != nil {
		os.Remove(newPath)
		return fmt.Errorf("write update script: %w", err)
	}
	myPID := os.Getpid()
	if err := runDetached(batchPath, fmt.Sprint(myPID)); err != nil {
		os.Remove(newPath)
		os.Remove(batchPath)
		return fmt.Errorf("start update script: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Update will apply when this process exits. Run 'ffsync version' to confirm %s.\n", rel.TagName)
	return nil
}

func currentAssetName() string {
	suffix := ""
	if runtime.GOOS == "windows" {
		suffix = ".exe"
	}
	return "ffsync-" + runtime.GOOS + "-" + runtime.GOARCH + suffix
}

func assetNames(a []githubAsset) []string {
	names := make([]string, len(a))
	for i := range a {
		names[i] = a[i].Name
	}
	return names
}

func downloadFile(url, path string) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/octet-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("download %s: %s", resp.Status, string(body))
	}

	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, resp.Body)
	return err
}

// runDetached runs the batch script in a minimized window with the given args (e.g. PID to wait for).
func runDetached(batchPath string, args ...string) error {
	// /min = minimized window so the batch survives closing the parent terminal
	cmd := exec.Command("cmd", append([]string{"/c", "start", "/min", "", batchPath}, args...)...)
	return cmd.Start()
}
