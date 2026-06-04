# Systems Mapping/Evaluation

This directory is reserved for local Systems Mapping/Evaluation process support.

It may be used to reason about:

- v2 design tradeoffs;
- issue decomposition;
- acceptance criteria coverage;
- documentation/process handoffs;
- reviewer summaries.

It is not:

- v2 runtime product behavior;
- a user-facing feature;
- a CLI input;
- a config source;
- a release artifact.

Tracked by default:

- this README;
- future templates or sanitized promoted summaries, if reviewed.

Local/private by default:

- `working-record.json`;
- raw working records;
- scratch exports;
- generated outputs.

Promotion policy:

Do not commit live working records. To preserve a useful conclusion, write a
small reviewed summary in the appropriate docs location or in a future tracked
summary/template file. Remove secrets, local machine paths, private app data,
copied credentials, and unreduced tool state before promotion.
