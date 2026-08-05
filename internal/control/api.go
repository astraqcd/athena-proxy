package control

const (
	Service = "athena-proxy"

	PathStatus  = "/v1/status"
	PathTargets = "/v1/targets"
)

type Status struct {
	Service string `json:"service"`
	Version string `json:"version"`
	PID     int    `json:"pid"`
}

type Target struct {
	Hostname  string `json:"hostname"`
	Name      string `json:"name"`
	LocalPort int    `json:"localPort"`
	LocalAddr string `json:"localAddr"`
}

type AddRequest struct {
	Hostname string `json:"hostname"`
	Name     string `json:"name,omitempty"`
	Port     int    `json:"port,omitempty"`
}

type AddResponse struct {
	Target     Target `json:"target"`
	Existing   bool   `json:"existing"`
	Reassigned bool   `json:"reassigned"`
	Requested  int    `json:"requestedPort,omitempty"`
}

type ListResponse struct {
	Targets []Target `json:"targets"`
}

type RemoveResponse struct {
	Target Target `json:"target"`
}

type Error struct {
	Message string `json:"error"`
}
