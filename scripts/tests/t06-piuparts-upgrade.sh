#!/bin/sh
# Verify an ordered Debian package upgrade only in the disposable E2 container.
set -eu

usage() {
	echo "usage: scripts/tests/t06-piuparts-upgrade.sh OLD-DEB NEW-DEB" >&2
	exit 2
}

test "$#" -eq 2 || usage
case "$1" in /*) old_package=$1 ;; *) old_package=$(pwd)/$1 ;; esac
case "$2" in /*) new_package=$2 ;; *) new_package=$(pwd)/$2 ;; esac
test -f "$old_package" || { echo "old package not found: $old_package" >&2; exit 2; }
test -f "$new_package" || { echo "new package not found: $new_package" >&2; exit 2; }

old_version=$(dpkg-deb -f "$old_package" Version)
new_version=$(dpkg-deb -f "$new_package" Version)
dpkg --compare-versions "$old_version" lt "$new_version" || {
	echo "new package version must be greater than old package version" >&2
	exit 2
}

root=$(CDPATH= cd "$(dirname "$0")/../.." && pwd)
e2="$root/scripts/e2.sh"
old_name=$(basename "$old_package")
new_name=$(basename "$new_package")

cleanup() {
	"$e2" reset
}
trap cleanup EXIT HUP INT TERM

run() {
	"$e2" sh "$1"
}

"$e2" reset
run 'export DEBIAN_FRONTEND=noninteractive; apt-get -qq update && apt-get -qq install -y piuparts debootstrap >/dev/null'
"$e2" push "$old_package"
"$e2" push "$new_package"
run "piuparts --distribution trixie --mirror http://deb.debian.org/debian /root/$old_name /root/$new_name >/root/provctl-piuparts-upgrade.log 2>&1 || { tail -n 120 /root/provctl-piuparts-upgrade.log >&2; exit 1; }"
run "export DEBIAN_FRONTEND=noninteractive; apt-get -qq update && apt-get -qq install -y cron zstd >/dev/null && dpkg -i /root/$old_name >/dev/null && printf '%s\\n' '# PROVCTL-CONFFILE-TEST' >> /etc/provctl/config.toml && sed -i 's|^vhosts.*|vhosts = \"/data/web/vhosts\"|' /etc/provctl/config.toml && dpkg -i /root/$new_name >/dev/null && grep -Fx '# PROVCTL-CONFFILE-TEST' /etc/provctl/config.toml >/dev/null && grep -Fx 'vhosts = \"/data/web/vhosts\"' /etc/provctl/config.toml >/dev/null && test ! -f /etc/provctl/config.toml.dpkg-dist"

echo 'PASS: T06 piuparts upgrade'
