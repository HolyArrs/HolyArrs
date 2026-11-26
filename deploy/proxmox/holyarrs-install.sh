#!/usr/bin/env bash
# Installs or updates HolyArrs inside a Debian or Ubuntu machine from the plain
# Linux download of a GitHub release. No Docker. Run as root.
#
#   holyarrs-install.sh install [version]
#   holyarrs-install.sh update  [version] [force]
#
# Without a version it uses the newest release. An update to an older version
# than the one installed is refused unless the last word is "force".
#
# The program goes in
# /opt/holyarrs, the database and settings in /var/lib/holyarrs, and it runs as
# the service user "holyarrs" under systemd (port 6262).
#
# HOLYARRS_DOWNLOAD_BASE replaces https://github.com/HolyArrs/HolyArrs/releases
# (for a mirror); it must answer /latest and /download/<version>/<file> the same way GitHub does.
set -euo pipefail

REPO_URL="${HOLYARRS_DOWNLOAD_BASE:-https://github.com/HolyArrs/HolyArrs/releases}"
APP_DIR="${HOLYARRS_APP_DIR:-/opt/holyarrs}"
CONFIG_DIR="${HOLYARRS_CONFIG_DIR:-/var/lib/holyarrs}"
BACKUP_DIR="${HOLYARRS_BACKUP_DIR:-/var/backups/holyarrs}"
UNIT_DIR="${HOLYARRS_UNIT_DIR:-/etc/systemd/system}"
SELF_COPY="${HOLYARRS_SELF_COPY:-/usr/local/bin/update-holyarrs}"
SYSTEMD_RUN="${HOLYARRS_SYSTEMD_RUN:-/run/systemd/system}"
SERVICE=holyarrs
USER_NAME=holyarrs
PORT=6262

say() { printf '%s\n' "$*"; }
fail() { printf 'HolyArrs install: %s\n' "$*" >&2; exit 1; }

[ "$(id -u)" = 0 ] || fail "run this as root"
command -v apt-get >/dev/null 2>&1 || fail "this needs Debian or Ubuntu (apt-get was not found)"

action="${1:-install}"
case "$action" in install | update) ;; *) fail "use: $0 install|update [version] [force]" ;; esac
wanted="${2:-}"
force="${3:-}"
if [ -n "$wanted" ] && ! printf '%s' "$wanted" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$'; then
  fail "the version must look like 1.2.3, not \"$wanted\""
fi

has_systemd() { [ -d "$SYSTEMD_RUN" ]; }

case "$(dpkg --print-architecture)" in
  amd64) arch=amd64 ;;
  arm64) arch=arm64 ;;
  armhf) arch=armv7 ;;
  *) fail "HolyArrs has downloads for amd64, arm64 and armv7 only, not $(dpkg --print-architecture)" ;;
esac

need=""
for tool in curl:curl sha256sum:coreutils tar:tar; do
  command -v "${tool%%:*}" >/dev/null 2>&1 || need="$need ${tool##*:}"
done
if [ -n "$need" ] || [ ! -e /etc/ssl/certs/ca-certificates.crt ]; then
  say "Installing $need ca-certificates"
  export DEBIAN_FRONTEND=noninteractive
  apt-get update -qq
  # shellcheck disable=SC2086
  apt-get install -y -qq --no-install-recommends ca-certificates $need >/dev/null
fi

if [ -z "$wanted" ]; then
  # The newest release is where /releases/latest redirects to; no API limit applies.
  final="$(curl -fsL -o /dev/null -w '%{url_effective}' "$REPO_URL/latest")" \
    || fail "could not reach $REPO_URL to find the newest version. Check that this machine can reach the internet, or pass a version"
  wanted="${final##*/}"
  printf '%s' "$wanted" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$' \
    || fail "could not read the newest version from $final. Pass a version, for example: $0 $action 1.1.3"
fi

if [ "$action" = install ] && [ -x "$APP_DIR/holyarrs" ]; then
  fail "HolyArrs is already installed here. To change the version run: update-holyarrs update [version]"
fi
if [ "$action" = update ] && [ ! -x "$APP_DIR/holyarrs" ]; then
  fail "HolyArrs is not installed here yet. Run: holyarrs-install.sh install"
fi

have=""
if [ "$action" = update ] && [ -f "$APP_DIR/version" ]; then
  have="$(tr -d '[:space:]' < "$APP_DIR/version")"
  printf '%s' "$have" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$' || have=""
fi
# Without the installed version the checks below cannot tell a real update from
# the same or an older version, and a wrong guess would replace the way back.
if [ "$action" = update ] && [ -z "$have" ] && [ "$force" != force ]; then
  fail "could not read which version is installed here ($APP_DIR/version is missing or not a version), so nothing was changed. If you know it, write it back and run the update again, for example: echo 1.2.3 > $APP_DIR/version. To update without it run, inside the container: update-holyarrs update $wanted force   (or on the Proxmox host: FORCE=1 VERSION=$wanted bash holyarrs-lxc.sh update <container number>). That replaces the old program kept in $APP_DIR/holyarrs.previous"
fi
# Installing the same version again must not replace the old program and the
# saved copy from the last real update: they are the way back.
same=""
if [ -n "$have" ] && [ "$have" = "$wanted" ]; then
  if [ "$force" != force ]; then
    say "HolyArrs $wanted is already installed, so nothing was changed."
    exit 0
  fi
  same=1
fi
if [ -n "$have" ] && [ -z "$same" ] && [ "$force" != force ] \
  && [ "$(printf '%s\n%s\n' "$wanted" "$have" | sort -V | head -n1)" = "$wanted" ]; then
  fail "version $wanted is older than the $have installed here, and an older program can break data a newer one changed. To go back, restore a copy from $BACKUP_DIR (see the Proxmox guide). To install it anyway run, inside the container: update-holyarrs update $wanted force   (or on the Proxmox host: FORCE=1 VERSION=$wanted bash holyarrs-lxc.sh update <container number>)"
fi

stem="holyarrs_${wanted}_linux_${arch}"
work="$(mktemp -d)"
restart=""
cleanup() {
  rm -rf "$work"
  # An update that stops half way must not leave HolyArrs stopped.
  if [ -n "$restart" ]; then systemctl start "$SERVICE" >/dev/null 2>&1 || true; fi
}
trap cleanup EXIT

say "Downloading HolyArrs $wanted ($arch)"
curl -fsL -o "$work/$stem.tar.gz" "$REPO_URL/download/$wanted/$stem.tar.gz" \
  || fail "could not download $REPO_URL/download/$wanted/$stem.tar.gz. Check the version exists on the releases page and that Linux downloads are attached (they start with 1.1.3)"
curl -fsL -o "$work/SHA256SUMS" "$REPO_URL/download/$wanted/SHA256SUMS" \
  || fail "could not download the SHA256SUMS file of version $wanted"

line="$(grep -F "  $stem.tar.gz" "$work/SHA256SUMS" || true)"
[ -n "$line" ] || fail "SHA256SUMS of $wanted has no line for $stem.tar.gz"
(cd "$work" && printf '%s\n' "$line" | sha256sum -c --status -) \
  || fail "the download does not match its checksum. Nothing was installed; run it again"
say "Checksum matches"

tar -xzf "$work/$stem.tar.gz" -C "$work"
[ -f "$work/$stem/holyarrs" ] || fail "the download does not hold the program"

if ! getent passwd "$USER_NAME" >/dev/null; then
  useradd --system --user-group --home-dir "$CONFIG_DIR" --no-create-home --shell /usr/sbin/nologin "$USER_NAME"
fi
install -d -o "$USER_NAME" -g "$USER_NAME" -m 0750 "$CONFIG_DIR"
install -d -o root -g root -m 0755 "$APP_DIR"

# The copy leaves out the picture cache, the logs and the app's own backups: they
# are big and HolyArrs makes them again (backups have their own page).
data="$(basename "$CONFIG_DIR")"
if [ "$action" = update ] && [ -z "$same" ]; then
  install -d -o root -g root -m 0700 "$BACKUP_DIR"
  need_kb="$(du -sk --exclude="$CONFIG_DIR/artwork" --exclude="$CONFIG_DIR/backups" --exclude="$CONFIG_DIR/logs" "$CONFIG_DIR" | awk '{print $1}')"
  free_kb="$(df -Pk "$BACKUP_DIR" | awk 'NR==2 {print $4}')"
  if [ "${free_kb:-0}" -lt $(( ${need_kb:-0} + 10240 )) ]; then
    fail "there is not enough room in $BACKUP_DIR to copy your settings and data first (${need_kb:-0} KB needed, ${free_kb:-0} KB free). HolyArrs was not stopped and nothing changed. Free some disk space, or make the container's disk bigger, then run it again"
  fi
fi

if has_systemd && systemctl is-active --quiet "$SERVICE" 2>/dev/null; then
  systemctl stop "$SERVICE"
  restart=1
fi

if [ -n "$same" ]; then
  say "Installing $wanted again. The saved copy and the old program from the last update are kept."
elif [ "$action" = update ]; then
  copy="$BACKUP_DIR/config-before-$wanted-$(date +%Y%m%d-%H%M%S).tar.gz"
  tar -czf "$copy" --exclude="$data/artwork" --exclude="$data/backups" --exclude="$data/logs" \
    -C "$(dirname "$CONFIG_DIR")" "$data" \
    || { rm -f "$copy"; fail "could not copy your settings and data to $copy (is the disk full?). Nothing was changed and HolyArrs is being started again"; }
  chmod 0600 "$copy"
  say "Copied your settings and data to $copy"
  # Only the newest two copies are kept, so the disk does not fill over time.
  # shellcheck disable=SC2012
  ls -1t "$BACKUP_DIR"/config-before-*.tar.gz 2>/dev/null | tail -n +3 | while read -r old; do rm -f "$old"; done
  [ -f "$APP_DIR/holyarrs" ] && cp -p "$APP_DIR/holyarrs" "$APP_DIR/holyarrs.previous"
  # The version of the old program is kept next to it, for going back.
  rm -f "$APP_DIR/version.previous"
  [ -f "$APP_DIR/version" ] && cp -p "$APP_DIR/version" "$APP_DIR/version.previous"
fi

install -o root -g root -m 0755 "$work/$stem/holyarrs" "$APP_DIR/holyarrs.new"
mv -f "$APP_DIR/holyarrs.new" "$APP_DIR/holyarrs"
printf '%s\n' "$wanted" > "$APP_DIR/version"
for f in LICENSE THIRD_PARTY_NOTICES.md; do
  [ -f "$work/$stem/$f" ] && install -o root -g root -m 0644 "$work/$stem/$f" "$APP_DIR/$f"
done

# This script stays on the machine so the update needs no second download.
self="$(readlink -f "$0" 2>/dev/null || true)"
if [ -n "$self" ] && [ -f "$self" ] && [ "$self" != "$SELF_COPY" ]; then
  install -o root -g root -m 0755 "$self" "$SELF_COPY"
fi

cat > "$UNIT_DIR/$SERVICE.service" <<UNIT
[Unit]
Description=HolyArrs
After=network-online.target
Wants=network-online.target

[Service]
User=$USER_NAME
Group=$USER_NAME
ExecStart=$APP_DIR/holyarrs -config $CONFIG_DIR
# A restart from the app and a backup restore both exit and expect to be started again.
Restart=always
RestartSec=3
NoNewPrivileges=true
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
UNIT

if has_systemd; then
  systemctl daemon-reload
  systemctl enable --now "$SERVICE" >/dev/null 2>&1 || fail "the service did not start. See: journalctl -u $SERVICE -n 50"
  restart=""
  # A port or address set in a drop-in (systemctl edit holyarrs) is the one to wait
  # on and to print: -addr on the command line first, then HOLYARRS_ADDR, then HOLYARRS_PORT.
  HOST=127.0.0.1
  listen=""
  env_port=""
  unit_env="$(systemctl show -p Environment --value "$SERVICE" 2>/dev/null || true)"
  for word in $unit_env; do
    case "$word" in
      HOLYARRS_ADDR=*) listen="${word#HOLYARRS_ADDR=}" ;;
      HOLYARRS_PORT=*) env_port="${word#HOLYARRS_PORT=}" ;;
    esac
  done
  unit_exec="$(systemctl show -p ExecStart --value "$SERVICE" 2>/dev/null || true)"
  next=""
  for word in $unit_exec; do
    if [ -n "$next" ]; then listen="$word"; next=""; continue; fi
    case "$word" in
      -addr|--addr) next=1 ;;
      -addr=*|--addr=*) listen="${word#*=}" ;;
    esac
  done
  addr_port="${listen##*:}"
  addr_host="${listen%:*}"
  [ "$addr_host" != "$listen" ] || addr_host=""
  for p in "$addr_port" "$env_port"; do
    if printf '%s' "$p" | grep -Eq '^[0-9]{1,5}$'; then PORT="$p"; break; fi
  done
  if [ -n "$addr_port" ] && [ "$PORT" = "$addr_port" ]; then
    case "$addr_host" in
      ''|0.0.0.0|::|'[::]'|'*') ;;
      *) HOST="$addr_host" ;;
    esac
  fi
  restarts() { systemctl show -p NRestarts --value "$SERVICE" 2>/dev/null | tr -cd '0-9' || true; }
  ok=""
  down=0
  first="$(restarts)"
  SECONDS=0
  back=""
  [ ! -f "$APP_DIR/holyarrs.previous" ] || back=" The old program is $APP_DIR/holyarrs.previous and your saved copy is in $BACKUP_DIR (see the Proxmox guide to go back)."
  # /healthz already answers while HolyArrs opens or updates its data; /readyz
  # answers only once it is really ready. /healthz says "failed" when it gave up.
  for _ in $(seq 1 300); do
    [ "$SECONDS" -lt 300 ] || break
    if curl -fsS --max-time 5 -o /dev/null "http://$HOST:$PORT/readyz" 2>/dev/null; then ok=1; break; fi
    state="$(curl -s --max-time 5 "http://$HOST:$PORT/healthz" 2>/dev/null || true)"
    case "$state" in
      *'"failed"'*) fail "HolyArrs $wanted could not start. The reason is in: journalctl -u $SERVICE -n 50.$back" ;;
    esac
    # A program that stops right after it starts is started again by systemd, over
    # and over. One stop is not enough to say so: systemd has to have started it
    # again three times, or, where it keeps no count, it was down ten checks in a row.
    if systemctl is-active --quiet "$SERVICE" 2>/dev/null; then down=0; else down=$((down + 1)); fi
    now="$(restarts)"
    if [ -n "$first" ] && [ -n "$now" ]; then again=$((now - first)); else again=0; fi
    [ "$again" -lt 3 ] && [ "$down" -lt 10 ] || fail "HolyArrs $wanted keeps stopping right after it starts. The reason is in: journalctl -u $SERVICE -n 50.$back"
    sleep 1
  done
  [ -n "$ok" ] || fail "HolyArrs was not ready on port $PORT within 5 minutes. It may still be updating its data, so look at: journalctl -u $SERVICE -n 50.$back"
  say "HolyArrs $wanted is running."
else
  say "HolyArrs $wanted is installed. This machine has no systemd, so start it yourself:"
  say "  su -s /bin/sh $USER_NAME -c '$APP_DIR/holyarrs -config $CONFIG_DIR'"
fi

ip="$(hostname -I 2>/dev/null | awk '{print $1}')"
if [ "$action" = update ]; then
  say "Open http://${ip:-this-machine}:$PORT"
else
  say "Open http://${ip:-this-machine}:$PORT and make the owner account right away; the first visitor becomes the owner."
fi
[ ! -f "$APP_DIR/holyarrs.previous" ] || say "The old program is kept as $APP_DIR/holyarrs.previous."
exit 0
