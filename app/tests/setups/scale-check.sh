#!/usr/bin/env bash
set -euo pipefail

artifact_dir="${1:?usage: scale-check.sh ARTIFACT_DIR}"
mkdir -p "$artifact_dir"

test_binary="$artifact_dir/holyarrs-scale.test"
dockerfile="$artifact_dir/Dockerfile.scale"
log_file="$artifact_dir/scale-combined.log"
result_file="$artifact_dir/scale-results.jsonl"
status_file="$artifact_dir/scale-exit-code.txt"
image="holyarrs-scale-ci:${GITHUB_SHA:-local}"

CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go test -c -o "$test_binary" ./app/backend/cmd/holyarrs

cat >"$dockerfile" <<'EOF'
FROM scratch
COPY --chmod=0755 holyarrs-scale.test /holyarrs-scale.test
WORKDIR /tmp
ENTRYPOINT ["/holyarrs-scale.test"]
EOF

# Userspace QEMU keeps emulation local to this image, without host binfmt changes.
if [[ -n "${QEMU_AARCH64_STATIC:-}" ]]; then
  cp "$QEMU_AARCH64_STATIC" "$artifact_dir/qemu-aarch64-static"
  cat >"$dockerfile" <<'EOF'
FROM scratch
COPY --chmod=0755 qemu-aarch64-static /qemu-aarch64-static
COPY --chmod=0755 holyarrs-scale.test /holyarrs-scale.test
ENV HOLYARRS_TEST_EXEC=/qemu-aarch64-static
WORKDIR /tmp
ENTRYPOINT ["/qemu-aarch64-static", "/holyarrs-scale.test"]
EOF
fi

docker build --platform linux/arm64 --file "$dockerfile" --tag "$image" "$artifact_dir"

set +e
docker run --rm --platform linux/arm64 \
  --memory 1g --memory-swap 1g --cpus 2 --network none \
  --env GOMEMLIMIT=384MiB \
  --env HOLYARRS_SCALE=combined \
  --env HOLYARRS_SCALE_IDLE=30m \
  "$image" \
  -test.run='^TestScaleAcceptance$' -test.v -test.count=1 -test.timeout=75m \
  2>&1 | sed -E 's/(auth: first run: setup code) [^ ]+/\1 [hidden]/' | tee "$log_file"
pipeline_status=("${PIPESTATUS[@]}")
status="${pipeline_status[0]}"
for code in "${pipeline_status[@]:1}"; do
  if [[ "$code" != "0" ]]; then
    status="$code"
  fi
done
set -e

sed -n 's/^.*SCALERESULT //p' "$log_file" >"$result_file"
if ! python3 - "$result_file" <<'PY'
import json
import pathlib
import sys

path = pathlib.Path(sys.argv[1])
lines = [line for line in path.read_text().splitlines() if line.strip()]
if len(lines) != 1:
    raise SystemExit(f"expected one combined SCALERESULT, found {len(lines)}")
result = json.loads(lines[0])
expected = {
    "name": "combined",
    "titles": 10000,
    "items": 55000,
    "feeds": 20,
    "rescan_folders": 2,
    "rescan_files": 29000,
    "rescan_found": 2,
    "rescan_unreachable": 0,
    "idle_seconds": 1800,
    "live_clients": 10,
    "live_rescan_events": 10,
}
wrong = {key: (result.get(key), value) for key, value in expected.items() if result.get(key) != value}
if wrong:
    raise SystemExit(f"combined SCALERESULT has wrong required fields: {wrong}")
PY
then
  status=1
fi
printf '%s\n' "$status" >"$status_file"
exit "$status"
