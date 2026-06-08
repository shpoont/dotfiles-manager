// Package tomldriver manages one selected scalar value inside one TOML file.
//
// This is the v2 selected-path vertical slice, not a general-purpose TOML
// document manager. Recipes provide explicit table path segments; this driver
// rejects expression-like selectors, wildcard traversal, array traversal, and
// non-scalar selected leaf values.
//
// MVP behavior is intentionally conservative:
//   - the root and intermediate path nodes must be TOML tables;
//   - selected leaf values must be TOML string, boolean, integer, or finite
//     float values;
//   - null, TOML date/time values, arrays, and tables are rejected as selected
//     leaves;
//   - create and delete are disabled by default and require explicit selector
//     policy opt-ins;
//   - duplicate or invalid TOML is rejected by the parser before selection or
//     mutation;
//   - writes use deterministic TOML output and do not preserve original
//     whitespace, key order, or comments.
package tomldriver
