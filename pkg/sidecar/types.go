package sidecar

// SidecarMode specifies the operational direction of the sidecar proxy.
type SidecarMode string

const (
	ModeIngress SidecarMode = "ingress"
	ModeEgress  SidecarMode = "egress"
	ModeDual    SidecarMode = "dual"
)

// SplitTarget captures an upstream target and its weight for traffic splitting.
type SplitTarget struct {
	URL    string
	Weight int
}
