"""Re-squeeze the big gzip layers of a built image with a stronger gzip tool.

    python3 deploy/release/resqueeze.py <oci-layout-folder>

<oci-layout-folder> is an image built with
`docker buildx build --provenance=false --sbom=false --output type=oci,dest=<folder>,tar=false`.

Docker's own gzip is weak. The same layer packed by 7-Zip's gzip is about
0.7 MB smaller for HolyArrs and is still ordinary gzip, so every Docker
version pulls it as before. The unpacked layer stays byte for byte the same
(the image's diff IDs do not change); only the packed copy, and so the
manifest and index digests, change. Without 7-Zip on PATH the script falls
back to Python's gzip at level 9, which gains about 0.3 MB.

It refuses an index that carries attestations: they name the image by its
old digest and would no longer match. Attest and sign after this step.
"""

from __future__ import annotations

import gzip
import hashlib
import json
import pathlib
import shutil
import subprocess
import sys
import tempfile

BIG = 1_000_000  # layers under 1 MB are left alone
GZIP_TYPES = ("application/vnd.oci.image.layer.v1.tar+gzip", "application/vnd.docker.image.rootfs.diff.tar.gzip")


def sha(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def strong_gzip(tar: bytes) -> bytes:
    """tar packed as small as the tools here can make a plain gzip stream."""
    best = gzip.compress(tar, compresslevel=9, mtime=0)
    tool = shutil.which("7z") or shutil.which("7zz")
    if tool:
        with tempfile.TemporaryDirectory() as tmp:
            src = pathlib.Path(tmp) / "layer.tar"
            out = pathlib.Path(tmp) / "layer.tar.gz"
            src.write_bytes(tar)
            subprocess.run([tool, "a", "-tgzip", "-mx=9", "-mfb=258", "-mpass=15", "-bso0", "-bsp0", str(out), str(src)], check=True)
            packed = out.read_bytes()
        if len(packed) < len(best):
            best = packed
    return best


def main() -> int:
    root = pathlib.Path(sys.argv[1])
    blobs = root / "blobs" / "sha256"
    index_path = root / "index.json"
    index = json.loads(index_path.read_text())

    def load(digest: str) -> bytes:
        return (blobs / digest.removeprefix("sha256:")).read_bytes()

    def store(data: bytes) -> str:
        digest = sha(data)
        (blobs / digest).write_bytes(data)
        return "sha256:" + digest

    def squeeze_manifest(desc: dict) -> dict:
        """desc with its manifest's big gzip layers re-squeezed; desc itself if nothing got smaller."""
        manifest = json.loads(load(desc["digest"]))
        changed = False
        for layer in manifest.get("layers", []):
            if layer["mediaType"] not in GZIP_TYPES or layer["size"] < BIG:
                continue
            old = load(layer["digest"])
            tar = gzip.decompress(old)
            new = strong_gzip(tar)
            if gzip.decompress(new) != tar:
                raise SystemExit("the re-squeezed layer does not unpack to the same bytes")
            if len(new) >= len(old):
                continue
            print(f"layer {layer['digest'][7:19]}: {len(old) / 1e6:.3f} MB -> {len(new) / 1e6:.3f} MB")
            layer["digest"], layer["size"] = store(new), len(new)
            changed = True
        if not changed:
            return desc
        data = json.dumps(manifest, separators=(",", ":")).encode()
        return {**desc, "digest": store(data), "size": len(data)}

    def squeeze(desc: dict) -> dict:
        if "vnd.docker.reference.type" in desc.get("annotations", {}):
            raise SystemExit("the image carries attestations; build with --provenance=false --sbom=false and attest after this step")
        body = json.loads(load(desc["digest"]))
        if "manifests" not in body:
            return squeeze_manifest(desc)
        inner = [squeeze(m) for m in body["manifests"]]
        if inner == body["manifests"]:
            return desc
        body["manifests"] = inner
        data = json.dumps(body, separators=(",", ":")).encode()
        return {**desc, "digest": store(data), "size": len(data)}

    index["manifests"] = [squeeze(m) for m in index["manifests"]]
    index_path.write_text(json.dumps(index, separators=(",", ":")))
    return 0


if __name__ == "__main__":
    sys.exit(main())
