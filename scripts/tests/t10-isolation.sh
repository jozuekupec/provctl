#!/bin/sh
# Verify that two provisioned subscriptions cannot cross their filesystem or
# PHP-FPM boundaries. Every command runs in the disposable Incus E2 container.
set -eu

usage() {
	echo "usage: scripts/tests/t10-isolation.sh PATH-TO-PROVCTL-DEB" >&2
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
run "export DEBIAN_FRONTEND=noninteractive; { apt-get -qq update && apt-get -qq install -y adduser apache2 cron curl zstd && dpkg -i /root/$package_name; } >/tmp/provctl-t10-install.log 2>&1 || { cat /tmp/provctl-t10-install.log >&2; exit 1; }"
run 'provctl bootstrap --install-missing --yes'
run 'provctl subscription create alfa && provctl subscription create beta'
run 'provctl website create alfa a.test --type php-fpm && provctl website create beta b.test --type php-fpm'
run 'printf "%s\n" TAJEMSTVI > /var/www/vhosts/alfa/sites/a.test/app/secret.txt && chown alfa:alfa /var/www/vhosts/alfa/sites/a.test/app/secret.txt && chmod 600 /var/www/vhosts/alfa/sites/a.test/app/secret.txt'

# Shell users cannot read either the private file or the other subscription home.
run 'if runuser -u beta -- cat /var/www/vhosts/alfa/sites/a.test/app/secret.txt >/dev/null 2>&1; then echo "beta read alfa secret" >&2; exit 1; fi'
run 'if runuser -u beta -- ls /var/www/vhosts/alfa >/dev/null 2>&1; then echo "beta listed alfa home" >&2; exit 1; fi'

# PHP-FPM is separately constrained by open_basedir.
run "printf '%s\\n' '<?php var_dump(@file_get_contents(\"/var/www/vhosts/alfa/sites/a.test/app/secret.txt\")); var_dump(@scandir(\"/var/www/vhosts/alfa\")); ?>' > /var/www/vhosts/beta/sites/b.test/public/evil.php && chown beta:beta /var/www/vhosts/beta/sites/b.test/public/evil.php"
run 'test "$(curl -fsS -H "Host: b.test" http://127.0.0.1/evil.php | grep -o "bool(false)" | wc -l)" -eq 2'

# Apache and www-data must not expose private application files.
run 'status=$(curl -sS -o /dev/null -w "%{http_code}" -H "Host: a.test" http://127.0.0.1/../app/secret.txt); case "$status" in 400|403|404) ;; *) echo "unexpected private-path HTTP status: $status" >&2; exit 1 ;; esac'
run 'if runuser -u www-data -- cat /var/www/vhosts/alfa/sites/a.test/app/secret.txt >/dev/null 2>&1; then echo "www-data read alfa secret" >&2; exit 1; fi'

# Sessions stay private to their subscription.
run 'grep -F "php_admin_value[session.save_path] = /var/www/vhosts/alfa/tmp/sessions" /etc/php/*/fpm/pool.d/provctl-alfa.conf'
run 'if runuser -u beta -- ls /var/www/vhosts/alfa/tmp/sessions >/dev/null 2>&1; then echo "beta listed alfa sessions" >&2; exit 1; fi'

# The root-owned log directory must reject a new symlink, not merely an
# overwrite of an existing log file.
run 'if runuser -u alfa -- ln -s /etc/shadow /var/log/provctl/alfa/a.test/shadow-link; then echo "alfa created a log symlink" >&2; exit 1; fi; test ! -e /var/log/provctl/alfa/a.test/shadow-link && test ! -L /var/log/provctl/alfa/a.test/shadow-link'
run 'test "$(stat -c "%U:%G %a" /var/log/provctl/alfa/a.test)" = "root:alfa 750"'
run 'runuser -u alfa -- head -1 /var/log/provctl/alfa/a.test/access.log >/dev/null'
run 'if runuser -u beta -- head -1 /var/log/provctl/alfa/a.test/access.log >/dev/null 2>&1; then echo "beta read alfa log" >&2; exit 1; fi'

echo 'PASS: T10 isolation'
