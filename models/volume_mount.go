package models

type VolumeMount struct {
	// Name of a volume declared in metadata.volumes
	// null means the runner-provided volume
	Name *string `json:"name"`

	// Path inside the container where the volume is mounted
	MountPath string `json:"mount_path"`
}
