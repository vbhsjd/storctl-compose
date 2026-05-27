package compose

import (
	"encoding/json"
	"fmt"
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

func PrintReport(reportDir string, out *os.File) error {
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
		summary.Hosts++
		if result.Status == "OK" {
			summary.Success++
		} else {
			summary.Failures++
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
		return err
	}
	fmt.Fprintln(out, "hosts\tsuccess\tfailures\tdegraded_tcp\tdriver_not_ready\tno_candidate_nic\tno_link_ready_nic\toptical_absent\toptical_fault\tlink_not_ready\trdma_failures\treboot_required\tall_candidate_nics_failed")
	fmt.Fprintf(out, "%d\t%d\t%d\t%d\t%d\t%d\t%d\t%d\t%d\t%d\t%d\t%d\t%d\n",
		summary.Hosts, summary.Success, summary.Failures, summary.DegradedTCP,
		summary.DriverNotReady, summary.NoCandidateNIC, summary.NoLinkReadyNIC,
		summary.OpticalAbsent, summary.OpticalFault, summary.LinkNotReady,
		summary.RDMAFailures, summary.RebootRequired, summary.AllCandidatesFail)
	return nil
}

func HasFailures(results []HostResult) bool {
	for _, result := range results {
		if result.Status != "OK" {
			return true
		}
	}
	return false
}
