#!/bin/sh

set -eu

die() {
  echo "kubectl-multi-get installer: $*" >&2
  exit 1
}

home=${HOME:-}
[ -n "$home" ] || die "HOME is not set"

command -v curl >/dev/null 2>&1 || die "curl is required"
command -v tar >/dev/null 2>&1 || die "tar is required"

if [ -n "${MULTI_GET_OS:-}" ]; then
  system_name=$MULTI_GET_OS
else
  system_name=$(uname -s)
fi

if [ -n "${MULTI_GET_ARCH:-}" ]; then
  machine_name=$MULTI_GET_ARCH
else
  machine_name=$(uname -m)
fi

case "$system_name" in
  Darwin)
    release_os=darwin
    ;;
  Linux)
    release_os=linux
    ;;
  *)
    die "unsupported operating system: $system_name (supported: macOS and Linux)"
    ;;
esac

case "$machine_name" in
  x86_64|amd64)
    release_arch=amd64
    ;;
  arm64|aarch64)
    release_arch=arm64
    ;;
  *)
    die "unsupported CPU architecture: $machine_name (supported: amd64 and arm64)"
    ;;
esac

base_url=${MULTI_GET_RELEASE_BASE_URL:-https://github.com/squatboy/multi-get/releases/latest/download}
base_url=${base_url%/}
archive_name="kubectl-multi-get_${release_os}_${release_arch}.tar.gz"
tmp_dir=
staged_target=

if ! tmp_dir=$(mktemp -d "${TMPDIR:-/tmp}/kubectl-multi-get-install.XXXXXX"); then
  die "could not create a temporary directory"
fi

cleanup() {
  rm -rf "$tmp_dir"
  if [ -n "$staged_target" ]; then
    rm -f "$staged_target"
  fi
}

trap cleanup 0

download() {
  url=$1
  destination=$2
  if ! curl -fsSL --retry 3 --retry-delay 1 --connect-timeout 10 "$url" -o "$destination"; then
    die "download failed: $url"
  fi
}

archive_path="$tmp_dir/$archive_name"
checksums_path="$tmp_dir/checksums.txt"

echo "Downloading kubectl-multi-get for $release_os/$release_arch..."
download "$base_url/$archive_name" "$archive_path"
download "$base_url/checksums.txt" "$checksums_path"

expected_checksum=$(awk -v archive="$archive_name" '$2 == archive || $2 == "*" archive { print $1; exit }' "$checksums_path")
case "$expected_checksum" in
  ''|*[!0-9A-Fa-f]*)
    die "checksum for $archive_name was not found"
    ;;
esac

if [ "${#expected_checksum}" -ne 64 ]; then
  die "invalid checksum for $archive_name"
fi

if command -v sha256sum >/dev/null 2>&1; then
  actual_checksum=$(sha256sum "$archive_path" | awk '{ print $1 }')
elif command -v shasum >/dev/null 2>&1; then
  actual_checksum=$(shasum -a 256 "$archive_path" | awk '{ print $1 }')
else
  die "sha256sum or shasum is required"
fi

expected_checksum=$(printf '%s' "$expected_checksum" | tr '[:upper:]' '[:lower:]')
actual_checksum=$(printf '%s' "$actual_checksum" | tr '[:upper:]' '[:lower:]')

if [ "$expected_checksum" != "$actual_checksum" ]; then
  die "checksum verification failed for $archive_name"
fi

extract_dir="$tmp_dir/extracted"
mkdir -p "$extract_dir"
if ! tar -xzf "$archive_path" -C "$extract_dir"; then
  die "could not extract $archive_name"
fi

binary_path="$extract_dir/kubectl-multi-get"
[ -f "$binary_path" ] || die "archive does not contain kubectl-multi-get"

install_dir="$home/.local/bin"
target_path="$install_dir/kubectl-multi-get"
if ! mkdir -p "$install_dir"; then
  die "could not create $install_dir"
fi

staged_target="$install_dir/.kubectl-multi-get.$$"
if ! cp "$binary_path" "$staged_target"; then
  die "could not stage kubectl-multi-get"
fi
if ! chmod 0755 "$staged_target"; then
  die "could not make kubectl-multi-get executable"
fi
if ! mv -f "$staged_target" "$target_path"; then
  die "could not install kubectl-multi-get into $install_dir"
fi
staged_target=

shell_name=${SHELL:-}
shell_name=${shell_name##*/}
shell_config=
case "$shell_name" in
  zsh)
    shell_config="$home/.zshrc"
    ;;
  bash)
    if [ "$release_os" = darwin ] && [ -f "$home/.bash_profile" ]; then
      shell_config="$home/.bash_profile"
    else
      shell_config="$home/.bashrc"
    fi
    ;;
esac

path_line='export PATH="$HOME/.local/bin:$PATH"'
path_status=
if [ -n "$shell_config" ]; then
  if [ -f "$shell_config" ] && grep -F '$HOME/.local/bin' "$shell_config" >/dev/null 2>&1; then
    path_status="already present"
  elif printf '\n# kubectl-multi-get\n%s\n' "$path_line" >> "$shell_config"; then
    path_status="added to $shell_config"
  else
    path_status="could not update $shell_config"
  fi
fi

echo "Installed kubectl-multi-get to $target_path."
if [ -n "$shell_config" ]; then
  echo "PATH entry: $path_status."
  echo "Reload your shell with: . \"$shell_config\""
else
  echo "Add $install_dir to PATH before running kubectl multi-get."
fi
