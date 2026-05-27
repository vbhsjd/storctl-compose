package compose

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
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
	Summary             ReportSummary   `json:"summary"`
	Failures            []ReportFailure `json:"failures"`
	Results             []HostResult    `json:"results"`
	IgnoredStaleRecords int             `json:"ignored_stale_records,omitempty"`
}

type ReportOptions struct {
	JSON    bool
	Verbose bool
	Hosts   []Host
	All     bool
}

func PrintReport(reportDir string, out io.Writer, opts ReportOptions) error {
	data, err := LoadReportWithOptions(reportDir, opts)
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
	return LoadReportWithOptions(reportDir, ReportOptions{All: true})
}

func LoadReportWithOptions(reportDir string, opts ReportOptions) (ReportData, error) {
	var report ReportData
	allResults := []HostResult{}
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
		allResults = append(allResults, result)
		return nil
	})
	if err != nil {
		return report, err
	}
	report.Results, report.IgnoredStaleRecords = filterReportResults(allResults, opts)
	sort.Slice(report.Results, func(i, j int) bool { return report.Results[i].Host < report.Results[j].Host })
	report.Summary, report.Failures = summarizeReport(report.Results)
	return report, nil
}

func filterReportResults(all []HostResult, opts ReportOptions) ([]HostResult, int) {
	if opts.All || len(opts.Hosts) == 0 {
		return all, 0
	}
	byHost := map[string]int{}
	byIP := map[string]int{}
	for i, result := range all {
		if result.Host != "" {
			byHost[result.Host] = i
		}
		if result.IP != "" {
			byIP[result.IP] = i
		}
	}
	selected := []HostResult{}
	used := map[int]bool{}
	for _, host := range opts.Hosts {
		idx, ok := byHost[host.Name]
		if !ok {
			idx, ok = byIP[host.IP]
		}
		if ok {
			result := all[idx]
			if result.IP == "" {
				result.IP = host.IP
			}
			if result.Host == "" {
				result.Host = host.Name
			}
			selected = append(selected, result)
			used[idx] = true
			continue
		}
		selected = append(selected, HostResult{
			Host:    host.Name,
			IP:      host.IP,
			Command: "",
			Status:  "MISS",
			Code:    "not_run",
			Message: "not run",
		})
	}
	return selected, len(all) - len(used)
}

func summarizeReport(results []HostResult) (ReportSummary, []ReportFailure) {
	var summary ReportSummary
	failures := []ReportFailure{}
	for _, result := range results {
		summary.Hosts++
		if result.Status == "OK" {
			summary.Success++
		} else {
			summary.Failures++
			failures = append(failures, ReportFailure{
				Host:    result.Host,
				Command: result.Command,
				Code:    result.Code,
				Message: displayMessage(result),
			})
		}
		if reportProtocol(result) == "tcp" {
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
	}
	sort.Slice(failures, func(i, j int) bool { return failures[i].Host < failures[j].Host })
	return summary, failures
}

func printDefaultReport(out io.Writer, report ReportData) {
	summary := report.Summary
	rdma, tcp := protocolCounts(report.Results)
	fmt.Fprintf(out, "summary hosts=%d ok=%d fail=%d rdma=%d tcp=%d ignored_stale_records=%d\n\n",
		summary.Hosts, summary.Success, summary.Failures, rdma, tcp, report.IgnoredStaleRecords)
	fmt.Fprintln(out, "ip\tcommand\tstatus\tcode\tprotocol\tmessage")
	for _, result := range report.Results {
		fmt.Fprintf(out, "%s\t%s\t%s\t%s\t%s\t%s\n",
			result.IP, result.Command, result.Status, reportCode(result), reportProtocol(result), displayMessage(result))
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

func successMessage(result HostResult) string {
	if result.SelectedNIC != "" {
		return "selected-nic " + result.SelectedNIC
	}
	lower := strings.ToLower(result.Message)
	switch {
	case strings.Contains(lower, "already mounted"):
		return "already_mounted"
	case strings.Contains(lower, "driver installed"):
		return "driver_installed"
	case strings.Contains(lower, "checked"):
		return "checked"
	case strings.Contains(lower, "copied"):
		return "copied"
	}
	msg := trimReportMessage(result.Message)
	if msg == "" {
		return "ok"
	}
	return msg
}

func WriteReportCSV(reportDir string, out io.Writer, opts ReportOptions) error {
	report, err := LoadReportWithOptions(reportDir, opts)
	if err != nil {
		return err
	}
	w := csv.NewWriter(out)
	if err := w.Write([]string{
		"ip",
		"command",
		"status",
		"code",
		"message",
		"protocol",
	}); err != nil {
		return err
	}
	for _, result := range report.Results {
		if err := w.Write([]string{
			result.IP,
			result.Command,
			result.Status,
			reportCode(result),
			displayMessage(result),
			reportProtocol(result),
		}); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}

func WriteReportXLSX(reportDir string, out io.Writer, opts ReportOptions) error {
	report, err := LoadReportWithOptions(reportDir, opts)
	if err != nil {
		return err
	}
	rows := [][]string{{"ip", "command", "status", "code", "message", "protocol"}}
	for _, result := range report.Results {
		rows = append(rows, []string{
			result.IP,
			result.Command,
			result.Status,
			reportCode(result),
			displayMessage(result),
			reportProtocol(result),
		})
	}
	zw := zip.NewWriter(out)
	files := map[string]string{
		"[Content_Types].xml":        contentTypesXML,
		"_rels/.rels":                rootRelsXML,
		"xl/workbook.xml":            workbookXML,
		"xl/_rels/workbook.xml.rels": workbookRelsXML,
		"xl/styles.xml":              stylesXML,
		"xl/worksheets/sheet1.xml":   sheetXML(rows),
		"docProps/core.xml":          coreXML,
		"docProps/app.xml":           appXML,
	}
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			_ = zw.Close()
			return err
		}
		if _, err := io.WriteString(w, body); err != nil {
			_ = zw.Close()
			return err
		}
	}
	return zw.Close()
}

func reportCode(result HostResult) string {
	if result.Status == "MISS" && result.Code == "" {
		return "not_run"
	}
	if result.Status == "OK" && result.Code == "" {
		return "ok"
	}
	return result.Code
}

func displayMessage(result HostResult) string {
	if result.Status == "OK" {
		return successMessage(result)
	}
	switch reportCode(result) {
	case "auth_failed":
		return "ssh auth failed"
	case "ssh_timeout":
		return "ssh connect timed out"
	case "ssh_refused":
		return "ssh connection refused"
	case "ssh_unreachable":
		return "ssh network unreachable"
	case "connection_lost":
		return "connection lost"
	case "networkmanager_down":
		return "NetworkManager is not running"
	case "mount_failed":
		return "nfs mount failed"
	case "not_run":
		return "not run"
	}
	msg := trimReportMessage(result.Message)
	if msg == "" {
		return reportCode(result)
	}
	return msg
}

func reportProtocol(result HostResult) string {
	if result.Degraded || strings.Contains(strings.ToLower(result.Message), "tcp") {
		return "tcp"
	}
	return "rdma"
}

func protocolCounts(results []HostResult) (rdma int, tcp int) {
	for _, result := range results {
		switch reportProtocol(result) {
		case "tcp":
			tcp++
		default:
			rdma++
		}
	}
	return rdma, tcp
}

func sheetXML(rows [][]string) string {
	lastRow := len(rows)
	if lastRow == 0 {
		lastRow = 1
	}
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`)
	b.WriteString(`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">`)
	b.WriteString(`<sheetViews><sheetView workbookViewId="0"><pane ySplit="1" topLeftCell="A2" activePane="bottomLeft" state="frozen"/></sheetView></sheetViews>`)
	b.WriteString(`<cols><col min="1" max="1" width="18" customWidth="1"/><col min="2" max="4" width="16" customWidth="1"/><col min="5" max="5" width="80" customWidth="1"/><col min="6" max="6" width="12" customWidth="1"/></cols>`)
	b.WriteString(`<sheetData>`)
	for r, row := range rows {
		b.WriteString(fmt.Sprintf(`<row r="%d">`, r+1))
		for c, value := range row {
			cell := fmt.Sprintf("%s%d", columnName(c+1), r+1)
			style := ""
			if r == 0 {
				style = ` s="1"`
			}
			b.WriteString(fmt.Sprintf(`<c r="%s" t="inlineStr"%s><is><t>`, cell, style))
			writeEscaped(&b, value)
			b.WriteString(`</t></is></c>`)
		}
		b.WriteString(`</row>`)
	}
	b.WriteString(`</sheetData>`)
	b.WriteString(fmt.Sprintf(`<autoFilter ref="A1:F%d"/>`, lastRow))
	b.WriteString(`<dataValidations count="1"><dataValidation type="list" allowBlank="1" sqref="F2:F1048576"><formula1>"rdma,tcp"</formula1></dataValidation></dataValidations>`)
	b.WriteString(`</worksheet>`)
	return b.String()
}

func columnName(n int) string {
	name := ""
	for n > 0 {
		n--
		name = string(rune('A'+n%26)) + name
		n /= 26
	}
	return name
}

func writeEscaped(b *bytes.Buffer, value string) {
	for _, r := range value {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			b.WriteString("&quot;")
		default:
			b.WriteRune(r)
		}
	}
}

const contentTypesXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
<Default Extension="xml" ContentType="application/xml"/>
<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>
<Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>
<Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/>
<Override PartName="/docProps/core.xml" ContentType="application/vnd.openxmlformats-package.core-properties+xml"/>
<Override PartName="/docProps/app.xml" ContentType="application/vnd.openxmlformats-officedocument.extended-properties+xml"/>
</Types>`

const rootRelsXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>
<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties" Target="docProps/core.xml"/>
<Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/extended-properties" Target="docProps/app.xml"/>
</Relationships>`

const workbookXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
<sheets><sheet name="storctl-compose" sheetId="1" r:id="rId1"/></sheets>
</workbook>`

const workbookRelsXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/>
<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>
</Relationships>`

const stylesXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
<fonts count="2"><font><sz val="11"/><name val="Calibri"/></font><font><b/><sz val="11"/><name val="Calibri"/></font></fonts>
<fills count="1"><fill><patternFill patternType="none"/></fill></fills>
<borders count="1"><border><left/><right/><top/><bottom/><diagonal/></border></borders>
<cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs>
<cellXfs count="2"><xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0"/><xf numFmtId="0" fontId="1" fillId="0" borderId="0" xfId="0"/></cellXfs>
</styleSheet>`

const coreXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties" xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title>storctl-compose report</dc:title></cp:coreProperties>`

const appXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties"><Application>storctl-compose</Application></Properties>`

func HasFailures(results []HostResult) bool {
	for _, result := range results {
		if result.Status != "OK" {
			return true
		}
	}
	return false
}
