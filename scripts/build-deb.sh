#!/bin/sh
set -eu

if [ -n "${VERSION:-}" ]; then
  version=$VERSION
else
  if tag=$(git describe --tags --exact-match 2>/dev/null); then
    case "$tag" in
      v[0-9]*) version=${tag#v} ;;
      [0-9]*) version=$tag ;;
      *) echo "release tag must begin with v followed by a digit: $tag" >&2; exit 2 ;;
    esac
  else
    revision=$(git rev-parse --short HEAD 2>/dev/null || printf 'dev')
    dirty=
    if ! git diff --quiet 2>/dev/null; then
      dirty=-dirty
    fi
    version=0.0.0+git.$revision$dirty
  fi
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
