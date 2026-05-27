package compose

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type ReportSummary struct {
	Hosts             int `json:"hosts"`
	Success           int `json:"success"`
	Failures          int `json:"failures"`
	DegradedTCP       int `json:"degraded_tcp"`
	DriverNotReady    int `json:"driver_not_ready"`
	NoCandidateNIC    int `json:"no_candidate_nic"`
	NoLinkReadyNIC    int `json:"no_link_ready_nic"`
	OpticalAbsent     int `json:"optical_absent"`
	OpticalFault      int `json:"optical_fault"`
	LinkNotReady      int `json:"link_not_ready"`
	RDMAFailures      int `json:"rdma_failures"`
	RebootRequired    int `json:"reboot_required"`
	AllCandidatesFail int `json:"all_candidate_nics_failed"`
}

type ReportFailure struct {
	Host    string `json:"host"`
	Command string `json:"command"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ReportData struct {
	Summary  ReportSummary   `json:"summary"`
	Failures []ReportFailure `json:"failures"`
	Results  []HostResult    `json:"results"`
}

type ReportOptions struct {
	JSON    bool
	Verbose bool
}

func PrintReport(reportDir string, out io.Writer, opts ReportOptions) error {
	data, err := LoadReport(reportDir)
	if err != nil {
		return err
	}
	if opts.JSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(data)
	}
	if opts.Verbose {
		printVerboseReport(out, data.Summary)
		return nil
	}
	printDefaultReport(out, data)
	return nil
}

func LoadReport(reportDir string) (ReportData, error) {
	var report ReportData
	var summary ReportSummary
	err := filepath.WalkDir(reportDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Base(path) != "last.json" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var result HostResult
		if err := json.Unmarshal(data, &result); err != nil {
			return err
		}
		report.Results = append(report.Results, result)
		summary.Hosts++
		if result.Status == "OK" {
			summary.Success++
		} else {
			summary.Failures++
			report.Failures = append(report.Failures, ReportFailure{
				Host:    result.Host,
				Command: result.Command,
				Code:    result.Code,
				Message: trimReportMessage(result.Message),
			})
		}
		if result.Degraded || strings.Contains(result.Message, "tcp_fallback_degraded") {
			summary.DegradedTCP++
		}
		switch result.Code {
		case "driver_not_ready", "driver_install_failed":
			summary.DriverNotReady++
		case "no_candidate_nic":
			summary.NoCandidateNIC++
		case "no_link_ready_nic":
			summary.NoLinkReadyNIC++
		case "all_candidate_nics_failed":
			summary.AllCandidatesFail++
		}
		for _, candidate := range result.Candidates {
			switch candidate.ProbeCode {
			case "optical_absent":
				summary.OpticalAbsent++
			case "optical_fault":
				summary.OpticalFault++
			case "link_not_ready":
				summary.LinkNotReady++
			}
		}
		if strings.Contains(strings.ToLower(result.Message), "rdma") && result.Status != "OK" {
			summary.RDMAFailures++
		}
		if result.RebootRequired {
			summary.RebootRequired++
		}
		return nil
	})
	if err != nil {
		return report, err
	}
	report.Summary = summary
	return report, nil
}

func printDefaultReport(out io.Writer, report ReportData) {
	summary := report.Summary
	fmt.Fprintln(out, "hosts\tsuccess\tfail\tdegraded\tdriver_not_ready\tno_candidate\tno_link_ready\treboot_required")
	fmt.Fprintf(out, "%d\t%d\t%d\t%d\t%d\t%d\t%d\t%d\n",
		summary.Hosts, summary.Success, summary.Failures, summary.DegradedTCP,
		summary.DriverNotReady, summary.NoCandidateNIC, summary.NoLinkReadyNIC,
		summary.RebootRequired)
	if len(report.Failures) == 0 {
		return
	}
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "failures:")
	for _, failure := range report.Failures {
		fmt.Fprintf(out, "%s\t%s\t%s\t%s\n", failure.Host, failure.Command, failure.Code, failure.Message)
	}
}

func printVerboseReport(out io.Writer, summary ReportSummary) {
	fmt.Fprintln(out, "hosts\tsuccess\tfailures\tdegraded_tcp\tdriver_not_ready\tno_candidate_nic\tno_link_ready_nic\toptical_absent\toptical_fault\tlink_not_ready\trdma_failures\treboot_required\tall_candidate_nics_failed")
	fmt.Fprintf(out, "%d\t%d\t%d\t%d\t%d\t%d\t%d\t%d\t%d\t%d\t%d\t%d\t%d\n",
		summary.Hosts, summary.Success, summary.Failures, summary.DegradedTCP,
		summary.DriverNotReady, summary.NoCandidateNIC, summary.NoLinkReadyNIC,
		summary.OpticalAbsent, summary.OpticalFault, summary.LinkNotReady,
		summary.RDMAFailures, summary.RebootRequired, summary.AllCandidatesFail)
}

func trimReportMessage(message string) string {
	message = strings.TrimSpace(message)
	message = strings.ReplaceAll(message, "\t", " ")
	message = strings.Join(strings.Fields(message), " ")
	if len(message) > 180 {
		return message[:177] + "..."
	}
	return message
}

func HasFailures(results []HostResult) bool {
	for _, result := range results {
		if result.Status != "OK" {
			return true
		}
	}
	return false
}
