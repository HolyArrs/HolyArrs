"""Build portable candidates from committed source; never install or publish."""
from __future__ import annotations

import argparse
import gzip
import hashlib
import io
import json
import os
from pathlib import Path, PurePosixPath
import re
import shutil
import subprocess
import tarfile
import tempfile
import zipfile

ROOT = Path(__file__).resolve().parents[2]
GO_IMAGE = "golang:1.27.3@sha256:e519d3b0a5b285e5aa53ab9eef02bc6bbb9cb04608389b743a136a9ce75f0d1e"
TARGETS = {"linux-amd64": ("linux", "amd64", ""),
           "linux-arm64": ("linux", "arm64", ""),
           "linux-armv7": ("linux", "arm", "7"),
           "windows-amd64": ("windows", "amd64", "")}
SOURCE_DIRS = {"app", "deploy", "scripts", "docs", ".github"}
SOURCE_FILES = {"go.mod", "go.sum", "LICENSE", "README.md", "SECURITY.md",
                "Makefile", ".gitignore", ".gitattributes", ".dockerignore"}


def digest(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def encoded(value: object) -> bytes:
    return (json.dumps(value, indent=2, sort_keys=True) + "\n").encode()


def run(args: list[str], cwd: Path = ROOT, env: dict | None = None) -> bytes:
    result = subprocess.run(args, cwd=cwd, env=env, capture_output=True)
    if result.returncode:
        raise RuntimeError(f"{args[0]} failed ({result.returncode}): " + result.stderr.decode(errors="replace"))
    return result.stdout


def checked_version(value: str) -> str:
    if not re.fullmatch(r"[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?", value):
        raise ValueError("version must be a filename-safe semantic version without a v prefix")
    return value


def archive(path: Path, members: dict[str, tuple[bytes, int]], epoch: int) -> None:
    for name in members:
        member = PurePosixPath(name)
        if member.is_absolute() or ".." in member.parts or "\\" in name or ":" in name:
            raise ValueError("unsafe archive member")
    if path.suffix == ".zip":
        with zipfile.ZipFile(path, "w", compression=zipfile.ZIP_DEFLATED, compresslevel=9) as out:
            for name, (data, mode) in sorted(members.items()):
                info = zipfile.ZipInfo(name, (1980, 1, 1, 0, 0, 0))
                info.create_system = 3
                info.external_attr = (0o100000 | mode) << 16
                info.compress_type = zipfile.ZIP_DEFLATED
                out.writestr(info, data)
        return
    with path.open("wb") as raw, gzip.GzipFile(filename="", fileobj=raw, mode="wb", mtime=epoch) as gz:
        with tarfile.open(fileobj=gz, mode="w", format=tarfile.USTAR_FORMAT) as out:
            for name, (data, mode) in sorted(members.items()):
                info = tarfile.TarInfo(name)
                info.size, info.mode, info.mtime = len(data), mode, epoch
                out.addfile(info, io.BytesIO(data))


def snapshot(ref: str, destination: Path) -> tuple[str, int, dict[str, int]]:
    commit = run(["git", "rev-parse", "--verify", "--end-of-options", ref + "^{commit}"]).decode().strip()
    epoch = int(run(["git", "show", "-s", "--format=%ct", commit]))
    data = run(["git", "archive", "--format=tar", commit])
    modes = {}
    with tarfile.open(fileobj=io.BytesIO(data)) as source:
        for member in source:
            p = PurePosixPath(member.name)
            if p.parts[0] not in SOURCE_DIRS and member.name not in SOURCE_FILES:
                continue
            if not member.isfile():
                if member.isdir():
                    continue
                raise ValueError("source snapshot contains a link or special file")
            if p.is_absolute() or ".." in p.parts:
                raise ValueError("unsafe source path")
            target = destination.joinpath(*p.parts)
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_bytes(source.extractfile(member).read())
            target.chmod(member.mode & 0o777)
            modes[member.name] = 0o755 if member.mode & 0o111 else 0o644
    return commit, epoch, modes


def build_ui(source: Path, node_modules: Path) -> Path:
    """Builds the web app from the snapshot and returns the finished dist folder.

    The build runs in a copy, so the source archive never holds installed
    packages or build output.
    """
    work = source.parent / "ui"
    shutil.copytree(source / "app/frontend", work, ignore=shutil.ignore_patterns("dist", "node_modules", ".svelte-kit"))
    shutil.copytree(node_modules, work / "node_modules")
    npm = "npm.cmd" if os.name == "nt" else "npm"
    run([npm, "run", "build"], cwd=work)
    return work / "dist"


def web_notices(source: Path, node_modules: Path) -> bytes:
    lock = json.loads((source / "app/frontend/package-lock.json").read_bytes())
    result = []
    required = {"svelte", "@sveltejs/kit", "esm-env", "devalue", "@fontsource-variable/geist",
                "@fontsource-variable/geist-mono", "@fontsource-variable/sora"}
    for location, info in sorted(lock["packages"].items()):
        if not location:
            continue
        name = location.removeprefix("node_modules/")
        package = node_modules.parent / location
        if not package.is_dir():
            if name in required or not info.get("optional", False):
                raise ValueError(f"missing bundled web dependency: {name}")
            continue
        for parent in (package, *package.parents):
            if parent.is_symlink() or os.path.isjunction(parent):
                raise ValueError(f"linked dependency path is refused: {name}")
        expected = info["version"]
        actual = json.loads((package / "package.json").read_bytes())["version"]
        if actual != expected:
            raise ValueError(f"web notice version differs from lock: {name}")
        licenses = sorted(p for p in package.iterdir() if p.is_file() and re.match(r"(?i)^(license|copying|notice|ofl)(?:\.|$)", p.name))
        if not licenses and name in required:
            raise ValueError(f"missing bundled web license: {name}")
        result.append(f"\n{name} {expected}; declared license: {info.get('license', 'see package source')}\n".encode())
        for p in licenses:
            if p.is_symlink() or os.path.isjunction(p):
                raise ValueError(f"linked license is refused: {name}")
            result.extend([p.name.encode() + b"\n", p.read_bytes(), b"\n"])
    return b"".join(result)


def build(args: argparse.Namespace) -> None:
    version = checked_version(args.version)
    targets = args.targets.split(",")
    if len(set(targets)) != len(targets) or any(t not in TARGETS for t in targets):
        raise ValueError("targets must be unique supported target names")
    output = Path(args.output).resolve()
    if output.exists():
        raise ValueError("output already exists; choose a new directory")
    if ROOT == output or ROOT in output.parents:
        raise ValueError("output must be outside the checkout")
    output.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix="holyarrs-package-", dir=output.parent) as temporary:
        temp = Path(temporary)
        source, result = temp / "source", temp / "result"
        source.mkdir()
        result.mkdir()
        commit, epoch, modes = snapshot(args.ref, source)
        version = version + "+g" + commit[:12] if "+" not in version else version + ".g" + commit[:12]
        limits = (["--cpus", "1", "--memory", "2g", "--memory-swap", "2g", "--pids-limit", "256"]
                  if args.low_resource else [])
        go_environment = (["-e", "GOMAXPROCS=1", "-e", "GOMEMLIMIT=768MiB", "-e", "GOGC=50",
                           "-e", "GOFLAGS=-p=1"] if args.low_resource else [])

        def go(command: list[str], target: tuple[str, str, str] = ("linux", "amd64", "")) -> bytes:
            goos, goarch, goarm = target
            return run(["docker", "run", "--rm", "--pull", "never", "--network", "none",
                        *limits, *go_environment,
                        "-v", f"{source}:/src", "-v", f"{args.module_cache}:/go/pkg/mod:ro",
                        "-v", "holyarrs-packaging-gocache:/root/.cache/go-build",
                        "-w", "/src", "-e", "GOTOOLCHAIN=local", "-e", "GOPROXY=off",
                        "-e", "GOSUMDB=off", "-e", "CGO_ENABLED=0", "-e", f"GOOS={goos}",
                        "-e", f"GOARCH={goarch}", "-e", f"GOARM={goarm}",
                        "-e", "GOAMD64=v1", "-e", "GOARM64=v8.0", GO_IMAGE, *command])

        compiler = go(["go", "version"]).decode().strip()
        print(compiler, flush=True)
        go(["go", "mod", "verify"])
        go(["go", "mod", "vendor"])
        notices = go(["cat", "/usr/local/go/LICENSE"])
        notices += web_notices(source, Path(os.path.abspath(args.web_node_modules)))
        for p in sorted((source / "vendor").rglob("*")):
            if p.is_file() and re.match(r"(?i)^(license|copying|notice)(?:\.|$)", p.name):
                notices += b"\n" + p.relative_to(source).as_posix().encode() + b"\n" + p.read_bytes() + b"\n"
        (source / "THIRD-PARTY-NOTICES.txt").write_bytes(notices)
        files = {}
        for p in sorted(source.rglob("*")):
            if p.is_file():
                name = p.relative_to(source).as_posix()
                files[name] = (p.read_bytes(), modes.get(name, 0o644))
        manifest = encoded({"commit": commit, "tree": run(["git", "rev-parse", commit + "^{tree}"]).decode().strip(),
                            "files": [{"path": p, "sha256": digest(d), "size": len(d), "mode": mode}
                                      for p, (d, mode) in sorted(files.items())]})
        files["source-manifest.json"] = manifest, 0o644
        source_name = f"holyarrs-{version}-source.tar.gz"
        archive(result / source_name, files, epoch)
        source_hash = digest((result / source_name).read_bytes())
        # The pages are built after the source archive, which holds committed files only.
        shutil.copytree(build_ui(source, Path(os.path.abspath(args.web_node_modules))),
                        source / "app/frontend/dist", dirs_exist_ok=True)
        for target in targets:
            print("Building " + target, flush=True)
            executable = "holyarrs.exe" if TARGETS[target][0] == "windows" else "holyarrs"
            go(["go", "build", "-mod=vendor", "-trimpath", "-buildvcs=false",
                f"-ldflags=-s -w -buildid= -X main.version={version}", "-o", "/src/" + executable,
                "./app/backend/cmd/holyarrs"], TARGETS[target])
            binary = (source / executable).read_bytes()
            build_info = go(["go", "version", "-m", "/src/" + executable])
            if version.encode() not in binary:
                raise ValueError("binary is missing the candidate version string")
            identity = {"version": version, "commit": commit, "target": target, "compiler": compiler,
                        "builder": GO_IMAGE, "cgo_enabled": False, "binary_sha256": digest(binary),
                        "source_archive": source_name, "source_sha256": source_hash,
                        "source_manifest_sha256": digest(manifest), "native_validation": "separate receipt required",
                        "source_scope": "project and vendored Go modules; frontend dependency source is external",
                        "target_tuple": TARGETS[target], "build_flags": ["-mod=vendor", "-trimpath", "-buildvcs=false",
                        "-ldflags=-s -w -buildid= -X main.version=" + version]}
            members = {executable: (binary, 0o755), "LICENSE": files["LICENSE"],
                       "THIRD-PARTY-NOTICES.txt": (notices, 0o644),
                       "source-manifest.json": (manifest, 0o644), "package.json": (encoded(identity), 0o644),
                       "build-info.txt": (build_info, 0o644)}
            members.update({p: value for p, value in files.items() if p.startswith("docs/")})
            suffix = "zip" if TARGETS[target][0] == "windows" else "tar.gz"
            archive(result / f"holyarrs-{version}-{target}.{suffix}", members, epoch)
            (source / executable).unlink()
        checksums = "".join(f"{digest(p.read_bytes())}  {p.name}\n" for p in sorted(result.iterdir()))
        (result / "SHA256SUMS").write_text(checksums, encoding="utf-8", newline="\n")
        result.rename(output)
    print(f"Packaged {commit}: {', '.join(targets)}")


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--ref", required=True, help="committed source revision")
    parser.add_argument("--version", required=True)
    parser.add_argument("--output", required=True, help="new directory outside the checkout")
    parser.add_argument("--web-node-modules", required=True, help="locked installed Svelte/font packages for their licenses")
    parser.add_argument("--module-cache", required=True, help="complete Go module cache path or Docker volume, mounted read-only")
    parser.add_argument("--targets", default=",".join(TARGETS))
    parser.add_argument("--low-resource", action="store_true", help="limit each builder to one CPU and 2 GiB")
    args = parser.parse_args()
    try:
        build(args)
    except (ValueError, RuntimeError, OSError, KeyError) as error:
        parser.exit(1, f"package: {error}\n")


if __name__ == "__main__":
    main()
