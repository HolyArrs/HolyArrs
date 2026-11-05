"""Render an unpublished Unraid template candidate; never install it."""
import argparse
from pathlib import Path, PurePosixPath
import re
import xml.etree.ElementTree as ET

# Optional media folders, one per library, as (name, container path, description).
MEDIA = (
    ("Movies", "/movies", "Optional. Movie Library folder."),
    ("Shows", "/tv", "Optional. Shows Library folder."),
    ("Music", "/music", "Optional. Music Library folder."),
    ("Downloads", "/downloads", "Optional. The Downloader's download folder."),
)


def _overlap(a: str, b: str) -> bool:
    a, b = PurePosixPath(a), PurePosixPath(b)
    return a == b or a in b.parents or b in a.parents


def render(image: str, config: str, port: int, media: dict | None = None, allowed_hosts: str = "") -> bytes:
    """media maps a container path such as "/movies" to a host path; missing ones stay empty.
    allowed_hosts is a comma-separated list of host names, empty by default."""
    media = media or {}
    if allowed_hosts and not re.fullmatch(r"[A-Za-z0-9.-]+(,[A-Za-z0-9.-]+)*", allowed_hosts):
        raise ValueError("allowed host names are plain names such as tower.local, separated by commas")
    if not re.fullmatch(r"[a-z0-9][a-z0-9./_-]*@sha256:[0-9a-f]{64}", image):
        raise ValueError("supply an immutable registry image reference, without credentials")
    unknown = set(media) - {target for _, target, _ in MEDIA}
    if unknown:
        raise ValueError(f"unknown media folder {sorted(unknown)[0]}")
    for value in (config, *(v for v in media.values() if v)):
        if not re.fullmatch(r"/mnt/[a-zA-Z0-9_./-]+", value) or ".." in PurePosixPath(value).parts:
            raise ValueError("paths must be absolute /mnt paths without traversal")
    pool = PurePosixPath(config).parts[2]
    if pool in {"user", "user0", "remotes", "disks"} or len(PurePosixPath(config).parts) < 5:
        raise ValueError("config needs a direct local pool path, for example /mnt/<pool>/appdata/holyarrs")
    if any(v and _overlap(config, v) for v in media.values()):
        raise ValueError("config and media paths must not overlap")
    if not 1024 <= port <= 65535:
        raise ValueError("host port must be between 1024 and 65535")
    root = ET.Element("Container", version="2")
    # In-app Restart, safe mode and restore exit and count on the restart policy.
    for name, text in {"Name": "holyarrs-candidate", "Repository": image, "Network": "bridge",
                       "Privileged": "false", "ExtraParams": "--restart unless-stopped",
                       "WebUI": "http://[IP]:[PORT:6262]/", "Category": "MediaApp:Other:",
                       "Overview": "Unpublished candidate. NAS installation remains unverified. Keep config on a direct local pool. "
                                   "HolyArrs runs as PUID:PGID (99:100 here) and gives the config folder to that user; "
                                   "media folders are never changed. For hardlinks, map one share holding downloads and "
                                   "Library folders instead of separate paths."}.items():
        ET.SubElement(root, name).text = text
    entries = [
        ("Web port", "6262", str(port), "Port", "tcp", "Choose an unused host port.", "always", "true"),
        ("Config", "/config", config, "Path", "rw", "Direct local pool only; do not use /mnt/user. HolyArrs gives it to PUID:PGID.", "always", "true"),
    ]
    entries += [(name, target, media.get(target, ""), "Path", "rw", description, "always", "false")
                for name, target, description in MEDIA]
    entries += [
        ("Completed downloads", "HOLYARRS_SAVE_PATH", "", "Variable", "",
         "The Downloader's save folder as the Downloader sees it; HolyArrs sends it with every download. "
         "Leave empty to keep the Downloader's own setting.", "always", "false"),
        ("Allowed host names", "HOLYARRS_ALLOWED_HOSTS", allowed_hosts, "Variable", "",
         "Optional. Names you open HolyArrs by besides its IP address, comma-separated, for example tower.local. "
         "The name you use at first-run setup is added for you.", "always", "false"),
        ("PUID", "PUID", "99", "Variable", "", "User id HolyArrs runs as.", "advanced", "false"),
        ("PGID", "PGID", "100", "Variable", "", "Group id HolyArrs runs as.", "advanced", "false"),
        ("UMASK", "UMASK", "022", "Variable", "", "Permission bits new files leave out; 002 lets the group write.", "advanced", "false"),
    ]
    for name, target, default, kind, mode, description, display, required in entries:
        ET.SubElement(root, "Config", Name=name, Target=target, Default=default, Mode=mode,
                      Description=description, Type=kind, Display=display, Required=required, Mask="false").text = default
    ET.indent(root)
    return ET.tostring(root, encoding="utf-8", xml_declaration=True) + b"\n"


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--image", required=True)
    parser.add_argument("--config", required=True)
    for _, target, _ in MEDIA:
        parser.add_argument("--" + target.lstrip("/"), default="", help=f"host path for {target} (optional)")
    parser.add_argument("--port", type=int, default=6262)
    parser.add_argument("--allowed-hosts", default="", help="comma-separated host names (optional)")
    parser.add_argument("--output", required=True)
    args = parser.parse_args()
    try:
        content = render(args.image, args.config, args.port,
                         {target: getattr(args, target.lstrip("/")) for _, target, _ in MEDIA},
                         allowed_hosts=args.allowed_hosts)
        with Path(args.output).open("xb") as output:
            output.write(content)
    except (ValueError, OSError) as error:
        parser.exit(1, f"nas-template: {error}\n")
