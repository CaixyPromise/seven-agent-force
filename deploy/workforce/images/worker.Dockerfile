FROM golang:1.26.10-alpine AS build

WORKDIR /src
COPY seven-agent-force-server ./seven-agent-force-server
RUN cd seven-agent-force-server && CGO_ENABLED=0 go build -trimpath -o /out/seven-worker ./cmd/seven-worker

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/seven-worker /usr/local/bin/seven-worker
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/seven-worker"]
