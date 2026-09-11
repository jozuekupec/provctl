#!/bin/sh
# Run the implemented package and E2 regression checks for one exact package.
set -eu

usage() {
	echo "usage: scripts/tests/run-all.sh CURRENT-DEB [PREVIOUS-DEB]" >&2
	echo "       PREVIOUS-DEB enables the ordered T06 package upgrade check." >&2
	exit 2
}

test "$#" -eq 1 || test "$#" -eq 2 || usage
current=$1
previous=${2:-}
root=$(CDPATH= cd "$(dirname "$0")/../.." && pwd)
tests="$root/scripts/tests"

run_stage() {
	name=$1
	shift
	if "$@"; then
		echo "PASS: $name"
		return 0
	else
		status=$?
	fi
	echo "FAIL: $name (exit $status)" >&2
	exit "$status"
}

run_stage T04 "$tests/t04-package.sh" "$current"
run_stage T05 "$tests/t05-piuparts.sh" "$current"
if test -n "$previous"; then
	run_stage T06 "$tests/t06-piuparts-upgrade.sh" "$previous" "$current"
fi
run_stage T10 "$tests/t10-isolation.sh" "$current"

echo 'PASS: implemented package and E2 regressions'
