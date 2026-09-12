#!/bin/sh
set -eu
mkdir -p /tmp/frontend
tar -C /app/frontend --exclude=node_modules --exclude=dist --exclude=.git -cf - . | tar -C /tmp/frontend -xf -
cd /tmp/frontend
npm ci --no-audit --no-fund
npm run test -- src/views/evaluation/EvaluationWorkbench.test.ts src/i18n/localeKeyAudit.test.ts
npm run type-check
npm run build > /app/artifacts/parser-benchmark/frontend-build.log 2>&1
mkdir -p /app/artifacts/parser-benchmark/frontend-dist
cp -R dist/. /app/artifacts/parser-benchmark/frontend-dist/
