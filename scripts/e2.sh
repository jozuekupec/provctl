#!/bin/sh
# Run a deliberately scoped command in the disposable Incus E2 container.
set -eu

# The defaults preserve the documented disposable test environment. Callers
# may opt into an isolated clone, which prevents a long-running scenario from
# replacing an interactive pv session.
instance=${PROVCTL_E2_INSTANCE:-pv}
snapshot=${PROVCTL_E2_SNAPSHOT:-clean}

usage() {
	cat <<'EOF'
usage: scripts/e2.sh <command> [argument]

Commands:
  status                 show the E2 container status
  reset                  restore the clean snapshot and wait for systemd
  push <local-file>      copy one package or fixture into /root in the container
  sh <command>           run an explicit shell command as root in the container

This helper is limited to the disposable "pv" Incus instance. It never runs
the supplied command on the host. Use "reset" before each mutating scenario.
EOF
}

wait_for_systemd() {
	attempt=0
	while [ "$attempt" -lt 60 ]; do
		state=$(incus exec "$instance" -- systemctl is-system-running 2>/dev/null || true)
		case "$state" in
			running|degraded) return 0 ;;
		esac
		attempt=$((attempt + 1))
		sleep 1
	done
	echo "systemd in $instance did not become ready within 60 seconds" >&2
	return 1
}

ensure_running() {
	state=$(incus list "$instance" --format csv -c s)
	if [ "$state" != "RUNNING" ]; then
		# A copied Incus instance can retain the source's volatile MAC address
		# through a snapshot. Opt in to regenerating it for isolated test clones.
		if [ "${PROVCTL_E2_REGENERATE_NIC:-false}" = "true" ]; then
			incus config unset "$instance" volatile.eth0.hwaddr 2>/dev/null || true
		fi
		incus start "$instance"
	fi
}

command=${1:-}
case "$command" in
	status)
		test "$#" -eq 1 || { usage >&2; exit 2; }
		incus list "$instance"
		;;
	reset)
		test "$#" -eq 1 || { usage >&2; exit 2; }
		# Stop first: restoring a snapshot of a copied container can otherwise
		# attempt to start it with the source's captured volatile MAC address.
		state=$(incus list "$instance" --format csv -c s)
		if [ "$state" = "RUNNING" ]; then
			incus stop --force "$instance"
		fi
		incus snapshot restore "$instance" "$snapshot"
		ensure_running
		wait_for_systemd
		;;
	push)
		test "$#" -eq 2 || { usage >&2; exit 2; }
		test -f "$2" || { echo "not a regular file: $2" >&2; exit 2; }
		incus file push --quiet "$2" "$instance/root/"
		;;
	sh)
		test "$#" -eq 2 || { usage >&2; exit 2; }
		# The caller intentionally supplies a test command; it executes only in pv.
		incus exec "$instance" -- sh -lc "$2"
		;;
	-h|--help|help|'')
		usage
		;;
	*)
		echo "unknown E2 command: $command" >&2
		usage >&2
		exit 2
		;;
esac
