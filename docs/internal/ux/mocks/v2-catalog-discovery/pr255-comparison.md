# PR #255 comparison against the runnable #228 UX mock

Status: recovery evidence for #228, created after Project Owner selected recovery
option 1.

Compared artifacts:

- Accepted design target: `docs/internal/ux/v2-catalog-discovery-storyboard.md`
- Runnable mock: `docs/internal/ux/mocks/v2-catalog-discovery/run-demo.sh`
- Golden mock output: `docs/internal/ux/mocks/v2-catalog-discovery/expected-demo.txt`
- Current implementation branch: PR #255 / `codex/228-built-in-local-catalogs`
- Reproducible implementation transcript runner:
  `docs/internal/ux/mocks/v2-catalog-discovery/run-pr255-comparison.sh`

## How to rerun

Requirements: Python for the standalone mock and Go for the PR comparison runner
(defaults to `$HOME/.asdf/shims/go`).

```bash
docs/internal/ux/mocks/v2-catalog-discovery/run-demo.sh --check
docs/internal/ux/mocks/v2-catalog-discovery/run-pr255-comparison.sh
```

Both scripts use temporary folders only. They do not read/write live app settings,
the user's real settings storage folder, real user catalog folders, secrets, or
network resources. The PR comparison runner forces `GOPROXY=off` and
`GOSUMDB=off` by default, so it fails rather than silently fetching Go modules
from the network. Set `DFM_PR255_ALLOW_GO_NETWORK=1` only for an explicit
non-gate convenience rerun.

## Comparison result

PR #255 matches the mock/design target on the main #228 catalog-discovery
contract points:

- app-first discovery through `list`, `search`, and `explain <app>`;
- built-in catalog is available offline and not removable;
- `catalog list` shows built-in/local/remote-not-supported state;
- invalid local catalog content fails closed before persistence;
- valid local catalog add validates support and records local-only/no-network
  behavior;
- built-in/local collision keeps built-in as default and shows the local source
  as a candidate;
- local-only support is visible in plain language before writes;
- local-catalog write paths are blocked until explicit write approval exists;
- disabling/removing a local catalog does not delete live settings, stored
  settings, or the catalog folder;
- unavailable managed-app status preserves source/provenance details;
- reserved GitHub-style remote catalog syntax is rejected without network
  behavior.

## Differences and decision points

### 1. `sync --dry-run` command shape does not match the storyboard/mock

The accepted storyboard and runnable mock use:

```bash
dotfiles-manager sync --dry-run example-tool
```

The current PR #255 implementation rejects that exact command with:

```text
unknown flag: --dry-run
```

PR #255 does provide an equivalent safety result through:

```bash
dotfiles-manager sync example-tool --non-interactive --user-id leon
```

That command shows the local catalog source/recipe and blocks before write. This
looks behaviorally safe, but it is still a public command-shape mismatch against
the accepted storyboard/mock. It needs an explicit decision before #228 can leave
recovery:

- either update the #228 design target/mock by managed change to use the existing
  `sync` preview/non-interactive grammar; or
- update the implementation so `sync --dry-run` works as the accepted transcript
  describes.

No product/runtime code change was made in this recovery artifact.

### 2. Unavailable-source status safety wording is weaker than the mock

The mock/status transcript says the unavailable managed-app path reads or changes
nothing live. The PR #255 status output says `No files changed` and shows a
`Live value: No live value found` section. This may be technically correct for
the implementation, but it is less clear as a safety signal than the mock and
storyboard. If #228 continues without changing it, the PR evidence should explain
whether live settings are actually read in this unavailable-source path and why
that is safe.

### 3. Wording and layout differences are mostly acceptable

PR #255 includes extra useful details not in the mock, such as managed-app
summary lines, `list --settings` guidance, more explicit Git exclusions, and
recipe origin URIs. These are compatible with the mock as long as the normal
path remains app-first and the safety guarantees stay visible.

## Recovery-gate conclusion

The runnable/replayable mock requirement is now satisfied as an artifact, but the
comparison is not a clean pass because of the `sync --dry-run` command-shape
mismatch. Continuing PR #255 implementation or coverage cleanup requires a
specific Project Owner decision on that mismatch, or a managed change that updates
the frozen #228 package.
