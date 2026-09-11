#!/bin/sh
# Verify that adoption rejects a live legacy Apache vhost before moving data.
# Every command runs in the disposable Incus E2 container.
set -eu

usage() {
	echo "usage: scripts/tests/t17-adopt.sh PATH-TO-PROVCTL-DEB" >&2
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
"$e2" push "$package"
run "export DEBIAN_FRONTEND=noninteractive; { apt-get -qq update && apt-get -qq install -y adduser apache2 cron curl php-fpm zstd && dpkg -i /root/$package_name; } >/tmp/provctl-t17-install.log 2>&1 || { cat /tmp/provctl-t17-install.log >&2; exit 1; }"
run 'provctl bootstrap --install-missing --yes'
run 'install -d -m 0755 /srv/legacy && printf "legacy\n" >/srv/legacy/index.html'
run 'printf "%s\n" "<VirtualHost *:80>" "    ServerName collision.test" "    DocumentRoot /srv/legacy" "</VirtualHost>" >/etc/apache2/sites-available/legacy-collision.conf && a2ensite legacy-collision.conf && apache2ctl configtest && systemctl reload apache2'

# The parser reads actual apache2ctl -S output. Adoption must stop before it
# creates a subscription, generated vhost, or moves the legacy document root.
run 'if provctl subscription adopt migrated --from /srv/legacy --domain collision.test --dry-run >/tmp/provctl-t17.out 2>/tmp/provctl-t17.err; then echo "adoption accepted an active legacy vhost" >&2; exit 1; fi; grep -F "already served by Apache" /tmp/provctl-t17.err'
run 'test -f /srv/legacy/index.html && ! test -e /var/www/vhosts/migrated && ! test -e /etc/apache2/sites-available/provctl-migrated-collision.test.conf && ! provctl subscription list | grep -Fx migrated'

# Once an operator explicitly disables the conflicting vhost, adoption moves
# the legacy root and makes it reachable through the generated PHP-FPM vhost.
run 'a2dissite legacy-collision.conf && apache2ctl configtest && systemctl reload apache2'
run 'provctl subscription adopt migrated --from /srv/legacy --domain collision.test --no-backup'
run 'test ! -e /srv/legacy && test -f /var/www/vhosts/migrated/sites/collision.test/public/index.html && grep -F "collision.test" /etc/apache2/sites-available/provctl-migrated-collision.test.conf && test "$(curl -fsS -H "Host: collision.test" http://127.0.0.1/)" = legacy && provctl subscription list | grep -E "^migrated[[:space:]]"'

echo 'PASS: T17 adoption conflict and migration'
