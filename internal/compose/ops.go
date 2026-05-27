package compose

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"storctl-compose/internal/assets"
)

type App struct {
	Dialer Dialer
	Out    *os.File
}

func NewApp() *App {
	return &App{Dialer: SSHDialer{}, Out: os.Stdout}
}

func (a *App) Copy(ctx context.Context, hosts HostsFile, cfg Config, opts Options) []HostResult {
	selected := FilterHosts(hosts.Hosts, opts.Limit)
	storctlBytes, err := loadStorctlBytes(cfg)
	if err != nil {
		results := failAll(selected, "copy", "storctl_asset", err.Error())
		for _, result := range results {
			a.printResult(result)
			_ = writeHostResult(NormalizeOptions(opts, cfg).ReportDir, result)
		}
		return results
	}
	return a.runHosts(ctx, selected, cfg, opts, "copy", func(ctx context.Context, host Host, remote Remote) HostResult {
		result := startResult(host, "copy")
		if err := remote.UploadBytes(ctx, cfg.RemoteBin, storctlBytes, 0755); err != nil {
			return finishFail(result, "copy_storctl_failed", err.Error())
		}
		if err := remote.UploadFile(ctx, cfg.ProfileFile, cfg.RemoteProfile, 0644); err != nil {
			return finishFail(result, "copy_profile_failed", err.Error())
		}
		if err := uploadDir(ctx, remote, cfg.ArtifactSrc, cfg.RemoteArtifact); err != nil {
			return finishFail(result, "copy_artifacts_failed", err.Error())
		}
		return finishOK(result, "copied")
	})
}

func (a *App) InstallDriver(ctx context.Context, hosts HostsFile, cfg Config, opts Options) []HostResult {
	return a.runHosts(ctx, hosts.Hosts, cfg, opts, "install-driver", func(ctx context.Context, host Host, remote Remote) HostResult {
		result := startResult(host, "install-driver")
		cmd := fmt.Sprintf("%s install-driver --nic-type 1823 --artifact-dir %s", shellQuote(cfg.RemoteBin), shellQuote(cfg.RemoteArtifact))
		if opts.UpgradeFirmware {
			cmd += " --upgrade-firmware"
		}
		out, err := remote.Run(ctx, cmd)
		result.Message = strings.TrimSpace(out.Stdout + out.Stderr)
		if strings.Contains(strings.ToLower(result.Message), "reboot") {
			result.RebootRequired = true
		}
		if err != nil {
			return finishFail(result, "driver_install_failed", trimMessage(result.Message))
		}
		return finishOK(result, "driver installed")
	})
}

func (a *App) Apply(ctx context.Context, hosts HostsFile, cfg Config, opts Options) []HostResult {
	return a.runHosts(ctx, hosts.Hosts, cfg, opts, "apply", func(ctx context.Context, host Host, remote Remote) HostResult {
		result := startResult(host, "apply")
		if ok, degraded, msg := precheckMounted(ctx, remote, cfg, opts.ReportDir, host.Name); ok {
			result.Degraded = degraded
			return finishOK(result, msg)
		}
		probeDir := filepath.Join(opts.ReportDir, host.Name, "nic-probe")
		candidates, err := discoverCandidates(ctx, remote, host.IP, probeDir)
		result.Candidates = candidates
		if err != nil {
			return finishFail(result, "discover_failed", err.Error())
		}
		if len(candidates) == 0 {
			return finishFail(result, "no_candidate_nic", "no physical 1823 NIC found by ethtool -i")
		}
		ready := readyCandidates(candidates)
		if len(ready) == 0 {
			return finishFail(result, "no_link_ready_nic", "no 1823 NIC has ready optical/module/link state")
		}
		attemptDir := filepath.Join(opts.ReportDir, host.Name, "attempts")
		_ = os.MkdirAll(attemptDir, 0755)
		last := ""
		for _, nic := range ready {
			cmd := applyCommand(cfg, host, nic.Name)
			out, err := remote.Run(ctx, cmd)
			_ = os.WriteFile(filepath.Join(attemptDir, nic.Name+".out"), []byte(out.Stdout), 0644)
			_ = os.WriteFile(filepath.Join(attemptDir, nic.Name+".err"), []byte(out.Stderr), 0644)
			last = trimMessage(out.Stdout + out.Stderr)
			if err == nil {
				result.SelectedNIC = nic.Name
				lower := strings.ToLower(last)
				result.Degraded = strings.Contains(lower, "degraded tcp-fallback") ||
					strings.Contains(lower, "tcp_fallback_degraded") ||
					strings.Contains(lower, "proto=tcp degraded")
				return finishOK(result, "selected "+nic.Name)
			}
		}
		return finishFail(result, "all_candidate_nics_failed", last)
	})
}

func (a *App) Check(ctx context.Context, hosts HostsFile, cfg Config, opts Options) []HostResult {
	return a.runHosts(ctx, hosts.Hosts, cfg, opts, "check", func(ctx context.Context, host Host, remote Remote) HostResult {
		result := startResult(host, "check")
		out, err := remote.Run(ctx, shellQuote(cfg.RemoteBin)+" check --json")
		if err := os.MkdirAll(opts.ReportDir, 0755); err != nil {
			return finishFail(result, "report_dir_failed", err.Error())
		}
		_ = os.WriteFile(filepath.Join(opts.ReportDir, host.Name+".json"), []byte(out.Stdout), 0644)
		result.Message = trimMessage(out.Stdout + out.Stderr)
		if strings.Contains(result.Message, "tcp_fallback_degraded") {
			result.Degraded = true
		}
		if err != nil {
			return finishFail(result, "check_failed", result.Message)
		}
		return finishOK(result, "checked")
	})
}

func (a *App) runHosts(ctx context.Context, hosts []Host, cfg Config, opts Options, command string, fn func(context.Context, Host, Remote) HostResult) []HostResult {
	hosts = FilterHosts(hosts, opts.Limit)
	opts = NormalizeOptions(opts, cfg)
	if len(hosts) == 0 {
		return nil
	}
	_ = os.MkdirAll(opts.ReportDir, 0755)
	jobs := make(chan Host)
	results := make(chan HostResult)
	workers := opts.Concurrency
	if workers > len(hosts) {
		workers = len(hosts)
	}
	for i := 0; i < workers; i++ {
		go func() {
			for host := range jobs {
				result := startResult(host, command)
				remote, err := a.Dialer.Dial(ctx, host)
				if err != nil {
					results <- finishFail(result, "ssh_failed", err.Error())
					continue
				}
				result = fn(ctx, host, remote)
				_ = remote.Close()
				results <- result
			}
		}()
	}
	go func() {
		for _, host := range hosts {
			jobs <- host
		}
		close(jobs)
	}()
	out := make([]HostResult, 0, len(hosts))
	for range hosts {
		result := <-results
		out = append(out, result)
		a.printResult(result)
		_ = writeHostResult(opts.ReportDir, result)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Host < out[j].Host })
	return out
}

func (a *App) printResult(result HostResult) {
	prefix := "OK"
	if result.Status != "OK" {
		prefix = "FAIL"
	}
	msg := result.Message
	if result.SelectedNIC != "" {
		msg = "selected-nic " + result.SelectedNIC
	}
	if result.RebootRequired {
		msg += " REBOOT_REQUIRED"
	}
	fmt.Fprintf(a.Out, "%s %s %s %s\n", prefix, result.Host, result.Command, msg)
}

func loadStorctlBytes(cfg Config) ([]byte, error) {
	if assets.HasEmbeddedStorctl() {
		return assets.StorctlLinuxARM64, nil
	}
	if cfg.StorctlBin == "" {
		return nil, fmt.Errorf("embedded storctl is not present; use a release binary or set compose.yaml storctl_bin")
	}
	return os.ReadFile(cfg.StorctlBin)
}

func uploadDir(ctx context.Context, remote Remote, localDir, remoteDir string) error {
	return filepath.WalkDir(localDir, func(localPath string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(localDir, localPath)
		if err != nil {
			return err
		}
		if rel == "." {
			return remote.MkdirAll(ctx, remoteDir, 0755)
		}
		remotePath := path.Join(remoteDir, filepath.ToSlash(rel))
		if d.IsDir() {
			return remote.MkdirAll(ctx, remotePath, 0755)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return remote.UploadFile(ctx, localPath, remotePath, info.Mode().Perm())
	})
}

func applyCommand(cfg Config, host Host, nic string) string {
	parts := []string{
		shellQuote(cfg.RemoteBin), "apply",
		"--profile-file", shellQuote(cfg.RemoteProfile),
		"--profile", shellQuote(cfg.Profile),
		"--nic", shellQuote(nic),
		"--nic-type", "1823",
		"--mgmt-ip", shellQuote(host.IP),
		"--qos", shellQuote(cfg.QoS),
	}
	if cfg.AllowTCPFallback {
		parts = append(parts, "--allow-tcp-fallback")
	}
	return strings.Join(parts, " ")
}

type storctlCheckReport struct {
	State *struct {
		Mounts []struct {
			MountPoint string
		}
		Degraded bool `json:"degraded"`
	} `json:"state"`
	Checks []struct {
		Name    string `json:"name"`
		Status  string `json:"status"`
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"checks"`
}

func precheckMounted(ctx context.Context, remote Remote, cfg Config, reportDir, hostName string) (bool, bool, string) {
	out, err := remote.Run(ctx, shellQuote(cfg.RemoteBin)+" check --json")
	if reportDir != "" && hostName != "" {
		dir := filepath.Join(reportDir, hostName)
		_ = os.MkdirAll(dir, 0755)
		_ = os.WriteFile(filepath.Join(dir, "precheck.json"), []byte(out.Stdout), 0644)
	}
	if err != nil || strings.TrimSpace(out.Stdout) == "" {
		return false, false, ""
	}
	var report storctlCheckReport
	if err := json.Unmarshal([]byte(out.Stdout), &report); err != nil {
		return false, false, ""
	}
	return checkReportAlreadyMounted(report, cfg.AllowTCPFallback)
}

func checkReportAlreadyMounted(report storctlCheckReport, allowTCPFallback bool) (bool, bool, string) {
	if report.State == nil || len(report.State.Mounts) == 0 {
		return false, false, ""
	}
	checkByMount := map[string]struct {
		Code    string
		Message string
	}{}
	for _, check := range report.Checks {
		target, ok := strings.CutPrefix(check.Name, "mount:")
		if !ok {
			continue
		}
		checkByMount[target] = struct {
			Code    string
			Message string
		}{Code: check.Code, Message: check.Message}
	}
	degraded := report.State.Degraded
	for _, mount := range report.State.Mounts {
		if mount.MountPoint == "" {
			return false, false, ""
		}
		check, ok := checkByMount[mount.MountPoint]
		if !ok {
			return false, false, ""
		}
		if check.Code == "mount_rdma" {
			continue
		}
		if allowTCPFallback && check.Code == "mount_not_rdma" && strings.Contains(strings.ToLower(check.Message), "nfs") {
			degraded = true
			continue
		}
		return false, false, ""
	}
	if degraded {
		return true, true, "already mounted degraded tcp-fallback"
	}
	return true, false, "already mounted"
}

func discoverCandidates(ctx context.Context, remote Remote, mgmtIP, probeDir string) ([]CandidateNIC, error) {
	out, err := remote.Run(ctx, "ls -1 /sys/class/net")
	if err != nil {
		return nil, err
	}
	hinicMap := parseHinicInfo(remoteStdout(ctx, remote, "hinicadm3 info 2>/dev/null || true"))
	var candidates []CandidateNIC
	for _, nic := range strings.Fields(out.Stdout) {
		if ignoredInterface(nic) {
			continue
		}
		if ok, _ := remoteBool(ctx, remote, "test -e /sys/class/net/"+shellQuote(nic)+"/device"); !ok {
			continue
		}
		if hasMgmtIP(ctx, remote, nic, mgmtIP) {
			continue
		}
		driver := strings.TrimSpace(remoteStdout(ctx, remote, "ethtool -i "+shellQuote(nic)+" 2>/dev/null | awk -F': *' '$1 == \"driver\" {print $2; exit}'"))
		if !strings.HasPrefix(driver, "hinic3") && !strings.HasPrefix(driver, "hinic") {
			continue
		}
		_, _ = remote.Run(ctx, "ip link set dev "+shellQuote(nic)+" up && sleep 2")
		speed, _ := strconv.Atoi(strings.TrimSpace(remoteStdout(ctx, remote, "cat /sys/class/net/"+shellQuote(nic)+"/speed 2>/dev/null || true")))
		carrier := strings.TrimSpace(remoteStdout(ctx, remote, "cat /sys/class/net/"+shellQuote(nic)+"/carrier 2>/dev/null || true")) == "1"
		up := strings.TrimSpace(remoteStdout(ctx, remote, "cat /sys/class/net/"+shellQuote(nic)+"/operstate 2>/dev/null || true")) == "up"
		hasIPv4 := strings.TrimSpace(remoteStdout(ctx, remote, "ip -o -4 addr show dev "+shellQuote(nic)+" 2>/dev/null")) != ""
		score := 0
		if carrier {
			score += 1000
		}
		if speed >= 100000 {
			score += 100
		}
		if !hasIPv4 {
			score += 10
		}
		if up {
			score++
		}
		candidate := CandidateNIC{Name: nic, Driver: driver, Speed: speed, Carrier: carrier, HasIPv4: hasIPv4, Up: up, Score: score, PortID: -1}
		if port, ok := hinicMap[nic]; ok {
			candidate.HinicDevice = port.Device
			candidate.PortID = port.PortID
			candidate = probeHilink(ctx, remote, probeDir, candidate)
		} else {
			candidate.ProbeStatus = "unknown"
			candidate.ProbeMessage = "hinicadm3 info did not map this Linux NIC; falling back to legacy candidate selection"
			_ = writeCandidateProbe(probeDir, candidate, "", "", "")
		}
		candidates = append(candidates, candidate)
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Score == candidates[j].Score {
			return candidates[i].Name < candidates[j].Name
		}
		return candidates[i].Score > candidates[j].Score
	})
	return candidates, nil
}

type hinicPort struct {
	Device string
	PortID int
}

func parseHinicInfo(info string) map[string]hinicPort {
	out := map[string]hinicPort{}
	device := ""
	portID := 0
	devRe := regexp.MustCompile(`\|----(hinic[0-9]+)`)
	nicRe := regexp.MustCompile(`NIC:([^)]+)`)
	for _, line := range strings.Split(info, "\n") {
		if m := devRe.FindStringSubmatch(line); len(m) == 2 {
			device = m[1]
			portID = 0
			continue
		}
		if device == "" {
			continue
		}
		if m := nicRe.FindStringSubmatch(line); len(m) == 2 {
			out[strings.TrimSpace(m[1])] = hinicPort{Device: device, PortID: portID}
			portID++
		}
	}
	return out
}

func probeHilink(ctx context.Context, remote Remote, probeDir string, candidate CandidateNIC) CandidateNIC {
	attempts := []string{candidate.HinicDevice}
	if candidate.Name != "" && candidate.Name != candidate.HinicDevice {
		attempts = append(attempts, candidate.Name)
	}
	var simple, full, count CommandResult
	var simpleErr, fullErr error
	for _, device := range attempts {
		candidate.ProbeDevice = device
		simple, full, count, simpleErr, fullErr = runHilinkProbe(ctx, remote, device, candidate.PortID)
		if simpleErr != nil || fullErr != nil {
			continue
		}
		text := simple.Stdout + "\n" + full.Stdout
		code, message := classifyHilink(text, candidate.Speed, candidate.Carrier)
		if code == "" {
			candidate.ProbeStatus = "ready"
			candidate.ProbeCode = ""
			candidate.ProbeMessage = "hilink ready"
			_ = writeCandidateProbe(probeDir, candidate, simple.Stdout+simple.Stderr, full.Stdout+full.Stderr, count.Stdout+count.Stderr)
			return candidate
		}
		candidate.ProbeStatus = "blocked"
		candidate.ProbeCode = code
		candidate.ProbeMessage = message
		_ = writeCandidateProbe(probeDir, candidate, simple.Stdout+simple.Stderr, full.Stdout+full.Stderr, count.Stdout+count.Stderr)
		return candidate
	}
	if simpleErr != nil || fullErr != nil {
		candidate.ProbeStatus = "failed"
		candidate.ProbeCode = "hilink_probe_failed"
		candidate.ProbeMessage = trimMessage(simple.Stdout + simple.Stderr + full.Stdout + full.Stderr)
		_ = writeCandidateProbe(probeDir, candidate, simple.Stdout+simple.Stderr, full.Stdout+full.Stderr, count.Stdout+count.Stderr)
		return candidate
	}
	return candidate
}

func runHilinkProbe(ctx context.Context, remote Remote, device string, portID int) (CommandResult, CommandResult, CommandResult, error, error) {
	base := "hinicadm3 hilink_port -i " + shellQuote(device) + " -p " + strconv.Itoa(portID)
	simple, simpleErr := remote.Run(ctx, base+" -s")
	full, fullErr := remote.Run(ctx, base)
	count, _ := remote.Run(ctx, "hinicadm3 hilink_count -i "+shellQuote(device)+" -p "+strconv.Itoa(portID))
	return simple, full, count, simpleErr, fullErr
}

func classifyHilink(text string, speed int, carrier bool) (string, string) {
	lower := strings.ToLower(text)
	switch {
	case regexp.MustCompile(`(?m)(^|\s|=)absent(\s|$)`).MatchString(lower):
		return "optical_absent", "optical module is absent"
	case regexp.MustCompile(`(?m)(^|\s|=)fault(\s|$)`).MatchString(lower):
		return "optical_fault", "optical module reports fault"
	case regexp.MustCompile(`rx_los\s*=\s*[1-9]`).MatchString(lower):
		return "link_not_ready", "rx_los is asserted"
	case regexp.MustCompile(`(?m)(^|\s|=)no link(\s|$)`).MatchString(lower):
		return "link_not_ready", "hilink reports no link"
	case speed > 0 && speed < 100000:
		return "speed_unexpected", fmt.Sprintf("link speed is %dMb/s, expected >=100000Mb/s", speed)
	case !carrier:
		return "link_not_ready", "kernel carrier is down"
	}
	if hilinkSpeed := parseHilinkSpeed(lower); hilinkSpeed > 0 && hilinkSpeed < 100 {
		return "speed_unexpected", fmt.Sprintf("hilink speed is %dG, expected >=100G", hilinkSpeed)
	}
	return "", ""
}

func parseHilinkSpeed(text string) int {
	re := regexp.MustCompile(`(?m)^speed\s*=\s*([0-9]+)`)
	m := re.FindStringSubmatch(text)
	if len(m) != 2 {
		return 0
	}
	v, _ := strconv.Atoi(m[1])
	return v
}

func readyCandidates(candidates []CandidateNIC) []CandidateNIC {
	var ready []CandidateNIC
	for _, candidate := range candidates {
		if candidate.ProbeStatus == "ready" || candidate.ProbeStatus == "unknown" {
			ready = append(ready, candidate)
		}
	}
	return ready
}

func writeCandidateProbe(probeDir string, candidate CandidateNIC, simple, full, count string) error {
	if probeDir == "" {
		return nil
	}
	if err := os.MkdirAll(probeDir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(candidate, "", "  ")
	if err != nil {
		return err
	}
	base := filepath.Join(probeDir, candidate.Name)
	if err := os.WriteFile(base+".json", append(data, '\n'), 0644); err != nil {
		return err
	}
	_ = os.WriteFile(base+".hilink-simple.txt", []byte(simple), 0644)
	_ = os.WriteFile(base+".hilink.txt", []byte(full), 0644)
	_ = os.WriteFile(base+".hilink-count.txt", []byte(count), 0644)
	return nil
}

func remoteStdout(ctx context.Context, remote Remote, command string) string {
	out, _ := remote.Run(ctx, command)
	return out.Stdout
}

func remoteBool(ctx context.Context, remote Remote, command string) (bool, error) {
	out, err := remote.Run(ctx, command)
	_ = out
	return err == nil, err
}

func hasMgmtIP(ctx context.Context, remote Remote, nic, mgmtIP string) bool {
	cmd := "ip -o -4 addr show dev " + shellQuote(nic) + " 2>/dev/null | awk '{print $4}' | cut -d/ -f1"
	for _, ip := range strings.Fields(remoteStdout(ctx, remote, cmd)) {
		if ip == mgmtIP {
			return true
		}
	}
	return false
}

func ignoredInterface(nic string) bool {
	if strings.Contains(nic, ".") {
		return true
	}
	prefixes := []string{"lo", "docker", "veth", "virbr", "br", "bond", "team", "tun", "tap", "kube", "cni", "flannel", "cali", "data0"}
	for _, prefix := range prefixes {
		if strings.HasPrefix(nic, prefix) {
			return true
		}
	}
	return false
}

func startResult(host Host, command string) HostResult {
	return HostResult{Host: host.Name, IP: host.IP, Command: command, Status: "FAIL", StartedAt: nowString()}
}

func finishOK(result HostResult, msg string) HostResult {
	result.Status = "OK"
	result.Code = ""
	result.Message = msg
	result.FinishedAt = nowString()
	return result
}

func finishFail(result HostResult, code, msg string) HostResult {
	result.Status = "FAIL"
	result.Code = code
	result.Message = trimMessage(msg)
	result.FinishedAt = nowString()
	return result
}

func trimMessage(msg string) string {
	msg = strings.TrimSpace(msg)
	if len(msg) > 600 {
		return msg[:600] + "..."
	}
	return msg
}

func writeHostResult(reportDir string, result HostResult) error {
	dir := filepath.Join(reportDir, result.Host)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "last.json"), append(data, '\n'), 0644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(reportDir, result.Host+".result.json"), append(data, '\n'), 0644)
}

func failAll(hosts []Host, command, code, msg string) []HostResult {
	out := make([]HostResult, 0, len(hosts))
	for _, host := range hosts {
		out = append(out, finishFail(startResult(host, command), code, msg))
	}
	return out
}
