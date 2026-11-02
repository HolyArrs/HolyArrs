#!/bin/sh
# Zero-downtime swap of the metadata server to a built image tag:
#
#   ./swap.sh live-20261027a
#
# 1. A stand-in container on the new image joins the tunnel network under
#    the same name (holyarrs-metadata). It listens at once and holds what
#    reaches it while it waits for the store.
# 2. The old container is stopped: it drains its own requests and exits,
#    which hands the store to the stand-in; the tunnel's next connections
#    find the stand-in.
# 3. Compose's container is made again on the new image the same way, and
#    the stand-in is stopped.
#
# Each container's log is saved to logs/ before it goes (newest 30 kept),
# so a swap no longer loses the old process's record.
#
# The stand-in's settings mirror compose.yaml; keep the two the same.
set -eu
cd /home/home/HolyArrs-Metadata
NEW=$1
docker image inspect "holyarrs-metadata:$NEW" >/dev/null
docker tag "holyarrs-metadata:$NEW" holyarrs-metadata:live

healthy() {
	i=0
	while [ "$(docker inspect -f '{{.State.Health.Status}}' "$1" 2>/dev/null)" != healthy ]; do
		i=$((i + 1))
		if [ $i -gt 150 ]; then
			echo "$1 not healthy after 150 s" >&2
			return 1
		fi
		sleep 1
	done
}

# keeplog NAME: save NAME's docker log, gzipped, under logs/; never fails.
keeplog() {
	mkdir -p logs
	docker logs -t "$1" 2>&1 | gzip >"logs/$(date -u +%Y%m%dT%H%M%SZ)-$1-before-$NEW.log.gz" || true
}

EGRESS=""
[ -f egress.env ] && EGRESS="--env-file egress.env"
docker rm -f holyarrs-metadata-next >/dev/null 2>&1 || true
# shellcheck disable=SC2086
docker run -d --name holyarrs-metadata-next \
	--env-file .env $EGRESS -e GOMEMLIMIT=12GiB -e HOLYARRS_MB_SLIM_ROOT=/music \
	--memory 16g --cpus 2.0 --pids-limit 256 --read-only \
	--tmpfs /tmp:size=256m,exec,mode=1777 \
	--security-opt no-new-privileges:true --cap-drop ALL \
	--label com.centurylinklabs.watchtower.enable=false \
	-v holyarrs-metadata_metadata-data:/data \
	-v "$PWD/secrets/apple_music.p8:/run/secrets/apple_music.p8:ro" \
	-v /home/home/music-next:/music:ro \
	--network holycore-bot_bot-network --network-alias holyarrs-metadata --network-alias metadata \
	--health-cmd "/metadata healthcheck" --health-interval 30s --health-timeout 5s --health-retries 3 \
	--health-start-period 2m --health-start-interval 1s \
	--log-driver json-file --log-opt max-size=10m --log-opt max-file=3 \
	--stop-timeout 30 \
	holyarrs-metadata:live >/dev/null
sleep 1
t0=$(date +%s)
docker stop -t 30 holyarrs-metadata >/dev/null
healthy holyarrs-metadata-next
echo "stand-in serving after $(( $(date +%s) - t0 )) s"
# Stopped but not yet removed: its log is still there.
keeplog holyarrs-metadata
# Compose's container starts on the new image and waits for the store.
docker compose up -d --no-deps --force-recreate metadata >/dev/null
sleep 1
docker stop -t 30 holyarrs-metadata-next >/dev/null
healthy holyarrs-metadata
keeplog holyarrs-metadata-next
docker rm holyarrs-metadata-next >/dev/null
ls -1t logs/*.log.gz 2>/dev/null | tail -n +31 | xargs -r rm -f -- || true
echo "holyarrs-metadata on $NEW, healthy after $(( $(date +%s) - t0 )) s"
