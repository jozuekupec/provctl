#!/bin/sh
# Verify actual HTTP-01 issuance, renewal and the deploy hook against Pebble.
# The Pebble process and Certbot run only inside the disposable Incus E2 image.
set -eu

usage() {
	echo "usage: scripts/tests/t16-ssl.sh PATH-TO-PROVCTL-DEB PATH-TO-PEBBLE-CHECKOUT" >&2
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
workdir=$(mktemp -d /tmp/provctl-pebble.XXXXXX)
pebble_bin="$workdir/pebble"

cleanup() {
	rm -rf "$workdir"
	"$e2" reset
}
trap cleanup EXIT HUP INT TERM

run() {
	"$e2" sh "$1"
}

# Build Pebble on the host with the project's Go toolchain, then execute that
# binary inside pv. This keeps its listening endpoints, validation requests,
# and CA trust entirely inside the disposable system container.
go -C "$pebble_source" build -o "$pebble_bin" ./cmd/pebble

"$e2" reset
"$e2" push "$package"
run 'install -d -m 0755 /root/pebble/test/certs'
incus file push --quiet "$pebble_bin" "$instance/root/pebble/pebble"
incus file push --quiet "$root/scripts/testdata/pebble-config.json" "$instance/root/pebble/pebble-config.json"
incus file push --quiet "$pebble_source/test/certs/pebble.minica.pem" "$instance/root/pebble/pebble.minica.pem"
incus file push --quiet -r "$pebble_source/test/certs/localhost" "$instance/root/pebble/test/certs/"

run "export DEBIAN_FRONTEND=noninteractive; { apt-get -qq update && apt-get -qq install -y adduser apache2 certbot cron curl openssl php-fpm zstd && dpkg -i /root/$package_name; } >/tmp/provctl-t16-install.log 2>&1 || { cat /tmp/provctl-t16-install.log >&2; exit 1; }"
run 'provctl bootstrap --install-missing --yes'
run 'install -d -m 0755 /root/pebble/test/certs && chmod 0755 /root/pebble/pebble && printf "127.0.0.1 ssl.test pebble\\n" >>/etc/hosts && cd /root/pebble; PEBBLE_VA_NOSLEEP=1 PEBBLE_WFE_NONCEREJECT=0 ./pebble -config ./pebble-config.json >/tmp/provctl-pebble.log 2>&1 & echo $! >/run/provctl-pebble.pid; sleep 1; test -s /run/provctl-pebble.pid && kill -0 "$(cat /run/provctl-pebble.pid)" || { cat /tmp/provctl-pebble.log >&2; exit 1; }'
# Pebble's HTTPS listener uses its static test CA; its /roots endpoint instead
# exposes the dynamically generated issuer of certificates it will issue.
run 'install -m 0644 /root/pebble/pebble.minica.pem /usr/local/share/ca-certificates/provctl-pebble-server.crt && update-ca-certificates >/dev/null; ready=false; for attempt in $(seq 1 20); do if curl --noproxy "*" --insecure --connect-timeout 2 --max-time 3 -sf https://pebble:15000/roots/0 >/dev/null; then ready=true && break; fi; sleep 1; done; test "$ready" = true || { cat /tmp/provctl-pebble.log >&2; exit 1; }'
run 'sed -i "s|email = \"\"|email = \"test@example.test\"|; s|server = \"\"|server = \"https://pebble:14000/dir\"|" /etc/provctl/config.toml'
run 'provctl subscription create acme && provctl website create acme ssl.test --type php-fpm'

# --force is intentional: ssl.test resolves to pv loopback, while validateDNS
# correctly excludes loopback addresses from its set of server interfaces.
run 'provctl ssl enable acme ssl.test --force'
run 'lineage=$(provctl ssl status acme ssl.test | sed -n "s/^lineage: //p"); test -n "$lineage" && test -f "/etc/letsencrypt/live/$lineage/fullchain.pem" && apache2ctl configtest && curl -skI https://ssl.test/ >/dev/null'
run 'provctl website set acme ssl.test --force-https && test "$(curl -s -o /dev/null -w "%{http_code}" http://ssl.test/)" = 301 && test "$(curl -s -o /dev/null -w "%{http_code}" http://ssl.test/.well-known/acme-challenge/x)" = 404'
run 'lineage=$(provctl ssl status acme ssl.test | sed -n "s/^lineage: //p"); grep -E "authenticator = webroot|webroot_path = /var/lib/provctl-acme-challenge" "/etc/letsencrypt/renewal/$lineage.conf"'
run 'lineage=$(provctl ssl status acme ssl.test | sed -n "s/^lineage: //p"); certbot renew --cert-name "$lineage" --force-renewal --no-random-sleep-on-renew && test -s /var/log/provctl/deploy-hook.log && provctl ssl disable acme ssl.test && ! grep -q "<VirtualHost \*:443>" /etc/apache2/sites-available/provctl-acme-ssl.test.conf'

echo 'PASS: T16 Pebble HTTP-01 issuance, renewal, deploy hook, and disable'
