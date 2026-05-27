package compose

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

func LoadInputs(hostsPath, configPath string) (HostsFile, Config, error) {
	var hosts HostsFile
	cfg := Config{AllowTCPFallback: true}
	if err := readYAML(hostsPath, &hosts); err != nil {
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
		return fmt.Errorf("hosts.yaml must contain at least one host")
	}
	seen := map[string]bool{}
	for i, host := range hosts.Hosts {
		if strings.TrimSpace(host.Name) == "" || strings.TrimSpace(host.IP) == "" || strings.TrimSpace(host.User) == "" {
			return fmt.Errorf("hosts[%d] requires name, ip, and user", i)
		}
		if host.User != "root" {
			return fmt.Errorf("host %s must use root login in v1", host.Name)
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
	if opts.ReportDir == "" {
		opts.ReportDir = cfg.ReportDir
	}
	opts.ReportDir = filepath.Clean(opts.ReportDir)
	return opts
}
