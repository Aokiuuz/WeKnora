#!/bin/sh
set -eu
# Linux-local source and dependencies avoid Windows bind-mount preprocessor timeouts.
build_dir=$(mktemp -d /tmp/weknora-frontend.XXXXXX)
tar -C /app/frontend --exclude=node_modules --exclude=dist --exclude=.git -cf - . | tar -C "$build_dir" -xf -
cd "$build_dir"
npm ci --no-audit --no-fund
npm run build
# Keep the served assets intact until compilation has completed successfully.
mkdir -p /app/.local-service/dist
cp -R dist/. /app/.local-service/dist/
