# recipe explain output goals

`recipe explain` is a read-only, metadata-only command. It should not
read live app values, desired payloads, or run native commands.

# What the user learns

It explains scopes: `shared`, `user`, `machine`, and `machine-user`.
It shows named locations, including default paths and whether profile
overrides are allowed. It distinguishes managed settings from
unmanaged, unsupported, risky, or blocked settings.

Settings groups are optional; they can describe safe bulk selection
or native grouping but are not required public nouns. The output also
summarizes redaction/sensitivity, lifecycle policy, resource/driver
support, and native export/import limitations without raw command
details.
