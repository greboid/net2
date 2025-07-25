FROM golang:1.24 as build

WORKDIR /app
COPY . /app

ENV GOBIN /build
RUN CGO_ENABLED=0 GOOS=linux go install -a -trimpath -ldflags="-extldflags \"-static\" -buildid= -s -w" ./...

FROM ghcr.io/greboid/dockerbase/nonroot:1.20250716.0

WORKDIR /
COPY --from=build /build/net2proxy /net2proxy

ENTRYPOINT ["/net2proxy"]
