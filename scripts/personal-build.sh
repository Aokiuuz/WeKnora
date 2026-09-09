#!/bin/sh
set -eu
cd /app
mkdir -p .local-service/bin/jieba
go build -buildvcs=false -tags sqlite_fts5 -o .local-service/bin/WeKnora.next ./cmd/server
dict_module=$(go list -m -f '{{.Dir}}' github.com/yanyiwu/gojieba)
chmod -R u+w .local-service/bin/jieba
cp -R "$dict_module"/deps/cppjieba/dict/. .local-service/bin/jieba/
chmod -R u+w .local-service/bin/jieba
mv .local-service/bin/WeKnora.next .local-service/bin/WeKnora
