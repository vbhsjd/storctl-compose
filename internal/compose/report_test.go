package compose

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrintReportDefaultIsCompact(t *testing.T) {
	dir := writeReportFixtures(t)
	var out bytes.Buffer
	if err := PrintReport(dir, &out, ReportOptions{}); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "hosts\tsuccess\tfail\tdegraded\tdriver_not_ready\tno_candidate\tno_link_ready\treboot_required") {
		t.Fatalf("compact header missing:\n%s", got)
	}
	if strings.Contains(got, "optical_absent") || strings.Contains(got, "all_candidate_nics_failed") {
		t.Fatalf("compact report leaked verbose fields:\n%s", got)
	}
	if !strings.Contains(got, "failures:\nnode-b\tapply\tno_link_ready_nic\tno 1823 NIC has ready optical/module/link state") {
		t.Fatalf("failure list missing:\n%s", got)
	}
	if !strings.Contains(got, "successes:\nnode-a\tapply\tdegraded\t") {
		t.Fatalf("success list missing:\n%s", got)
	}
}

func TestPrintReportJSONIncludesFullSummary(t *testing.T) {
	dir := writeReportFixtures(t)
	var out bytes.Buffer
	if err := PrintReport(dir, &out, ReportOptions{JSON: true}); err != nil {
		t.Fatal(err)
	}
	var report ReportData
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.Summary.Hosts != 2 || report.Summary.NoLinkReadyNIC != 1 || len(report.Results) != 2 {
		t.Fatalf("unexpected report: %+v", report)
	}
}

func TestPrintReportVerboseKeepsDetailedColumns(t *testing.T) {
	dir := writeReportFixtures(t)
	var out bytes.Buffer
	if err := PrintReport(dir, &out, ReportOptions{Verbose: true}); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "optical_absent") || !strings.Contains(got, "all_candidate_nics_failed") {
		t.Fatalf("verbose columns missing:\n%s", got)
	}
}

func TestWriteReportCSVIncludesAllHosts(t *testing.T) {
	dir := writeReportFixtures(t)
	var out bytes.Buffer
	if err := WriteReportCSV(dir, &out); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "host,ip,command,status,code,message,selected_nic,degraded,reboot_required,candidate_count") {
		t.Fatalf("csv header missing:\n%s", got)
	}
	if !strings.Contains(got, "node-a,,apply,OK,degraded,") {
		t.Fatalf("success row missing:\n%s", got)
	}
	if !strings.Contains(got, "node-b,,apply,FAIL,no_link_ready_nic,no 1823 NIC has ready optical/module/link state") {
		t.Fatalf("failure row missing:\n%s", got)
	}
}

func writeReportFixtures(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeLast := func(host string, result HostResult) {
		t.Helper()
		hostDir := filepath.Join(dir, host)
		if err := os.MkdirAll(hostDir, 0755); err != nil {
			t.Fatal(err)
		}
		data, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(hostDir, "last.json"), data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	writeLast("node-a", HostResult{Host: "node-a", Command: "apply", Status: "OK", Degraded: true})
	writeLast("node-b", HostResult{
		Host:    "node-b",
		Command: "apply",
		Status:  "FAIL",
		Code:    "no_link_ready_nic",
		Message: "no 1823 NIC has ready optical/module/link state",
		Candidates: []CandidateNIC{
			{ProbeCode: "optical_absent"},
		},
	})
	return dir
}
