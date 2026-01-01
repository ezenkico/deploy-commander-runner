package models

type ScaleMode string

const (
	ScaleModeSingle        ScaleMode = "single"
	ScaleModeAutoscale     ScaleMode = "autoscale"
	ScaleModeAutoscaleCore ScaleMode = "autoscale-core"
	ScaleModeGlobal        ScaleMode = "global"
)

type ScaleSpec struct {
	Mode string `json:"mode"` // single | autoscale | autoscale-core | global
	Min  *int   `json:"min,omitempty"`
	Max  *int   `json:"max,omitempty"`
}
