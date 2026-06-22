# Content-Addressable Code Sync Engine (Over SSH)

**Status:** Future / design research

Instead of relying on OS-level utilities like rsync, `appa deploy` could
leverage a Content-Addressable Storage (CAS) approach executed entirely over
SSH. Files are tracked, diffed, and validated globally using cryptographic
hashes (SHA-256) rather than file sizes or modification timestamps.

By using Go's native `crypto/ssh` package on the client, the CLI acts as a
pure, lightweight transport layer that pipes data streams directly into a
dedicated server-side agent (`appa-agent`). This avoids exposing public HTTP
upload endpoints and keeps all build pipeline orchestration on the server.

## Phase 1: Local Manifest Generation (Go CLI)

- Directory Scan: The CLI recursively walks the local workspace directory,
  interpreting and applying local `.gitignore` specifications.
- Content Hashing: Every discovered file is processed through a fast, secure
  hashing function.
- Inventory Assembly: The CLI creates an in-memory JSON payload mapping file
  paths to their content hashes:

```json
{
  "project_id": "proj_12345",
  "manifest": {
    "main.go": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
    "pkg/utils/utils.go": "8a3243a789182312bca32487abcdf123485712345678abcdf1234567890abcdef"
  }
}
```

## Phase 2: Remote Manifest Diffing (SSH Pipe)

- The CLI opens an SSH session and executes `appa-agent diff-manifest`.
- The CLI pipes the JSON manifest directly into the remote command's stdin.
- The remote agent reads stdin, cross-references the incoming hashes against
  the server's global or project asset cache, and prints an array of only the
  missing hashes to stdout.
- The CLI reads the SSH stdout to determine which files need to be uploaded.

## Phase 3: Targeted Compression & Streaming (SSH Pipe)

- Selective Archiving: The CLI filters the local file list against the
  missing-hash array returned by the server.
- The CLI opens a second SSH session to execute:
  `appa-agent ingest-build <project_id>`.
- Direct Streaming: The CLI packages only the missing files into an in-memory,
  compressed `.tar.gz` archive and streams the raw bytes directly through the
  SSH connection into the remote agent's stdin.
- Server Assembly & Execution: The server-side agent unpacks the stream in
  real-time into an isolated workspace directory (`/tmp/appa/builds/<build_id>/src`).
  It uses the original manifest to copy or hard-link the remaining cached
  files into place, then kicks off the server-side build pipeline.

## Architectural Advantages

- **Zero OS Binary Dependencies:** Pure Go using standard library
  (`crypto/ssh`, `crypto/sha256`, `archive/tar`). Neither the developer
  machine nor the Appa host requires rsync.
- **Built-in Security Model:** Leverages existing SSH keys for
  authentication, access control, and encrypted data transfer. No new web
  APIs or auth protocols needed.
- **In-Memory Streaming Efficiency:** Go pipes an `io.Reader` (compressed
  tarball) directly into the SSH channel's `io.Writer`. No temp files
  written to disk.
- **Total Build Isolation & Immutability:** The manifest JSON is a
  point-in-time snapshot. Renamed or deleted files are naturally handled
  because the server builds the staging folder from scratch using only the
  files declared in the manifest.
