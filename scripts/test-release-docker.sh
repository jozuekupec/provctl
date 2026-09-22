#!/bin/sh
# Build and exercise a disposable Debian 13 systemd server from the public APT
# repository. It intentionally needs privileged Docker/cgroup access, but it
# never mounts the workspace or host paths into the container.
set -eu

if [ "${PROVCTL_ALLOW_PRIVILEGED:-}" != "1" ]; then
	echo "refusing privileged Docker server test on this host" >&2
	echo "use the Incus E2 environment, or run only in a disposable VM with:" >&2
	echo "  PROVCTL_ALLOW_PRIVILEGED=1 $0" >&2
	exit 2
fi

repo=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
image=${PROVCTL_DOCKER_IMAGE:-provctl-release-server:test}
name="provctl-release-test-$$"
expected=${PROVCTL_EXPECTED_VERSION:-0.1.7}

cleanup() {
	docker rm --force "$name" >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM

docker build --pull --no-cache --tag "$image" --file "$repo/docker/release-server/Dockerfile" "$repo/docker/release-server"
docker run --detach --rm --privileged --cgroupns=private --name "$name" "$image" >/dev/null

attempt=0
while [ "$attempt" -lt 60 ]; do
	case "$(docker exec "$name" systemctl is-system-running 2>/dev/null || true)" in
		running|degraded) break ;;
	esac
	attempt=$((attempt + 1))
	sleep 1
done
if [ "$attempt" -eq 60 ]; then
	docker logs "$name" >&2 || true
	echo "systemd did not become ready within 60 seconds" >&2
	exit 1
fi

docker exec --env "PROVCTL_EXPECTED_VERSION=$expected" "$name" /usr/local/lib/provctl-release-smoke
