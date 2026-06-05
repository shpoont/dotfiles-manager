package ledger

type RunStatus string

const (
	RunStatusVerified RunStatus = "verified"
	RunStatusFailed   RunStatus = "failed"
	RunStatusPartial  RunStatus = "partial"
	RunStatusDryRun   RunStatus = "dry-run"
)

type ItemResult string

const (
	ItemResultVerified  ItemResult = "verified"
	ItemResultFailed    ItemResult = "failed"
	ItemResultSkipped   ItemResult = "skipped"
	ItemResultDryRun    ItemResult = "dry-run"
	ItemResultUnchanged ItemResult = "unchanged"
)

type RunRecord struct {
	Schema        string       `json:"schema"`
	SchemaVersion int          `json:"schemaVersion"`
	RunID         string       `json:"runId"`
	StartedAt     string       `json:"startedAt"`
	FinishedAt    string       `json:"finishedAt"`
	Command       string       `json:"command"`
	ProfileStack  []string     `json:"profileStack"`
	Status        RunStatus    `json:"status"`
	Summary       RunSummary   `json:"summary"`
	Items         []ItemRecord `json:"items"`
}

type RunSummary struct {
	Status   RunStatus `json:"status"`
	Verified int       `json:"verified"`
	Failed   int       `json:"failed"`
	Skipped  int       `json:"skipped"`
	DryRun   int       `json:"dryRun"`
}

type LedgerEntry struct {
	Schema        string     `json:"schema"`
	SchemaVersion int        `json:"schemaVersion"`
	RunID         string     `json:"runId"`
	Timestamp     string     `json:"timestamp"`
	Command       string     `json:"command"`
	ProfileStack  []string   `json:"profileStack"`
	Item          ItemRecord `json:"item"`
}

type ItemRecord struct {
	TargetRef      string          `json:"targetRef"`
	SettingRef     string          `json:"settingRef"`
	Operation      string          `json:"operation"`
	ResourceID     string          `json:"resourceId"`
	Driver         string          `json:"driver"`
	DriverVersion  string          `json:"driverVersion"`
	DesiredURI     string          `json:"desiredUri,omitempty"`
	DesiredRelPath string          `json:"desiredRelPath,omitempty"`
	LivePath       string          `json:"livePath,omitempty"`
	DesiredPath    string          `json:"desiredPath,omitempty"`
	ArtifactRefs   ArtifactRefs    `json:"artifactRefs"`
	Before         NormalizedState `json:"before"`
	Desired        NormalizedState `json:"desired"`
	VerifiedState  NormalizedState `json:"verifiedState"`
	BackupRefs     []string        `json:"backupRefs,omitempty"`
	Verification   Verification    `json:"verification"`
	Result         ItemResult      `json:"result"`
	Diagnostics    []Diagnostic    `json:"diagnostics,omitempty"`
}

type ArtifactRefs struct {
	Desired       string `json:"desired,omitempty"`
	DesiredURI    string `json:"desiredUri,omitempty"`
	DesiredPath   string `json:"desiredPath,omitempty"`
	LivePath      string `json:"livePath,omitempty"`
	Backup        string `json:"backup,omitempty"`
	BackupPayload string `json:"backupPayload,omitempty"`
	RunRecord     string `json:"runRecord,omitempty"`
	Ledger        string `json:"ledger,omitempty"`
}

type NormalizedState struct {
	Exists        bool   `json:"exists" yaml:"exists"`
	Hash          string `json:"hash,omitempty" yaml:"hash,omitempty"`
	Normalizer    string `json:"normalizer,omitempty" yaml:"normalizer,omitempty"`
	DriverVersion string `json:"driverVersion,omitempty" yaml:"driverVersion,omitempty"`
	Size          int    `json:"size,omitempty" yaml:"size,omitempty"`
	EntryCount    int    `json:"entryCount,omitempty" yaml:"entryCount,omitempty"`
	FileCount     int    `json:"fileCount,omitempty" yaml:"fileCount,omitempty"`
	DirCount      int    `json:"dirCount,omitempty" yaml:"dirCount,omitempty"`
}

type Verification struct {
	Verified bool   `json:"verified"`
	Result   string `json:"result"`
	Message  string `json:"message,omitempty"`
}

type Diagnostic struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Path    string `json:"path,omitempty"`
}

type BackupMetadata struct {
	Schema        string       `yaml:"schema" json:"schema"`
	SchemaVersion int          `yaml:"schemaVersion" json:"schemaVersion"`
	RunID         string       `yaml:"runId" json:"runId"`
	CreatedAt     string       `yaml:"createdAt" json:"createdAt"`
	Items         []BackupItem `yaml:"items" json:"items"`
}

type BackupItem struct {
	Ref            string               `yaml:"ref" json:"ref"`
	TargetRef      string               `yaml:"targetRef" json:"targetRef"`
	SettingRef     string               `yaml:"settingRef" json:"settingRef"`
	ResourceID     string               `yaml:"resourceId" json:"resourceId"`
	Driver         string               `yaml:"driver" json:"driver"`
	DriverVersion  string               `yaml:"driverVersion" json:"driverVersion"`
	LivePath       string               `yaml:"livePath" json:"livePath"`
	PayloadRelPath string               `yaml:"payloadRelPath,omitempty" json:"payloadRelPath,omitempty"`
	CreatedAt      string               `yaml:"createdAt" json:"createdAt"`
	Before         NormalizedState      `yaml:"before" json:"before"`
	Restore        RestoreCompatibility `yaml:"restore" json:"restore"`
}

type RestoreCompatibility struct {
	Compatible    bool   `yaml:"compatible" json:"compatible"`
	Driver        string `yaml:"driver" json:"driver"`
	DriverVersion string `yaml:"driverVersion" json:"driverVersion"`
	Normalizer    string `yaml:"normalizer" json:"normalizer"`
	Message       string `yaml:"message" json:"message"`
}

func summarizeItems(items []ItemRecord) RunSummary {
	var summary RunSummary
	for _, item := range items {
		switch item.Result {
		case ItemResultVerified, ItemResultUnchanged:
			summary.Verified++
		case ItemResultFailed:
			summary.Failed++
		case ItemResultSkipped:
			summary.Skipped++
		case ItemResultDryRun:
			summary.DryRun++
		}
	}
	switch {
	case summary.Failed > 0 && summary.Verified > 0:
		summary.Status = RunStatusPartial
	case summary.Failed > 0:
		summary.Status = RunStatusFailed
	case summary.DryRun > 0 && summary.Verified == 0 && summary.Skipped == 0:
		summary.Status = RunStatusDryRun
	default:
		summary.Status = RunStatusVerified
	}
	return summary
}
