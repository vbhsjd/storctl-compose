package compose

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadInputsParsesPasswordAndKey(t *testing.T) {
	dir := t.TempDir()
	write := func(name, data string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(data), 0644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	profile := write("profiles.json", `{"profiles":{"c4":{"vlan_id":172,"gateway":"172.27.0.1","prefix":18,"mounts":[{"server":"172.27.0.50","export":"/Share","mount_point":"/mnt/share"}]}}}`)
	drivers := filepath.Join(dir, "drivers")
	if err := os.Mkdir(drivers, 0755); err != nil {
		t.Fatal(err)
	}
	hosts := write("hosts.yaml", `hosts:
  - name: node-a
    ip: 80.5.21.122
    user: root
    password: secret
  - name: node-b
    ip: 80.5.21.123
    user: root
    key_file: /tmp/key
`)
	cfgPath := write("compose.yaml", "profile: c4\nprofile_file: "+profile+"\nartifact_src: "+drivers+"\nnic_type: \"1823\"\n")
	gotHosts, cfg, err := LoadInputs(hosts, cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(gotHosts.Hosts) != 2 || gotHosts.Hosts[0].Password != "secret" || gotHosts.Hosts[1].KeyFile != "/tmp/key" {
		t.Fatalf("unexpected hosts: %+v", gotHosts.Hosts)
	}
	if cfg.RemoteBin != "/usr/local/bin/storctl" || cfg.RemoteArtifact != "/root/storage_pkgs" || !cfg.AllowTCPFallback {
		t.Fatalf("defaults not applied: %+v", cfg)
	}
}

func TestLoadInputsRejectsUnsupportedNICType(t *testing.T) {
	dir := t.TempDir()
	profile := filepath.Join(dir, "profiles.json")
	drivers := filepath.Join(dir, "drivers")
	_ = os.WriteFile(profile, []byte("{}"), 0644)
	_ = os.Mkdir(drivers, 0755)
	hosts := filepath.Join(dir, "hosts.yaml")
	cfg := filepath.Join(dir, "compose.yaml")
	_ = os.WriteFile(hosts, []byte("hosts:\n  - name: node\n    ip: 1.1.1.1\n    user: root\n    password: x\n"), 0644)
	_ = os.WriteFile(cfg, []byte("profile: c4\nprofile_file: "+profile+"\nartifact_src: "+drivers+"\nnic_type: cx7\n"), 0644)
	if _, _, err := LoadInputs(hosts, cfg); err == nil {
		t.Fatal("expected unsupported nic_type error")
	}
}

func TestNormalizeOptionsClampsConcurrency(t *testing.T) {
	opts := NormalizeOptions(Options{Concurrency: 99}, Config{ReportDir: "reports"})
	if opts.Concurrency != MaxConcurrency {
		t.Fatalf("Concurrency=%d", opts.Concurrency)
	}
	opts = NormalizeOptions(Options{Concurrency: 0}, Config{ReportDir: "reports"})
	if opts.Concurrency != DefaultConcurrency {
		t.Fatalf("Concurrency=%d", opts.Concurrency)
	}
}
