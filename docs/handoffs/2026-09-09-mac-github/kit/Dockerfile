FROM golang:1.26.7-bookworm@sha256:e8c859f5632dcfde7b32d2012b4351728f6437930887c2f6a91ea242459e5514
RUN apt-get update && apt-get install -y --no-install-recommends python3 libsqlite3-dev build-essential ca-certificates git
ENV CGO_ENABLED=1 GOTOOLCHAIN=local GOMAXPROCS=2 GOFLAGS="-mod=readonly -p=2" PYTHONDONTWRITEBYTECODE=1
WORKDIR /source
COPY source.tar /tmp/source.tar
RUN tar -xf /tmp/source.tar -C /source
COPY source-identity.json /kit/source-identity.json
COPY in-container.sh summarize.py /kit/
RUN go mod download && go mod verify
RUN go build -tags sqlite_fts5 -o /usr/local/bin/weknora-server ./cmd/server
RUN go build -o /usr/local/bin/evaluation-reproduce ./cmd/evaluation-reproduce
ENTRYPOINT ["bash", "/kit/in-container.sh"]
