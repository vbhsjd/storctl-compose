package compose

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeDialer struct {
	remotes map[string]*fakeRemote
}

func (d fakeDialer) Dial(ctx context.Context, host Host) (Remote, error) {
	_ = ctx
	r := d.remotes[host.Name]
	if r == nil {
		return nil, fmt.Errorf("missing fake remote")
	}
	return r, nil
}

type fakeRemote struct {
	outputs map[string]CommandResult
	errs    map[string]error
	runs    []string
	uploads map[string][]byte
}

func (r *fakeRemote) Run(ctx context.Context, command string) (CommandResult, error) {
	_ = ctx
	r.runs = append(r.runs, command)
	if out, ok := r.outputs[command]; ok {
		return out, r.errs[command]
	}
	return CommandResult{ExitCode: 1}, fmt.Errorf("unexpected command: %s", command)
}

func (r *fakeRemote) UploadBytes(ctx context.Context, remotePath string, data []byte, mode os.FileMode) error {
	_ = ctx
	_ = mode
	if r.uploads == nil {
		r.uploads = map[string][]byte{}
	}
	r.uploads[remotePath] = append([]byte(nil), data...)
	return nil
}

func (r *fakeRemote) UploadFile(ctx context.Context, localPath, remotePath string, mode os.FileMode) error {
	_ = mode
	data, err := os.ReadFile(localPath)
	if err != nil {
		return err
	}
	return r.UploadBytes(ctx, remotePath, data, mode)
}

func (r *fakeRemote) MkdirAll(ctx context.Context, remotePath string, mode os.FileMode) error {
	_ = ctx
	_ = remotePath
	_ = mode
	return nil
}

func (r *fakeRemote) Close() error { return nil }

func TestDiscoverCandidatesFiltersAndSorts(t *testing.T) {
	r := &fakeRemote{outputs: map[string]CommandResult{}, errs: map[string]error{}}
	addIface := func(name, driver, speed, carrier, state, ip string, physical bool) {
		r.outputs["test -e /sys/class/net/"+shellQuote(name)+"/device"] = CommandResult{}
		if !physical {
			r.errs["test -e /sys/class/net/"+shellQuote(name)+"/device"] = fmt.Errorf("missing")
		}
		r.outputs["ip -o -4 addr show dev "+shellQuote(name)+" 2>/dev/null | awk '{print $4}' | cut -d/ -f1"] = CommandResult{Stdout: ip}
		r.outputs["ethtool -i "+shellQuote(name)+" 2>/dev/null | awk -F': *' '$1 == \"driver\" {print $2; exit}'"] = CommandResult{Stdout: driver + "\n"}
		r.outputs["cat /sys/class/net/"+shellQuote(name)+"/speed 2>/dev/null || true"] = CommandResult{Stdout: speed + "\n"}
		r.outputs["cat /sys/class/net/"+shellQuote(name)+"/carrier 2>/dev/null || true"] = CommandResult{Stdout: carrier + "\n"}
		r.outputs["cat /sys/class/net/"+shellQuote(name)+"/operstate 2>/dev/null || true"] = CommandResult{Stdout: state + "\n"}
		r.outputs["ip -o -4 addr show dev "+shellQuote(name)+" 2>/dev/null"] = CommandResult{Stdout: ip}
	}
	r.outputs["ls -1 /sys/class/net"] = CommandResult{Stdout: "ethmgmt0\nenp23s0f0\nenp23s0f1\ndocker0\nmlx0\n"}
	addIface("ethmgmt0", "e1000e", "1000", "1", "up", "80.5.21.122\n", true)
	addIface("enp23s0f0", "hinic3", "200000", "1", "up", "", true)
	addIface("enp23s0f1", "hinic3", "50000", "1", "up", "", true)
	addIface("mlx0", "mlx5_core", "200000", "1", "up", "", true)
	candidates, err := discoverCandidates(context.Background(), r, "80.5.21.122", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 2 || candidates[0].Name != "enp23s0f0" || candidates[1].Name != "enp23s0f1" {
		t.Fatalf("unexpected candidates: %+v", candidates)
	}
}

func TestParseHinicInfoMapsLinuxNICsToPorts(t *testing.T) {
	info := `Card num:1
Device Information:
     Card         PCIe Function
|----hinic0(CAL_2X200G_INTERNET)
         |--------0000:17:00.0(NIC:enp23s0f0)
         |--------0000:17:00.1(NIC:enp23s0f1)
|----hinic1(CAL_2X200G_INTERNET)
         |--------0000:18:00.0(NIC:eth2)`
	mapped := parseHinicInfo(info)
	if mapped["enp23s0f0"].Device != "hinic0" || mapped["enp23s0f0"].PortID != 0 {
		t.Fatalf("unexpected f0 mapping: %+v", mapped["enp23s0f0"])
	}
	if mapped["enp23s0f1"].Device != "hinic0" || mapped["enp23s0f1"].PortID != 1 {
		t.Fatalf("unexpected f1 mapping: %+v", mapped["enp23s0f1"])
	}
	if mapped["eth2"].Device != "hinic1" || mapped["eth2"].PortID != 0 {
		t.Fatalf("unexpected eth2 mapping: %+v", mapped["eth2"])
	}
}

func TestDiscoverCandidatesBringsDownHinicPortUpAndProbesHilink(t *testing.T) {
	dir := t.TempDir()
	r := &fakeRemote{outputs: map[string]CommandResult{}, errs: map[string]error{}}
	r.outputs["ls -1 /sys/class/net"] = CommandResult{Stdout: "enp23s0f1\n"}
	addCandidateInterface(r, "enp23s0f1", "hinic3", "200000", "1", "up", "")
	r.outputs["hinicadm3 info 2>/dev/null || true"] = CommandResult{Stdout: "Card num:1\n|----hinic0(CAL_2X200G_INTERNET)\n         |--------0000:17:00.0(NIC:enp23s0f1)\n"}
	r.outputs["ip link set dev 'enp23s0f1' up && sleep 2"] = CommandResult{}
	r.outputs["hinicadm3 hilink_port -i 'hinic0' -p 0 -s"] = CommandResult{Stdout: "link\npresent\nspeed = 200GE\n"}
	r.outputs["hinicadm3 hilink_port -i 'hinic0' -p 0"] = CommandResult{Stdout: "link_state = link\nrx_los = 0\nfec = RSFEC\n"}
	r.outputs["hinicadm3 hilink_count -i 'hinic0' -p 0"] = CommandResult{Stdout: "Pre-Ber:0.00\n"}
	candidates, err := discoverCandidates(context.Background(), r, "80.5.21.122", dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || candidates[0].ProbeStatus != "ready" || candidates[0].HinicDevice != "hinic0" || candidates[0].PortID != 0 {
		t.Fatalf("unexpected candidates: %+v", candidates)
	}
	if !containsRun(r.runs, "ip link set dev 'enp23s0f1' up && sleep 2") {
		t.Fatalf("interface was not brought up: %+v", r.runs)
	}
	if _, err := os.Stat(filepath.Join(dir, "enp23s0f1.hilink.txt")); err != nil {
		t.Fatal(err)
	}
}

func TestApplyTriesNextCandidate(t *testing.T) {
	dir := t.TempDir()
	r := &fakeRemote{outputs: map[string]CommandResult{}, errs: map[string]error{}}
	setupTwoCandidateRemote(r)
	r.outputs["'/usr/local/bin/storctl' check --json"] = CommandResult{Stdout: `{"checks":[]}`}
	failCmd := applyCommand(testConfig(dir), Host{Name: "node", IP: "80.5.21.122", User: "root", Password: "x"}, "enp23s0f0")
	okCmd := applyCommand(testConfig(dir), Host{Name: "node", IP: "80.5.21.122", User: "root", Password: "x"}, "enp23s0f1")
	r.outputs[failCmd] = CommandResult{Stdout: "FAIL rdma mount\n", ExitCode: 1}
	r.errs[failCmd] = fmt.Errorf("exit 1")
	r.outputs[okCmd] = CommandResult{Stdout: "OK mount /mnt/share proto=rdma\n"}
	app := &App{Dialer: fakeDialer{remotes: map[string]*fakeRemote{"node": r}}, Out: os.Stdout}
	results := app.Apply(context.Background(), HostsFile{Hosts: []Host{{Name: "node", IP: "80.5.21.122", User: "root", Password: "x"}}}, testConfig(dir), Options{ReportDir: dir, Concurrency: 1})
	if len(results) != 1 || results[0].Status != "OK" || results[0].SelectedNIC != "enp23s0f1" {
		t.Fatalf("unexpected results: %+v", results)
	}
	if _, err := os.Stat(filepath.Join(dir, "node", "attempts", "enp23s0f0.out")); err != nil {
		t.Fatal(err)
	}
}

func TestApplySkipsWhenAlreadyMounted(t *testing.T) {
	dir := t.TempDir()
	r := &fakeRemote{outputs: map[string]CommandResult{
		"'/usr/local/bin/storctl' check --json": {Stdout: `{
  "state": {"mounts": [{"MountPoint": "/mnt/share"}, {"MountPoint": "/mnt/weight"}]},
  "checks": [
    {"name": "mount:/mnt/share", "code": "mount_rdma", "message": "proto=rdma"},
    {"name": "mount:/mnt/weight", "code": "mount_rdma", "message": "proto=rdma"}
  ]
}`},
	}, errs: map[string]error{}}
	app := &App{Dialer: fakeDialer{remotes: map[string]*fakeRemote{"node": r}}, Out: os.Stdout}
	results := app.Apply(context.Background(), HostsFile{Hosts: []Host{{Name: "node", IP: "80.5.21.122", User: "root", Password: "x"}}}, testConfig(dir), Options{ReportDir: dir, Concurrency: 1})
	if len(results) != 1 || results[0].Status != "OK" || results[0].Message != "already mounted" {
		t.Fatalf("unexpected results: %+v", results)
	}
	if len(r.runs) != 1 || r.runs[0] != "'/usr/local/bin/storctl' check --json" {
		t.Fatalf("apply should only run check: %+v", r.runs)
	}
	if _, err := os.Stat(filepath.Join(dir, "node", "precheck.json")); err != nil {
		t.Fatal(err)
	}
}

func TestApplyDoesNotSkipPartialMount(t *testing.T) {
	dir := t.TempDir()
	r := &fakeRemote{outputs: map[string]CommandResult{}, errs: map[string]error{}}
	setupTwoCandidateRemote(r)
	r.outputs["'/usr/local/bin/storctl' check --json"] = CommandResult{Stdout: `{
  "state": {"mounts": [{"MountPoint": "/mnt/share"}, {"MountPoint": "/mnt/weight"}]},
  "checks": [
    {"name": "mount:/mnt/share", "code": "mount_rdma", "message": "proto=rdma"},
    {"name": "mount:/mnt/weight", "code": "mount_missing", "message": "missing"}
  ]
}`}
	okCmd := applyCommand(testConfig(dir), Host{Name: "node", IP: "80.5.21.122", User: "root", Password: "x"}, "enp23s0f0")
	r.outputs[okCmd] = CommandResult{Stdout: "OK mount /mnt/share proto=rdma\n"}
	app := &App{Dialer: fakeDialer{remotes: map[string]*fakeRemote{"node": r}}, Out: os.Stdout}
	results := app.Apply(context.Background(), HostsFile{Hosts: []Host{{Name: "node", IP: "80.5.21.122", User: "root", Password: "x"}}}, testConfig(dir), Options{ReportDir: dir, Concurrency: 1})
	if len(results) != 1 || results[0].Status != "OK" || results[0].SelectedNIC != "enp23s0f0" {
		t.Fatalf("unexpected results: %+v", results)
	}
}

func TestApplySkipsExistingTCPWhenFallbackAllowed(t *testing.T) {
	dir := t.TempDir()
	r := &fakeRemote{outputs: map[string]CommandResult{
		"'/usr/local/bin/storctl' check --json": {Stdout: `{
  "state": {"degraded": true, "mounts": [{"MountPoint": "/mnt/share"}]},
  "checks": [
    {"name": "mount:/mnt/share", "code": "mount_not_rdma", "message": "nfs4 rw,proto=tcp"}
  ]
}`},
	}, errs: map[string]error{}}
	app := &App{Dialer: fakeDialer{remotes: map[string]*fakeRemote{"node": r}}, Out: os.Stdout}
	results := app.Apply(context.Background(), HostsFile{Hosts: []Host{{Name: "node", IP: "80.5.21.122", User: "root", Password: "x"}}}, testConfig(dir), Options{ReportDir: dir, Concurrency: 1})
	if len(results) != 1 || results[0].Status != "OK" || !results[0].Degraded || results[0].Message != "already mounted degraded tcp-fallback" {
		t.Fatalf("unexpected results: %+v", results)
	}
}

func TestApplySkipsNoLinkCandidateAndUsesNextReadyPort(t *testing.T) {
	dir := t.TempDir()
	r := &fakeRemote{outputs: map[string]CommandResult{}, errs: map[string]error{}}
	setupTwoCandidateRemote(r)
	r.outputs["'/usr/local/bin/storctl' check --json"] = CommandResult{Stdout: `{"checks":[]}`}
	r.outputs["hinicadm3 info 2>/dev/null || true"] = CommandResult{Stdout: "Card num:1\n|----hinic0(CAL_2X200G_INTERNET)\n         |--------0000:17:00.0(NIC:enp23s0f0)\n         |--------0000:17:00.1(NIC:enp23s0f1)\n"}
	r.outputs["ip link set dev 'enp23s0f0' up && sleep 2"] = CommandResult{}
	r.outputs["ip link set dev 'enp23s0f1' up && sleep 2"] = CommandResult{}
	r.outputs["hinicadm3 hilink_port -i 'hinic0' -p 0 -s"] = CommandResult{Stdout: "no link\npresent\n"}
	r.outputs["hinicadm3 hilink_port -i 'hinic0' -p 0"] = CommandResult{Stdout: "link_state = no link\nrx_los = 1\n"}
	r.outputs["hinicadm3 hilink_count -i 'hinic0' -p 0"] = CommandResult{}
	r.outputs["hinicadm3 hilink_port -i 'hinic0' -p 1 -s"] = CommandResult{Stdout: "link\npresent\nspeed = 200GE\n"}
	r.outputs["hinicadm3 hilink_port -i 'hinic0' -p 1"] = CommandResult{Stdout: "link_state = link\nrx_los = 0\n"}
	r.outputs["hinicadm3 hilink_count -i 'hinic0' -p 1"] = CommandResult{}
	okCmd := applyCommand(testConfig(dir), Host{Name: "node", IP: "80.5.21.122", User: "root", Password: "x"}, "enp23s0f1")
	r.outputs[okCmd] = CommandResult{Stdout: "OK mount /mnt/share proto=rdma\n"}
	app := &App{Dialer: fakeDialer{remotes: map[string]*fakeRemote{"node": r}}, Out: os.Stdout}
	results := app.Apply(context.Background(), HostsFile{Hosts: []Host{{Name: "node", IP: "80.5.21.122", User: "root", Password: "x"}}}, testConfig(dir), Options{ReportDir: dir, Concurrency: 1})
	if len(results) != 1 || results[0].Status != "OK" || results[0].SelectedNIC != "enp23s0f1" {
		t.Fatalf("unexpected results: %+v", results)
	}
	if containsRun(r.runs, applyCommand(testConfig(dir), Host{Name: "node", IP: "80.5.21.122", User: "root", Password: "x"}, "enp23s0f0")) {
		t.Fatalf("blocked no-link nic should not be attempted: %+v", r.runs)
	}
	if len(results[0].Candidates) != 2 || results[0].Candidates[0].ProbeCode != "link_not_ready" {
		t.Fatalf("candidate probe summary missing: %+v", results[0].Candidates)
	}
}

func TestApplyReportsNoLinkReadyNIC(t *testing.T) {
	dir := t.TempDir()
	r := &fakeRemote{outputs: map[string]CommandResult{}, errs: map[string]error{}}
	setupTwoCandidateRemote(r)
	r.outputs["'/usr/local/bin/storctl' check --json"] = CommandResult{Stdout: `{"checks":[]}`}
	r.outputs["hinicadm3 info 2>/dev/null || true"] = CommandResult{Stdout: "Card num:1\n|----hinic0(CAL_2X200G_INTERNET)\n         |--------0000:17:00.0(NIC:enp23s0f0)\n         |--------0000:17:00.1(NIC:enp23s0f1)\n"}
	for i, name := range []string{"enp23s0f0", "enp23s0f1"} {
		port := fmt.Sprintf("%d", i)
		r.outputs["ip link set dev '"+name+"' up && sleep 2"] = CommandResult{}
		r.outputs["hinicadm3 hilink_port -i 'hinic0' -p "+port+" -s"] = CommandResult{Stdout: "absent\nno link\n"}
		r.outputs["hinicadm3 hilink_port -i 'hinic0' -p "+port] = CommandResult{Stdout: "gpio_insert = absent\nlink_state = no link\n"}
		r.outputs["hinicadm3 hilink_count -i 'hinic0' -p "+port] = CommandResult{}
	}
	app := &App{Dialer: fakeDialer{remotes: map[string]*fakeRemote{"node": r}}, Out: os.Stdout}
	results := app.Apply(context.Background(), HostsFile{Hosts: []Host{{Name: "node", IP: "80.5.21.122", User: "root", Password: "x"}}}, testConfig(dir), Options{ReportDir: dir, Concurrency: 1})
	if len(results) != 1 || results[0].Code != "no_link_ready_nic" {
		t.Fatalf("unexpected results: %+v", results)
	}
}

func TestApplyReportsNoCandidate(t *testing.T) {
	dir := t.TempDir()
	r := &fakeRemote{outputs: map[string]CommandResult{
		"'/usr/local/bin/storctl' check --json":                                      {Stdout: `{"checks":[]}`},
		"ls -1 /sys/class/net":                                                       {Stdout: "eth0\n"},
		"test -e /sys/class/net/'eth0'/device":                                       {},
		"ip -o -4 addr show dev 'eth0' 2>/dev/null | awk '{print $4}' | cut -d/ -f1": {Stdout: "80.5.21.122\n"},
	}, errs: map[string]error{}}
	app := &App{Dialer: fakeDialer{remotes: map[string]*fakeRemote{"node": r}}, Out: os.Stdout}
	results := app.Apply(context.Background(), HostsFile{Hosts: []Host{{Name: "node", IP: "80.5.21.122", User: "root", Password: "x"}}}, testConfig(dir), Options{ReportDir: dir, Concurrency: 1})
	if len(results) != 1 || results[0].Code != "no_candidate_nic" {
		t.Fatalf("unexpected results: %+v", results)
	}
}

func TestInstallDriverUsesFixed1823NICType(t *testing.T) {
	dir := t.TempDir()
	r := &fakeRemote{outputs: map[string]CommandResult{
		"'/usr/local/bin/storctl' install-driver --nic-type 1823 --artifact-dir '/root/storage_pkgs'": {Stdout: "OK driver\n"},
	}, errs: map[string]error{}}
	app := &App{Dialer: fakeDialer{remotes: map[string]*fakeRemote{"node": r}}, Out: os.Stdout}
	results := app.InstallDriver(context.Background(), HostsFile{Hosts: []Host{{Name: "node", IP: "80.5.21.122", User: "root", Password: "x"}}}, testConfig(dir), Options{ReportDir: dir, Concurrency: 1})
	if len(results) != 1 || results[0].Status != "OK" {
		t.Fatalf("unexpected results: %+v", results)
	}
	if len(r.runs) != 1 || !strings.Contains(r.runs[0], "--nic-type 1823") {
		t.Fatalf("install-driver command did not pin 1823: %+v", r.runs)
	}
}

func TestCopyUploadsStorctlProfileAndArtifacts(t *testing.T) {
	dir := t.TempDir()
	cfg := testConfig(dir)
	storctl := filepath.Join(dir, "storctl")
	profile := filepath.Join(dir, "profiles.json")
	artifact := filepath.Join(dir, "drivers", "storctl-artifacts.json")
	_ = os.WriteFile(storctl, []byte("storctl-bin"), 0755)
	_ = os.WriteFile(profile, []byte("{}"), 0644)
	_ = os.MkdirAll(filepath.Dir(artifact), 0755)
	_ = os.WriteFile(artifact, []byte("{}"), 0644)
	cfg.StorctlBin = storctl
	cfg.ProfileFile = profile
	cfg.ArtifactSrc = filepath.Join(dir, "drivers")
	r := &fakeRemote{}
	app := &App{Dialer: fakeDialer{remotes: map[string]*fakeRemote{"node": r}}, Out: os.Stdout}
	results := app.Copy(context.Background(), HostsFile{Hosts: []Host{{Name: "node", IP: "1.1.1.1", User: "root", Password: "x"}}}, cfg, Options{ReportDir: dir, Concurrency: 1})
	if len(results) != 1 || results[0].Status != "OK" {
		t.Fatalf("unexpected result: %+v", results)
	}
	if string(r.uploads["/usr/local/bin/storctl"]) != "storctl-bin" || string(r.uploads["/etc/storctl/profiles.json"]) != "{}" {
		t.Fatalf("uploads missing: %+v", r.uploads)
	}
}

func setupTwoCandidateRemote(r *fakeRemote) {
	r.outputs["ls -1 /sys/class/net"] = CommandResult{Stdout: "enp23s0f0\nenp23s0f1\n"}
	for _, name := range []string{"enp23s0f0", "enp23s0f1"} {
		addCandidateInterface(r, name, "hinic3", "200000", "1", "up", "")
	}
}

func addCandidateInterface(r *fakeRemote, name, driver, speed, carrier, state, ip string) {
	r.outputs["test -e /sys/class/net/"+shellQuote(name)+"/device"] = CommandResult{}
	r.outputs["ip -o -4 addr show dev "+shellQuote(name)+" 2>/dev/null | awk '{print $4}' | cut -d/ -f1"] = CommandResult{Stdout: ip}
	r.outputs["ethtool -i "+shellQuote(name)+" 2>/dev/null | awk -F': *' '$1 == \"driver\" {print $2; exit}'"] = CommandResult{Stdout: driver + "\n"}
	r.outputs["cat /sys/class/net/"+shellQuote(name)+"/speed 2>/dev/null || true"] = CommandResult{Stdout: speed + "\n"}
	r.outputs["cat /sys/class/net/"+shellQuote(name)+"/carrier 2>/dev/null || true"] = CommandResult{Stdout: carrier + "\n"}
	r.outputs["cat /sys/class/net/"+shellQuote(name)+"/operstate 2>/dev/null || true"] = CommandResult{Stdout: state + "\n"}
	r.outputs["ip -o -4 addr show dev "+shellQuote(name)+" 2>/dev/null"] = CommandResult{Stdout: ip}
}

func containsRun(runs []string, want string) bool {
	for _, run := range runs {
		if run == want {
			return true
		}
	}
	return false
}

func testConfig(reportDir string) Config {
	return Config{
		Profile:          "c4",
		RemoteBin:        "/usr/local/bin/storctl",
		RemoteProfile:    "/etc/storctl/profiles.json",
		RemoteArtifact:   "/root/storage_pkgs",
		AllowTCPFallback: true,
		QoS:              "off",
		ReportDir:        reportDir,
	}
}

func TestIgnoredInterface(t *testing.T) {
	for _, name := range []string{"lo", "docker0", "veth1", "virbr0", "br-storage", "bond0", "team0", "data0.172", "eth0.172"} {
		if !ignoredInterface(name) {
			t.Fatalf("%s should be ignored", name)
		}
	}
	if ignoredInterface("enp23s0f1") || strings.Contains("enp23s0f1", ".") {
		t.Fatal("physical nic ignored")
	}
}
