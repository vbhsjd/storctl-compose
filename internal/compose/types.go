package compose

import "time"

const (
	DefaultConcurrency = 30
	MaxConcurrency     = 50
)

type HostsFile struct {
	Hosts []Host `yaml:"hosts"`
}

type Host struct {
	Name     string `yaml:"name"`
	IP       string `yaml:"ip"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	KeyFile  string `yaml:"key_file"`
	Port     int    `yaml:"port"`
}

type Config struct {
	Profile          string `yaml:"profile"`
	StorctlBin       string `yaml:"storctl_bin"`
	ProfileFile      string `yaml:"profile_file"`
	ArtifactSrc      string `yaml:"artifact_src"`
	RemoteBin        string `yaml:"remote_bin"`
	RemoteProfile    string `yaml:"remote_profile_file"`
	RemoteArtifact   string `yaml:"remote_artifact_dir"`
	AllowTCPFallback bool   `yaml:"allow_tcp_fallback"`
	QoS              string `yaml:"qos"`
	ReportDir        string `yaml:"report_dir"`
	NICType          string `yaml:"nic_type"`
}

type Options struct {
	HostsPath       string
	ConfigPath      string
	ReportDir       string
	Limit           string
	Concurrency     int
	UpgradeFirmware bool
}

type HostResult struct {
	Host           string `json:"host"`
	IP             string `json:"ip"`
	Command        string `json:"command"`
	Status         string `json:"status"`
	Code           string `json:"code,omitempty"`
	Message        string `json:"message,omitempty"`
	SelectedNIC    string `json:"selected_nic,omitempty"`
	RebootRequired bool   `json:"reboot_required,omitempty"`
	Degraded       bool   `json:"degraded,omitempty"`
	StartedAt      string `json:"started_at"`
	FinishedAt     string `json:"finished_at"`
}

type CommandResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

type CandidateNIC struct {
	Name    string
	Driver  string
	Speed   int
	Carrier bool
	HasIPv4 bool
	Up      bool
	Score   int
}

type VersionInfo struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	BuiltAt string `json:"built_at"`
}

func nowString() string {
	return time.Now().Format(time.RFC3339)
}
