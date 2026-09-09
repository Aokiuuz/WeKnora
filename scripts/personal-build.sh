#!/bin/sh
set -eu
cd /app
mkdir -p .local-service/bin/jieba
version=$(tr -d '\r\n' < VERSION)
build_time=$(date -u '+%Y-%m-%dT%H:%M:%SZ')
go build -buildvcs=false -tags sqlite_fts5 \
  -ldflags "-X github.com/Tencent/WeKnora/internal/buildinfo.Version=$version -X github.com/Tencent/WeKnora/internal/buildinfo.BuildTime=$build_time -X github.com/Tencent/WeKnora/internal/buildinfo.CommitID=${WEKNORA_BUILD_COMMIT:-unknown}" \
  -o .local-service/bin/WeKnora.next ./cmd/server
dict_module=$(go list -m -f '{{.Dir}}' github.com/yanyiwu/gojieba)
chmod -R u+w .local-service/bin/jieba
cp -R "$dict_module"/deps/cppjieba/dict/. .local-service/bin/jieba/
chmod -R u+w .local-service/bin/jieba
mv .local-service/bin/WeKnora.next .local-service/bin/WeKnora
