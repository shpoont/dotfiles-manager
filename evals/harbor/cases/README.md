# Harbor v2 case index

These cases are local-private reviewer aids. Each case contains:

- `README.md` with the spec/risk/pass/fail/out-of-scope mapping;
- `instruction.md` for the Harbor agent;
- `fixture/context.md` with sanitized local task context;
- `environment/context/context.md`, a mirrored Docker build-context copy
  of `fixture/context.md` because Harbor currently builds from
  `environment/`; `evals/harbor/validate.sh` keeps the two files byte-for-byte
  identical so the duplicated context cannot silently drift;
- `fixture/sample-pass.md` and `fixture/sample-fail.md` for deterministic
  objective-script validation;
- `tests/objective.sh` for local deterministic checks;
- `tests/review.toml` for subjective RewardKit/Codex review criteria;
- `environment/.dockerignore` so copied local auth never enters Docker
  build contexts or images;
- `environment/codex-auth/README.md` and `config.example.toml` documenting
  the non-secret verifier config defaults; the real `config.toml` is generated
  by the local prepare script, ignored, and removed after runs;
- `environment/docker-compose.yaml` with a local-private read-only
  runtime source mount for verifier Codex auth;
- `tests/test.sh` verifier setup that copies that source into a writable
  container-local temporary `CODEX_HOME` and removes it on exit, because
  Codex cannot initialize from a read-only home;
- `task.toml` and `environment/` so the case can be run through Harbor.

Current cases:

1. `issue-quality-v2-specs`
2. `happy-path-ux-v2-cli`
3. `recipe-explain-clarity`
4. `native-safety-review`
