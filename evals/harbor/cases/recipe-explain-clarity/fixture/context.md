# Context: recipe explain mental model

`recipe explain <target>` exists so a user or reviewer can understand
a supported target before trusting or selecting it.

It may read static bundled or local recipe metadata and safe profile
selection metadata. It must not bootstrap identity, mutate local
state, read live app/filesystem values, read desired payloads, or run
driver/native operations.

Required categories from the CLI and recipe specs include target
support, selection summary, settings, optional settings groups,
resources/drivers, named locations, native import/export summaries,
safety/limitations, and diagnostics.

Scope names are `shared`, `user`, `machine`, and `machine-user`.
Named locations keep recipe defaults separate from profile-layer
user overrides. Groups may help safe defaults or native grouping, but
the user should not need to use groups at all.
