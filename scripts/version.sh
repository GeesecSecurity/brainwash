# shellcheck shell=bash
# Resolve the app version from BRAINWASH_VERSION, then the git tag on HEAD.

brainwash_version() {
  local require_tag="${1:-}"
  if [[ -n "${BRAINWASH_VERSION:-}" ]]; then
    echo "${BRAINWASH_VERSION#v}"
    return 0
  fi
  local root
  root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
  local tag
  if tag=$(git -C "$root" describe --tags --exact-match HEAD 2>/dev/null); then
    echo "${tag#v}"
    return 0
  fi
  if [[ "$require_tag" == "--release" ]]; then
    echo "HEAD is not tagged. Tag first (git tag 0.1.1) or set BRAINWASH_VERSION." >&2
    return 1
  fi
  if tag=$(git -C "$root" describe --tags --always --dirty 2>/dev/null); then
    echo "${tag#v}"
    return 0
  fi
  echo "0.0.0-dev"
}

brainwash_commit() {
  local root
  root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
  git -C "$root" rev-parse --short HEAD 2>/dev/null || echo unknown
}

brainwash_date() {
  date -u +%Y-%m-%dT%H:%M:%SZ
}

brainwash_ldflags() {
  local version="${1:-}"
  if [[ -z "$version" ]]; then
    version="$(brainwash_version)"
  fi
  local commit date
  commit="$(brainwash_commit)"
  date="$(brainwash_date)"
  printf -- '-s -w -X brainwash/internal/version.Version=%s -X brainwash/internal/version.Commit=%s -X brainwash/internal/version.Date=%s' "$version" "$commit" "$date"
}

brainwash_stamp_plist() {
  local plist="$1"
  local version="$2"
  /usr/libexec/PlistBuddy -c "Set :CFBundleShortVersionString $version" "$plist" || \
    /usr/libexec/PlistBuddy -c "Add :CFBundleShortVersionString string $version" "$plist"
  /usr/libexec/PlistBuddy -c "Set :CFBundleVersion $version" "$plist" || \
    /usr/libexec/PlistBuddy -c "Add :CFBundleVersion string $version" "$plist"
}
