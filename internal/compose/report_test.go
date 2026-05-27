package compose

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
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
	if !strings.Contains(got, "summary hosts=2 ok=1 fail=1 rdma=1 tcp=1 ignored_stale_records=0") {
		t.Fatalf("compact header missing:\n%s", got)
	}
	if strings.Contains(got, "optical_absent") || strings.Contains(got, "all_candidate_nics_failed") {
		t.Fatalf("compact report leaked verbose fields:\n%s", got)
	}
	if !strings.Contains(got, "ip\tcommand\tstatus\tcode\tprotocol\tmessage") ||
		!strings.Contains(got, "\tapply\tFAIL\tno_link_ready_nic\trdma\tno 1823 NIC has ready optical/module/link state") ||
		!strings.Contains(got, "\tapply\tOK\tok\ttcp\t") {
		t.Fatalf("rows missing:\n%s", got)
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
	if err := WriteReportCSV(dir, &out, ReportOptions{}); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "ip,command,status,code,message,protocol") {
		t.Fatalf("csv header missing:\n%s", got)
	}
	if !strings.Contains(got, ",apply,OK,ok,ok,tcp") {
		t.Fatalf("success row missing:\n%s", got)
	}
	if !strings.Contains(got, ",apply,FAIL,no_link_ready_nic,no 1823 NIC has ready optical/module/link state,rdma") {
		t.Fatalf("failure row missing:\n%s", got)
	}
}

func TestWriteReportXLSXHasFilterWidthsAndProtocolDropdown(t *testing.T) {
	dir := writeReportFixtures(t)
	var out bytes.Buffer
	if err := WriteReportXLSX(dir, &out, ReportOptions{}); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(out.Bytes()), int64(out.Len()))
	if err != nil {
		t.Fatal(err)
	}
	data := ""
	for _, f := range zr.File {
		if f.Name != "xl/worksheets/sheet1.xml" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		data = string(body)
	}
	if !strings.Contains(data, "autoFilter") || !strings.Contains(data, "dataValidations") || !strings.Contains(data, "rdma,tcp") {
		t.Fatalf("xlsx missing filter or dropdown")
	}
}

func TestLoadReportFiltersCurrentHostsAndAddsMiss(t *testing.T) {
	dir := writeReportFixtures(t)
	report, err := LoadReportWithOptions(dir, ReportOptions{Hosts: []Host{
		{Name: "node-a", IP: "80.5.21.122"},
		{Name: "node-c", IP: "80.5.21.124"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if report.Summary.Hosts != 2 || report.IgnoredStaleRecords != 1 {
		t.Fatalf("unexpected summary: %+v", report)
	}
	if report.Results[1].Host != "node-c" || report.Results[1].Status != "MISS" || report.Results[1].Code != "not_run" {
		t.Fatalf("missing host not represented: %+v", report.Results)
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
