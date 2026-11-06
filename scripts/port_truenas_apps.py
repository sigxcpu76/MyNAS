#!/usr/bin/env python3
"""
Port TrueNAS SCALE Applications to MyNAS / nasctl Docker Compose Units.

Discovers materialized Docker Compose stacks from TrueNAS SCALE's .ix-apps dataset,
extracts app metadata (icons, version, web portals), normalizes volume paths, and ports
them into the MyNAS Docker unit repository or deploys them via the `nasctl` CLI.

Example Usage:
  # Dry-run inspection:
  python3 scripts/port_truenas_apps.py --dry-run

  # Port to MyNAS state directory (auto-detected or specified):
  python3 scripts/port_truenas_apps.py --source /mnt/.ix-apps

  # Export materialized compose files to a custom directory:
  python3 scripts/port_truenas_apps.py --source /mnt/.ix-apps --output-dir ./imported-stacks

  # Port and immediately deploy running stacks via nasctl CLI:
  python3 scripts/port_truenas_apps.py --source /mnt/.ix-apps --deploy
"""

import argparse
import json
import os
import re
import shutil
import subprocess
import sys
from pathlib import Path
from typing import Any, Dict, List, Optional, Tuple

try:
    import yaml
except ImportError:
    print("Error: PyYAML is required. Install with: pip install pyyaml", file=sys.stderr)
    sys.exit(1)


# ---------------------------------------------------------------------------
# ZFS & Environment Helpers
# ---------------------------------------------------------------------------

def run_cmd(cmd: List[str]) -> Tuple[int, str, str]:
    try:
        res = subprocess.run(cmd, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, check=False)
        return res.returncode, res.stdout.strip(), res.stderr.strip()
    except Exception as e:
        return -1, "", str(e)


def run_cmd_with_sudo_fallback(cmd: List[str]) -> Tuple[int, str, str]:
    """Runs a command, falling back to sudo if permission is denied or returncode != 0."""
    code, stdout, stderr = run_cmd(cmd)
    if code == 0:
        return code, stdout, stderr
    if os.geteuid() != 0 and shutil.which("sudo"):
        sudo_cmd = ["sudo"] + cmd
        s_code, s_out, s_err = run_cmd(sudo_cmd)
        if s_code == 0:
            return s_code, s_out, s_err
    return code, stdout, stderr


def get_zfs_mountpoints() -> Dict[str, str]:
    """Returns mapping of dataset name -> mountpoint from active ZFS system."""
    code, out, _ = run_cmd(["zfs", "get", "-H", "-o", "name,value", "mountpoint"])
    if code != 0:
        return {}
    mounts = {}
    for line in out.splitlines():
        parts = line.split("\t")
        if len(parts) == 2:
            ds, mp = parts[0].strip(), parts[1].strip()
            if mp and mp not in ("none", "legacy", "-"):
                mounts[ds] = mp
    return mounts


def inspect_ix_apps_datasets(root_ds: str) -> List[Dict[str, str]]:
    """
    Queries ZFS for all datasets under root_ds (e.g. storage/ix-apps).
    Returns list of dicts with keys: name, canmount, mounted, mountpoint.
    """
    code, out, _ = run_cmd(["zfs", "list", "-H", "-o", "name,canmount,mounted,mountpoint", "-r", root_ds])
    if code != 0:
        return []
    datasets = []
    for line in out.splitlines():
        parts = line.split("\t")
        if len(parts) >= 4:
            datasets.append({
                "name": parts[0].strip(),
                "canmount": parts[1].strip(),
                "mounted": parts[2].strip(),
                "mountpoint": parts[3].strip(),
            })
    return datasets


def configure_zfs_automount(
    datasets: List[Dict[str, str]],
    dry_run: bool = False,
    verbose: bool = False,
) -> Tuple[List[str], List[str], List[str]]:
    """
    For TrueNAS SCALE ix-apps datasets that are set to canmount=no / canmount=off,
    enables automounting by setting canmount=on and triggers mounting.

    Returns (planned_or_executed_cmds, fixed_datasets, errors).
    """
    cmds_to_run: List[str] = []
    fixed: List[str] = []
    errors: List[str] = []

    for ds in datasets:
        name = ds["name"]
        canmount = ds["canmount"]
        mounted = ds["mounted"]
        mp = ds["mountpoint"]

        needs_canmount_fix = canmount != "on"
        needs_mount = (mounted != "yes") and (mp not in ("none", "legacy", "-"))

        if needs_canmount_fix:
            cmd_str = f"zfs set canmount=on {name}"
            cmds_to_run.append(cmd_str)
            if not dry_run:
                code, _, err = run_cmd_with_sudo_fallback(["zfs", "set", "canmount=on", name])
                if code == 0:
                    fixed.append(f"{name} (canmount=on)")
                else:
                    errors.append(f"Failed '{cmd_str}': {err}")

        if needs_mount:
            cmd_str = f"zfs mount {name}"
            cmds_to_run.append(cmd_str)
            if not dry_run:
                code, _, err = run_cmd_with_sudo_fallback(["zfs", "mount", name])
                if code == 0:
                    fixed.append(f"{name} (mounted at {mp})")
                else:
                    # Ignore if already mounted or no error
                    if "already mounted" not in err.lower():
                        errors.append(f"Failed '{cmd_str}': {err}")

    # Also include zfs mount -a to ensure all children are mounted
    if cmds_to_run:
        cmds_to_run.append("zfs mount -a")
        if not dry_run:
            run_cmd_with_sudo_fallback(["zfs", "mount", "-a"])

    return cmds_to_run, fixed, errors


def detect_ix_apps_source(preferred: Optional[str] = None) -> Optional[Path]:
    """Finds the root path of the .ix-apps dataset."""
    if preferred:
        p = Path(preferred).expanduser().resolve()
        if p.exists():
            return p
        print(f"Warning: Specified source path {preferred} does not exist.", file=sys.stderr)

    # 1. Check live ZFS mountpoints
    zfs_mounts = get_zfs_mountpoints()
    for ds, mp in zfs_mounts.items():
        if ds.endswith("/ix-apps") or ds == "ix-apps":
            p = Path(mp)
            if p.exists() and (p / "app_configs").exists():
                return p

    # 2. Check standard candidate directories
    candidates = [
        "/mnt/.ix-apps",
        "/mnt/storage/.ix-apps",
        "/mnt/storage/ix-apps",
        "/storage/.ix-apps",
        "/storage/ix-apps",
        "/mnt/.ix-apps/app_configs",  # if user passed app_configs directly
    ]
    for c in candidates:
        p = Path(c)
        if p.exists():
            if (p / "app_configs").exists():
                return p
            if p.name == "app_configs":
                return p.parent

    return None


def detect_nasctl_docker_dir(preferred: Optional[str] = None, pool_hint: Optional[str] = None) -> Path:
    """Detects target docker unit state directory for MyNAS."""
    if preferred:
        return Path(preferred).expanduser().resolve()

    # 1. If main pool is known or detectable, check .nas-system dataset
    zfs_mounts = get_zfs_mountpoints()
    pools = []
    if pool_hint:
        pools.append(pool_hint)
    # Detect pool from storage datasets
    for ds, mp in zfs_mounts.items():
        pool = ds.split("/")[0]
        if pool not in pools and pool not in ("rpool", "bpool", "boot-pool"):
            pools.append(pool)

    for p in pools:
        candidates = [
            Path(f"/mnt/{p}/.nas-system/docker"),
            Path(f"/mnt/{p}/system/docker"),
            Path(f"/nas/{p}/.nas-system/docker"),
            Path(f"/{p}/.nas-system/docker"),
        ]
        for c in candidates:
            if c.parent.exists():
                return c

    # 2. System-wide /var/lib/nasctl/docker
    var_lib = Path("/var/lib/nasctl/docker")
    if var_lib.parent.exists() or os.access("/var/lib", os.W_OK):
        return var_lib

    # 3. User local share
    home_share = Path.home() / ".local" / "share" / "nasctl" / "docker"
    return home_share


def find_nasctl_binary() -> Optional[str]:
    """Finds nasctl CLI executable if present."""
    candidates = [
        shutil.which("nasctl"),
        str(Path.cwd() / "bin" / "nasctl"),
        "/usr/local/bin/nasctl",
        "/usr/bin/nasctl",
        str(Path.home() / "go" / "bin" / "nasctl"),
    ]
    for c in candidates:
        if c and os.path.isfile(c) and os.access(c, os.X_OK):
            return c
    return None


# ---------------------------------------------------------------------------
# TrueNAS App Discovery & Parsing
# ---------------------------------------------------------------------------

class TrueNASApp:
    def __init__(self, app_id: str, app_dir: Path):
        self.app_id = app_id
        self.app_dir = app_dir
        self.version = ""
        self.compose_path: Optional[Path] = None
        self.metadata_path: Optional[Path] = None
        self.env_path: Optional[Path] = None
        self.metadata: Dict[str, Any] = {}
        self.raw_compose: Dict[str, Any] = {}
        self.services: List[str] = []
        self.images: List[str] = []
        self.ports: List[str] = []
        self.volumes: List[Tuple[str, str]] = []  # (source, target)
        self.portals: List[Dict[str, Any]] = []

    def __repr__(self) -> str:
        return f"<TrueNASApp {self.app_id} v{self.version or 'unknown'}>"


def discover_apps(source_root: Path, filter_apps: Optional[List[str]] = None) -> List[TrueNASApp]:
    """Scans <source_root>/app_configs for all applications and their materialized compose files."""
    app_configs_dir = source_root / "app_configs"
    if not app_configs_dir.exists():
        # Maybe source_root IS app_configs
        if source_root.name == "app_configs":
            app_configs_dir = source_root
        else:
            return []

    discovered = []

    for entry in sorted(app_configs_dir.iterdir()):
        if not entry.is_dir() or entry.name.startswith("."):
            continue

        app_id = entry.name
        if filter_apps and app_id not in filter_apps:
            continue

        app = TrueNASApp(app_id=app_id, app_dir=entry)

        # 1. Discover metadata.yaml
        meta_candidates = [
            entry / "metadata.yaml",
            entry / "metadata.yml",
        ]
        for mc in meta_candidates:
            if mc.is_file():
                app.metadata_path = mc
                try:
                    with open(mc, "r", encoding="utf-8") as f:
                        app.metadata = yaml.safe_load(f) or {}
                except Exception as e:
                    print(f"Warning: Failed to parse metadata {mc}: {e}", file=sys.stderr)
                break

        # 2. Discover materialized compose file
        compose_candidates = []

        # (a) Check versions subdirectories
        versions_dir = entry / "versions"
        if versions_dir.exists() and versions_dir.is_dir():
            v_dirs = [vd for vd in versions_dir.iterdir() if vd.is_dir()]
            # Sort version folders, preferentially latest modified or semver
            v_dirs.sort(key=lambda d: d.stat().st_mtime, reverse=True)

            for vd in v_dirs:
                app.version = vd.name
                compose_candidates.extend([
                    vd / "templates" / "rendered" / "docker-compose.yaml",
                    vd / "templates" / "rendered" / "docker-compose.yml",
                    vd / "templates" / "rendered" / "compose.yaml",
                    vd / "templates" / "rendered" / "compose.yml",
                    vd / "docker-compose.yaml",
                    vd / "docker-compose.yml",
                    vd / "compose.yaml",
                    vd / "compose.yml",
                ])
                # Check for .env in version directory
                for env_name in [vd / "templates" / "rendered" / ".env", vd / ".env", vd / "templates" / "rendered" / "env"]:
                    if env_name.is_file() and not app.env_path:
                        app.env_path = env_name

        # (b) Check direct templates/rendered or top-level compose files
        compose_candidates.extend([
            entry / "templates" / "rendered" / "docker-compose.yaml",
            entry / "templates" / "rendered" / "docker-compose.yml",
            entry / "templates" / "rendered" / "compose.yaml",
            entry / "templates" / "rendered" / "compose.yml",
            entry / "docker-compose.yaml",
            entry / "docker-compose.yml",
            entry / "compose.yaml",
            entry / "compose.yml",
        ])

        # (c) Recursive fallback search if not found
        for cand in compose_candidates:
            if cand.is_file() and cand.stat().st_size > 0:
                app.compose_path = cand
                break

        if not app.compose_path:
            for p in entry.rglob("*.y*ml"):
                if "compose" in p.name.lower():
                    try:
                        with open(p, "r", encoding="utf-8") as f:
                            content = yaml.safe_load(f)
                            if isinstance(content, dict) and "services" in content:
                                app.compose_path = p
                                break
                    except Exception:
                        continue

        if not app.compose_path:
            # Not a compose stack or not materialized
            continue

        # Extract version from metadata if not yet found
        if not app.version and app.metadata:
            app.version = str(app.metadata.get("app_version") or app.metadata.get("version") or "")

        # Also check for .env in app root if not found
        if not app.env_path:
            for cand_env in [entry / ".env", entry / "env"]:
                if cand_env.is_file():
                    app.env_path = cand_env
                    break

        discovered.append(app)

    return discovered


# ---------------------------------------------------------------------------
# YAML Normalization & Volume Transformation
# ---------------------------------------------------------------------------

def extract_portals(app: TrueNASApp, compose_data: Dict[str, Any]) -> List[Dict[str, Any]]:
    """Extracts or translates web portal definitions into MyNAS x-portals format."""
    portals: List[Dict[str, Any]] = []

    # 1. Existing x-portals or portals in compose file
    raw_portals = compose_data.get("x-portals") or compose_data.get("portals")
    if raw_portals:
        if isinstance(raw_portals, list):
            for p in raw_portals:
                if isinstance(p, dict):
                    portals.append({
                        "name": str(p.get("name") or "Web UI"),
                        "scheme": str(p.get("scheme") or p.get("protocol") or "http"),
                        "port": int(p.get("port") or 80),
                        "path": str(p.get("path") or "/"),
                    })
        elif isinstance(raw_portals, dict):
            for k, p in raw_portals.items():
                if isinstance(p, dict):
                    portals.append({
                        "name": str(p.get("name") or k),
                        "scheme": str(p.get("scheme") or p.get("protocol") or "http"),
                        "port": int(p.get("port") or 80),
                        "path": str(p.get("path") or "/"),
                    })

    if portals:
        return portals

    # 2. Extract from TrueNAS metadata.yaml
    meta_portals = app.metadata.get("portals")
    if meta_portals:
        if isinstance(meta_portals, dict):
            for name, cfg in meta_portals.items():
                if isinstance(cfg, dict):
                    ports = cfg.get("ports", [80])
                    protocols = cfg.get("protocols", ["http"])
                    port = ports[0] if (isinstance(ports, list) and ports) else 80
                    scheme = protocols[0] if (isinstance(protocols, list) and protocols) else "http"
                    path = cfg.get("path", "/")
                    portals.append({
                        "name": name.replace("_", " ").title(),
                        "scheme": scheme,
                        "port": int(port),
                        "path": path,
                    })
        elif isinstance(meta_portals, list):
            for item in meta_portals:
                if isinstance(item, dict):
                    portals.append({
                        "name": str(item.get("name") or "Web UI"),
                        "scheme": str(item.get("scheme") or item.get("protocol") or "http"),
                        "port": int(item.get("port") or 80),
                        "path": str(item.get("path") or "/"),
                    })

    return portals


def transform_compose(
    app: TrueNASApp,
    remap_mode: str,
    target_mount_base: Optional[str],
    source_mount_base: Optional[str],
    zfs_mounts: Dict[str, str],
    target_pool: str,
) -> Tuple[Dict[str, Any], str]:
    """
    Parses, cleans, and rewrites the Docker Compose YAML.
    Returns (parsed_dict, yaml_string).
    """
    with open(app.compose_path, "r", encoding="utf-8") as f:
        data = yaml.safe_load(f)

    if not isinstance(data, dict):
        raise ValueError(f"Invalid compose YAML in {app.compose_path}")

    # Remove TrueNAS UI-specific internal artifacts that aren't Docker spec
    for key in list(data.keys()):
        if key.startswith("ix_") or key in ("portal_config", "schema"):
            del data[key]

    services = data.get("services", {})
    if not isinstance(services, dict):
        services = {}

    app.services = list(services.keys())
    app.images = []
    app.ports = []
    app.volumes = []

    # Detect default source mount base if not specified (e.g. /mnt/.ix-apps/app_mounts)
    default_src_mount = source_mount_base or "/mnt/.ix-apps/app_mounts"

    for svc_name, svc_cfg in services.items():
        if not isinstance(svc_cfg, dict):
            continue

        if "image" in svc_cfg and svc_cfg["image"]:
            app.images.append(str(svc_cfg["image"]))

        # Ports
        if "ports" in svc_cfg and isinstance(svc_cfg["ports"], list):
            for p in svc_cfg["ports"]:
                app.ports.append(str(p))

        # Volumes
        if "volumes" in svc_cfg and isinstance(svc_cfg["volumes"], list):
            new_vols = []
            for vol in svc_cfg["volumes"]:
                if isinstance(vol, str):
                    parts = vol.split(":")
                    if not parts or not parts[0]:
                        continue
                    src = parts[0].strip()
                    tgt = parts[1].strip() if len(parts) > 1 else ""
                    mode = parts[2].strip() if len(parts) > 2 else ""

                    # Filter out TrueNAS settings mistakenly output as volumes
                    if src in ("true", "false", "acl_enable", "dataset_name", "host_path_config"):
                        continue

                    # Handle volume remapping
                    new_src = remap_volume_source(
                        app_id=app.app_id,
                        src_path=src,
                        remap_mode=remap_mode,
                        source_mount_base=default_src_mount,
                        target_mount_base=target_mount_base,
                        zfs_mounts=zfs_mounts,
                        target_pool=target_pool,
                    )
                    app.volumes.append((new_src, tgt))
                    if tgt:
                        vol_spec = f"{new_src}:{tgt}" + (f":{mode}" if mode else "")
                    else:
                        vol_spec = new_src
                    new_vols.append(vol_spec)

                elif isinstance(vol, dict) and vol.get("type") == "bind":
                    src = vol.get("source", "")
                    tgt = vol.get("target", "")
                    new_src = remap_volume_source(
                        app_id=app.app_id,
                        src_path=src,
                        remap_mode=remap_mode,
                        source_mount_base=default_src_mount,
                        target_mount_base=target_mount_base,
                        zfs_mounts=zfs_mounts,
                        target_pool=target_pool,
                    )
                    app.volumes.append((new_src, tgt))
                    vol["source"] = new_src
                    new_vols.append(vol)
                else:
                    new_vols.append(vol)
            svc_cfg["volumes"] = new_vols

    # Extract & Inject Portals and MyNAS metadata extensions
    portals = extract_portals(app, data)
    app.portals = portals
    if portals:
        data["x-portals"] = portals

    title = app.metadata.get("title") or app.app_id.replace("-", " ").title()
    icon = app.metadata.get("icon") or app.metadata.get("icon_url")

    data["x-nas-catalog-id"] = app.app_id
    if app.version:
        data["x-nas-catalog-version"] = app.version
    if icon:
        data["x-nas-icon-url"] = icon

    # Dump to formatted YAML string
    yaml_text = yaml.dump(data, sort_keys=False, default_flow_style=False, indent=2)

    # Prefix with clear header
    header = (
        f"# ====================================================================\n"
        f"# Ported from TrueNAS SCALE App: {app.app_id}\n"
        f"# Stack Title: {title}\n"
        f"# Version: {app.version or 'latest'}\n"
        f"# ====================================================================\n\n"
    )
    return data, header + yaml_text


def remap_volume_source(
    app_id: str,
    src_path: str,
    remap_mode: str,
    source_mount_base: str,
    target_mount_base: Optional[str],
    zfs_mounts: Dict[str, str],
    target_pool: str,
) -> str:
    """Remaps a volume host path based on specified mode and ZFS mount inspection."""
    if not src_path.startswith("/"):
        # Named or relative volume
        return src_path

    # Clean double slashes
    clean_src = os.path.normpath(src_path)

    # Mode: Keep explicitly requested
    if remap_mode == "keep":
        return clean_src

    # 1. Check if dataset is mounted directly in ZFS right now
    # e.g., dataset storage/ix-apps/app_mounts/<app_id>/data -> /mnt/storage/...
    for ds_name, mp in zfs_mounts.items():
        if f"/app_mounts/{app_id}" in ds_name or ds_name.endswith(f"/app_mounts/{app_id}"):
            # Check if clean_src references this dataset's subpath
            # e.g. clean_src: /mnt/.ix-apps/app_mounts/nginx-proxy-manager/certs
            ds_suffix = ds_name.split(f"/app_mounts/{app_id}")[-1]  # e.g. /certs or empty
            expected_old_prefix = f"/mnt/.ix-apps/app_mounts/{app_id}{ds_suffix}"
            if clean_src.startswith(expected_old_prefix):
                remainder = clean_src[len(expected_old_prefix):].lstrip("/")
                return os.path.normpath(os.path.join(mp, remainder))

    # 2. Check explicit target_mount_base
    if target_mount_base:
        # If clean_src contains /app_mounts/
        if "/app_mounts/" in clean_src:
            sub = clean_src.split("/app_mounts/", 1)[1]
            return os.path.normpath(os.path.join(target_mount_base, sub))
        elif clean_src.startswith(source_mount_base):
            sub = clean_src[len(source_mount_base):].lstrip("/")
            return os.path.normpath(os.path.join(target_mount_base, sub))

    # 3. Mode: Relocate to standard MyNAS pool apps dataset (/mnt/<pool>/apps/<app_id>/...)
    if remap_mode == "relocate":
        if "/app_mounts/" in clean_src:
            sub = clean_src.split(f"/app_mounts/{app_id}/", 1)
            leaf = sub[1] if len(sub) > 1 else "data"
            return f"/mnt/{target_pool}/apps/{app_id}/{leaf}"
        return clean_src

    # 4. Mode: Auto
    # If the source path actually exists on host, keep it!
    if os.path.exists(clean_src):
        return clean_src

    # If /mnt/.ix-apps doesn't exist, but /mnt/<pool>/ix-apps or /mnt/<pool>/app_mounts exists
    alt_candidates = [
        clean_src.replace("/mnt/.ix-apps/", f"/mnt/{target_pool}/.ix-apps/"),
        clean_src.replace("/mnt/.ix-apps/", f"/mnt/{target_pool}/ix-apps/"),
        clean_src.replace("/mnt/.ix-apps/app_mounts/", f"/mnt/{target_pool}/app_mounts/"),
        clean_src.replace("/mnt/.ix-apps/app_mounts/", f"/mnt/{target_pool}/apps/"),
    ]
    for alt in alt_candidates:
        if os.path.exists(alt) or os.path.exists(os.path.dirname(alt)):
            return os.path.normpath(alt)

    # Fallback: keep original clean path
    return clean_src


# ---------------------------------------------------------------------------
# CLI & Execution Logic
# ---------------------------------------------------------------------------

def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Port TrueNAS SCALE materialized Docker Compose applications into MyNAS / nasctl.",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  # 1. Preview all apps detected in TrueNAS dataset without making changes:
  python3 scripts/port_truenas_apps.py --dry-run

  # 2. Port directly into MyNAS units directory (auto-detects storage pool):
  python3 scripts/port_truenas_apps.py --source /mnt/.ix-apps

  # 3. Export all converted compose stacks to a local review directory:
  python3 scripts/port_truenas_apps.py --source /mnt/.ix-apps --output-dir ./imported-apps

  # 4. Port and immediately deploy running containers via nasctl CLI:
  python3 scripts/port_truenas_apps.py --source /mnt/.ix-apps --deploy
        """,
    )
    parser.add_argument(
        "-s", "--source",
        help="Path to TrueNAS .ix-apps root directory or dataset mount (default: auto-detected, e.g. /mnt/.ix-apps)",
    )
    parser.add_argument(
        "-d", "--dest",
        help="Destination directory for MyNAS docker units (default: auto-detected, e.g. /mnt/<pool>/.nas-system/docker or /var/lib/nasctl/docker)",
    )
    parser.add_argument(
        "-o", "--output-dir",
        help="Optional staging/export directory to save ported compose files instead of or in addition to the system state dir",
    )
    parser.add_argument(
        "-p", "--pool",
        default="storage",
        help="Primary ZFS storage pool name (default: storage)",
    )
    parser.add_argument(
        "--remap-volumes",
        choices=["auto", "keep", "relocate"],
        default="auto",
        help="Volume remapping strategy: 'auto' checks live mounts & fallbacks; 'keep' preserves paths; 'relocate' moves to /mnt/<pool>/apps/<app>/ (default: auto)",
    )
    parser.add_argument(
        "--enable-automount",
        action=argparse.BooleanOptionalAction,
        default=True,
        help="Enable automounting on ix-apps datasets by setting 'canmount=on' and mounting unmounted datasets (default: true)",
    )
    parser.add_argument(
        "--ix-dataset",
        help="Root ZFS dataset for TrueNAS ix-apps (default: <pool>/ix-apps, e.g. storage/ix-apps)",
    )
    parser.add_argument(
        "--generate-automount-script",
        help="Path to generate a standalone bash script containing ZFS canmount=on and mount commands (e.g. enable_automount.sh)",
    )
    parser.add_argument(
        "--target-mount-base",
        help="Override base path for app_mounts (e.g. /mnt/storage/ix-apps/app_mounts)",
    )
    parser.add_argument(
        "--apps",
        help="Comma-separated list of specific app names to import (e.g. 'nginx-proxy-manager,postgres')",
    )
    parser.add_argument(
        "--deploy",
        action="store_true",
        help="Automatically trigger 'nasctl app create' for each imported stack",
    )
    parser.add_argument(
        "-n", "--dry-run",
        action="store_true",
        help="Perform a trial run: print discovered stacks, volumes, and portals without writing files",
    )
    parser.add_argument(
        "-v", "--verbose",
        action="store_true",
        help="Enable detailed diagnostic logging",
    )
    return parser.parse_args()


def main():
    args = parse_args()

    print("====================================================================")
    print(" MyNAS TrueNAS SCALE Application Porter")
    print("====================================================================")

    target_pool = args.pool
    root_ix_ds = args.ix_dataset or f"{target_pool}/ix-apps"

    # 1. ZFS Automount Inspection & Activation (canmount=on)
    ix_datasets = inspect_ix_apps_datasets(root_ix_ds)
    if ix_datasets:
        print(f"[*] ZFS ix-apps datasets found: {len(ix_datasets)} under {root_ix_ds}")
        off_count = sum(1 for d in ix_datasets if d["canmount"] != "on")
        unmounted_count = sum(1 for d in ix_datasets if d["mounted"] != "yes" and d["mountpoint"] not in ("none", "legacy", "-"))
        if off_count > 0 or unmounted_count > 0:
            print(f"[*] Detected TrueNAS restrictions: {off_count} datasets with canmount!=on, {unmounted_count} unmounted.")
            if args.enable_automount:
                print(f"[*] Enabling automounting (canmount=on) and mounting datasets...")
                planned_cmds, fixed, errors = configure_zfs_automount(ix_datasets, dry_run=args.dry_run, verbose=args.verbose)
                if args.dry_run:
                    print(f"[DRY-RUN] Planned ZFS commands ({len(planned_cmds)}):")
                    for cmd in planned_cmds:
                        print(f"    {cmd}")
                else:
                    if fixed:
                        print(f"    ✓ Successfully enabled automount / mounted {len(fixed)} items")
                    if errors:
                        print(f"    ⚠️ Warning during ZFS configuration:", file=sys.stderr)
                        for err in errors:
                            print(f"      - {err}", file=sys.stderr)
                        print("    Tip: Run with root / sudo permissions to modify ZFS properties.", file=sys.stderr)
            else:
                print("[*] Note: --no-enable-automount was specified; skipping ZFS canmount modifications.")
    elif args.verbose:
        print(f"[*] No live ZFS datasets reported under {root_ix_ds} (or 'zfs' CLI not found)")

    # 1b. Standalone automount script generation if requested
    if args.generate_automount_script:
        script_path = Path(args.generate_automount_script).expanduser().resolve()
        script_cmds = []
        if ix_datasets:
            for d in ix_datasets:
                script_cmds.append(f"zfs set canmount=on {d['name']}")
                if d['mountpoint'] not in ('none', 'legacy', '-'):
                    script_cmds.append(f"zfs mount {d['name']} 2>/dev/null || true")
            script_cmds.append("zfs mount -a")
        else:
            # Fallback template based on standard TrueNAS hierarchy
            script_cmds = [
                f"# Set canmount=on and mount all datasets under {root_ix_ds}",
                f"for ds in $(zfs list -H -o name -r {root_ix_ds}); do",
                '  echo "Enabling automount on $ds..."',
                '  zfs set canmount=on "$ds"',
                '  zfs mount "$ds" 2>/dev/null || true',
                'done',
                'zfs mount -a',
                'echo "Done! All ix-apps datasets are set to canmount=on and automounted."',
            ]
        script_content = "#!/usr/bin/env bash\nset -euo pipefail\n# Generated by MyNAS port_truenas_apps.py\n\n" + "\n".join(script_cmds) + "\n"
        with open(script_path, "w", encoding="utf-8") as f:
            f.write(script_content)
        os.chmod(script_path, 0o755)
        print(f"[*] Generated standalone automount script: {script_path}")
        if not args.source:
            # If source wasn't explicitly given and cannot be auto-detected yet (since datasets are unmounted)
            test_src = detect_ix_apps_source(None)
            if not test_src:
                print("\nAutomount script generated. Execute it with root privileges on the NAS to enable automounting:")
                print(f"  sudo {script_path}")
                print("Once mounted, run this script again to import your applications into MyNAS:")
                print(f"  python3 {sys.argv[0]} --source /mnt/.ix-apps")
                sys.exit(0)

    # 2. Resolve Source Path
    source_root = detect_ix_apps_source(args.source)
    if not source_root:
        print(f"Error: Could not locate TrueNAS .ix-apps directory.", file=sys.stderr)
        print("Please supply the path with --source /path/to/.ix-apps", file=sys.stderr)
        if not ix_datasets and not shutil.which("zfs"):
            print("If the dataset is unmounted due to canmount=no, run the generated automount script or 'zfs set canmount=on <ds>' first.", file=sys.stderr)
        sys.exit(1)

    print(f"[*] TrueNAS .ix-apps source : {source_root}")

    # 3. Resolve ZFS Mountpoints & Storage Pool
    zfs_mounts = get_zfs_mountpoints()
    if zfs_mounts:
        print(f"[*] Live ZFS datasets detected: {len(zfs_mounts)} active mounts")

    # 4. Resolve Destination State Directory
    if args.output_dir:
        dest_dir = Path(args.output_dir).expanduser().resolve()
        is_staging_dir = True
    else:
        dest_dir = detect_nasctl_docker_dir(args.dest, pool_hint=target_pool)
        is_staging_dir = False

    print(f"[*] Target destination dir : {dest_dir} {'(staging/export)' if is_staging_dir else '(MyNAS state store)'}")
    print(f"[*] Volume remapping mode  : {args.remap_volumes}")
    if args.dry_run:
        print("[*] Mode                     : DRY-RUN (no files will be written)")
    print()

    # 5. Discover Applications
    filter_list = [a.strip() for a in args.apps.split(",")] if args.apps else None
    apps = discover_apps(source_root, filter_apps=filter_list)

    if not apps:
        print("No materialized Docker Compose applications found in:", source_root / "app_configs")
        print("Ensure the TrueNAS dataset contains app configurations under app_configs/<app_name>/...")
        sys.exit(0)

    print(f"Discovered {len(apps)} TrueNAS applications:")
    for app in apps:
        print(f"  • {app.app_id} (version: {app.version or 'n/a'}, compose: {app.compose_path.name if app.compose_path else 'none'})")
    print()

    # 5. Process each application
    results = []
    nasctl_bin = find_nasctl_binary() if args.deploy else None

    for app in apps:
        print(f"--------------------------------------------------------------------")
        print(f"Processing App: {app.app_id}")
        print(f"  Source file: {app.compose_path}")
        if app.env_path:
            print(f"  Env file:    {app.env_path}")

        try:
            compose_dict, final_yaml = transform_compose(
                app=app,
                remap_mode=args.remap_volumes,
                target_mount_base=args.target_mount_base,
                source_mount_base=str(source_root / "app_mounts"),
                zfs_mounts=zfs_mounts,
                target_pool=target_pool,
            )
        except Exception as e:
            print(f"  [ERROR] Failed to transform compose file: {e}", file=sys.stderr)
            continue

        print(f"  Services:    {', '.join(app.services) or 'none'}")
        if app.images:
            print(f"  Images:      {', '.join(app.images)}")
        if app.ports:
            print(f"  Ports:       {', '.join(app.ports)}")
        if app.portals:
            p_desc = [f"{p['name']} ({p['scheme']}://:{p['port']}{p['path']})" for p in app.portals]
            print(f"  Web Portals: {', '.join(p_desc)}")
        if app.volumes:
            print("  Volumes:")
            for src, tgt in app.volumes:
                exists_tag = "✓ exists" if os.path.exists(src) else "⚠️ not found on host"
                print(f"    - {src} -> {tgt} ({exists_tag})")

        target_app_dir = dest_dir / app.app_id
        target_compose = target_app_dir / "compose.yaml"
        target_env = target_app_dir / ".env"

        if args.dry_run:
            print(f"\n[DRY RUN] Would write: {target_compose}")
            if args.verbose:
                print("\n--- GENERATED COMPOSE.YAML ---")
                print(final_yaml)
                print("--- END COMPOSE.YAML ---\n")
            results.append((app.app_id, "PREVIEWED"))
            continue

        # Write out ported stack
        target_app_dir.mkdir(parents=True, exist_ok=True)
        with open(target_compose, "w", encoding="utf-8") as f:
            f.write(final_yaml)
        print(f"  [SAVED] {target_compose}")

        # Copy .env if available
        if app.env_path and app.env_path.exists():
            shutil.copyfile(app.env_path, target_env)
            print(f"  [SAVED] Copied environment variables to {target_env}")

        # Trigger live deployment if requested
        if args.deploy:
            if not nasctl_bin:
                print(f"  [WARN] 'nasctl' executable not found. Skipping live deploy for {app.app_id}.")
                results.append((app.app_id, "SAVED (DEPLOY SKIPPED)"))
            else:
                title = app.metadata.get("title") or app.app_id.replace("-", " ").title()
                deploy_cmd = [nasctl_bin, "app", "create", app.app_id, "--compose", str(target_compose), "--title", title]
                print(f"  [DEPLOY] Running: {' '.join(deploy_cmd)}")
                d_code, d_out, d_err = run_cmd(deploy_cmd)
                if d_code == 0:
                    print(f"  [DEPLOYED] {d_out}")
                    results.append((app.app_id, "DEPLOYED"))
                else:
                    print(f"  [DEPLOY ERROR] {d_err or d_out}", file=sys.stderr)
                    results.append((app.app_id, "SAVED (DEPLOY FAILED)"))
        else:
            results.append((app.app_id, "SAVED"))

    # 6. Final Summary
    print("\n====================================================================")
    print(" PORTING SUMMARY")
    print("====================================================================")
    print(f"{'APP ID':<25} {'STATUS':<20} {'SERVICES':<15} {'PORTALS'}")
    print("-" * 75)
    app_map = {a.app_id: a for a in apps}
    for app_id, status in results:
        a = app_map.get(app_id)
        svc_count = f"{len(a.services)} services" if a else ""
        portal_count = f"{len(a.portals)} portals" if (a and a.portals) else "none"
        print(f"{app_id:<25} {status:<20} {svc_count:<15} {portal_count}")

    print("\nDone! All materialized Docker Compose stacks have been processed.")
    if not args.dry_run and not args.deploy:
        print(f"\nNext Steps:")
        print(f"  1. Start or restart nasctl to discover the new apps:")
        print(f"     nasctl start (or 'systemctl restart nasctl')")
        print(f"  2. Or start individual stacks using the CLI:")
        for app_id, _ in results:
            print(f"     nasctl app start {app_id}")


if __name__ == "__main__":
    main()
