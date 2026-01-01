package models

type BindingSpec struct {
	ContainerPort *int    `json:"container_port,omitempty"`
	HostPort      *int    `json:"host_port,omitempty"`
	HostIP        *string `json:"host_ip,omitempty"`
	ContainerIP   *string `json:"container_ip,omitempty"`
}
