FROM cgr.dev/chainguard/go as build

WORKDIR /app
COPY . /app

ENV GOBIN /build
RUN CGO_ENABLED=0 GOOS=linux go install -a -trimpath -ldflags="-extldflags \"-static\" -buildid= -s -w" ./...

FROM cgr.dev/chainguard/static

WORKDIR /
COPY --from=build /build/net2proxy /net2proxy

ENTRYPOINT ["/net2proxy"]
