package app

func operationPhaseAlias(phase string) string {
	switch phase {
	case "copy":
		return "deploy"
	case "update_managed":
		return "import"
	case "add_unmanaged":
		return "incoming_unmanaged"
	default:
		return phase
	}
}

func phaseHeaderAlias(label string) string {
	switch label {
	case "copy":
		return "deploy"
	case "update-managed":
		return "import"
	case "add-unmanaged":
		return "incoming-unmanaged"
	default:
		return ""
	}
}
