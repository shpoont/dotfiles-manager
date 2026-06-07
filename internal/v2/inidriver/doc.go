// Package inidriver manages one selected section/key inside one INI-like file.
//
// This is the v2 selected-key vertical slice, not a general-purpose INI parser.
// It is intentionally conservative so recipes can layer application-specific
// policy on top of deterministic low-level behavior.
//
// MVP dialect:
//   - bracket sections such as [user]
//   - key=value and key = value assignments
//   - blank lines
//   - full-line # and ; comments
//
// Section and key matching is exact and case-sensitive after trimming only the
// structural whitespace around a section name or the left-hand key name. A
// selector value with surrounding whitespace is invalid. Duplicate exact
// selected sections or duplicate exact selected keys are rejected as ambiguous.
//
// Selected-key apply preserves unrelated sections, unrelated keys, blank lines,
// and comments byte-for-byte. The edited selected-key line may be rewritten in
// the driver's canonical key = value form. Inline comments, multi-line values,
// includes, conditional includes, and subsection-specific semantics are
// preserved when unrelated, but this driver does not claim semantic support for
// them.
//
// Desired state has an explicit intent: set a scalar selected value or delete
// the selected key. Desired set values are single-line scalar data; CR, LF, and
// NUL are rejected before rendering so values cannot inject unrelated INI lines
// or sections. Create and delete are disabled by default unless the selector
// policy explicitly allows the missing section/key or delete behavior.
package inidriver
