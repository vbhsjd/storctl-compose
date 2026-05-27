package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"storctl-compose/internal/compose"
)

var (
	version = "dev"
	commit  = "unknown"
	builtAt = "unknown"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		usage()
		return 0
	}
	switch args[0] {
	case "version":
		return runVersion(args[1:])
	case "report":
		fs := flag.NewFlagSet("report", flag.ContinueOnError)
		reportDir := fs.String("report-dir", "reports", "report directory")
		jsonOut := fs.Bool("json", false, "print full JSON summary")
		verbose := fs.Bool("verbose", false, "print detailed human summary")
		csvPath := fs.String("csv", "", "write all host results to CSV file; use - for stdout")
		xlsxPath := fs.String("xlsx", "", "write formatted Excel report")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		if *xlsxPath != "" {
			if err := writeXLSXReport(*reportDir, *xlsxPath); err != nil {
				fmt.Fprintf(os.Stderr, "FAIL report: %v\n", err)
				return 1
			}
			return 0
		}
		if *csvPath != "" {
			if err := writeCSVReport(*reportDir, *csvPath); err != nil {
				fmt.Fprintf(os.Stderr, "FAIL report: %v\n", err)
				return 1
			}
			return 0
		}
		if err := compose.PrintReport(*reportDir, os.Stdout, compose.ReportOptions{JSON: *jsonOut, Verbose: *verbose}); err != nil {
			fmt.Fprintf(os.Stderr, "FAIL report: %v\n", err)
			return 1
		}
		return 0
	case "copy", "install-driver", "apply", "check":
		return runWorkflow(args[0], args[1:])
	default:
		fmt.Fprintf(os.Stderr, "FAIL unknown command %s\n", args[0])
		usage()
		return 2
	}
}

func writeXLSXReport(reportDir, xlsxPath string) error {
	f, err := os.Create(xlsxPath)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := compose.WriteReportXLSX(reportDir, f); err != nil {
		return err
	}
	fmt.Printf("OK report xlsx %s\n", xlsxPath)
	return nil
}

func writeCSVReport(reportDir, csvPath string) error {
	if csvPath == "-" {
		return compose.WriteReportCSV(reportDir, os.Stdout)
	}
	f, err := os.Create(csvPath)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := compose.WriteReportCSV(reportDir, f); err != nil {
		return err
	}
	fmt.Printf("OK report csv %s\n", csvPath)
	return nil
}

func runWorkflow(command string, args []string) int {
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	var opts compose.Options
	fs.StringVar(&opts.HostsPath, "hosts", "hosts.csv", "hosts CSV or YAML")
	fs.StringVar(&opts.ConfigPath, "config", "compose.yaml", "compose YAML")
	fs.StringVar(&opts.Limit, "limit", "", "comma-separated host names or IPs")
	fs.IntVar(&opts.Concurrency, "concurrency", compose.DefaultConcurrency, "parallel hosts, max 50")
	fs.BoolVar(&opts.UpgradeFirmware, "upgrade-firmware", false, "install-driver only: upgrade firmware")
	timeoutRaw := fs.String("timeout", compose.DefaultTimeout.String(), "per-host timeout, for example 30s, 5m, 1h")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	timeout, err := parseTimeout(*timeoutRaw)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL args: %v\n", err)
		return 2
	}
	opts.Timeout = timeout
	hosts, cfg, err := compose.LoadInputs(opts.HostsPath, opts.ConfigPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL args: %v\n", err)
		return 2
	}
	opts = compose.NormalizeOptions(opts, cfg)
	app := compose.NewApp()
	var results []compose.HostResult
	ctx := context.Background()
	switch command {
	case "copy":
		results = app.Copy(ctx, hosts, cfg, opts)
	case "install-driver":
		results = app.InstallDriver(ctx, hosts, cfg, opts)
	case "apply":
		results = app.Apply(ctx, hosts, cfg, opts)
	case "check":
		results = app.Check(ctx, hosts, cfg, opts)
	}
	if compose.HasFailures(results) {
		return 1
	}
	return 0
}

func parseTimeout(raw string) (time.Duration, error) {
	timeout, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid --timeout %q: %w", raw, err)
	}
	if timeout <= 0 {
		return 0, fmt.Errorf("--timeout must be greater than 0")
	}
	return timeout, nil
}

func runVersion(args []string) int {
	fs := flag.NewFlagSet("version", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	info := compose.VersionInfo{Version: version, Commit: commit, BuiltAt: builtAt}
	if *jsonOut {
		_ = json.NewEncoder(os.Stdout).Encode(info)
		return 0
	}
	fmt.Printf("storctl-compose %s\ncommit %s\nbuilt_at %s\n", info.Version, info.Commit, info.BuiltAt)
	return 0
}

func usage() {
	fmt.Println(`storctl-compose - batch 1823 storage onboarding

usage:
  storctl-compose copy
  storctl-compose install-driver [--upgrade-firmware]
  storctl-compose apply
  storctl-compose check
  storctl-compose report [--json|--verbose|--csv result.csv|--xlsx result.xlsx]
  storctl-compose version [--json]

notes:
  - defaults: --hosts hosts.csv --config compose.yaml --report-dir reports
  - copy/install-driver/apply/check default to --timeout 30m per host
  - only 1823 is supported in storctl-compose
  - non-root SSH users require passwordless sudo
  - drivers stay in the external artifact_src directory`)
}
