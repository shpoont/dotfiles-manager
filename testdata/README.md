# Test data layout

Repository-level fixtures and expected outputs used by integration/contract/performance tests.

- `fixtures/`: filesystem fixture inputs (`source/`, `target/`, config)
  - `minimal/`: baseline tiny fixture
  - `perf-1k/`: performance fixture template (runtime generator expands to ~1,000 files)
- `expected/`: golden outputs by command and test layer
- `logs/`: logging contract/redaction samples
