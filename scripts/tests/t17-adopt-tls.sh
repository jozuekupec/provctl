#!/bin/sh
# Verify adoption preserves a real legacy Certbot lineage and reconfigures it
# against the local Pebble ACME server. Everything runs in an isolated E2
# instance and its snapshot is restored on exit.
set -eu

usage() {
	echo "usage: scripts/tests/t17-adopt-tls.sh PATH-TO-PROVCTL-DEB PATH-TO-PEBBLE-CHECKOUT" >&2
	exit 2
}

test "$#" -eq 2 || usage
case "$1" in
	/*) package=$1 ;;
	*) package=$(pwd)/$1 ;;
esac
case "$2" in
	/*) pebble_source=$2 ;;
	*) pebble_source=$(pwd)/$2 ;;
esac
test -f "$package" || { echo "package not found: $package" >&2; exit 2; }
test -f "$pebble_source/go.mod" || { echo "Pebble checkout not found: $pebble_source" >&2; exit 2; }
test -d "$pebble_source/test/certs/localhost" || { echo "Pebble localhost test certificate is missing" >&2; exit 2; }

root=$(CDPATH= cd "$(dirname "$0")/../.." && pwd)
e2="$root/scripts/e2.sh"
instance=${PROVCTL_E2_INSTANCE:-pv}
package_name=$(basename "$package")
workdir=$(mktemp -d /tmp/provctl-pebble-adopt.XXXXXX)
pebble_bin="$workdir/pebble"

cleanup() {
	rm -rf "$workdir"
	"$e2" reset
}
trap cleanup EXIT HUP INT TERM

run() {
	"$e2" sh "$1"
}

go -C "$pebble_source" build -o "$pebble_bin" ./cmd/pebble

"$e2" reset
"$e2" push "$package"
run 'install -d -m 0755 /root/pebble/test/certs'
incus file push --quiet "$pebble_bin" "$instance/root/pebble/pebble"
incus file push --quiet "$root/scripts/testdata/pebble-config.json" "$instance/root/pebble/pebble-config.json"
incus file push --quiet "$pebble_source/test/certs/pebble.minica.pem" "$instance/root/pebble/pebble.minica.pem"
incus file push --quiet -r "$pebble_source/test/certs/localhost" "$instance/root/pebble/test/certs/"

run "export DEBIAN_FRONTEND=noninteractive; { apt-get -qq update && apt-get -qq install -y adduser apache2 certbot cron curl openssl php-fpm zstd && dpkg -i /root/$package_name; } >/tmp/provctl-t17b-install.log 2>&1 || { cat /tmp/provctl-t17b-install.log >&2; exit 1; }"
run 'provctl bootstrap --install-missing --yes'
run 'install -d -m 0755 /root/pebble/test/certs && chmod 0755 /root/pebble/pebble && printf "127.0.0.1 legacy.test pebble\\n" >>/etc/hosts && systemd-run --quiet --unit=provctl-pebble --collect --setenv=PEBBLE_VA_NOSLEEP=1 --setenv=PEBBLE_WFE_NONCEREJECT=0 --working-directory=/root/pebble /root/pebble/pebble -config /root/pebble/pebble-config.json && sleep 1; systemctl is-active --quiet provctl-pebble || { journalctl -u provctl-pebble --no-pager >&2; exit 1; }'
run 'install -m 0644 /root/pebble/pebble.minica.pem /usr/local/share/ca-certificates/provctl-pebble-server.crt && update-ca-certificates >/dev/null; ready=false; for attempt in $(seq 1 20); do if curl --noproxy "*" --insecure --connect-timeout 2 --max-time 3 -sf https://127.0.0.1:15000/roots/0 >/dev/null; then ready=true && break; fi; sleep 1; done; test "$ready" = true || { journalctl -u provctl-pebble --no-pager >&2; exit 1; }'
run 'sed -i "s|email = \"\"|email = \"test@example.test\"|; s|staging = true|staging = false|; s|server = \"\"|server = \"https://pebble:14000/dir\"|" /etc/provctl/config.toml'

# Establish a legacy HTTP vhost and a real, externally owned lineage before
# provctl sees either the source directory or the certificate.
run 'install -d -m 0755 /srv/legacy && printf "legacy TLS\\n" >/srv/legacy/index.html && printf "%s\\n" "<VirtualHost *:80>" "    ServerName legacy.test" "    DocumentRoot /srv/legacy" "    <Directory /srv/legacy>" "        Require all granted" "    </Directory>" "</VirtualHost>" >/etc/apache2/sites-available/legacy-tls.conf && a2ensite legacy-tls.conf && apache2ctl configtest && systemctl reload apache2 && test "$(curl --noproxy "*" -s -H "Host: legacy.test" http://127.0.0.1/)" = "legacy TLS"'
run 'certbot certonly --webroot -w /srv/legacy --cert-name legacy-adopt-test -d legacy.test --server https://pebble:14000/dir --agree-tos --email test@example.test --non-interactive && test -f /etc/letsencrypt/live/legacy-adopt-test/fullchain.pem'
run 'a2dissite legacy-tls.conf && apache2ctl configtest && systemctl reload apache2'

run 'provctl subscription adopt migrated-tls --from /srv/legacy --domain legacy.test --no-backup'
run 'test ! -e /srv/legacy && test -f /var/www/vhosts/migrated-tls/sites/legacy.test/public/index.html && grep -F "legacy-adopt-test" /etc/apache2/sites-available/provctl-migrated-tls-legacy.test.conf && apache2ctl configtest'
run 'test "$(provctl ssl status migrated-tls legacy.test | sed -n "s/^lineage: //p")" = legacy-adopt-test && curl --noproxy "*" -sk --resolve legacy.test:443:127.0.0.1 https://legacy.test/ | grep -F "legacy TLS"'
run 'grep -E "authenticator = webroot|webroot_path = /var/lib/provctl-acme-challenge" /etc/letsencrypt/renewal/legacy-adopt-test.conf && certbot renew --cert-name legacy-adopt-test --force-renewal --no-random-sleep-on-renew'

# A pre-existing lineage remains Certbot-owned after the provctl website goes.
run 'provctl website delete migrated-tls legacy.test --confirm-domain legacy.test --yes-i-am-sure && test -f /etc/letsencrypt/live/legacy-adopt-test/fullchain.pem && test -f /etc/letsencrypt/renewal/legacy-adopt-test.conf && ! test -e /etc/apache2/sites-enabled/provctl-migrated-tls-legacy.test.conf'

echo 'PASS: T17b Pebble TLS lineage adoption, renewal, and retained legacy certificate'
