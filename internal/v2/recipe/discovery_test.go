package recipe

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDiscoverClassifiesBundledTargetStatesWithFixtures(t *testing.T) {
	t.Parallel()

	t.Run("installed without config", func(t *testing.T) {
		t.Parallel()

		home := t.TempDir()
		report, err := Discover(DiscoverOptions{
			Target:        GitTarget,
			GOOS:          "darwin",
			LocationRoots: map[string]string{"home": home},
			CommandLookup: installedCommands("git"),
		})
		require.NoError(t, err)
		target := requireDiscoveredTarget(t, report, GitTarget)
		require.Equal(t, DiscoverStateInstalled, target.State)
		require.Equal(t, DiscoverBinaryInstalled, target.BinaryState)
		require.Equal(t, DiscoverConfigMissing, target.ConfigState)
		require.Len(t, target.ConfigProbes, 1)
		require.Equal(t, ".gitconfig", target.ConfigProbes[0].Path)
	})

	t.Run("config present wins over missing command", func(t *testing.T) {
		t.Parallel()

		home := t.TempDir()
		writeDiscoveryFile(t, filepath.Join(home, ".gitconfig"))
		report, err := Discover(DiscoverOptions{
			Target:        GitTarget,
			GOOS:          "darwin",
			LocationRoots: map[string]string{"home": home},
			CommandLookup: missingCommands(),
		})
		require.NoError(t, err)
		target := requireDiscoveredTarget(t, report, GitTarget)
		require.Equal(t, DiscoverStateConfigPresent, target.State)
		require.Equal(t, DiscoverBinaryMissing, target.BinaryState)
		require.Equal(t, DiscoverConfigPresent, target.ConfigState)
		require.Equal(t, DiscoverProbePresent, target.ConfigProbes[0].State)
	})

	t.Run("config missing when command and config are missing", func(t *testing.T) {
		t.Parallel()

		home := t.TempDir()
		report, err := Discover(DiscoverOptions{
			Target:        GitTarget,
			GOOS:          "darwin",
			LocationRoots: map[string]string{"home": home},
			CommandLookup: missingCommands(),
		})
		require.NoError(t, err)
		target := requireDiscoveredTarget(t, report, GitTarget)
		require.Equal(t, DiscoverStateConfigMissing, target.State)
		require.Equal(t, DiscoverBinaryMissing, target.BinaryState)
		require.Equal(t, DiscoverConfigMissing, target.ConfigState)
	})

	t.Run("unsupported platform skips probes", func(t *testing.T) {
		t.Parallel()

		report, err := Discover(DiscoverOptions{
			Target:        NvimTarget,
			GOOS:          "windows",
			LocationRoots: map[string]string{"config": t.TempDir()},
			CommandLookup: installedCommands("nvim"),
		})
		require.NoError(t, err)
		target := requireDiscoveredTarget(t, report, NvimTarget)
		require.Equal(t, DiscoverStateUnsupportedPlatform, target.State)
		require.Equal(t, DiscoverPlatformUnsupported, target.PlatformState)
		require.Equal(t, DiscoverBinaryNotApplicable, target.BinaryState)
		require.Equal(t, DiscoverConfigNotApplicable, target.ConfigState)
		require.Empty(t, target.CommandProbes)
		require.Empty(t, target.ConfigProbes)
		requireDiagnosticCode(t, target.Diagnostics, DiscoverCodeUnsupportedPlatform)
	})

	t.Run("custom files is not applicable", func(t *testing.T) {
		t.Parallel()

		report, err := Discover(DiscoverOptions{Target: CustomFilesTarget, GOOS: "darwin"})
		require.NoError(t, err)
		target := requireDiscoveredTarget(t, report, CustomFilesTarget)
		require.Equal(t, DiscoverStateNotApplicable, target.State)
		require.Equal(t, DiscoverBinaryNotApplicable, target.BinaryState)
		require.Equal(t, DiscoverConfigNotApplicable, target.ConfigState)
	})
}

func TestFriendlyDiscoveryHelpersCoverFallbackBranches(t *testing.T) {
	t.Parallel()

	require.Contains(t, friendlyDiscoverErrorText(&DiscoverReport{Error: &ExplainErrorObject{Message: "boom"}}), "boom")
	require.Contains(t, friendlyDiscoverErrorText(nil), "The command could not complete.")

	require.Equal(t, "Raycast", friendlyDiscoverTargetName(DiscoveredTarget{DisplayName: "Raycast"}))
	require.Equal(t, "Custom App", friendlyDiscoverTargetName(DiscoveredTarget{ID: "custom.app"}))
	require.Equal(t, "App", friendlyDiscoverTargetName(DiscoveredTarget{}))

	stateCases := map[string]string{
		DiscoverStateConfigPresent:       "installed or configured; a known config file is present",
		DiscoverStateInstalled:           "installed; no known config file found yet",
		DiscoverStateConfigMissing:       "supported, but no known config file found",
		DiscoverStateUnsupportedPlatform: "not supported on this platform",
		DiscoverStateAmbiguous:           "needs review because detection was ambiguous",
		DiscoverStateNotApplicable:       "detection is not applicable",
		"":                               "not checked",
		"custom-state":                   "custom state",
	}
	for state, want := range stateCases {
		t.Run("state "+state, func(t *testing.T) {
			require.Equal(t, want, friendlyDiscoverState(DiscoveredTarget{State: state}))
		})
	}

	probeCases := map[string]string{
		"installed":        "installed",
		"present":          "present",
		"config-present":   "present",
		"missing":          "missing",
		"config-missing":   "missing",
		"ambiguous":        "needs review",
		"config-ambiguous": "needs review",
		"not-applicable":   "not applicable",
		"":                 "not checked",
		"custom-state":     "custom state",
	}
	for state, want := range probeCases {
		t.Run("probe "+state, func(t *testing.T) {
			require.Equal(t, want, friendlyProbeState(state))
		})
	}

	require.Equal(t, "", pluralWord(1))
	require.Equal(t, "s", pluralWord(2))
}

func TestDiscoverFriendlyTextCoversResultBranches(t *testing.T) {
	t.Parallel()

	require.Contains(t, DiscoverText(nil), "The command could not complete.")
	require.Contains(t, DiscoverText(&DiscoverReport{Error: &ExplainErrorObject{Message: "boom"}}), "boom")
	require.Contains(t, DiscoverText(&DiscoverReport{}), "No supported apps were found")

	report := &DiscoverReport{Discovery: DiscoveryResult{
		Targets: []DiscoveredTarget{
			{
				ID:          GitTarget,
				DisplayName: "Git",
				State:       DiscoverStateConfigPresent,
				CommandProbes: []DiscoveryCommandProbe{{
					Command: "git",
					State:   "installed",
				}},
				ConfigProbes: []DiscoveryConfigProbe{{
					LocationID: "home",
					Path:       ".gitconfig",
					State:      "config-present",
				}},
				Diagnostics: []ExplainDiagnostic{{Severity: ExplainSeverityError, Message: "config blocked"}},
			},
			{
				ID:    "custom.app",
				State: "custom-state",
			},
		},
		Diagnostics: []ExplainDiagnostic{{Message: "global discovery issue"}},
	}}
	text := DiscoverText(report)
	require.Contains(t, text, "Git")
	require.Contains(t, text, "Detected: installed or configured")
	require.Contains(t, text, "Commands:")
	require.Contains(t, text, "git — installed")
	require.Contains(t, text, "$HOME/.gitconfig — present")
	require.Contains(t, text, "Supported settings:")
	require.Contains(t, text, "Not managed:")
	require.Contains(t, text, "Problem: config blocked")
	require.Contains(t, text, "Next:")
	require.Contains(t, text, "Custom App")
	require.Contains(t, text, "global discovery issue")
	require.Contains(t, text, "Summary: 2 apps checked.")
}

func TestDiscoverAmbiguousConfigStatesAreStableAndNonTraversing(t *testing.T) {
	t.Parallel()

	t.Run("ssh symlink is ambiguous without following", func(t *testing.T) {
		t.Parallel()

		home := t.TempDir()
		sshDir := filepath.Join(home, ".ssh")
		require.NoError(t, os.MkdirAll(sshDir, 0o755))
		writeDiscoveryFile(t, filepath.Join(home, "real-config"))
		require.NoError(t, os.Symlink(filepath.Join(home, "real-config"), filepath.Join(sshDir, "config")))

		report, err := Discover(DiscoverOptions{
			Target:        SSHTarget,
			GOOS:          "darwin",
			LocationRoots: map[string]string{"home": home},
			CommandLookup: missingCommands(),
		})
		require.NoError(t, err)
		target := requireDiscoveredTarget(t, report, SSHTarget)
		require.Equal(t, DiscoverStateAmbiguous, target.State)
		require.Equal(t, DiscoverConfigAmbiguous, target.ConfigState)
		require.Equal(t, DiscoverProbeAmbiguous, target.ConfigProbes[0].State)
		require.Equal(t, "symlink", target.ConfigProbes[0].ActualType)
		requireDiagnosticCode(t, target.Diagnostics, DiscoverCodeConfigSymlinkBlocked)
	})

	t.Run("file tree root wrong type is ambiguous and does not scan children", func(t *testing.T) {
		t.Parallel()

		configRoot := t.TempDir()
		writeDiscoveryFile(t, filepath.Join(configRoot, "nvim"))
		var probed []string
		report, err := Discover(DiscoverOptions{
			Target:        NvimTarget,
			GOOS:          "darwin",
			LocationRoots: map[string]string{"config": configRoot},
			CommandLookup: missingCommands(),
			Lstat: func(path string) (os.FileInfo, error) {
				probed = append(probed, filepath.ToSlash(path))
				return os.Lstat(path)
			},
		})
		require.NoError(t, err)
		target := requireDiscoveredTarget(t, report, NvimTarget)
		require.Equal(t, DiscoverStateAmbiguous, target.State)
		require.Equal(t, DiscoverConfigAmbiguous, target.ConfigState)
		require.Len(t, target.ConfigProbes, 1)
		require.Equal(t, "nvim", target.ConfigProbes[0].Path)
		require.Equal(t, []string{filepath.ToSlash(filepath.Join(configRoot, "nvim"))}, probed)
		requireDiagnosticCode(t, target.Diagnostics, DiscoverCodeConfigTypeMismatch)
	})

	t.Run("command lookup error makes binary axis ambiguous", func(t *testing.T) {
		t.Parallel()

		report, err := Discover(DiscoverOptions{
			Target:        GitTarget,
			GOOS:          "darwin",
			LocationRoots: map[string]string{"home": t.TempDir()},
			CommandLookup: func(command string, pathEnv string) (string, error) {
				return "", errors.New("lookup unavailable")
			},
		})
		require.NoError(t, err)
		target := requireDiscoveredTarget(t, report, GitTarget)
		require.Equal(t, DiscoverStateAmbiguous, target.State)
		require.Equal(t, DiscoverBinaryAmbiguous, target.BinaryState)
		requireDiagnosticCode(t, target.Diagnostics, DiscoverCodeCommandLookupError)
	})
}

func TestDiscoverJSONTextAndUnknownTargetAreStable(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	writeDiscoveryFile(t, filepath.Join(home, ".gitconfig"))
	report, err := Discover(DiscoverOptions{
		Target: GitTarget,
		GOOS:   "darwin",
		UserHomeExpand: func(value string) (string, error) {
			require.Equal(t, "~", value)
			return home, nil
		},
		CommandLookup: installedCommands("git"),
	})
	require.NoError(t, err)
	jsonPayload, err := DiscoverJSON(report)
	require.NoError(t, err)
	require.Contains(t, jsonPayload, `"command": "recipe.discover"`)
	require.Contains(t, jsonPayload, `"schemaVersion": 1`)
	require.Contains(t, jsonPayload, `"state": "config-present"`)
	require.NotContains(t, jsonPayload, home)
	textPayload := DiscoverText(report)
	require.Contains(t, textPayload, "Discover supported app settings")
	require.Contains(t, textPayload, "Git")
	require.Contains(t, textPayload, "$HOME/.gitconfig — present")
	require.NotContains(t, textPayload, "state=config-present")
	require.NotContains(t, textPayload, home)
	verboseText := DiscoverVerboseText(report)
	require.Contains(t, verboseText, "recipe discover")
	require.Contains(t, verboseText, "git state=config-present")

	errorReport, err := Discover(DiscoverOptions{Target: "missing"})
	require.Error(t, err)
	require.Equal(t, "error", errorReport.Summary.Status)
	require.Equal(t, DiscoverCodeUnknownTarget, errorReport.Error.Code)

	allReport, err := Discover(DiscoverOptions{
		GOOS:          "darwin",
		LocationRoots: map[string]string{"home": t.TempDir(), "config": t.TempDir()},
		CommandLookup: missingCommands(),
	})
	require.NoError(t, err)
	require.Len(t, allReport.Discovery.Targets, 7)
	require.Equal(t, CustomFilesTarget, allReport.Discovery.Targets[0].ID)

	nilJSON, err := DiscoverJSON(nil)
	require.NoError(t, err)
	require.Contains(t, nilJSON, `"status": "error"`)
	require.Contains(t, DiscoverText(nil), "The command could not complete")
	require.Equal(t, "recipe discover\nsummary status=error targets=0", DiscoverVerboseText(nil))
}

func TestDiscoverInternalHelpers(t *testing.T) {
	t.Parallel()

	binDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "git"), []byte("#!/bin/sh\n"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "not-executable"), []byte("nope"), 0o644))
	require.NoError(t, os.Mkdir(filepath.Join(binDir, "directory"), 0o755))

	path, err := lookupCommandInPath("git", binDir)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(binDir, "git"), path)
	_, err = lookupCommandInPath("missing", binDir)
	require.ErrorIs(t, err, exec.ErrNotFound)
	_, err = lookupCommandInPath("", binDir)
	require.ErrorIs(t, err, exec.ErrNotFound)
	_, err = lookupCommandInPath("sub/git", binDir)
	require.ErrorIs(t, err, exec.ErrNotFound)
	_, err = lookupCommandInPath("not-executable", binDir)
	require.ErrorIs(t, err, exec.ErrNotFound)
	_, err = lookupCommandInPath("directory", binDir)
	require.ErrorIs(t, err, exec.ErrNotFound)
	require.NotEmpty(t, currentGOOS())

	rec := BundledGitRecipe()
	root, err := discoveryLocationRoot(rec, "home", DiscoverOptions{LocationRoots: map[string]string{"home": "/override"}})
	require.NoError(t, err)
	require.Equal(t, "/override", root)
	root, err = discoveryLocationRoot(rec, "home", DiscoverOptions{UserHomeExpand: func(value string) (string, error) {
		require.Equal(t, "~", value)
		return "/expanded-home", nil
	}})
	require.NoError(t, err)
	require.Equal(t, "/expanded-home", root)
	_, err = discoveryLocationRoot(rec, "missing", DiscoverOptions{})
	require.Error(t, err)

	require.Equal(t, "file", discoveryExpectedType(IniFileDriverID))
	require.Equal(t, "file", discoveryExpectedType(JSONFileDriverID))
	require.Equal(t, "file", discoveryExpectedType(YAMLFileDriverID))
	require.Equal(t, "file", discoveryExpectedType(TOMLFileDriverID))
	require.Equal(t, "file", discoveryExpectedType(PlistFileDriverID))
	require.Equal(t, "directory", discoveryExpectedType(FileTreeDriverID))
	require.Equal(t, "", discoveryExpectedType("native"))
	require.True(t, actualTypeMatches("file", "symlink"))
	require.True(t, actualTypeMatches("file", "file"))
	require.False(t, actualTypeMatches("directory", "file"))
	require.Equal(t, DiscoverStateNotApplicable, summarizeDiscoveryState(DiscoverPlatformUnknown, DiscoverBinaryNotApplicable, DiscoverConfigNotApplicable))
	require.Equal(t, DiscoverStateAmbiguous, summarizeDiscoveryState(DiscoverPlatformSupported, DiscoverBinaryAmbiguous, DiscoverConfigPresent))
}

func installedCommands(names ...string) func(string, string) (string, error) {
	installed := map[string]bool{}
	for _, name := range names {
		installed[name] = true
	}
	return func(command string, pathEnv string) (string, error) {
		if installed[command] {
			return "/fixture/bin/" + command, nil
		}
		return "", exec.ErrNotFound
	}
}

func missingCommands() func(string, string) (string, error) {
	return installedCommands()
}

func requireDiscoveredTarget(t *testing.T, report *DiscoverReport, targetID string) DiscoveredTarget {
	t.Helper()
	require.NotNil(t, report)
	for _, target := range report.Discovery.Targets {
		if target.ID == targetID {
			return target
		}
	}
	require.Failf(t, "missing discovered target", "target %s not found in %#v", targetID, report.Discovery.Targets)
	return DiscoveredTarget{}
}

func requireDiagnosticCode(t *testing.T, diagnostics []ExplainDiagnostic, code string) {
	t.Helper()
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			return
		}
	}
	require.Failf(t, "missing diagnostic", "diagnostic %s not found in %#v", code, diagnostics)
}

func writeDiscoveryFile(t *testing.T, path string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("fixture content must not be read\n"), 0o644))
}

func TestDiscoverConfigLocationContractUsesHomeConfigNotProcessXDG(t *testing.T) {
	home := t.TempDir()
	xdgConfig := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", xdgConfig)

	writeDiscoveryFile(t, filepath.Join(xdgConfig, "starship.toml"))
	require.NoError(t, os.MkdirAll(filepath.Join(xdgConfig, "nvim"), 0o755))

	starshipXDGOnly, err := Discover(DiscoverOptions{Target: StarshipTarget, GOOS: "darwin", CommandLookup: missingCommands()})
	require.NoError(t, err)
	starshipTarget := requireDiscoveredTarget(t, starshipXDGOnly, StarshipTarget)
	require.Equal(t, DiscoverConfigMissing, starshipTarget.ConfigState)
	require.Len(t, starshipTarget.ConfigProbes, 1)
	require.Equal(t, "~/.config", starshipTarget.ConfigProbes[0].LocationDefault)
	require.Equal(t, "~/.config/starship.toml", starshipTarget.ConfigProbes[0].DisplayPath)
	starshipText := DiscoverText(starshipXDGOnly)
	require.Contains(t, starshipText, "~/.config/starship.toml — missing")
	require.NotContains(t, starshipText, "$XDG_CONFIG_HOME")

	nvimXDGOnly, err := Discover(DiscoverOptions{Target: NvimTarget, GOOS: "darwin", CommandLookup: missingCommands()})
	require.NoError(t, err)
	nvimTarget := requireDiscoveredTarget(t, nvimXDGOnly, NvimTarget)
	require.Equal(t, DiscoverConfigMissing, nvimTarget.ConfigState)
	require.Len(t, nvimTarget.ConfigProbes, 1)
	require.Equal(t, "~/.config", nvimTarget.ConfigProbes[0].LocationDefault)
	require.Equal(t, "~/.config/nvim", nvimTarget.ConfigProbes[0].DisplayPath)
	nvimText := DiscoverText(nvimXDGOnly)
	require.Contains(t, nvimText, "~/.config/nvim — missing")
	require.NotContains(t, nvimText, "$XDG_CONFIG_HOME")

	writeDiscoveryFile(t, filepath.Join(home, ".config", "starship.toml"))
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".config", "nvim"), 0o755))

	starshipHomeConfig, err := Discover(DiscoverOptions{Target: StarshipTarget, GOOS: "darwin", CommandLookup: missingCommands()})
	require.NoError(t, err)
	starshipTarget = requireDiscoveredTarget(t, starshipHomeConfig, StarshipTarget)
	require.Equal(t, DiscoverConfigPresent, starshipTarget.ConfigState)
	require.Equal(t, "~/.config/starship.toml", starshipTarget.ConfigProbes[0].DisplayPath)
	require.Contains(t, DiscoverText(starshipHomeConfig), "~/.config/starship.toml — present")

	nvimHomeConfig, err := Discover(DiscoverOptions{Target: NvimTarget, GOOS: "darwin", CommandLookup: missingCommands()})
	require.NoError(t, err)
	nvimTarget = requireDiscoveredTarget(t, nvimHomeConfig, NvimTarget)
	require.Equal(t, DiscoverConfigPresent, nvimTarget.ConfigState)
	require.Equal(t, "~/.config/nvim", nvimTarget.ConfigProbes[0].DisplayPath)
	require.Contains(t, DiscoverText(nvimHomeConfig), "~/.config/nvim — present")
}

func TestDiscoverConfigLocationOverrideIsExplicit(t *testing.T) {
	override := t.TempDir()
	writeDiscoveryFile(t, filepath.Join(override, "starship.toml"))
	require.NoError(t, os.MkdirAll(filepath.Join(override, "nvim"), 0o755))

	starship, err := Discover(DiscoverOptions{Target: StarshipTarget, GOOS: "darwin", LocationRoots: map[string]string{"config": override}, CommandLookup: missingCommands()})
	require.NoError(t, err)
	starshipTarget := requireDiscoveredTarget(t, starship, StarshipTarget)
	require.Equal(t, DiscoverConfigPresent, starshipTarget.ConfigState)
	require.Equal(t, filepath.ToSlash(filepath.Join(override, "starship.toml")), starshipTarget.ConfigProbes[0].DisplayPath)
	require.Contains(t, DiscoverText(starship), filepath.ToSlash(filepath.Join(override, "starship.toml"))+" — present")

	nvim, err := Discover(DiscoverOptions{Target: NvimTarget, GOOS: "darwin", LocationRoots: map[string]string{"config": override}, CommandLookup: missingCommands()})
	require.NoError(t, err)
	nvimTarget := requireDiscoveredTarget(t, nvim, NvimTarget)
	require.Equal(t, DiscoverConfigPresent, nvimTarget.ConfigState)
	require.Equal(t, filepath.ToSlash(filepath.Join(override, "nvim")), nvimTarget.ConfigProbes[0].DisplayPath)
	require.Contains(t, DiscoverText(nvim), filepath.ToSlash(filepath.Join(override, "nvim"))+" — present")
}
