#!/usr/bin/env bash
# Makes a Debian container (LXC) on a Proxmox VE host and installs HolyArrs in
# it from the plain Linux download of a GitHub release. No Docker. Run it in the
# Proxmox host shell as root:
#
#   bash holyarrs-lxc.sh                 make a new container and install
#   bash holyarrs-lxc.sh update <id>     update HolyArrs in container <id>
#
# Settings are environment variables, all optional:
#   CTID          container number            (default: the next free one)
#   CT_NAME       host name                   (default: holyarrs)
#   CORES, RAM    cores and MiB of memory     (default: 2 and 1024)
#   DISK          disk size in GB             (default: 8, for settings and data)
#   STORAGE       where the disk lives        (default: the first that holds containers)
#   TEMPLATE_STORAGE  where templates live    (default: the first that holds templates)
#   BRIDGE, VLAN  network bridge and tag      (default: vmbr0, no tag)
#   IP, GATEWAY   "dhcp" or an address like 192.168.1.50/24 with GATEWAY=192.168.1.1
#   MOUNT         a host folder to share into the container as /data (downloads and
#                 library together, so files move instead of copying)
#   VERSION       a HolyArrs version, such as 1.1.3 (default: the newest)
#   PRIVILEGED=1  make a privileged container (default: unprivileged)
#   INSTALL_SCRIPT_URL  where to fetch holyarrs-install.sh when it is not next to this file
set -euo pipefail

fail() { printf 'HolyArrs on Proxmox: %s\n' "$*" >&2; exit 1; }
say() { printf '%s\n' "$*"; }

[ "$(id -u)" = 0 ] || fail "run this as root on the Proxmox host"
if ! command -v pct >/dev/null 2>&1 || ! command -v pveam >/dev/null 2>&1 || ! command -v pvesm >/dev/null 2>&1; then
  fail "pct, pveam or pvesm was not found. This runs on a Proxmox VE host, not inside a container or on a plain Debian machine"
fi

if [ "${1:-}" = update ]; then
  id="${2:-}"
  printf '%s' "$id" | grep -Eq '^[0-9]+$' || fail "use: $0 update <container number>"
  pct status "$id" >/dev/null 2>&1 || fail "there is no container $id on this host"
  [ "$(pct status "$id" | awk '{print $2}')" = running ] || fail "container $id is not running. Start it with: pct start $id"
  pct exec "$id" -- test -x /usr/local/bin/update-holyarrs \
    || fail "container $id was not made by this script (no /usr/local/bin/update-holyarrs inside it)"
  # FORCE=1 allows an older version than the one installed.
  [ -z "${FORCE:-}" ] || [ -n "${VERSION:-}" ] || fail "FORCE=1 needs VERSION=<the version> as well"
  # The installer in the container is refreshed first, so fixes reach old containers.
  here="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" 2>/dev/null && pwd || true)"
  if [ -n "$here" ] && [ -f "$here/holyarrs-install.sh" ]; then
    pct push "$id" "$here/holyarrs-install.sh" /usr/local/bin/update-holyarrs --perms 0755
  fi
  # shellcheck disable=SC2086
  exec pct exec "$id" -- /usr/local/bin/update-holyarrs update ${VERSION:-} ${FORCE:+force}
fi
[ -z "${1:-}" ] || fail "use: $0   or   $0 update <container number>"

CT_NAME="${CT_NAME:-holyarrs}"
CORES="${CORES:-2}"
RAM="${RAM:-1024}"
DISK="${DISK:-8}"
BRIDGE="${BRIDGE:-vmbr0}"
IP="${IP:-dhcp}"
VERSION="${VERSION:-}"
for pair in CORES:"$CORES" RAM:"$RAM" DISK:"$DISK"; do
  printf '%s' "${pair#*:}" | grep -Eq '^[1-9][0-9]*$' || fail "${pair%%:*} must be a whole number above 0"
done
[ "$RAM" -ge 512 ] || fail "RAM must be at least 512 (MiB); 1024 is a good start"
printf '%s' "$CT_NAME" | grep -Eq '^[A-Za-z0-9]([A-Za-z0-9-]*[A-Za-z0-9])?$' || fail "CT_NAME may hold only letters, numbers and dashes"
if [ -n "$VERSION" ] && ! printf '%s' "$VERSION" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$'; then
  fail "VERSION must look like 1.2.3"
fi
if [ "$IP" != dhcp ]; then
  [ -n "${GATEWAY:-}" ] || fail "IP=$IP needs GATEWAY, for example GATEWAY=192.168.1.1"
  printf '%s' "$IP" | grep -Eq '^[0-9.]+/[0-9]+$' || fail "IP must be dhcp or an address with its size, like 192.168.1.50/24"
fi
if [ -n "${MOUNT:-}" ]; then
  case "$MOUNT" in /*) ;; *) fail "MOUNT must be a full path" ;; esac
  [ -d "$MOUNT" ] || fail "MOUNT=$MOUNT is not a folder on this host"
fi
if [ -n "${VLAN:-}" ]; then printf '%s' "$VLAN" | grep -Eq '^[0-9]+$' || fail "VLAN must be a number"; fi

CTID="${CTID:-$(pvesh get /cluster/nextid)}"
printf '%s' "$CTID" | grep -Eq '^[0-9]+$' || fail "CTID must be a number"
if pct status "$CTID" >/dev/null 2>&1 || qm status "$CTID" >/dev/null 2>&1; then
  fail "number $CTID is already used by a container or virtual machine. Pick another with CTID=<number>"
fi
ip link show "$BRIDGE" >/dev/null 2>&1 || fail "the network bridge $BRIDGE does not exist on this host. Set BRIDGE=<name> (see Network in the Proxmox web page)"

first_storage() { pvesm status --content "$1" --enabled 1 2>/dev/null | awk 'NR>1 {print $1; exit}'; }
STORAGE="${STORAGE:-$(first_storage rootdir)}"
TEMPLATE_STORAGE="${TEMPLATE_STORAGE:-$(first_storage vztmpl)}"
[ -n "$STORAGE" ] || fail "no storage that can hold containers was found. Set STORAGE=<name>"
[ -n "$TEMPLATE_STORAGE" ] || fail "no storage that can hold container templates was found. Set TEMPLATE_STORAGE=<name>"

say "Looking for a Debian template"
pveam update >/dev/null 2>&1 || say "(could not refresh the template list; using what is already known)"
# Older Proxmox VE 8 releases refuse Debian 13, so it is tried only on 9 and newer (or when the version is unknown).
pve_major="$(pveversion 2>/dev/null | sed -n 's|^pve-manager/\([0-9][0-9]*\)\..*|\1|p' | head -n1)"
templates="$(pveam available --section system 2>/dev/null | awk '$2 ~ /^debian-1[23]-standard/ {print $2}' | sort -rV)"
if [ -n "$pve_major" ] && [ "$pve_major" -lt 9 ]; then
  templates="$(printf '%s\n' "$templates" | grep '^debian-12-standard' || true)"
fi
[ -n "$templates" ] || fail "no Debian 12 template is listed (Debian 13 needs Proxmox VE 9). Check that this host can reach the internet, then run: pveam update"

net="name=eth0,bridge=$BRIDGE,ip=$IP"
[ "$IP" = dhcp ] || net="$net,gw=$GATEWAY"
[ -z "${VLAN:-}" ] || net="$net,tag=$VLAN"
unpriv=1
[ "${PRIVILEGED:-}" != 1 ] || unpriv=0

args=(--hostname "$CT_NAME" --cores "$CORES" --memory "$RAM" --swap 256 --rootfs "$STORAGE:$DISK"
  --net0 "$net" --ostype debian --onboot 1 --tags holyarrs --unprivileged "$unpriv" --features nesting=1)
[ -z "${MOUNT:-}" ] || args+=(--mp0 "$MOUNT,mp=/data")

made=""
for template in $templates; do
  if ! pveam list "$TEMPLATE_STORAGE" 2>/dev/null | grep -q "$template"; then
    say "Downloading $template to $TEMPLATE_STORAGE"
    pveam download "$TEMPLATE_STORAGE" "$template" >/dev/null || { say "the download of $template failed"; continue; }
  fi
  say "Creating container $CTID ($CT_NAME) on $STORAGE from $template"
  if why="$(pct create "$CTID" "$TEMPLATE_STORAGE:vztmpl/$template" "${args[@]}" 2>&1)"; then made=1; break; fi
  say "Proxmox could not create the container from $template:"
  say "$why"
done
[ -n "$made" ] || fail "no container could be created (the Proxmox messages are above)"

left_in_place() {
  [ "$?" = 0 ] || say "Container $CTID was left in place so you can look at it. Remove it with: pct stop $CTID; pct destroy $CTID"
}
trap left_in_place EXIT

pct start "$CTID"

say "Waiting for the container's network"
up=""
for _ in $(seq 1 40); do
  if pct exec "$CTID" -- getent hosts github.com >/dev/null 2>&1; then up=1; break; fi
  sleep 2
done
[ -n "$up" ] || fail "container $CTID could not reach github.com after 80 seconds. Check BRIDGE, VLAN, IP and GATEWAY, and that this network lets containers reach the internet"

here="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" 2>/dev/null && pwd || true)"
if [ -n "$here" ] && [ -f "$here/holyarrs-install.sh" ]; then
  pct push "$CTID" "$here/holyarrs-install.sh" /root/holyarrs-install.sh --perms 0755
else
  url="${INSTALL_SCRIPT_URL:-https://raw.githubusercontent.com/HolyArrs/HolyArrs/main/deploy/proxmox/holyarrs-install.sh}"
  say "Fetching the installer from $url"
  # Fetched inside the container, so the Proxmox host needs no extra tools.
  pct exec "$CTID" -- sh -c 'command -v curl >/dev/null 2>&1 || { apt-get update -qq && apt-get install -y -qq curl ca-certificates >/dev/null; }'
  pct exec "$CTID" -- curl -fsSL -o /root/holyarrs-install.sh "$url" || fail "could not download the installer from $url"
fi

# shellcheck disable=SC2086
pct exec "$CTID" -- bash /root/holyarrs-install.sh install $VERSION
pct exec "$CTID" -- rm -f /root/holyarrs-install.sh
trap - EXIT

addr="$(pct exec "$CTID" -- hostname -I 2>/dev/null | awk '{print $1}')"
say ""
say "Done. HolyArrs is at http://${addr:-<the container address>}:6262"
say "Make the owner account right away; the first visitor becomes the owner."
if [ -n "${MOUNT:-}" ]; then
  uid="$(pct exec "$CTID" -- id -u holyarrs 2>/dev/null || true)"
  gid="$(pct exec "$CTID" -- id -g holyarrs 2>/dev/null || true)"
  say ""
  say "Folder $MOUNT is /data in the container. So that HolyArrs can write to it, run this on the Proxmox host:"
  # An unprivileged container shifts every number by 100000 on the host.
  shift_by=100000
  [ "$unpriv" = 1 ] || shift_by=0
  if printf '%s' "$uid" | grep -Eq '^[0-9]+$' && printf '%s' "$gid" | grep -Eq '^[0-9]+$'; then
    say "  chown -R $((shift_by + uid)):$((shift_by + gid)) $MOUNT"
  elif [ "$unpriv" = 1 ]; then
    say "  chown -R <100000 plus the user number>:<100000 plus the group number> $MOUNT   (the numbers are in: pct exec $CTID -- id holyarrs)"
  else
    say "  chown -R <the user number>:<the group number> $MOUNT   (the numbers are in: pct exec $CTID -- id holyarrs)"
  fi
fi
say "Update later with: bash holyarrs-lxc.sh update $CTID   (or run update-holyarrs update inside the container)"
