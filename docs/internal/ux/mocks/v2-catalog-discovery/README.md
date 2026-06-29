# v2 catalog discovery runnable UX mock

This directory is recovery design evidence for #228. It exists because the
updated Project Execution Standard requires runnable or replayable usage evidence
for meaningful CLI public-surface work unless the decision owner explicitly
waives it.

The mock models the accepted CLI target from
`docs/internal/ux/v2-catalog-discovery-storyboard.md`. It is not product code and
must not be used as runtime behavior.

## Safety

- The mock never reads live app settings.
- The mock never writes live app settings.
- The mock never reads or writes the user's real settings storage folder.
- The mock never reads real user catalog contents.
- The mock never uses the network.
- The only state it writes is a JSON file selected by
  `DOTFILES_MANAGER_UX_MOCK_STATE`; `run-demo.sh` places that file in a temporary
  directory and removes it after the run.

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

- first-run/offline built-in app discovery;
- app-first search and explanation without teaching the `recipe` noun;
- built-in catalog listing;
- invalid local catalog rejection;
- valid local catalog add;
- built-in/local collision with bundled-first default;
- local-only app explanation;
- before-write local-source summary and write block;
- local catalog disable/list unavailable support;
- unavailable managed app status after disabled/removed source;
- local catalog enable and remove;
- reserved remote GitHub catalog syntax rejection for #229.

## Compare PR #255

The implementation comparison runner builds the currently checked-out branch with
`GOPROXY=off` and `GOSUMDB=off` forced by default, then runs equivalent commands
inside temporary settings/home/catalog fixtures:

```bash
docs/internal/ux/mocks/v2-catalog-discovery/run-pr255-comparison.sh
```

It is expected to be run on PR #255's branch,
`codex/228-built-in-local-catalogs`, unless the comparison note is updated for a
different branch.

## Expected relationship to PR #255

PR #255 should be compared against this mock at the command-contract level before
continuing implementation or coverage work. Exact wording can differ where the
PR has equally clear product language, but the safety boundaries, nouns,
commands, source visibility, failure paths, and no-network/no-delete guarantees
must match or be explicitly recorded as managed changes.
