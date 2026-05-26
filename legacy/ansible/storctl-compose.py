#!/usr/bin/env python3
"""Small wrapper for the storctl-compose Ansible workflow."""

from __future__ import annotations

import argparse
import json
import os
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path

try:
    import yaml
except ImportError:  # pragma: no cover - exercised on user machines.
    yaml = None


ROOT = Path(__file__).resolve().parent
PLAYBOOKS = {
    "copy": ROOT / "playbooks" / "10_copy_bundle.yml",
    "install-driver": ROOT / "playbooks" / "20_install_driver.yml",
    "apply": ROOT / "playbooks" / "40_apply.yml",
    "check": ROOT / "playbooks" / "50_check.yml",
}


def load_yaml(path: Path) -> dict:
    if not path.exists():
        raise SystemExit(f"FAIL missing {path}")
    text = path.read_text(encoding="utf-8")
    data = yaml.safe_load(text) if yaml else parse_simple_yaml(text)
    data = data or {}
    if not isinstance(data, dict):
        raise SystemExit(f"FAIL {path} must be a YAML object")
    return data


def parse_simple_yaml(text: str) -> dict:
    """Parse the small hosts.yaml/compose.yaml subset used by this project."""
    lines = []
    for raw in text.splitlines():
        line = raw.split("#", 1)[0].rstrip()
        if line.strip():
            lines.append(line)
    if not lines:
        return {}
    if lines[0].strip() == "hosts:":
        hosts = []
        current = None
        for line in lines[1:]:
            stripped = line.strip()
            if stripped.startswith("- "):
                current = {}
                hosts.append(current)
                rest = stripped[2:].strip()
                if rest:
                    key, value = split_yaml_pair(rest)
                    current[key] = parse_scalar(value)
                continue
            if current is None:
                raise SystemExit("FAIL invalid hosts.yaml")
            key, value = split_yaml_pair(stripped)
            current[key] = parse_scalar(value)
        return {"hosts": hosts}
    out = {}
    for line in lines:
        key, value = split_yaml_pair(line.strip())
        out[key] = parse_scalar(value)
    return out


def split_yaml_pair(line: str) -> tuple[str, str]:
    if ":" not in line:
        raise SystemExit(f"FAIL invalid YAML line: {line}")
    key, value = line.split(":", 1)
    return key.strip(), value.strip()


def parse_scalar(value: str):
    value = value.strip()
    if len(value) >= 2 and value[0] == value[-1] and value[0] in "'\"":
        return value[1:-1]
    if value.lower() == "true":
        return True
    if value.lower() == "false":
        return False
    if value.lower() in {"null", "none", ""}:
        return None
    try:
        return int(value)
    except ValueError:
        return value


def require_tool(name: str) -> None:
    if shutil.which(name) is None:
        raise SystemExit(f"FAIL dependency: {name} not found")


def build_inventory(hosts_doc: dict, cfg: dict, path: Path) -> None:
    hosts = hosts_doc.get("hosts")
    if not isinstance(hosts, list) or not hosts:
        raise SystemExit("FAIL hosts.yaml must contain a non-empty hosts list")
    storage_hosts = {}
    password_used = False
    for item in hosts:
        if not isinstance(item, dict):
            raise SystemExit("FAIL each host entry must be an object")
        name = str(item.get("name") or item.get("ip") or "").strip()
        ip = str(item.get("ip") or "").strip()
        user = str(item.get("user") or "").strip()
        password = item.get("password")
        if not name or not ip or not user:
            raise SystemExit("FAIL each host requires name, ip, and user")
        host_vars = {
            "ansible_host": ip,
            "ansible_user": user,
            "ansible_ssh_common_args": "-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null",
        }
        if password is not None:
            host_vars["ansible_password"] = str(password)
            password_used = True
        if "port" in item:
            host_vars["ansible_port"] = int(item["port"])
        storage_hosts[name] = host_vars
    if password_used:
        require_tool("sshpass")
    inventory = {
        "all": {
            "children": {
                "storage": {
                    "hosts": storage_hosts,
                    "vars": compose_vars(cfg),
                }
            }
        }
    }
    with path.open("w", encoding="utf-8") as fh:
        if yaml:
            yaml.safe_dump(inventory, fh, sort_keys=False, allow_unicode=True)
        else:
            json.dump(inventory, fh, indent=2)


def compose_vars(cfg: dict) -> dict:
    required = ["profile", "storctl_bin", "profile_file", "artifact_src"]
    missing = [key for key in required if not cfg.get(key)]
    if missing:
        raise SystemExit("FAIL compose.yaml missing: " + ", ".join(missing))
    nic_type = str(cfg.get("nic_type", "1823"))
    if nic_type != "1823":
        raise SystemExit("FAIL storctl-compose only supports nic_type: 1823")
    return {
        "storage_profile": cfg["profile"],
        "nic_type": "1823",
        "storctl_local_bin": cfg["storctl_bin"],
        "storctl_profile_src": cfg["profile_file"],
        "storctl_artifact_src": cfg["artifact_src"],
        "storctl_remote_bin": cfg.get("remote_bin", "/usr/local/bin/storctl"),
        "storctl_profile_file": cfg.get("remote_profile_file", "/etc/storctl/profiles.json"),
        "storctl_artifact_dir": cfg.get("remote_artifact_dir", "/root/storage_pkgs"),
        "storctl_qos": cfg.get("qos", "off"),
        "storctl_allow_tcp_fallback": bool(cfg.get("allow_tcp_fallback", True)),
        "storctl_report_dir": cfg.get("report_dir", "./reports"),
        "storctl_auto_log_dir": cfg.get("auto_log_dir", "/var/lib/storctl-compose/apply"),
    }


def run_playbook(command: str, args: argparse.Namespace) -> int:
    require_tool("ansible-playbook")
    hosts_doc = load_yaml(Path(args.hosts))
    cfg = load_yaml(Path(args.config))
    with tempfile.TemporaryDirectory(prefix="storctl-compose-") as tmp:
        tmp_path = Path(tmp)
        inventory = tmp_path / "inventory.yml"
        extra_vars = tmp_path / "extra-vars.json"
        build_inventory(hosts_doc, cfg, inventory)
        values = {
            "storctl_upgrade_firmware": bool(args.upgrade_firmware),
        }
        with extra_vars.open("w", encoding="utf-8") as fh:
            json.dump(values, fh)
        cmd = ["ansible-playbook", "-i", str(inventory), str(PLAYBOOKS[command]), "-e", f"@{extra_vars}"]
        if args.limit:
            cmd.extend(["--limit", args.limit])
        extra = list(args.extra or [])
        if extra and extra[0] == "--":
            extra = extra[1:]
        if extra:
            cmd.extend(extra)
        return subprocess.run(cmd, cwd=ROOT).returncode


def run_report(args: argparse.Namespace) -> int:
    script = ROOT / "scripts" / "collect-report.sh"
    return subprocess.run([str(script), args.report_dir], cwd=ROOT).returncode


def main(argv: list[str]) -> int:
    parser = argparse.ArgumentParser(prog="storctl-compose")
    sub = parser.add_subparsers(dest="command", required=True)
    for name in ["copy", "install-driver", "apply", "check"]:
        p = sub.add_parser(name)
        p.add_argument("--hosts", default="hosts.yaml")
        p.add_argument("--config", default="compose.yaml")
        p.add_argument("--limit", default="")
        p.add_argument("--upgrade-firmware", action="store_true")
        p.add_argument("extra", nargs=argparse.REMAINDER)
    report = sub.add_parser("report")
    report.add_argument("--report-dir", default="reports")
    args = parser.parse_args(argv)
    if args.command == "report":
        return run_report(args)
    return run_playbook(args.command, args)


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
