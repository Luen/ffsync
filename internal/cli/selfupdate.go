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
	githubReleasesURL = "https://api.github.com/repos/ffsync/ffsync/releases/latest"
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
		Long:  "Downloads the latest release from GitHub (ffsync/ffsync) for the current OS and architecture, then replaces the running binary.",
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

	// Windows: cannot replace running exe. Write a batch that runs after we exit.
	batchPath := filepath.Join(dir, "ffsync-update.bat")
	batchContent := fmt.Sprintf("@echo off\nping -n 3 127.0.0.1 >nul\ndel %q\nren %q %s\nexit\n",
		exePath, newPath, filepath.Base(exePath))
	if err := os.WriteFile(batchPath, []byte(batchContent), 0600); err != nil {
		os.Remove(newPath)
		return fmt.Errorf("write update script: %w", err)
	}
	// Run batch in background (detached) so it runs after we exit
	if err := runDetached(batchPath); err != nil {
		os.Remove(newPath)
		os.Remove(batchPath)
		return fmt.Errorf("start update script: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Update downloaded. Exit this process and run 'ffsync' again to complete the update (%s).\n", rel.TagName)
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

// runDetached runs the batch script in the background so it executes after we exit (Windows).
func runDetached(batchPath string) error {
	cmd := exec.Command("cmd", "/c", "start", "/b", "", batchPath)
	return cmd.Start()
}
