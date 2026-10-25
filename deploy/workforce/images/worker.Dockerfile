FROM golang:1.26.10-alpine AS build

WORKDIR /src
COPY seven-agent-force-server ./seven-agent-force-server
RUN cd seven-agent-force-server && CGO_ENABLED=0 go build -trimpath -o /out/seven-worker ./cmd/seven-worker

# The daemon invokes Git/SSH for materialization and delivery, and Docker CLI
# for isolated runtime operations; a static-only image cannot execute tasks.
FROM alpine:3.22
RUN apk add --no-cache ca-certificates git openssh-client docker-cli && \
    addgroup -g 10001 worker && \
    adduser -D -u 10001 -G worker -h /home/worker worker

COPY --from=build /out/seven-worker /usr/local/bin/seven-worker
USER 10001:10001
ENTRYPOINT ["/usr/local/bin/seven-worker"]
