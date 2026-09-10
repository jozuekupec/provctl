#!/bin/sh
# Validate the structural Debian package invariants without installing it.
set -eu

usage() {
	echo "usage: scripts/tests/t04-package.sh PATH-TO-PROVCTL-DEB" >&2
	exit 2
}

test "$#" -eq 1 || usage
package=$1
test -f "$package" || { echo "package not found: $package" >&2; exit 2; }

test "$(dpkg-deb -f "$package" Package)" = provctl
test "$(dpkg-deb -f "$package" Architecture)" = amd64

depends=$(dpkg-deb -f "$package" Depends)
depends_compact=$(printf '%s' "$depends" | tr -d ' ')
case ",$depends_compact," in
	*,apache2,*) ;;
	*) echo "package does not depend on apache2" >&2; exit 1 ;;
esac
case ",$depends," in
	*php*) echo "PHP must remain a Suggests dependency, not Depends" >&2; exit 1 ;;
esac

control_dir=$(mktemp -d)
cleanup() {
	rm -rf "$control_dir"
}
trap cleanup EXIT HUP INT TERM
dpkg-deb -e "$package" "$control_dir"
grep -Fx '/etc/provctl/config.toml' "$control_dir/conffiles" >/dev/null
if grep -F '/usr/share/provctl/templates/' "$control_dir/conffiles" >/dev/null; then
	echo "templates must not be Debian conffiles" >&2
	exit 1
fi

contents=$(dpkg-deb -c "$package")
require_entry() {
	pattern=$1
	echo "$contents" | grep -E "$pattern" >/dev/null || {
		echo "missing or incorrectly permissioned package entry: $pattern" >&2
		exit 1
	}
}

require_entry '^-rwxr-xr-x root/root .* ./usr/bin/provctl$'
require_entry '^drwx------ root/root .* ./var/lib/provctl/$'
require_entry '^drwxr-x--- root/root .* ./var/log/provctl/$'
if echo "$contents" | grep -F ' ./var/www/' >/dev/null; then
	echo "package must not contain customer data below /var/www" >&2
	exit 1
fi

size=$(stat -c '%s' "$package")
test "$size" -lt $((30 * 1024 * 1024)) || {
	echo "package is unexpectedly larger than 30 MiB" >&2
	exit 1
}

echo 'PASS: T04 package structure'
