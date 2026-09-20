#!/bin/sh
# Build the current package, restore a disposable Incus TUI fixture, and open
# provctl in the caller's real terminal. Nothing runs on the host as root.
set -eu

repo=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
instance=${PROVCTL_TUI_INSTANCE:-pv}
snapshot=${PROVCTL_TUI_SNAPSHOT:-clean}

usage() {
	cat <<'EOF'
usage: scripts/dev/run-tui-test.sh [package.deb]

Builds the current package when no package is supplied, restores the selected
Incus snapshot, installs the package in that disposable container, then opens
provctl interactively. Environment overrides:

  PROVCTL_TUI_INSTANCE  Incus instance (default: pv)
  PROVCTL_TUI_SNAPSHOT  snapshot (default: clean)

The selected instance is restored before the run and again when the TUI exits,
so do not point it at a container containing work you need to keep.
EOF
}

case ${1:-} in
	-h|--help|help)
		usage
		exit 0
		;;
esac

if [ "$#" -gt 1 ]; then
	usage >&2
	exit 2
fi

if [ "$#" -eq 1 ]; then
	package=$1
	[ -f "$package" ] || { echo "package not found: $package" >&2; exit 2; }
else
	"$repo/scripts/build-deb.sh"
	revision=$(git -C "$repo" rev-parse --short HEAD)
	dirty=
	if ! git -C "$repo" diff --quiet; then
		dirty=-dirty
	fi
	package="$repo/dist/provctl_0.0.0+git.$revision$dirty"_amd64.deb
	[ -f "$package" ] || {
		echo "current package was not produced: $package" >&2
		exit 1
	}
fi

state=$(incus list "$instance" --format csv -c s)
if [ "$state" = "RUNNING" ]; then
	incus stop --force "$instance"
fi
incus snapshot restore "$instance" "$snapshot"
# A non-stateful snapshot can retain the source container's volatile MAC.
# Regenerate it before start so this isolated fixture cannot collide with pv.
incus config unset "$instance" volatile.eth0.hwaddr 2>/dev/null || true
if [ "$(incus list "$instance" --format csv -c s)" != "RUNNING" ]; then
	incus start "$instance"
fi

attempt=0
while [ "$attempt" -lt 60 ]; do
	case "$(incus exec "$instance" -- systemctl is-system-running 2>/dev/null || true)" in
		running|degraded) break ;;
	esac
	attempt=$((attempt + 1))
	sleep 1
done
[ "$attempt" -lt 60 ] || {
	echo "systemd in $instance did not become ready within 60 seconds" >&2
	exit 1
}

cleanup() {
	incus stop --force "$instance" >/dev/null 2>&1 || true
	incus snapshot restore "$instance" "$snapshot" >/dev/null 2>&1 || true
}
trap cleanup EXIT HUP INT TERM

incus file push --quiet "$package" "$instance/tmp/provctl-tui-test.deb"
# Fixtures can contain a development package whose Debian version sorts above
# the current Git build; the fixture is disposable, so this replacement is
# deliberate.
incus exec "$instance" -- apt-get -o Acquire::ForceIPv4=true update
incus exec "$instance" -- env DEBIAN_FRONTEND=noninteractive apt-get -o Acquire::ForceIPv4=true install -y --allow-downgrades /tmp/provctl-tui-test.deb
incus exec "$instance" -- /usr/bin/provctl bootstrap --install-missing --yes

echo "Opening provctl in $instance from snapshot $snapshot. Press q to quit; it will then be restored."
incus exec "$instance" -- /usr/bin/provctl
