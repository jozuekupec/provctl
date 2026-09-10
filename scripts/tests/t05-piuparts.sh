#!/bin/sh
# Run the Debian install/purge lifecycle check only in the disposable E2 VM.
set -eu

usage() {
	echo "usage: scripts/tests/t05-piuparts.sh PATH-TO-PROVCTL-DEB" >&2
	exit 2
}

test "$#" -eq 1 || usage
case "$1" in
	/*) package=$1 ;;
	*) package=$(pwd)/$1 ;;
esac
test -f "$package" || { echo "package not found: $package" >&2; exit 2; }

root=$(CDPATH= cd "$(dirname "$0")/../.." && pwd)
e2="$root/scripts/e2.sh"
package_name=$(basename "$package")

cleanup() {
	"$e2" reset
}
trap cleanup EXIT HUP INT TERM

run() {
	"$e2" sh "$1"
}

"$e2" reset
run 'export DEBIAN_FRONTEND=noninteractive; apt-get -qq update && apt-get -qq install -y piuparts debootstrap >/dev/null'
"$e2" push "$package"
run "piuparts --distribution trixie --mirror http://deb.debian.org/debian /root/$package_name >/root/provctl-piuparts.log 2>&1 || { tail -n 120 /root/provctl-piuparts.log >&2; exit 1; }"

echo 'PASS: T05 piuparts install/purge'
