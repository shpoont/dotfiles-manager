// Package plistdriver manages one selected scalar value inside one XML or
// binary Apple property-list file.
//
// This is the v2 selected-path vertical slice, not a general-purpose plist
// document manager and not a macOS defaults-domain driver. Recipes provide an
// explicit dictionary key path. Dots inside key names are ordinary key bytes,
// not path separators; callers should keep the path as []string and display it
// with quoted/JSON-array-style summaries.
//
// MVP behavior is intentionally conservative:
//   - only XML and Binary plist formats are accepted;
//   - OpenStep/GNUStep/invalid formats are rejected before selected-value work;
//   - existing XML stays XML, existing Binary stays Binary, and new files are
//     created as deterministic pretty XML;
//   - root and intermediate path nodes must be dictionaries;
//   - arrays are not traversed and read-only container selection is deferred;
//   - selected leaves must be JSON-compatible plist scalars: string, bool,
//     signed-int64-compatible integer, or finite float;
//   - null, data, date, UID, arrays, and dictionaries are rejected as selected
//     leaves;
//   - plist.UID anywhere in a write-capable document is rejected because it
//     indicates keyed-archive/opaque object-graph risk;
//   - binary plist integer tokens wider than 64 bits are rejected fail-closed;
//   - duplicate dictionary keys are rejected before mutation;
//   - create and delete are disabled by default and require explicit selector
//     policy opt-ins;
//   - previews and backups expose only hashes and redaction-safe metadata; raw
//     selected values and whole-file backup payloads are not JSON-serialized.
package plistdriver
