"""Render an unpublished Unraid template candidate; never install it."""
import argparse
from pathlib import Path, PurePosixPath
import re
import xml.etree.ElementTree as ET


def render(image: str, config: str, data: str, port: int) -> bytes:
    if not re.fullmatch(r"[a-z0-9][a-z0-9./_-]*@sha256:[0-9a-f]{64}", image):
        raise ValueError("supply an immutable registry image reference, without credentials")
    for value in (config, data):
        if not re.fullmatch(r"/mnt/[a-zA-Z0-9_./-]+", value) or ".." in PurePosixPath(value).parts:
            raise ValueError("paths must be absolute /mnt paths without traversal")
    pool = PurePosixPath(config).parts[2]
    if pool in {"user", "user0", "remotes", "disks"} or len(PurePosixPath(config).parts) < 5:
        raise ValueError("config needs a direct local pool path, for example /mnt/cache/appdata/holyarrs")
    if config == data or PurePosixPath(config) in PurePosixPath(data).parents or PurePosixPath(data) in PurePosixPath(config).parents:
        raise ValueError("config and data paths must not overlap")
    if not 1024 <= port <= 65535:
        raise ValueError("host port must be between 1024 and 65535")
    root = ET.Element("Container", version="2")
    for name, text in {"Name": "holyarrs-candidate", "Repository": image, "Network": "bridge",
                       "Privileged": "false", "ExtraParams": "--user=1000:1000",
                       "WebUI": "http://[IP]:[PORT:18730]/", "Category": "MediaApp:Other:",
                       "Overview": "Unpublished candidate. NAS installation remains unverified. Keep config on a direct local pool; keep downloads and Library folders in one data filesystem."}.items():
        ET.SubElement(root, name).text = text
    for name, target, default, kind, mode, description in (
        ("Web port", "18730", str(port), "Port", "tcp", "Choose an unused host port."),
        ("Config", "/config", config, "Path", "rw", "Direct local pool only; do not use /mnt/user. UID 1000 must have write access."),
        ("Data", "/data", data, "Path", "rw", "One shared tree for downloads and Library folders; hardlinks need one filesystem."),
        ("Memory target", "GOMEMLIMIT", "384MiB", "Variable", "", "Go memory target; not a hard process memory limit."),
        ("Completed downloads", "HOLYARRS_SAVE_PATH", "/data/downloads", "Variable", "", "Match the Downloader path."),
    ):
        ET.SubElement(root, "Config", Name=name, Target=target, Default=default, Mode=mode,
                      Description=description, Type=kind, Display="always", Required="true", Mask="false").text = default
    ET.indent(root)
    return ET.tostring(root, encoding="utf-8", xml_declaration=True) + b"\n"


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--image", required=True)
    parser.add_argument("--config", required=True)
    parser.add_argument("--data", required=True)
    parser.add_argument("--port", type=int, default=18730)
    parser.add_argument("--output", required=True)
    args = parser.parse_args()
    try:
        content = render(args.image, args.config, args.data, args.port)
        with Path(args.output).open("xb") as output:
            output.write(content)
    except (ValueError, OSError) as error:
        parser.exit(1, f"nas-template: {error}\n")
