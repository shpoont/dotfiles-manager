// Package jsondriver manages one selected scalar value inside one JSON file.
//
// This is the v2 selected-path vertical slice, not a general-purpose JSON
// document manager. Recipes provide explicit object-member path segments; this
// driver rejects expression-like selectors, JSONPath, wildcard traversal, array
// traversal, and object/array selected leaf values.
//
// MVP behavior is intentionally conservative:
//   - the root and intermediate path nodes must be JSON objects;
//   - selected leaf values must be JSON string, number, boolean, or null;
//   - create and delete are disabled by default and require explicit selector
//     policy opt-ins;
//   - duplicate object keys anywhere in the document are rejected before
//     selection or mutation;
//   - writes use deterministic pretty JSON and do not preserve original
//     whitespace, key order, or comments.
package jsondriver
