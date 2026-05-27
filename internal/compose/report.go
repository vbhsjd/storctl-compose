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
	sort.Slice(report.Results, func(i, j int) bool { return report.Results[i].Host < report.Results[j].Host })
	sort.Slice(report.Failures, func(i, j int) bool { return report.Failures[i].Host < report.Failures[j].Host })
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
	if len(report.Failures) > 0 {
		fmt.Fprintln(out, "")
		fmt.Fprintln(out, "failures:")
		for _, failure := range report.Failures {
			fmt.Fprintf(out, "%s\t%s\t%s\t%s\n", failure.Host, failure.Command, failure.Code, failure.Message)
		}
	}
	if summary.Success > 0 {
		fmt.Fprintln(out, "")
		fmt.Fprintln(out, "successes:")
		for _, result := range report.Results {
			if result.Status != "OK" {
				continue
			}
			code := "ok"
			if result.Degraded {
				code = "degraded"
			}
			fmt.Fprintf(out, "%s\t%s\t%s\t%s\n", result.Host, result.Command, code, successMessage(result))
		}
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
	return trimReportMessage(result.Message)
}

func WriteReportCSV(reportDir string, out io.Writer) error {
	report, err := LoadReport(reportDir)
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
			reportMessage(result),
			reportProtocol(result),
		}); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}

func WriteReportXLSX(reportDir string, out io.Writer) error {
	report, err := LoadReport(reportDir)
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
			reportMessage(result),
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
	if result.Status == "OK" && result.Code == "" {
		return "ok"
	}
	return result.Code
}

func reportMessage(result HostResult) string {
	if result.Status == "OK" {
		return successMessage(result)
	}
	return trimReportMessage(result.Message)
}

func reportProtocol(result HostResult) string {
	if result.Degraded || strings.Contains(strings.ToLower(result.Message), "tcp") {
		return "tcp"
	}
	return "rdma"
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
