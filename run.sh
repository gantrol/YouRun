#!/usr/bin/env bash
set -euo pipefail

usage() {
  printf '%s\n' \
    'Usage: bash run.sh [mygo|wails|mygo-native] [--rebuild-frontend]' \
    'Default: mygo. Uses go run; reuses unchanged frontend output.'
}

framework=mygo
framework_set=false
rebuild_frontend=false
for arg in "$@"; do
  case "$arg" in
    mygo|wails|mygo-native)
      if "$framework_set"; then
        printf 'Choose one framework.\n' >&2
        exit 2
      fi
      framework=$arg
      framework_set=true
      ;;
    --rebuild-frontend) rebuild_frontend=true ;;
    -h|--help) usage; exit 0 ;;
    *) printf 'Unknown argument: %s\n' "$arg" >&2; usage >&2; exit 2 ;;
  esac
done

project_dir=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
frontend_dir="$project_dir/frontend"

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    printf 'Required command not found: %s\n' "$1" >&2
    exit 1
  fi
}

frontend_changed() {
  local entry="$frontend_dir/dist/index.html" input
  if "$rebuild_frontend" || [[ ! -s "$entry" || ! -d "$frontend_dir/dist/assets" ]]; then
    return 0
  fi
  # Include directories so removing a source file also invalidates the output.
  for input in "$frontend_dir/src" "$frontend_dir/public" \
    "$frontend_dir/index.html" "$frontend_dir/package.json" \
    "$frontend_dir/package-lock.json" "$frontend_dir/tsconfig.json" \
    "$frontend_dir/vite.config.ts" "$project_dir/resources/locales" \
    "$project_dir/resources/skins.json" "$project_dir/resources/branding/yourun-rabbit-paths.json"; do
    if [[ -e "$input" ]] && [[ -n "$(find "$input" -iname '*trash*' -prune -o -newer "$entry" -print)" ]]; then
      return 0
    fi
  done
  return 1
}

require_command go
if [[ "$framework" != mygo-native ]] && frontend_changed; then
  require_command node
  require_command npm
  (
    cd -- "$frontend_dir"
    installed=node_modules/.package-lock.json
    if [[ ! -f "$installed" || package-lock.json -nt "$installed" || package.json -nt "$installed" ]]; then
      npm ci --ignore-scripts --no-audit --no-fund
    fi
    npm run build
  )
fi

# A normal development launch must not impersonate an update candidate.
unset COMMAND_MANAGER_UPDATE_PARENT COMMAND_MANAGER_UPDATE_TOKEN COMMAND_MANAGER_UPDATE_REDIRECT
cd -- "$project_dir/$framework"
exec go run .
