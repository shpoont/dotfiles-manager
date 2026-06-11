# Local Codex auth source

This directory is a local-private runtime auth source for Harbor verifier runs.

Committed files in this directory are non-secret documentation and examples
only. The real `auth.json` and generated `config.toml` are copied here by the
local Harbor prepare script, are ignored by git, and must never be committed,
printed, uploaded, or baked into Docker images.

The Docker build context excludes this directory. At runtime,
`docker-compose.yaml` mounts it read-only at `/run/codex-auth-source`; the case
verifier copies those files into a container-local temporary writable
`CODEX_HOME` and removes that temporary home on exit.
