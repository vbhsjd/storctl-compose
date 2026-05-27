package compose

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

func LoadInputs(hostsPath, configPath string) (HostsFile, Config, error) {
	var hosts HostsFile
	cfg := Config{AllowTCPFallback: true}
	resolvedHostsPath := resolveHostsPath(hostsPath)
	if err := readHosts(resolvedHostsPath, &hosts); err != nil {
		return hosts, cfg, err
	}
	if err := readYAML(configPath, &cfg); err != nil {
		return hosts, cfg, err
	}
	applyConfigDefaults(&cfg)
	if err := validateHosts(hosts); err != nil {
		return hosts, cfg, err
	}
	if err := validateConfig(cfg); err != nil {
		return hosts, cfg, err
	}
	return hosts, cfg, nil
}

func resolveHostsPath(path string) string {
	if path == "hosts.csv" {
		if _, err := os.Stat(path); err == nil {
			return path
		}
		if _, err := os.Stat("hosts.yaml"); err == nil {
			return "hosts.yaml"
		}
	}
	return path
}

func readHosts(path string, hosts *HostsFile) error {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".csv":
		return readHostsCSV(path, hosts)
	default:
		return readYAML(path, hosts)
	}
}

func readYAML(path string, out any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	if err := yaml.Unmarshal(data, out); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	return nil
}

func readHostsCSV(path string, hosts *HostsFile) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	r.TrimLeadingSpace = true
	records, err := r.ReadAll()
	if err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	if len(records) == 0 {
		return fmt.Errorf("%s must contain at least one host", path)
	}
	start := 0
	cols := map[string]int{"ip": 0, "password": 1, "user": 2}
	if looksLikeHeader(records[0]) {
		start = 1
		cols = parseHostsCSVHeader(records[0])
	}
	for line, rec := range records[start:] {
		if isEmptyCSVRecord(rec) {
			continue
		}
		host := Host{
			Name:     csvField(rec, cols["ip"]),
			IP:       csvField(rec, cols["ip"]),
			Password: csvField(rec, cols["password"]),
			User:     csvField(rec, cols["user"]),
		}
		if host.User == "" {
			host.User = "root"
		}
		if host.Name == "" {
			return fmt.Errorf("%s row %d requires ip", path, start+line+1)
		}
		hosts.Hosts = append(hosts.Hosts, host)
	}
	if len(hosts.Hosts) == 0 {
		return fmt.Errorf("%s must contain at least one host", path)
	}
	return nil
}

func looksLikeHeader(rec []string) bool {
	for _, field := range rec {
		switch normalizeCSVHeader(field) {
		case "ip", "password", "passwd", "user", "username", "密码", "账号", "用户名":
			return true
		}
	}
	return false
}

func parseHostsCSVHeader(header []string) map[string]int {
	cols := map[string]int{"ip": -1, "password": -1, "user": -1}
	for i, field := range header {
		switch normalizeCSVHeader(field) {
		case "ip", "host", "hostname":
			cols["ip"] = i
		case "password", "passwd", "密码":
			cols["password"] = i
		case "user", "username", "账号", "用户名":
			cols["user"] = i
		}
	}
	return cols
}

func normalizeCSVHeader(field string) string {
	return strings.ToLower(strings.TrimSpace(strings.TrimPrefix(field, "\ufeff")))
}

func csvField(rec []string, index int) string {
	if index < 0 || index >= len(rec) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(rec[index], "\ufeff"))
}

func isEmptyCSVRecord(rec []string) bool {
	for _, field := range rec {
		if strings.TrimSpace(field) != "" {
			return false
		}
	}
	return true
}

func applyConfigDefaults(cfg *Config) {
	if cfg.RemoteBin == "" {
		cfg.RemoteBin = "/usr/local/bin/storctl"
	}
	if cfg.RemoteProfile == "" {
		cfg.RemoteProfile = "/etc/storctl/profiles.json"
	}
	if cfg.RemoteArtifact == "" {
		cfg.RemoteArtifact = "/root/storage_pkgs"
	}
	if cfg.QoS == "" {
		cfg.QoS = "off"
	}
	if cfg.ReportDir == "" {
		cfg.ReportDir = "reports"
	}
}

func validateHosts(hosts HostsFile) error {
	if len(hosts.Hosts) == 0 {
		return fmt.Errorf("hosts file must contain at least one host")
	}
	seen := map[string]bool{}
	for i, host := range hosts.Hosts {
		if strings.TrimSpace(host.Name) == "" {
			host.Name = host.IP
		}
		if strings.TrimSpace(host.User) == "" {
			host.User = "root"
		}
		hosts.Hosts[i] = host
		if strings.TrimSpace(host.Name) == "" || strings.TrimSpace(host.IP) == "" || strings.TrimSpace(host.User) == "" {
			return fmt.Errorf("hosts[%d] requires ip; user defaults to root", i)
		}
		if host.Password == "" && host.KeyFile == "" {
			return fmt.Errorf("host %s requires password or key_file", host.Name)
		}
		if host.Port == 0 {
			host.Port = 22
		}
		if seen[host.Name] {
			return fmt.Errorf("duplicate host name %s", host.Name)
		}
		seen[host.Name] = true
	}
	return nil
}

func validateConfig(cfg Config) error {
	missing := []string{}
	for name, value := range map[string]string{
		"profile":      cfg.Profile,
		"profile_file": cfg.ProfileFile,
		"artifact_src": cfg.ArtifactSrc,
	} {
		if strings.TrimSpace(value) == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("compose.yaml missing %s", strings.Join(missing, ", "))
	}
	if cfg.QoS != "off" && cfg.QoS != "apply" {
		return fmt.Errorf("qos must be off or apply")
	}
	if _, err := os.Stat(cfg.ProfileFile); err != nil {
		return fmt.Errorf("profile_file %s: %w", cfg.ProfileFile, err)
	}
	if st, err := os.Stat(cfg.ArtifactSrc); err != nil || !st.IsDir() {
		if err == nil {
			err = fmt.Errorf("not a directory")
		}
		return fmt.Errorf("artifact_src %s: %w", cfg.ArtifactSrc, err)
	}
	if cfg.StorctlBin != "" {
		if _, err := os.Stat(cfg.StorctlBin); err != nil {
			return fmt.Errorf("storctl_bin %s: %w", cfg.StorctlBin, err)
		}
	}
	return nil
}

func FilterHosts(hosts []Host, limit string) []Host {
	if strings.TrimSpace(limit) == "" {
		return hosts
	}
	want := map[string]bool{}
	for _, part := range strings.Split(limit, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			want[part] = true
		}
	}
	out := []Host{}
	for _, host := range hosts {
		if want[host.Name] || want[host.IP] {
			out = append(out, host)
		}
	}
	return out
}

func NormalizeOptions(opts Options, cfg Config) Options {
	if opts.Concurrency == 0 {
		opts.Concurrency = DefaultConcurrency
	}
	if opts.Concurrency < 1 {
		opts.Concurrency = 1
	}
	if opts.Concurrency > MaxConcurrency {
		opts.Concurrency = MaxConcurrency
	}
	if opts.Timeout == 0 {
		opts.Timeout = DefaultTimeout
	}
	if opts.ReportDir == "" {
		opts.ReportDir = cfg.ReportDir
	}
	opts.ReportDir = filepath.Clean(opts.ReportDir)
	return opts
}
