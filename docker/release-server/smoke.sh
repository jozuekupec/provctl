#!/bin/sh
# Runs only inside the disposable systemd-capable Docker server container.
set -eu

expected=${PROVCTL_EXPECTED_VERSION:-}
actual=$(dpkg-query -W -f='${Version}' provctl)
if [ -n "$expected" ] && [ "$actual" != "$expected" ]; then
	echo "expected provctl $expected from GitHub Pages, got $actual" >&2
	exit 1
fi

case "$(systemctl is-system-running 2>/dev/null || true)" in
	running|degraded) ;;
	*) echo "systemd is not ready" >&2; exit 1 ;;
esac

# Some development hosts provide IPv4 NAT for Docker but no routed IPv6. Keep
# this disposable network test deterministic; production users keep their
# normal APT transport policy.
printf 'Acquire::ForceIPv4 "true";\n' > /etc/apt/apt.conf.d/99provctl-release-smoke-ipv4

# Bootstrap owns package installation and service setup. The Dockerfile itself
# only proves that provctl was installed from the public APT repository.
apt-get update
provctl bootstrap --install-missing --yes
provctl doctor

provctl subscription create release
provctl website create release release.test --type static
printf '<!doctype html><title>provctl release smoke</title>release smoke\n' > /var/www/vhosts/release/sites/release.test/public/index.html

curl --fail --silent --show-error --resolve release.test:80:127.0.0.1 http://release.test/ | grep -q 'release smoke'
test -s /var/log/provctl/release/release.test/access.log

provctl website disable release release.test

# Apache may correctly fall back to its generated catch-all vhost with HTTP
# 200. What must disappear is this subscription's own content, not TCP/HTTP.
if curl --silent --show-error --resolve release.test:80:127.0.0.1 http://release.test/ | grep -q 'release smoke'; then
	echo "disabled website still served its own content" >&2
	exit 1
fi
provctl website enable release release.test
curl --fail --silent --show-error --resolve release.test:80:127.0.0.1 http://release.test/ | grep -q 'release smoke'

echo "PASS: GitHub Pages release package $actual bootstrapped and served release.test"
