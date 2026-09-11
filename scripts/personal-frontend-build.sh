#!/bin/sh
set -eu
# Linux-local source and dependencies avoid Windows bind-mount preprocessor timeouts.
build_dir=$(mktemp -d /tmp/weknora-frontend.XXXXXX)
tar -C /app/frontend --exclude=node_modules --exclude=dist --exclude=.git -cf - . | tar -C "$build_dir" -xf -
cd "$build_dir"
export NODE_OPTIONS="${NODE_OPTIONS:---max-old-space-size=4608}"
if [ -f /app/artifacts/parser-benchmark/report/index.html ]; then
  export VITE_PARSER_BENCHMARK_URL="${VITE_PARSER_BENCHMARK_URL:-http://127.0.0.1:18090}"
fi
npm ci --no-audit --no-fund
npm run build
# Keep the served assets intact until compilation has completed successfully.
mkdir -p /app/.local-service/dist
cp -R dist/. /app/.local-service/dist/
