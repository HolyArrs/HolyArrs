#!/usr/bin/env python3
"""Build the experimental Synology package (.spk) for one HolyArrs version.

The program inside the package is the one from the published Docker image
holyarrs/holyarrs:<version>, copied out per CPU type, so a package always
matches the image of the same version. Nothing is compiled here.

    python3 deploy/synology/build.py --version 1.0.19 --arch all --output dist

Writes holyarrs-<version>-<arch>.spk for each arch and one checksum file,
holyarrs-<version>-synology-SHA256SUMS. Every package is checked after it is
written; a failed check removes the file and ends with an error.

Needs Python 3 and Docker (only to pull the image and copy two files out; the
image is never run). Works on Linux and under Git Bash on Windows.
"""
import argparse
import gzip
import hashlib
import io
import json
import re
import struct
import subprocess
import sys
import tarfile
import tempfile
import time
from pathlib import Path

PACKAGE = "holyarrs"
PORT = "6262"
IMAGE = "holyarrs/holyarrs"
SOURCES = Path(__file__).resolve().parent / "package"

# Per package arch: the Docker platform, the ELF machine and word size of the
# program, and the DSM platform names the package installs on (Synology's
# "Platform and Arch Value Mapping" table, plus the family name).
ARCHES = {
    "x86_64": {
        "platform": "linux/amd64", "machine": 62, "bits": 64,
        "dsm": "x86_64 apollolake avoton braswell broadwell broadwellnk broadwellnkv2 broadwellntb "
               "broadwellntbap bromolow cedarview coffeelake denverton epyc7002 geminilake geminilakenk "
               "grantley kvmx64 purley r1000 r1000nk skylaked v1000 v1000nk",
    },
    "aarch64": {
        "platform": "linux/arm64", "machine": 183, "bits": 64,
        "dsm": "armv8 aarch64 rtd1296 rtd1619 rtd1619b armada37xx",
    },
    "armv7": {
        "platform": "linux/arm/v7", "machine": 40, "bits": 32,
        "dsm": "armv7 alpine alpine4k armada370 armada375 armada38x armadaxp comcerto2k monaco",
    },
}

SCRIPTS = ["start-stop-status", "preinst", "postinst", "preuninst", "postuninst", "preupgrade", "postupgrade"]
TOP_FILES = ["PACKAGE_ICON.PNG", "PACKAGE_ICON_256.PNG", "conf/privilege", "conf/resource", "conf/holyarrs.sc",
             "WIZARD_UIFILES/install_uifile", "WIZARD_UIFILES/upgrade_uifile.sh"] + ["scripts/" + s for s in SCRIPTS]
RUNNABLE = {"WIZARD_UIFILES/upgrade_uifile.sh"} | {"scripts/" + s for s in SCRIPTS}
REQUIRED = ["INFO", "package.tgz", "LICENSE"] + TOP_FILES
INFO_KEYS = ["package", "version", "os_min_ver", "arch", "displayname", "description", "maintainer",
             "adminport", "thirdparty", "startable", "checksum"]
VERSION_RE = re.compile(r"^\d+\.\d+\.\d+$")


class BuildError(Exception):
    pass


def source_bytes(name):
    """A package source file, with LF line ends whatever the checkout did."""
    data = (SOURCES / name).read_bytes()
    if name.endswith(".PNG"):
        return data
    return data.replace(b"\r\n", b"\n")


def add(tar, name, data, mode, mtime):
    info = tarfile.TarInfo(name)
    info.size = len(data)
    info.mode = mode
    info.mtime = mtime
    info.uid = info.gid = 0
    info.uname = info.gname = "root"
    tar.addfile(info, io.BytesIO(data))


def add_dir(tar, name, mtime):
    info = tarfile.TarInfo(name)
    info.type = tarfile.DIRTYPE
    info.mode = 0o755
    info.mtime = mtime
    info.uid = info.gid = 0
    info.uname = info.gname = "root"
    tar.addfile(info)


def package_tgz(binary, licence, mtime):
    """The app itself: what DSM unpacks into the package's target folder."""
    raw = io.BytesIO()
    with gzip.GzipFile(fileobj=raw, mode="wb", mtime=0) as gz:
        with tarfile.open(fileobj=gz, mode="w", format=tarfile.GNU_FORMAT) as tar:
            add_dir(tar, "bin", mtime)
            add(tar, "bin/holyarrs", binary, 0o755, mtime)
            # The port file sits here too: DSM reads conf/resource's
            # protocol-file from the target folder.
            add_dir(tar, "conf", mtime)
            add(tar, "conf/holyarrs.sc", source_bytes("conf/holyarrs.sc"), 0o644, mtime)
            add(tar, "LICENSE", licence, 0o644, mtime)
    return raw.getvalue()


def info_text(version, build, arch, checksum):
    text = source_bytes("INFO.template").decode("utf-8")
    values = {"version": "%s-%04d" % (version, build), "arch": ARCHES[arch]["dsm"], "checksum": checksum}
    for key, value in values.items():
        text = text.replace("{{%s}}" % key, value)
    return text.encode("utf-8")


def build_spk(binary, licence, version, arch, out_dir, build=1, mtime=None):
    """Write holyarrs-<version>-<arch>.spk into out_dir and return its path."""
    if not VERSION_RE.match(version):
        raise BuildError("not a version: %r (want 1.2.3)" % version)
    if arch not in ARCHES:
        raise BuildError("unknown arch %r (want one of %s)" % (arch, ", ".join(ARCHES)))
    problem = elf_problem(binary, arch)
    if problem:
        raise BuildError(problem)
    mtime = int(time.time()) if mtime is None else mtime
    tgz = package_tgz(binary, licence, mtime)
    out_dir = Path(out_dir)
    out_dir.mkdir(parents=True, exist_ok=True)
    path = out_dir / ("%s-%s-%s.spk" % (PACKAGE, version, arch))
    with tarfile.open(path, mode="w", format=tarfile.GNU_FORMAT) as tar:
        add(tar, "INFO", info_text(version, build, arch, hashlib.md5(tgz).hexdigest()), 0o644, mtime)
        add(tar, "package.tgz", tgz, 0o644, mtime)
        add(tar, "LICENSE", licence, 0o644, mtime)
        made = set()
        for name in TOP_FILES:
            folder = name.rpartition("/")[0]
            if folder and folder not in made:
                add_dir(tar, folder, mtime)
                made.add(folder)
            add(tar, name, source_bytes(name), 0o755 if name in RUNNABLE else 0o644, mtime)
    return path


def elf_problem(data, arch):
    """Why these bytes are not a Linux program for arch, or an empty string."""
    want = ARCHES[arch]
    if len(data) < 20 or data[:4] != b"\x7fELF":
        return "the program is not an ELF file"
    bits = {1: 32, 2: 64}.get(data[4])
    machine = struct.unpack("<H", data[18:20])[0]
    if bits != want["bits"] or machine != want["machine"]:
        return "the program is built for another CPU (ELF machine %d, %s-bit), not %s" % (machine, bits, arch)
    return ""


def parse_info(text):
    out = {}
    for line in text.splitlines():
        m = re.match(r'^([a-z_]+)="(.*)"$', line)
        if m:
            out[m.group(1)] = m.group(2)
    return out


def png_size(data):
    if data[:8] != b"\x89PNG\r\n\x1a\n" or data[12:16] != b"IHDR":
        return None
    return struct.unpack(">II", data[16:24])


def validate_spk(path, version=None, arch=None):
    """Check a .spk's structure. Returns a list of problems; empty means good."""
    problems = []
    try:
        with tarfile.open(path, mode="r:") as tar:
            members = {m.name: m for m in tar.getmembers()}
            files = {n: tar.extractfile(m).read() for n, m in members.items() if m.isfile()}
    except (tarfile.TarError, OSError) as err:
        return ["not a plain tar file: %s" % err]

    for name in REQUIRED:
        if name not in files:
            problems.append("missing %s" % name)
    if problems:
        return problems

    info = parse_info(files["INFO"].decode("utf-8", "replace"))
    for key in INFO_KEYS:
        if not info.get(key):
            problems.append("INFO has no %s" % key)
    if b"{{" in files["INFO"]:
        problems.append("INFO still has an unfilled {{...}} value")
    if info.get("package") != PACKAGE:
        problems.append("INFO package is %r, not %r" % (info.get("package"), PACKAGE))
    if not re.match(r"^\d+\.\d+\.\d+-\d{4}$", info.get("version", "")):
        problems.append("INFO version %r is not like 1.2.3-0001" % info.get("version"))
    elif version and info["version"].split("-")[0] != version:
        problems.append("INFO version %r is not %s" % (info["version"], version))
    if info.get("adminport") != PORT:
        problems.append("INFO adminport is %r, not %s" % (info.get("adminport"), PORT))
    if info.get("checksum") != hashlib.md5(files["package.tgz"]).hexdigest():
        problems.append("INFO checksum does not match package.tgz")
    names = info.get("arch", "").split()
    if arch and arch not in ARCHES:
        problems.append("unknown arch %r" % arch)
        arch = None
    if arch and names != ARCHES[arch]["dsm"].split():
        problems.append("INFO arch is not the %s list" % arch)

    for name in sorted(RUNNABLE):
        data = files[name]
        if not data.startswith(b"#!/bin/sh\n"):
            problems.append("%s does not start with #!/bin/sh" % name)
        if b"\r" in data:
            problems.append("%s has Windows line ends" % name)
        if not members[name].mode & 0o100:
            problems.append("%s is not runnable" % name)

    def load(name):
        try:
            return json.loads(files[name].decode("utf-8"))
        except ValueError as err:
            problems.append("%s is not valid JSON: %s" % (name, err))
            return None

    privilege = load("conf/privilege")
    if privilege is not None and (privilege.get("defaults") or {}).get("run-as") != "package":
        problems.append("conf/privilege does not run as the package user")
    resource = load("conf/resource")
    if resource is not None:
        if (resource.get("port-config") or {}).get("protocol-file") != "conf/holyarrs.sc":
            problems.append("conf/resource does not point at conf/holyarrs.sc")
        shares = (resource.get("data-share") or {}).get("shares") or []
        if not any(s.get("name") == "{{wizard_media_share}}" and PACKAGE in (s.get("permission") or {}).get("rw", [])
                   for s in shares if isinstance(s, dict)):
            problems.append("conf/resource does not give the package read and write on the wizard's shared folder")
    wizard = load("WIZARD_UIFILES/install_uifile")
    if wizard is not None and '"wizard_media_share"' not in json.dumps(wizard):
        problems.append("the install wizard does not ask for wizard_media_share")
    if ('dst.ports="%s/tcp"' % PORT).encode() not in files["conf/holyarrs.sc"]:
        problems.append("conf/holyarrs.sc does not open %s/tcp" % PORT)

    for name, side in (("PACKAGE_ICON.PNG", 64), ("PACKAGE_ICON_256.PNG", 256)):
        if png_size(files[name]) != (side, side):
            problems.append("%s is not a %dx%d PNG" % (name, side, side))

    try:
        with tarfile.open(fileobj=io.BytesIO(files["package.tgz"]), mode="r:gz") as tar:
            inner = {m.name: m for m in tar.getmembers()}
            program = tar.extractfile(inner["bin/holyarrs"]).read() if "bin/holyarrs" in inner else None
            port_file = tar.extractfile(inner["conf/holyarrs.sc"]).read() if "conf/holyarrs.sc" in inner else None
    except (tarfile.TarError, OSError) as err:
        return problems + ["package.tgz is not a gzip tar: %s" % err]
    if program is None:
        problems.append("package.tgz has no bin/holyarrs")
    else:
        if not inner["bin/holyarrs"].mode & 0o100:
            problems.append("bin/holyarrs is not runnable")
        if arch and elf_problem(program, arch):
            problems.append(elf_problem(program, arch))
    if port_file != files["conf/holyarrs.sc"]:
        problems.append("package.tgz has no matching conf/holyarrs.sc")
    return problems


def docker(*args):
    done = subprocess.run(["docker", *args], capture_output=True, text=True)
    if done.returncode != 0:
        raise BuildError("docker %s failed: %s" % (" ".join(args[:2]), done.stderr.strip() or done.stdout.strip()))
    return done.stdout.strip()


def from_image(image, version, arch):
    """Copy /holyarrs and /LICENSE out of the published image for arch."""
    ref = "%s:%s" % (image, version)
    platform = ARCHES[arch]["platform"]
    docker("pull", "--quiet", "--platform", platform, ref)
    # The image is never started; the command is only a placeholder.
    container = docker("create", "--platform", platform, ref, "-healthcheck")
    try:
        with tempfile.TemporaryDirectory() as tmp:
            out = {}
            for name in ("holyarrs", "LICENSE"):
                docker("cp", "%s:/%s" % (container, name), str(Path(tmp) / name))
                out[name] = (Path(tmp) / name).read_bytes()
    finally:
        subprocess.run(["docker", "rm", "-f", container], capture_output=True)
    return out["holyarrs"], out["LICENSE"]


def write_sums(out_dir, version):
    out_dir = Path(out_dir)
    lines = []
    for spk in sorted(out_dir.glob("%s-%s-*.spk" % (PACKAGE, version))):
        lines.append("%s  %s\n" % (hashlib.sha256(spk.read_bytes()).hexdigest(), spk.name))
    path = out_dir / ("%s-%s-synology-SHA256SUMS" % (PACKAGE, version))
    path.write_bytes("".join(lines).encode("ascii"))
    return path


def main(argv=None):
    ap = argparse.ArgumentParser(description="Build the experimental HolyArrs Synology package.")
    ap.add_argument("--version", required=True, help="a published version, such as 1.0.19")
    ap.add_argument("--arch", default="all", choices=["all", *ARCHES], help="CPU type (default: all three)")
    ap.add_argument("--output", required=True, help="folder for the .spk files")
    ap.add_argument("--image", default=IMAGE, help="Docker Hub image to copy the program from")
    ap.add_argument("--build", type=int, default=1, help="package build number, raised when the same version is repacked")
    ap.add_argument("--binary", help="use this program file, not the image (one arch only; needs --licence)")
    ap.add_argument("--licence", help="licence file to put in the package, with --binary")
    args = ap.parse_args(argv)

    arches = list(ARCHES) if args.arch == "all" else [args.arch]
    try:
        if not VERSION_RE.match(args.version):
            raise BuildError("not a version: %r (want 1.2.3)" % args.version)
        if args.binary and (len(arches) != 1 or not args.licence):
            raise BuildError("--binary needs one --arch and --licence")
        for arch in arches:
            if args.binary:
                binary, licence = Path(args.binary).read_bytes(), Path(args.licence).read_bytes()
            else:
                binary, licence = from_image(args.image, args.version, arch)
            path = build_spk(binary, licence, args.version, arch, args.output, build=args.build)
            problems = validate_spk(path, args.version, arch)
            if problems:
                path.unlink()
                raise BuildError("%s failed its check:\n  %s" % (path.name, "\n  ".join(problems)))
            print("%s  %d bytes  checked" % (path.name, path.stat().st_size))
        print(write_sums(args.output, args.version).name)
    except BuildError as err:
        print("error: %s" % err, file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
