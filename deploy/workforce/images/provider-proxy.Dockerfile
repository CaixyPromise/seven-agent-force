FROM golang:1.26.10-alpine AS build

ARG SEVEN_VERSION=0.2.4-provider-gateway
ARG SEVEN_COMMIT=0000000
ARG SEVEN_BUILD_DATE=unknown

WORKDIR /src/seven-agent-force-server
COPY seven-agent-force-server ./
COPY deploy/workforce/images/provider-proxy-manifest.json /tmp/provider-proxy-manifest.json
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X github.com/CaixyPromise/seven-agent/force/internal/shared/buildinfo.Version=${SEVEN_VERSION} -X github.com/CaixyPromise/seven-agent/force/internal/shared/buildinfo.Commit=${SEVEN_COMMIT} -X github.com/CaixyPromise/seven-agent/force/internal/shared/buildinfo.BuildDate=${SEVEN_BUILD_DATE}" -o /out/seven-provider-proxy ./cmd/seven-provider-proxy && \
    proxy_sha="$(sha256sum /out/seven-provider-proxy | cut -d' ' -f1)" && \
    sed -i "s/0.2.4-provider-gateway/${SEVEN_VERSION}/g; s/0000000/${SEVEN_COMMIT}/g; s/unknown/${SEVEN_BUILD_DATE}/g; s/__PROXY_SHA256__/${proxy_sha}/g" /tmp/provider-proxy-manifest.json

FROM gcr.io/distroless/static-debian12:nonroot

ARG SEVEN_VERSION=0.2.4-provider-gateway
ARG SEVEN_COMMIT=0000000

COPY --from=build /out/seven-provider-proxy /usr/local/bin/seven-provider-proxy
COPY --from=build /tmp/provider-proxy-manifest.json /opt/seven-worker/contract/manifest.json
LABEL io.github.caixypromise.seven-worker.image.contract="seven.worker-image/v1" \
      io.github.caixypromise.seven-worker.image.role="provider-proxy" \
      io.github.caixypromise.seven-worker.image.manifest="/opt/seven-worker/contract/manifest.json" \
      io.github.caixypromise.seven-worker.provider-gateway.mode="gateway-client" \
      io.github.caixypromise.seven-worker.version="${SEVEN_VERSION}"
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/seven-provider-proxy"]
