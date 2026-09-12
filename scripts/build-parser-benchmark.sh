#!/bin/sh
set -eu
cd /workspace
mkdir -p artifacts/parser-benchmark/bin/jieba
binary_path="${PARSER_BENCHMARK_BINARY:-artifacts/parser-benchmark/bin/parser-benchmark}"
if [ -e "$binary_path" ]; then
  echo 'The selected executable is frozen. Set PARSER_BENCHMARK_BINARY to a new path for another build.' >&2
  exit 1
fi
gofmt -w cmd/parser-benchmark/*.go
go test -tags sqlite_fts5 ./cmd/parser-benchmark
go build -buildvcs=false -tags sqlite_fts5 \
  -ldflags "-X main.commitID=${PARSER_BENCHMARK_COMMIT:-unknown}" \
  -o "$binary_path" ./cmd/parser-benchmark
dict_module=$(go list -m -f '{{.Dir}}' github.com/yanyiwu/gojieba)
for source_dictionary in "$dict_module"/deps/cppjieba/dict/*; do
  target_dictionary="artifacts/parser-benchmark/bin/jieba/$(basename "$source_dictionary")"
  if [ -e "$target_dictionary" ]; then
    cmp -s "$source_dictionary" "$target_dictionary" || {
      echo 'The frozen Jieba dictionary differs. Build in a separate workspace to preserve this run.' >&2
      exit 1
    }
  else
    cp "$source_dictionary" "$target_dictionary"
  fi
done
