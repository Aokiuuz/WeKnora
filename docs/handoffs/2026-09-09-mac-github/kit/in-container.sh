#!/usr/bin/env bash
set -euo pipefail
cd /source
export GITHUB_SHA
GITHUB_SHA="$(python3 -c 'import json; print(json.load(open("/kit/source-identity.json"))["commit"])')"
export JIEBA_DICT_DIR
JIEBA_DICT_DIR="$(go list -m -f '{{.Dir}}' github.com/yanyiwu/gojieba)/deps/cppjieba/dict"
test -f "$JIEBA_DICT_DIR/jieba.dict.utf8"
cp /kit/source-identity.json /evidence/source-identity.json
{
  go version
  go env GOOS GOARCH CGO_ENABLED GOTOOLCHAIN GOFLAGS
  python3 --version
  gcc --version
  g++ --version
  make --version
  cat /etc/os-release
  dpkg-query -W
  python3 -c 'import sqlite3; print("Python SQLite", sqlite3.sqlite_version)'
} > /evidence/software-versions.txt
go version -m /usr/local/bin/weknora-server > /evidence/server-build.txt
{
  go version -m /usr/local/bin/weknora-server
  go version -m /usr/local/bin/evaluation-reproduce
} > /evidence/go-modules.txt
sha256sum /usr/local/bin/weknora-server /usr/local/bin/evaluation-reproduce > /evidence/binaries.sha256
find dataset -type f -print0 | sort -z | xargs -0 sha256sum > /evidence/dataset-files.sha256
set +e
make evaluation-reproduce EVALUATION_REPORT_DIR=/evidence/golden 2>&1 | tee /evidence/golden.log
golden_exit=${PIPESTATUS[0]}
python3 scripts/prepare-public-evaluation.py --check > /evidence/public-check.log 2>&1
public_exit=$?
python3 -B scripts/prepare-public-evaluation-test.py > /evidence/public-tests.log 2>&1
public_tests_exit=$?
python3 scripts/check-product-version.py > /evidence/product-version.log 2>&1
version_exit=$?
python3 scripts/evaluation-rag-http-smoke.py --server-binary /usr/local/bin/weknora-server --output /evidence/http 2>&1 | tee /evidence/http.log
http_exit=${PIPESTATUS[0]}
set -e
python3 /kit/summarize.py /evidence "$golden_exit" "$public_exit" "$public_tests_exit" "$version_exit" "$http_exit"
