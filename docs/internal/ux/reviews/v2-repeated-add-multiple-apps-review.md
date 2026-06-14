# v2 repeated add multiple apps storyboard review

Status: issue #183 checked-in persona review.
Last updated: 2026-06-14.
Reviewed artifact: `../v2-repeated-add-multiple-apps-storyboard.md`.
Scope: UX/documentation review only; no CLI behavior, renderer, JSON schema,
shell exit-code, v1 output, or native export/import changes.
External validation: Pro pre-validation and implementation-plan approval are
recorded on #183. This checked-in review intentionally does not include a
ChatGPT conversation URL.

## Review method

The review checks whether the storyboard helps users understand a repeated
`add` flow across more than one supported app without overclaiming current CLI
support.

Required questions for every persona:

1. Can the persona tell what was selected?
2. Can the persona tell what has not been saved yet?
3. Can the persona identify a safe next command?
4. Can the persona tell what data will not be managed?
5. Does any shown syntax look like unsupported magic?

## Persona 1: Git-literate first-time user

Profile: understands Git and shell commands, but has not learned
`dotfiles-manager` internals.

Answers:

1. What was selected? Yes. The transcript shows Git user email, Git user name,
   Starship add newline, and Starship command timeout under app headings.
2. What has not been saved yet? Yes. `list` says `Desired state: not saved yet`
   for each selected setting, and the explanation separates selection from
   saved desired state.
3. Safe next command? Yes. The next commands use supported single setting refs
   such as `git:user.email` and a broad read-only `status` command.
4. What will not be managed? Yes. Git credentials, tokens, credential helpers,
   signing/auth state, includes, aliases, repo-local config, Starship unrelated
   modules, terminal state, and lifecycle/package/plugin actions are excluded.
5. Unsupported magic? No. The only multi-target add example is explicitly
   labeled `future / not currently supported`, and the main transcript uses
   repeated current commands.

Verdict: passes for this persona.

## Persona 2: cautious non-expert Mac user

Profile: can copy terminal commands but worries about damaging real app config
or leaking private data.

Answers:

1. What was selected? Yes. The selected settings are listed in plain app groups
   with display labels.
2. What has not been saved yet? Yes. The transcript repeatedly says selection
   did not save values, and `Desired state: not saved yet` appears before any
   save/apply step.
3. Safe next command? Yes. The storyboard points to dry-run save and status
   commands, not confirmed apply commands.
4. What will not be managed? Yes. The review copy and storyboard explain that
   secrets, credentials, history, cache/session data, plugin state, and native
   app export/import are not part of the flow.
5. Unsupported magic? No. The unsupported single-command multi-add syntax is
   visually marked as future-only, reducing the risk that the user copies it.

Residual caution: the setup uses a temporary HOME, and the warning not to remove
`HOME="$DFM_HOME"` should remain visible near the setup commands.

Verdict: passes for this persona.

## Persona 3: power user / maintainer

Profile: cares about command contracts, scriptability boundaries, and whether
UX docs accidentally define implementation or JSON behavior.

Answers:

1. What was selected? Yes. Public refs are visible where needed, while internal
   resource/driver/selector IDs remain out of default text.
2. What has not been saved yet? Yes. The doc names the profile-selection state
   separately from desired-state files and live app writes.
3. Safe next command? Yes. The commands use current CLI shapes: repeated
   `add <target>`, single setting refs, and broad read-only status.
4. What will not be managed? Yes. Support boundaries are explicit for Git,
   Starship, and Zsh-adjacent exclusions.
5. Unsupported magic? No. The doc avoids fake subset commands and labels the
   multi-target add idea as future-only. It does not introduce JSON fields,
   shell exit semantics, or native export/import behavior.

Verdict: passes for this persona.

## Required fixes before implementation consumption

None for the storyboard/review artifact.

Future implementation issues should continue to verify exact command behavior
with executable transcripts before converting this storyboard into renderer or
copy changes.
