// Package macosdefaultsdriver reads one selected scalar from a current-user
// macOS defaults domain without writing preferences.
//
// The driver is intentionally read-only. It exists so recipes can explain,
// status, and diff selected defaults keys while defaults writes remain outside
// the MVP until lifecycle, rollback, and app-state semantics are designed.
package macosdefaultsdriver
