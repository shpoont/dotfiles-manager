# perf-1k fixture

Template fixture for performance tests.

The performance test harness copies `.dotfiles-manager.yaml` from this directory and generates
a deterministic ~1,000-file tree under `source/` and `~/.config/perf` at runtime.

Only `source/managed` <-> `~/.config/perf/managed` is synced for measured commands.
Additional files are generated outside sync roots to keep fixture scale realistic.
