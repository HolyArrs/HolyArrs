"""Builds the plain Linux downloads (no Docker) the GitHub release carries.

    python3 deploy/release/linux_package.py --version 1.1.3 --output dist

Makes holyarrs_<version>_linux_<arch>.tar.gz for amd64, arm64 and armv7, plus
SHA256SUMS. The web app and the go build use the same steps and flags as
deploy/Dockerfile, so the program in a download is the program in the image.
Run it from a checkout; it writes only into the output folder and the
checkout's app/frontend/dist.
"""
from __future__ import annotations

import argparse
import gzip
import hashlib
import io
import os
from pathlib import Path
import re
import subprocess
import sys
import tarfile

ROOT = Path(__file__).resolve().parents[2]
# Go's own names for each download name (GOARM only matters for armv7).
ARCHES = {"amd64": ("amd64", ""), "arm64": ("arm64", ""), "armv7": ("arm", "7")}
# Inlining is off in these packages, the same list as deploy/Dockerfile.
NO_INLINE = (
    "github.com/HolyArrs/holyarrs/... net/http/... crypto/tls/... crypto/x509/... net net/url net/textproto "
    "net/mail net/netip net/smtp mime/... os os/exec os/signal os/user log/... encoding/asn1 encoding/pem "
    "encoding/csv archive/... vendor/... io/fs path/filepath html/... text/... flag embed "
    "github.com/go-jose/... github.com/coreos/... golang.org/x/oauth2/... github.com/spf13/... go4.org/... "
    "github.com/google/... github.com/dustin/... github.com/hashicorp/..."
).split()

README = """HolyArrs {version} for Linux ({arch})

One program, no Docker. It holds the web app, the languages and the time zone
data. It needs a folder of its own for the database and settings, on local disk.

Run it (as a normal user, never root):

    ./holyarrs -config /var/lib/holyarrs

then open http://your-server:6262 and make the owner account; the first
visitor becomes the owner. -addr :8080 (or HOLYARRS_PORT=8080) moves the port.

Needs: a Linux kernel and the usual CA certificates (the ca-certificates
package) so it can reach the internet over https. Keep the program and the
config folder in separate folders. To update, stop it, replace the program and
start it again; the config folder stays.

Licence: AGPL-3.0-only, see LICENSE. Home: https://github.com/HolyArrs/HolyArrs
"""


def checked_version(value: str) -> str:
    if not re.fullmatch(r"[0-9]+\.[0-9]+\.[0-9]+", value):
        raise ValueError("version must look like 1.2.3")
    return value


def run(args: list[str], cwd: Path, env: dict | None = None) -> None:
    result = subprocess.run(args, cwd=cwd, env=env)
    if result.returncode:
        raise RuntimeError(f"{args[0]} failed ({result.returncode})")


def build_web(source: Path, install: bool, prebuilt: bool) -> None:
    web = source / "app" / "frontend"
    if prebuilt:
        # The pages were built already, with HOLYARRS_LANGUAGES_PLAIN=1.
        run([sys.executable, str(source / "deploy" / "pack_languages.py"), str(web / "dist")], source)
        return
    npm = "npm.cmd" if os.name == "nt" else "npm"
    if install:
        run([npm, "ci", "--ignore-scripts", "--no-audit", "--no-fund"], web)
    # The pages just built are what the program embeds; stale ones never are.
    dist = web / "dist"
    if dist.is_dir():
        for p in sorted(dist.rglob("*"), reverse=True):
            p.rmdir() if p.is_dir() else p.unlink()
    run([npm, "run", "build"], web, {**os.environ, "HOLYARRS_LANGUAGES_PLAIN": "1"})
    run([sys.executable, str(source / "deploy" / "pack_languages.py"), str(dist)], source)


def go_build(source: Path, version: str, arch: str, out: Path) -> None:
    goarch, goarm = ARCHES[arch]
    flags = [f"-gcflags={p}=-l" for p in NO_INLINE]
    env = {**os.environ, "CGO_ENABLED": "0", "GOOS": "linux", "GOARCH": goarch, "GOARM": goarm}
    run(["go", "build", "-trimpath", *flags, f"-ldflags=-s -w -X main.version={version}",
         "-o", str(out), "./app/backend/cmd/holyarrs"], source, env)


def tarball(path: Path, folder: str, members: dict[str, tuple[bytes, int]], epoch: int) -> None:
    """Writes a tar.gz whose bytes depend only on the inputs."""
    with path.open("wb") as raw, gzip.GzipFile(filename="", fileobj=raw, mode="wb", mtime=epoch) as gz:
        with tarfile.open(fileobj=gz, mode="w", format=tarfile.USTAR_FORMAT) as out:
            for name, (data, mode) in sorted(members.items()):
                info = tarfile.TarInfo(f"{folder}/{name}")
                info.size, info.mode, info.mtime = len(data), mode, epoch
                info.uid = info.gid = 0
                info.uname = info.gname = "root"
                out.addfile(info, io.BytesIO(data))


def build_archive(output: Path, version: str, arch: str, binary: bytes, source: Path, epoch: int) -> Path:
    members = {"holyarrs": (binary, 0o755),
               "LICENSE": ((source / "LICENSE").read_bytes(), 0o644),
               "README.txt": (README.format(version=version, arch=arch).encode(), 0o644)}
    notices = source / "THIRD_PARTY_NOTICES.md"
    if notices.is_file():
        members["THIRD_PARTY_NOTICES.md"] = (notices.read_bytes(), 0o644)
    stem = f"holyarrs_{version}_linux_{arch}"
    path = output / f"{stem}.tar.gz"
    tarball(path, stem, members, epoch)
    return path


def checksums(files: list[Path]) -> str:
    return "".join(f"{hashlib.sha256(p.read_bytes()).hexdigest()}  {p.name}\n" for p in sorted(files))


def source_epoch(source: Path) -> int:
    if os.environ.get("SOURCE_DATE_EPOCH", "").isdigit():
        return int(os.environ["SOURCE_DATE_EPOCH"])
    version_file = source / "VERSION"
    return int(version_file.stat().st_mtime) if version_file.is_file() else 0


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--version", required=True, help="for example 1.1.3, the same as the VERSION file")
    parser.add_argument("--output", required=True, help="folder for the archives (made if missing)")
    parser.add_argument("--source", default=str(ROOT), help="the checkout to build (default: this one)")
    parser.add_argument("--arches", default=",".join(ARCHES), help="comma separated: amd64, arm64, armv7")
    parser.add_argument("--skip-npm-ci", action="store_true", help="use the node_modules already installed")
    parser.add_argument("--prebuilt-web", action="store_true",
                        help="app/frontend/dist is built already (plain languages); only pack the languages")
    args = parser.parse_args()
    try:
        version = checked_version(args.version)
        arches = args.arches.split(",")
        if any(a not in ARCHES for a in arches) or len(set(arches)) != len(arches):
            raise ValueError("arches must be unique names from: " + ", ".join(ARCHES))
        source, output = Path(args.source).resolve(), Path(args.output).resolve()
        output.mkdir(parents=True, exist_ok=True)
        epoch = source_epoch(source)
        build_web(source, not args.skip_npm_ci, args.prebuilt_web)
        files = []
        for arch in arches:
            print("Building " + arch, flush=True)
            binary = output / f".holyarrs-{arch}"
            go_build(source, version, arch, binary)
            data = binary.read_bytes()
            binary.unlink()
            if version.encode() not in data:
                raise ValueError(f"{arch}: the program does not carry version {version}")
            files.append(build_archive(output, version, arch, data, source, epoch))
        (output / "SHA256SUMS").write_text(checksums(files), encoding="utf-8", newline="\n")
        print("Packaged: " + ", ".join(p.name for p in files))
    except (ValueError, RuntimeError, OSError) as error:
        parser.exit(1, f"linux_package: {error}\n")


if __name__ == "__main__":
    main()
