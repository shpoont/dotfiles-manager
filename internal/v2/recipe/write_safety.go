package recipe

import "fmt"

func (r *Recipe) ValidateWriteSafety(ctx WriteSafetyContext) error {
	diagnostics := r.WriteSafetyDiagnostics(ctx)
	var blocking []ValidationDiagnostic
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity == "" || diagnostic.Severity == ValidationSeverityError {
			blocking = append(blocking, diagnostic)
		}
	}
	return validationError(blocking)
}

func (r *Recipe) WriteSafetyDiagnostics(ctx WriteSafetyContext) []ValidationDiagnostic {
	if r == nil {
		return []ValidationDiagnostic{validationDiagnostic("recipe.required", "$", "recipe is required")}
	}
	if diagnostics := r.ValidationDiagnostics(); hasErrorDiagnostics(diagnostics) {
		return diagnostics
	}

	var diagnostics []ValidationDiagnostic
	add := func(code string, path string, message string) {
		diagnostics = append(diagnostics, writeSafetyDiagnostic(code, path, ValidationSeverityError, message))
	}
	addWarning := func(code string, path string, message string) {
		diagnostics = append(diagnostics, writeSafetyDiagnostic(code, path, ValidationSeverityWarning, message))
	}

	hasWriteCapableSubject := false
	for _, settingID := range sortedKeys(r.Settings) {
		setting := r.Settings[settingID]
		if !isWriteCapableCapability(effectiveSettingCapability(r, setting)) {
			continue
		}
		hasWriteCapableSubject = true
		settingPath := "$.settings." + settingID
		if setting.Sensitivity == "" {
			add("writeSafety.setting.sensitivity.required", settingPath+".sensitivity", fmt.Sprintf("write-capable setting %s requires sensitivity metadata", settingID))
		} else {
			diagnostics = append(diagnostics, sensitivityWriteDiagnostics(setting.Sensitivity, settingPath+".sensitivity", fmt.Sprintf("setting %s", settingID), ctx)...)
		}
		if setting.Redaction == "" {
			add("writeSafety.setting.redaction.required", settingPath+".redaction", fmt.Sprintf("write-capable setting %s requires redaction metadata", settingID))
		} else {
			diagnostics = append(diagnostics, redactionWriteDiagnostics(setting.Redaction, settingPath+".redaction", fmt.Sprintf("setting %s", settingID), ctx)...)
		}
		if setting.Lifecycle != "" {
			diagnostics = append(diagnostics, lifecycleWriteDiagnostics(setting.Lifecycle, settingPath+".lifecycle", fmt.Sprintf("setting %s", settingID), ctx, addWarning)...)
		}
	}

	for _, resourceID := range sortedKeys(r.Resources) {
		resource := r.Resources[resourceID]
		if !isWriteCapableCapability(effectiveResourceCapability(r, resource)) {
			continue
		}
		hasWriteCapableSubject = true
		resourcePath := "$.resources." + resourceID
		if resource.Sensitivity == "" {
			add("writeSafety.resource.sensitivity.required", resourcePath+".sensitivity", fmt.Sprintf("write-capable resource %s requires sensitivity metadata", resourceID))
		} else {
			diagnostics = append(diagnostics, sensitivityWriteDiagnostics(resource.Sensitivity, resourcePath+".sensitivity", fmt.Sprintf("resource %s", resourceID), ctx)...)
		}
		if resource.Redaction == "" {
			add("writeSafety.resource.redaction.required", resourcePath+".redaction", fmt.Sprintf("write-capable resource %s requires redaction metadata", resourceID))
		} else {
			diagnostics = append(diagnostics, redactionWriteDiagnostics(resource.Redaction, resourcePath+".redaction", fmt.Sprintf("resource %s", resourceID), ctx)...)
		}
		if resource.Lifecycle == "" {
			add("writeSafety.resource.lifecycle.required", resourcePath+".lifecycle", fmt.Sprintf("write-capable resource %s requires lifecycle metadata", resourceID))
		} else {
			diagnostics = append(diagnostics, lifecycleWriteDiagnostics(resource.Lifecycle, resourcePath+".lifecycle", fmt.Sprintf("resource %s", resourceID), ctx, addWarning)...)
		}
	}

	if hasWriteCapableSubject {
		switch ctx.Source {
		case "":
			add("writeSafety.trust.sourceRequired", "$", "write safety context source is required before write planning")
		case RecipeSourceLocal:
			if !ctx.Trusted {
				add("writeSafety.trust.untrusted", "$", "local write-capable recipes require explicit trust before write planning")
			}
		case RecipeSourceBundled:
		default:
			add("writeSafety.trust.sourceUnsupported", "$", "write safety context source must be bundled or local")
		}
	}

	return normalizeDiagnostics(diagnostics)
}

func sensitivityWriteDiagnostics(value string, path string, subject string, ctx WriteSafetyContext) []ValidationDiagnostic {
	switch value {
	case SensitivitySecret:
		if !ctx.AllowSensitive {
			return []ValidationDiagnostic{writeSafetyDiagnostic("writeSafety.sensitivity.secretBlocked", path, ValidationSeverityError, fmt.Sprintf("%s sensitivity policy secret requires explicit sensitive-value approval", subject))}
		}
	case SensitivityUnknown:
		if !ctx.AllowUnknownSensitivity {
			return []ValidationDiagnostic{writeSafetyDiagnostic("writeSafety.sensitivity.unknownBlocked", path, ValidationSeverityError, fmt.Sprintf("%s sensitivity policy unknown requires explicit unknown-sensitivity approval", subject))}
		}
	}
	return nil
}

func redactionWriteDiagnostics(value string, path string, subject string, ctx WriteSafetyContext) []ValidationDiagnostic {
	switch value {
	case RedactionBlockedSave:
		return []ValidationDiagnostic{writeSafetyDiagnostic("writeSafety.redaction.blockedSave", path, ValidationSeverityError, fmt.Sprintf("%s redaction policy blocks save/apply planning", subject))}
	case RedactionUnavailable:
		if !ctx.AllowOpaque {
			return []ValidationDiagnostic{writeSafetyDiagnostic("writeSafety.redaction.unavailable", path, ValidationSeverityError, fmt.Sprintf("%s redaction policy requires explicit opaque-artifact approval", subject))}
		}
	}
	return nil
}

func lifecycleWriteDiagnostics(value string, path string, subject string, ctx WriteSafetyContext, addWarning func(string, string, string)) []ValidationDiagnostic {
	switch value {
	case LifecycleBlocked:
		return []ValidationDiagnostic{writeSafetyDiagnostic("writeSafety.lifecycle.blocked", path, ValidationSeverityError, fmt.Sprintf("%s lifecycle policy blocks write planning", subject))}
	case LifecycleAskToQuit, LifecycleQuitIfRunning, LifecycleBlockIfRunning, LifecycleReopenIfStoppedByTool:
		if !ctx.HandlesLifecycleActions {
			return []ValidationDiagnostic{writeSafetyDiagnostic("writeSafety.lifecycle.actionRequired", path, ValidationSeverityError, fmt.Sprintf("%s lifecycle policy requires lifecycle-action handling before write planning", subject))}
		}
	case LifecycleWarn:
		if addWarning != nil {
			addWarning("writeSafety.lifecycle.warn", path, fmt.Sprintf("%s lifecycle policy requires a user-visible warning", subject))
		}
	}
	return nil
}

func effectiveSettingCapability(r *Recipe, setting Setting) string {
	if setting.Capability != "" {
		return setting.Capability
	}
	if r == nil {
		return ""
	}
	return r.Capability
}

func effectiveResourceCapability(r *Recipe, resource Resource) string {
	if resource.Capability != "" {
		return resource.Capability
	}
	if r == nil {
		return ""
	}
	return r.Capability
}

func isWriteCapableCapability(value string) bool {
	switch value {
	case "read-write", "import-only", "export-only":
		return true
	default:
		return false
	}
}

func hasErrorDiagnostics(diagnostics []ValidationDiagnostic) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity == "" || diagnostic.Severity == ValidationSeverityError {
			return true
		}
	}
	return false
}

func writeSafetyDiagnostic(code string, path string, severity string, message string) ValidationDiagnostic {
	return ValidationDiagnostic{Code: code, Path: path, Severity: severity, Message: message}
}
