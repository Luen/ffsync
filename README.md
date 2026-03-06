# ffsync

A reliable, single-binary Go CLI that syncs a local directory to [FolderFort](https://folderfort.com) using the FolderFort web sync API. One-way mirror (local is source of truth), with optional deletes and dry-run. Similar in spirit to [rclone](https://rclone.org), but focused only on FolderFort.

**Requirements:** A [FolderFort](https://folderfort.com) account.

---

## Download and install

### Quickstart (all platforms)

1. **Download** the latest binary for your OS and CPU from [Releases](https://github.com/Luen/ffsync/releases).
2. **Extract** the executable (`ffsync` or `ffsync.exe` on Windows) and put it somewhere on your PATH.
3. **Configure** once with `ffsync config init`, then `ffsync config set email <your@email>` and `ffsync config set password <yourpassword>` (see [Configuration](#configuration)).
4. **Sync** with `ffsync sync <local_dir> remote:<path>`.

### Windows

1. Open the [Releases](https://github.com/Luen/ffsync/releases) page and download **ffsync-windows-amd64.exe** (or the ARM64 build if you use an ARM PC).
2. Rename it to `ffsync.exe` and move it to a folder on your PATH (e.g. `C:\Program Files\ffsync` or a folder you added to PATH).
3. Open PowerShell or CMD and run:

   ```powershell
   ffsync version
   ```

4. Configure (see [Configuration](#configuration)), then run e.g.:

   ```powershell
   ffsync sync "C:\Users\Downloads\my-files" remote:backup
   ```

**Optional – install via Go (if you have Go installed):**

```powershell
go install github.com/Luen/ffsync/cmd/ffsync@latest
```

The binary is installed to `%USERPROFILE%\go\bin\ffsync.exe`; ensure that directory is on your PATH.

### macOS

1. Open [Releases](https://github.com/Luen/ffsync/releases) and download the archive for your Mac:
   - **Apple Silicon (M1/M2/M3):** `ffsync-darwin-arm64`
   - **Intel:** `ffsync-darwin-amd64`
2. Unzip and move the binary to your PATH, e.g.:

   ```bash
   chmod +x ffsync-darwin-arm64
   sudo mv ffsync-darwin-arm64 /usr/local/bin/ffsync
   ```

3. If you see a "developer cannot be verified" warning, run:

   ```bash
   xattr -d com.apple.quarantine /usr/local/bin/ffsync
   ```

4. Configure (see [Configuration](#configuration)).

**Optional – install via Go:**

```bash
go install github.com/Luen/ffsync/cmd/ffsync@latest
```

The binary is installed to `~/go/bin/ffsync`; ensure `~/go/bin` is on your PATH.

### Linux

1. Open [Releases](https://github.com/Luen/ffsync/releases) and download the binary for your architecture:
   - **x86_64 / amd64:** `ffsync-linux-amd64`
   - **Raspberry Pi 4 / aarch64:** `ffsync-linux-arm64`
2. Install to a directory on your PATH, e.g.:

   ```bash
   chmod +x ffsync-linux-amd64
   sudo mv ffsync-linux-amd64 /usr/local/bin/ffsync
   ```

3. Configure (see [Configuration](#configuration)).

### Updating

To update to the latest release from GitHub:

```bash
ffsync selfupdate
```

This downloads the release that matches your OS and architecture and replaces the current binary. On Windows, exit the terminal after the command and run `ffsync` again to complete the update.

---

## Configuration

ffsync needs your FolderFort **email** and **password**. You can use a config file, environment variables, or both (env overrides the file).

### Option 1: Config file (recommended)

1. Create the config file and set your credentials:

   ```bash
   ffsync config init
   ffsync config set email "your@email.com"
   ffsync config set password "yourpassword"
   ```

2. Optionally set the server (if not using the default):

   ```bash
   ffsync config set base_url "https://na.folderfort.com"
   ```

3. Check that config is loaded (values are redacted):

   ```bash
   ffsync config show
   ```

**Config file location (first that exists wins):**

| Source | Path |
|--------|------|
| Env `FOLDERFORT_CONFIG` | Path you set |
| Default | `~/.config/ffsync/ffsync.conf` (Linux/macOS) or `%AppData%\ffsync\ffsync.conf` (Windows) |
| Local | `./.ffsync` in the current directory |

### Option 2: Environment variables

You can skip the config file and set:

- `FOLDERFORT_EMAIL` – your FolderFort email  
- `FOLDERFORT_PASSWORD` – your FolderFort password  
- `FOLDERFORT_BASE_URL` – (optional) e.g. `https://na.folderfort.com`  
- `FOLDERFORT_CONFIG` – (optional) path to a config file  
- `FOLDERFORT_STATE_DIR` – (optional) directory for state cache  

Example (Linux/macOS):

```bash
export FOLDERFORT_EMAIL="your@email.com"
export FOLDERFORT_PASSWORD="yourpassword"
ffsync sync ./my-files remote:backup
```

Example (PowerShell):

```powershell
$env:FOLDERFORT_EMAIL = "your@email.com"
$env:FOLDERFORT_PASSWORD = "yourpassword"
ffsync sync "C:\my-files" remote:backup
```

**FolderFort server options:** `https://na.folderfort.com` (default), `https://na2.folderfort.com`, `https://na3.folderfort.com`, `https://eu.folderfort.com`, `https://eu2.folderfort.com`.

---

## Commands

| Command | Description |
|--------|-------------|
| `ffsync version` | Print version |
| `ffsync config init` | Create config file with defaults |
| `ffsync config show` | Show config (sensitive values redacted) |
| `ffsync config set <key> <value>` | Set `base_url`, `email`, or `password` |
| `ffsync ls remote:[path]` | List remote path (default root) |
| `ffsync mkdir remote:path` | Create remote directory |
| `ffsync copy <local> remote:path` | Copy local to remote (no deletes) |
| `ffsync sync <local> remote:path` | One-way sync; use `--delete` to remove remote-only files |
| `ffsync check <local> remote:path` | Compare local and remote (by size) |

### Sync options

- `--delete` – Delete remote files/folders not present locally  
- `--dry-run` – Show what would be done; no changes  
- `--max-delete N` – Abort if planned deletes exceed N (default 10); use `--force` to override  
- `--transfers N` – Number of concurrent uploads (default 4)  
- `--exclude GLOB` – Exclude paths (repeatable)  
- `--include GLOB` – Include paths (repeatable)  

### Safety

- **Lock file:** `ffsync sync` creates `.ffsync.lock` in the sync root so two runs do not overlap.
- **Max-delete:** Without `--force`, sync stops if the number of planned deletes is above `--max-delete`.
- **Updates:** New file is uploaded first; the old remote file is deleted only after success (no delete-before-upload).

### Exit codes

- `0` – Success  
- `1` – Usage or runtime error  
- `2` – Config or authentication error  

---

## Building from source

Requires [Go 1.22+](https://go.dev/dl/).

```bash
git clone https://github.com/Luen/ffsync
cd ffsync
go build -o ffsync ./cmd/ffsync
```

Cross-compile for other platforms:

```bash
GOOS=linux GOARCH=amd64 go build -o ffsync-linux-amd64 ./cmd/ffsync
GOOS=darwin GOARCH=arm64 go build -o ffsync-darwin-arm64 ./cmd/ffsync
GOOS=windows GOARCH=amd64 go build -o ffsync-windows-amd64.exe ./cmd/ffsync
```

Or use the Makefile: `make all` (builds all platform binaries).

---

## Development

### Prerequisites

- [Go 1.22+](https://go.dev/dl/) (see [Building from source](#building-from-source))
- A [FolderFort](https://folderfort.com) account (for testing sync)

### Get the repo and build

Clone and build as in [Building from source](#building-from-source). On Windows use `go build -o ffsync.exe ./cmd/ffsync` (or rename the output).

### Project structure

| Path | Purpose |
|------|---------|
| `cmd/ffsync/` | Main entrypoint; wires up commands and flags |
| `internal/cli/` | Cobra commands: `config`, `sync`, `copy`, `ls`, `mkdir`, `check`, `selfupdate`, `version` |
| `internal/config/` | Config file and env loading |
| `internal/client/` | HTTP client and FolderFort API (login, presign, create entry) |
| `internal/remote/` | Remote “filesystem” (list, mkdir, upload) |
| `internal/local/` | Local scan and file listing (with default excludes) |
| `internal/plan/` | Sync plan (diff local vs remote) |
| `internal/sync/` | Sync execution (lock, upload, delete) |
| `pkg/pathutil/` | Path normalization and glob matching |

### Run tests

```bash
go test ./...
```

### Try it locally

After building, set up config (see [Configuration](#configuration)), then run a dry-run sync:

```bash
./ffsync sync ./some-local-dir remote:path --dry-run
```

Use `-v` for verbose logs.

### How to release

Releases are built and published to [GitHub Releases](https://github.com/Luen/ffsync/releases) by [GitHub Actions](.github/workflows/release.yml) when you push a version tag. Release notes are generated from merged pull requests and commit messages since the previous tag.

1. Ensure main (or your default branch) is in a good state and all changes are committed.
2. Create and push an annotated tag (e.g. for version 1.0.0):

   ```bash
   git tag -a v1.0.0 -m "Release v1.0.0"
   git push origin v1.0.0
   ```

3. The workflow runs automatically, builds binaries for Linux, macOS, and Windows (amd64 and arm64 where applicable), and creates the release with artifacts and generated notes.
4. Optionally edit the release description or add more assets in the GitHub Releases UI.

FolderFort is built on top of [BeDrive](https://codecanyon.net/item/bedrive-mobile-native-flutter-android-and-ios-app-for-file-storage-php-script/31088424).
