# LayerFS Simulator Design

## Goal and scope

This project models a small, useful subset of a union/copy-on-write filesystem
in memory. It is intended for learning and deterministic tests, not as a kernel
filesystem, FUSE implementation, or secure storage boundary. It never reads or
writes host files.

## Data model

`FS` contains three kinds of state:

- **Lower layers:** immutable maps from normalized relative paths to bytes.
  Layers are ordered by precedence; earlier layers win when the same file is
  present in more than one layer.
- **Upper layer:** a writable map containing files created or replaced via
  `WriteFile`. Any upper file takes precedence over all lower copies.
- **Whiteouts:** paths removed from the merged view. A whiteout hides the same
  lower path and every descendant. An upper file written at a hidden path is
  still visible, as upper-layer entries take precedence over whiteouts.

Directories are inferred from visible file paths. Empty directories and
directory metadata are not stored. All public paths are slash-separated;
leading slashes are accepted and normalized away internally.

## Read resolution

For a normalized file path, `ReadFile` checks the upper layer first. If there
is no upper file, it checks whiteouts, then searches lower layers in priority
order. Returned bytes are copied so callers cannot mutate internal state.

The merged directory view applies the same visibility rules to paths and
derives immediate children from the resulting visible files. Results are
sorted to remain deterministic.

## Mutations

### WriteFile

`WriteFile` writes a copy of the supplied bytes to the upper layer. This is the
simulator's copy-on-write behavior: lower-layer content remains untouched.
Parent directories are implicit, but a path cannot be used as a file if it is
already a visible directory, nor can a file be created beneath a visible file.

### Remove

`Remove` first verifies that the target exists in the merged view. It removes
any matching upper-layer subtree, clears redundant descendant whiteouts, then
places a whiteout at the requested path. This hides lower-layer files at that
path and below it. Removing a directory therefore hides its entire tree.

## Path handling

Paths are cleaned using POSIX slash semantics. Leading slashes are optional;
`.` components are normalized. Paths that resolve to the root or escape above
the root using `..` are rejected for file operations. The simulator does not
model permissions, symlinks, hard links, mount options, timestamps, ownership,
or concurrent access.

## Package layout

- `overlay.go` — filesystem state, resolution, mutations, and merged listings.
- `overlay_test.go` — tests for precedence, copy-on-write, whiteouts, listing,
  path safety, and returned-byte isolation.
- `cmd/layerfs/main.go` — an interactive REPL backed by illustrative in-memory
  lower layers.

## Possible extensions

The model can be expanded with explicit directories, metadata, opaque-directory
whiteouts, persistence, configurable layers, or a FUSE adapter. Those features
are intentionally out of scope for this simplified simulator.