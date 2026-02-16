---
owner: Core Engineering
status: Contract v1
last-updated: 2026-02-16
canonical-source: docs/internal/contracts/metadata-contract.md
---

# dotfiles-manager: metadata contract

This document defines what "preserve metadata as much as possible" means for v1.

## 1) Data and type guarantees (strict)

For both `deploy` and `import`, dotfiles-manager must preserve:
- file/dir/symlink **type** according to command direction source
- regular file **content bytes**
- symlink **link target value**

Type mismatches are resolved by replacement (`replace_type`).

Failures in these strict operations are runtime errors (fail-fast).

## 2) Metadata preservation tiers

## Tier A (required when supported)

If the underlying platform/filesystem supports these attributes, dotfiles-manager must preserve:
- POSIX mode bits on files/directories (including executable bit)
- modification time (`mtime`)

If an attribute is supported but cannot be applied for an operation (for example permission denied), command fails (fail-fast).

## Tier B (best-effort)

dotfiles-manager should attempt to preserve when available:
- access time (`atime`)
- extended attributes (`xattrs`)
- ACL entries

If unsupported (`ENOTSUP`, `EOPNOTSUPP`, `ENOSYS`), continue without error.
If supported but apply fails for other reasons, command fails (fail-fast).

## 3) Explicit non-goals for v1

Not preserved as part of compatibility guarantees:
- owner/group (`uid`/`gid`) portability
- inode-level identity
- creation/birth time portability guarantees

## 4) Directory semantics

- Directory creation/removal follows sync semantics first.
- Directory mode/time metadata follows same tier rules as files.

## 5) Symlink semantics

- Symlinks are treated as symlink entries, not dereferenced.
- Metadata on symlink itself is platform-dependent and not guaranteed.

## 6) JSON reporting hooks

When `--json` is used:
- Tier A failure causes `ok=false` and non-zero exit.
- Tier B unsupported attributes may be counted in `summary.metadata_unsupported` (optional informational field).
