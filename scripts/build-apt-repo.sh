#!/bin/sh
# Build from a fresh input tree containing stable/*.deb and testing/*.deb.
set -eu

incoming=${1:?usage: build-apt-repo.sh INPUT_DIRECTORY NEW_OUTPUT_DIRECTORY}
output=${2:?usage: build-apt-repo.sh INPUT_DIRECTORY NEW_OUTPUT_DIRECTORY}
: "${APT_SIGNING_FPR:?set the signing subkey fingerprint}"

case "$APT_SIGNING_FPR" in
  *[!A-Fa-f0-9]*|'') echo 'invalid signing fingerprint' >&2; exit 2 ;;
esac
test "${#APT_SIGNING_FPR}" -eq 40
test -d "$incoming"
test ! -e "$output"
mkdir -p "$output/debian"
output=$(cd "$output" && pwd)
incoming=$(cd "$incoming" && pwd)
cp "${APT_PUBLIC_KEY:-packaging/apt/provctl.asc}" "$output/debian/provctl.asc"

# Per-suite pool directories retain all historical versions without collisions.
for suite in stable testing; do
  pool="$output/debian/pool/main/p/provctl/$suite"
  mkdir -p "$pool"
  for deb in "$incoming/$suite/"*.deb; do
    test -f "$deb" || continue
    test "$(dpkg-deb -f "$deb" Package)" = provctl
    arch=$(dpkg-deb -f "$deb" Architecture)
    case "$arch" in amd64|arm64) ;; *) echo "unsupported architecture: $arch" >&2; exit 2 ;; esac
    cp "$deb" "$pool/"
  done
  for arch in amd64 arm64; do
    index="$output/debian/dists/$suite/main/binary-$arch"
    mkdir -p "$index"
    (cd "$output/debian" && apt-ftparchive -a "$arch" packages "pool/main/p/provctl/$suite") > "$index/Packages"
    gzip -n -9 -c "$index/Packages" > "$index/Packages.gz"
  done
  release="$output/debian/dists/$suite/Release"
  (cd "$output/debian" && apt-ftparchive \
    -o APT::FTPArchive::Release::Origin=provctl \
    -o APT::FTPArchive::Release::Label=provctl \
    -o APT::FTPArchive::Release::Codename="$suite" \
    -o APT::FTPArchive::Release::Suite="$suite" \
    -o APT::FTPArchive::Release::Architectures='amd64 arm64' \
    -o APT::FTPArchive::Release::Components=main \
    release "dists/$suite") > "$output/release.tmp"
  mv "$output/release.tmp" "$release"
  for mode in clearsign detach-sign; do
    target="$release.gpg"
    test "$mode" != clearsign || target="$output/debian/dists/$suite/InRelease"
    printf '%s\n' "${APT_GPG_PASSPHRASE:-}" |
      gpg --batch --pinentry-mode loopback --passphrase-fd 0 \
        --local-user "${APT_SIGNING_FPR}!" --armor --output "$target" "--$mode" "$release"
  done
done

# Independently verify using only the distributed public key.
keyring="$output/verify.gpg"
gpg --batch --dearmor --output "$keyring" "$output/debian/provctl.asc"
for suite in stable testing; do
  gpgv --keyring "$keyring" "$output/debian/dists/$suite/InRelease"
  gpgv --keyring "$keyring" "$output/debian/dists/$suite/Release.gpg" "$output/debian/dists/$suite/Release"
done
