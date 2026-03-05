# ffsync

A reliable, single-binary Go CLI that syncs a local directory to [FolderFort](https://folderfort.com) using the FolderFort web sync API. One-way mirror (local is source of truth), with optional deletes and dry-run.

## Requirements

- Go 1.22+
- FolderFort account

## Install

```bash
go install github.com/ffsync/ffsync/cmd/ffsync@latest
```

Or build from source:

```bash
git clone https://github.com/ffsync/ffsync
cd ffsync
go build -o ffsync ./cmd/ffsync
```

## Configuration

Config file is read from (in order):

1. `FOLDERFORT_CONFIG` (env) if set
2. `~/.config/ffsync/ffsync.conf` (or platform config dir)
3. `./.ffsync` in the current directory

Environment variables override file settings:

- `FOLDERFORT_BASE_URL` – API base URL (default: `https://na.folderfort.com`)
- `FOLDERFORT_EMAIL` – account email
- `FOLDERFORT_PASSWORD` – account password
- `FOLDERFORT_STATE_DIR` – directory for state file (optional)

### FolderFort server options

- `https://na.folderfort.com` (default)
- `https://na2.folderfort.com`
- `https://na3.folderfort.com`
- `https://eu.folderfort.com`
- `https://eu2.folderfort.com`

### Commands

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
- `--dry-run` – Log planned actions only; no changes
- `--max-delete N` – Abort if planned deletes exceed N (default 10); use `--force` to override
- `--transfers N` – Number of concurrent uploads (default 4)
- `--exclude GLOB` – Exclude paths (can be repeated)
- `--include GLOB` – Include paths (can be repeated)

### Safety

- **Lock file**: `ffsync sync` creates `.ffsync.lock` in the local sync root; a second run on the same root will fail until the first finishes.
- **Max-delete**: Without `--force`, sync aborts if the number of planned deletes exceeds `--max-delete`.
- **Updates**: Updated files are uploaded first, then the old remote file is deleted (no delete-before-upload to avoid data loss).

### Exit codes

- `0` – Success
- `1` – Usage or runtime error
- `2` – Config or authentication error

## Build for multiple platforms

```bash
# macOS (amd64, arm64), Linux (amd64, arm64), Windows (amd64)
GOOS=darwin GOARCH=amd64 go build -o ffsync-darwin-amd64 ./cmd/ffsync
GOOS=darwin GOARCH=arm64 go build -o ffsync-darwin-arm64 ./cmd/ffsync
GOOS=linux GOARCH=amd64 go build -o ffsync-linux-amd64 ./cmd/ffsync
GOOS=linux GOARCH=arm64 go build -o ffsync-linux-arm64 ./cmd/ffsync
GOOS=windows GOARCH=amd64 go build -o ffsync-windows-amd64.exe ./cmd/ffsync
```

## Development

```bash
go test ./...
go build ./cmd/ffsync
```

FolderFort is built on top of [BeDrive](https://github.com).
