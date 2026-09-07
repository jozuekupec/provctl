#!/bin/sh
set -eu

version=${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || printf '0.0.0~dev')}
arch=${ARCH:-$(go env GOARCH)}

case "$arch" in
  amd64|arm64) ;;
  *) echo "unsupported Debian architecture: $arch" >&2; exit 2 ;;
esac

mkdir -p dist
GOOS=linux GOARCH=$arch CGO_ENABLED=0 go build -trimpath -ldflags "-X provctl/internal/meta.Version=$version" -o dist/provctl ./cmd/provctl
VERSION=$version ARCH=$arch nfpm package --config packaging/nfpm.yaml --packager deb --target dist
