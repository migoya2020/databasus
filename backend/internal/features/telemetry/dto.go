package telemetry

type DatabaseEntry struct {
	Type         string `json:"type"`
	Version      string `json:"version"`
	RawSizeMb    int64  `json:"rawSizeMb,omitzero"`
	BackupSizeMb int64  `json:"backupSizeMb,omitzero"`
}

type CollectRequest struct {
	InstanceID  string          `json:"instanceID"`
	AppVersion  string          `json:"appVersion"`
	OS          string          `json:"os"`
	Arch        string          `json:"arch"`
	InstalledAt string          `json:"installedAt,omitempty"`
	Databases   []DatabaseEntry `json:"databases"`
	Storages    []string        `json:"storages"`
	Notifiers   []string        `json:"notifiers"`
}
