#!/bin/sh
set -eu

if [ -n "${VERSION:-}" ]; then
  version=$VERSION
else
  described=$(git describe --tags --always --dirty 2>/dev/null || printf 'dev')
  case "$described" in
    v[0-9]*) version=${described#v} ;;
    [0-9]*) version=$described ;;
    *) version=0.0.0+git.$described ;;
  esac
fi

case "$version" in
  [0-9]*) ;;
  *) echo "Debian package version must begin with a digit: $version" >&2; exit 2 ;;
esac
arch=${ARCH:-$(go env GOARCH)}

case "$arch" in
  amd64|arm64) ;;
  *) echo "unsupported Debian architecture: $arch" >&2; exit 2 ;;
esac

mkdir -p dist
GOOS=linux GOARCH=$arch CGO_ENABLED=0 go build -trimpath -ldflags "-X provctl/internal/meta.Version=$version" -o dist/provctl ./cmd/provctl
VERSION=$version ARCH=$arch nfpm package --config packaging/nfpm.yaml --packager deb --target dist
