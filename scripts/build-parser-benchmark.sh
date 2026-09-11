#!/bin/sh
set -eu
cd /workspace
mkdir -p artifacts/parser-benchmark/bin/jieba
gofmt -w cmd/parser-benchmark/*.go
go test -tags sqlite_fts5 ./cmd/parser-benchmark
go build -buildvcs=false -tags sqlite_fts5 \
  -ldflags "-X main.commitID=${PARSER_BENCHMARK_COMMIT:-unknown}" \
  -o artifacts/parser-benchmark/bin/parser-benchmark ./cmd/parser-benchmark
dict_module=$(go list -m -f '{{.Dir}}' github.com/yanyiwu/gojieba)
cp "$dict_module"/deps/cppjieba/dict/* artifacts/parser-benchmark/bin/jieba/
