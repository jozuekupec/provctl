FROM debian:13

ARG PROVCTL_APT_URL=https://jozuekupec.github.io/provctl/debian
ARG PROVCTL_APT_SUITE=stable

ENV container=docker

RUN apt-get update \
    && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
        ca-certificates curl gpg systemd systemd-sysv \
    && install -d -m 0755 /etc/apt/keyrings \
    && curl -fsSLo /tmp/provctl.asc "$PROVCTL_APT_URL/provctl.asc" \
    && gpg --batch --dearmor --output /etc/apt/keyrings/provctl.gpg /tmp/provctl.asc \
    && printf 'deb [signed-by=/etc/apt/keyrings/provctl.gpg] %s %s main\n' "$PROVCTL_APT_URL" "$PROVCTL_APT_SUITE" > /etc/apt/sources.list.d/provctl.list \
    && apt-get update \
    && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends provctl \
    && rm -rf /var/lib/apt/lists/* /tmp/provctl.asc

COPY smoke.sh /usr/local/lib/provctl-release-smoke
RUN chmod 0755 /usr/local/lib/provctl-release-smoke

STOPSIGNAL SIGRTMIN+3
CMD ["/sbin/init"]
