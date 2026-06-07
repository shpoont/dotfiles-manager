// Package yamldriver implements the v2 MVP yaml-file selected-path driver.
//
// This package intentionally manages one selected YAML scalar at a time. It uses
// explicit mapping-key path segments, rejects YAML path expressions and sequence
// traversal, normalizes supported selected scalars to canonical JSON scalar
// bytes, and keeps raw scalar bytes out of exported preview/snapshot JSON. It is
// an internal vertical slice only; recipe schema integration and user-facing CLI
// wiring are deliberately out of scope.
package yamldriver
