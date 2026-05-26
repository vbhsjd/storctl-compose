package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

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
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		if err := compose.PrintReport(*reportDir, os.Stdout); err != nil {
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

func runWorkflow(command string, args []string) int {
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	var opts compose.Options
	fs.StringVar(&opts.HostsPath, "hosts", "hosts.yaml", "hosts YAML")
	fs.StringVar(&opts.ConfigPath, "config", "compose.yaml", "compose YAML")
	fs.StringVar(&opts.Limit, "limit", "", "comma-separated host names or IPs")
	fs.IntVar(&opts.Concurrency, "concurrency", compose.DefaultConcurrency, "parallel hosts, max 50")
	fs.BoolVar(&opts.UpgradeFirmware, "upgrade-firmware", false, "install-driver only: upgrade firmware")
	if err := fs.Parse(args); err != nil {
		return 2
	}
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
  storctl-compose copy --hosts hosts.yaml --config compose.yaml [--concurrency 30]
  storctl-compose install-driver --hosts hosts.yaml --config compose.yaml [--upgrade-firmware]
  storctl-compose apply --hosts hosts.yaml --config compose.yaml
  storctl-compose check --hosts hosts.yaml --config compose.yaml
  storctl-compose report --report-dir reports
  storctl-compose version [--json]

notes:
  - only 1823 is supported in storctl-compose v1
  - target hosts must allow root SSH login
  - drivers stay in the external artifact_src directory`)
}
