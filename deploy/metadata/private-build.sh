#!/bin/sh
set -eu
umask 077
release="${1:?release ID required}"
case "$release" in *[!a-z0-9-]*|'') echo 'invalid release'; exit 1;; esac
case "$release" in
  20261015a) release_folder=2026-10-15-build-1 ;;
  20261015b) release_folder=2026-10-15-build-2 ;;
  20261015c) release_folder=2026-10-15-build-3 ;;
  20261015d) release_folder=2026-10-15-build-4 ;;
  20261015e) release_folder=2026-10-15-build-5 ;;
  20261015f) release_folder=2026-10-15-build-6 ;;
  20261015g) release_folder=2026-10-15-build-7 ;;
  20261015h) release_folder=2026-10-15-build-8 ;;
  *) echo 'unknown release'; exit 1 ;;
esac
root=/home/home/holyarrs-metadata-server
source="$root/application/versions/$release_folder/source"
build="$root/application/builds/$release_folder"
evidence="$root/results/history/$release_folder"
dependency_cache="$root/application/build-cache/dependencies"
compiler_cache="$root/application/build-cache/compiler"
available=$(df --output=avail -B1 / | tail -1)
[ "$available" -ge 26843545600 ] || { echo 'low disk reserve'; exit 1; }
[ -d "$source/services/metadata" ] || { echo 'missing sanitized source'; exit 1; }
[ ! -e "$build" ] || { echo 'build release already exists; use a new release ID'; exit 1; }
[ ! -e "$evidence/native-build-tests.txt" ] || { echo 'build evidence already exists'; exit 1; }
mkdir -p "$build/context" "$evidence" "$dependency_cache" "$compiler_cache"
builder_ref=golang@sha256:820153096fda4de6ac1b7194d12e416b7dd8f4e9a33d17e0345b111b945162d8
docker pull --platform linux/arm64 "$builder_ref" > "$evidence/builder-pull.txt" 2>&1
builder=$(docker image inspect "$builder_ref" --format '{{.Id}}')
[ "$(docker image inspect "$builder_ref" --format '{{.Architecture}}')" = arm64 ] || exit 1
docker image inspect "$builder_ref" --format '{{.Id}} {{.Architecture}} {{.Size}} {{json .RepoDigests}}' > "$evidence/builder-identity.txt"
date -u +%FT%TZ > "$evidence/build-start.txt"
docker run --rm --name "metadata-build-$release" --cpus 2 --memory 4g --pids-limit 256 --label com.centurylinklabs.watchtower.enable=false \
 -e CGO_ENABLED=1 -e GOTOOLCHAIN=local \
 -v "$source/services/metadata:/work:ro" -v "$dependency_cache:/go/pkg/mod" -v "$compiler_cache:/root/.cache/go-build" -v "$build/context:/out" \
 -w /work "$builder" sh -c 'set -eu; go version; go mod download; go mod verify; go test -mod=readonly -race -count=1 -timeout=3m ./...; go vet -mod=readonly ./...; CGO_ENABLED=0 go build -mod=readonly -trimpath -ldflags="-s -w" -o /out/metadata ./cmd/metadata; CGO_ENABLED=0 go build -mod=readonly -trimpath -ldflags="-s -w" -o /out/fixture ./tests/fixture; cp /etc/ssl/certs/ca-certificates.crt /out/ca-certificates.crt; mkdir -p /out/rootfs/etc/ssl/certs /out/fixturefs; cp /out/metadata /out/rootfs/metadata; cp /out/ca-certificates.crt /out/rootfs/etc/ssl/certs/; cp /out/fixture /out/fixturefs/fixture; tar -C /out/rootfs -cf /out/metadata-rootfs.tar .; tar -C /out/fixturefs -cf /out/fixture-rootfs.tar .' > "$evidence/native-build-tests.txt" 2>&1
cp "$source/deploy/metadata/Dockerfile" "$source/deploy/metadata/fixture.Dockerfile" "$build/context/"
docker image import --platform linux/arm64 --change 'USER 10001:10001' --change 'ENV GOMEMLIMIT=768MiB' --change 'ENTRYPOINT ["/metadata"]' "$build/context/metadata-rootfs.tar" "holyarrs-metadata:private-$release" > "$evidence/image-build.txt" 2>&1
docker image import --platform linux/arm64 --change 'USER 10001:10001' --change 'ENTRYPOINT ["/fixture"]' "$build/context/fixture-rootfs.tar" "holyarrs-metadata-fixture:private-$release" > "$evidence/fixture-image-build.txt" 2>&1
docker image inspect "holyarrs-metadata:private-$release" "holyarrs-metadata-fixture:private-$release" --format '{{.Id}} {{.Architecture}} {{.Size}}' > "$evidence/image-identities.txt"
sha256sum "$build/context/metadata" "$build/context/fixture" > "$evidence/binary-sha256.txt"
wc -c "$build/context/metadata" "$build/context/fixture" > "$evidence/binary-sizes.txt"
date -u +%FT%TZ > "$evidence/build-end.txt"
echo 'native private build complete'
