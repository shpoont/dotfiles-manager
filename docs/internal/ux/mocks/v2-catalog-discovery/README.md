# v2 catalog discovery runnable UX mock

This directory contains runnable/replayable design evidence for the recontracted
#228 catalog-discovery scope.

It supersedes the earlier PR #255 mock/evidence that modeled local catalog
lifecycle. The new target is normal discovery from dotfiles-manager official
catalog metadata:

- real official-catalog apps/tools only in `list`;
- no internal pseudo-app targets in normal discovery;
- no local or remote catalog lifecycle commands in #228;
- no first-run download/update implementation in #228;
- official-catalog download/update and additional remote catalogs are future #229 work.

The mock is not product code.

## Safety

- The mock never reads live app settings.
- The mock never writes live app settings.
- The mock never reads or writes the user's real settings storage folder.
- The mock never reads real user catalog contents.
- The mock never changes catalog state.
- The mock never uses the network.

## Run the replayable demo

From the repository root. Requires Python; set `PYTHON=/path/to/python` if
`python3` is not the desired interpreter.

```bash
docs/internal/ux/mocks/v2-catalog-discovery/run-demo.sh
```

To mechanically verify the demo output against the checked-in golden transcript:

```bash
docs/internal/ux/mocks/v2-catalog-discovery/run-demo.sh --check
```

## What the demo covers

- official-catalog app discovery from deterministic mock catalog metadata;
- app-first search and explanation without teaching the `recipe` noun;
- unsupported app search that points to future catalog capabilities without
  exposing unavailable update/add commands;
- internal pseudo-app targets excluded from normal discovery;
- official catalog listing with concise version and updated-time metadata.
