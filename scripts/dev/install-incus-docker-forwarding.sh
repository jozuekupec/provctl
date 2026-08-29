#!/bin/sh
set -eu

if [ "$(id -u)" -ne 0 ]; then
	echo "run this script with sudo" >&2
	exit 1
fi

uplink=$(ip route get 1.1.1.1 | awk '/dev/ { for (i = 1; i <= NF; i++) if ($i == "dev") { print $(i + 1); exit } }')
if [ -z "$uplink" ]; then
	echo "cannot determine the default IPv4 uplink" >&2
	exit 1
fi
case "$uplink" in
	*[!A-Za-z0-9_.:-]*) echo "unsafe uplink name: $uplink" >&2; exit 1 ;;
esac

while /usr/sbin/iptables -C DOCKER-USER -i incusbr0 -o "$uplink" -j ACCEPT 2>/dev/null; do
	/usr/sbin/iptables -D DOCKER-USER -i incusbr0 -o "$uplink" -j ACCEPT
done
while /usr/sbin/iptables -C DOCKER-USER -i "$uplink" -o incusbr0 -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT 2>/dev/null; do
	/usr/sbin/iptables -D DOCKER-USER -i "$uplink" -o incusbr0 -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT
done

install -D -m 0644 scripts/dev/incus-forwarding.conf /etc/sysctl.d/90-incus-forwarding.conf
sed "s/@UPLINK@/$uplink/g" scripts/dev/incus-docker-forward.service.in > /etc/systemd/system/incus-docker-forward.service
chmod 0644 /etc/systemd/system/incus-docker-forward.service
/usr/sbin/sysctl --system
systemctl daemon-reload
systemctl enable --now incus-docker-forward.service
systemctl status --no-pager incus-docker-forward.service
