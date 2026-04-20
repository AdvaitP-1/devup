package api

// Mount describes a host->guest bind mount for workspace access
type Mount struct {
	HostPath  string `json:"host_path"`            // VM-visible path (e.g. /mnt/host/...)
	GuestPath string `json:"guest_path"`           // Where to bind inside VM (e.g. /workspace)
	ReadOnly  bool   `json:"read_only,omitempty"` // If true, mount read-only
}

// ResourceLimits specifies cgroup v2 resource constraints for a job.
type ResourceLimits struct {
	MemoryMB   int `json:"memory_mb,omitempty"`   // 0 = unlimited
	CPUPercent int `json:"cpu_percent,omitempty"` // 1-100 of one core; maps to cpu.max
	PidsMax    int `json:"pids_max,omitempty"`    // 0 = unlimited
}

// RunRequest is the JSON body for POST /run
type RunRequest struct {
	RequestID  string            `json:"request_id"`
	Cmd        []string          `json:"cmd"`
	Env        map[string]string `json:"env"`
	Cwd        string            `json:"cwd"`
	Mounts     []Mount           `json:"mounts,omitempty"`
	Limits     *ResourceLimits   `json:"limits,omitempty"`
	Overlay    bool              `json:"overlay,omitempty"`
	NetIsolate bool              `json:"net_isolate,omitempty"`
}

// HealthResponse is the JSON response for GET /health
type HealthResponse struct {
	Status      string `json:"status"`
	Version     string `json:"version"`
	DefaultHome string `json:"default_home,omitempty"`
}

// StartRequest is the JSON body for POST /start
type StartRequest struct {
	RequestID  string            `json:"request_id"`
	Cmd        []string          `json:"cmd"`
	Env        map[string]string `json:"env,omitempty"`
	Cwd        string            `json:"cwd,omitempty"`
	Mounts     []Mount           `json:"mounts,omitempty"`
	Limits     *ResourceLimits   `json:"limits,omitempty"`
	Overlay    bool              `json:"overlay,omitempty"`
	NetIsolate bool              `json:"net_isolate,omitempty"`
}

// StartResponse is the JSON response for POST /start
type StartResponse struct {
	JobID string `json:"job_id"`
}

// JobInfo describes a job for /ps and job state
type JobInfo struct {
	JobID      string          `json:"job_id"`
	Cmd        []string        `json:"cmd"`
	Status     string          `json:"status"` // running|exited|stopped|failed
	ExitCode   int             `json:"exit_code,omitempty"`
	StartedAt  int64           `json:"started_at_unix"`
	FinishedAt int64           `json:"finished_at_unix,omitempty"`
	Limits     *ResourceLimits `json:"limits,omitempty"`
}

// PsResponse is the JSON response for GET /ps
type PsResponse struct {
	Jobs []JobInfo `json:"jobs"`
}

// StopResponse is the JSON response for POST /stop
type StopResponse struct {
	JobID  string `json:"job_id"`
	Status string `json:"status"`
}

// ToolInfo describes a single tool's version check result
type ToolInfo struct {
	Status  string `json:"status"`  // "ok" or "missing"
	Version string `json:"version"` // version string or "-"
}

// SystemInfoResponse is the JSON response for GET /system/info
type SystemInfoResponse struct {
	Tools map[string]ToolInfo `json:"tools"`
}

// PeerInfo describes a discovered node in the cluster.
type PeerInfo struct {
	NodeID     string  `json:"node_id"`
	Addr       string  `json:"addr"`
	Port       int     `json:"port"`
	SlotsFree  int     `json:"slots_free"`
	Version    string  `json:"version"`
	Status     string  `json:"status"`      // "online" or "local"
	LastSeen   int64   `json:"last_seen"`   // unix timestamp
	ActiveJobs int     `json:"active_jobs"`
	MemTotalMB int     `json:"mem_total_mb,omitempty"`
	MemFreeMB  int     `json:"mem_free_mb,omitempty"`
	LoadAvg1   float64 `json:"load_avg_1,omitempty"`
}

// ClusterResponse is the JSON response for GET /cluster
type ClusterResponse struct {
	Peers []PeerInfo `json:"peers"`
}

// UploadResponse is the JSON response for POST /upload
type UploadResponse struct {
	WorkspacePath string `json:"workspace_path"`
}
