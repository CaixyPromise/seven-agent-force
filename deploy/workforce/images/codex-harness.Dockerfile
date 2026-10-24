ARG RUNTIME_IMAGE=node:22-bookworm-slim

FROM golang:1.26.10-alpine AS build

ARG SEVEN_VERSION=0.1.0-dev
ARG SEVEN_COMMIT=0000000
ARG SEVEN_BUILD_DATE=unknown
ARG GO_PROXY=https://proxy.golang.org,direct

WORKDIR /src/seven-agent-force-server
COPY seven-agent-force-server ./
RUN GOPROXY="${GO_PROXY}" CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w -X github.com/CaixyPromise/seven-agent/force/internal/shared/buildinfo.Version=${SEVEN_VERSION} -X github.com/CaixyPromise/seven-agent/force/internal/shared/buildinfo.Commit=${SEVEN_COMMIT} -X github.com/CaixyPromise/seven-agent/force/internal/shared/buildinfo.BuildDate=${SEVEN_BUILD_DATE}" \
    -o /out/seven-worker-cli ./cmd/seven-worker-cli

FROM ${RUNTIME_IMAGE}

USER root

ARG CODEX_VERSION=0.147.0
ARG TARGETARCH
RUN if ! command -v git >/dev/null 2>&1 || ! command -v rg >/dev/null 2>&1 || ! command -v python3 >/dev/null 2>&1; then \
      apt-get update && apt-get install -y --no-install-recommends ca-certificates git ripgrep patch diffutils coreutils python3 && rm -rf /var/lib/apt/lists/*; \
    fi && \
    if ! command -v codex >/dev/null 2>&1; then \
      case "${TARGETARCH}" in \
        amd64) codex_platform=x64 ;; \
        arm64) codex_platform=arm64 ;; \
        *) echo "unsupported Codex architecture: ${TARGETARCH}" >&2; exit 1 ;; \
      esac && \
      npm install --global \
        "@openai/codex@${CODEX_VERSION}" \
        "@openai/codex-linux-${codex_platform}@npm:@openai/codex@${CODEX_VERSION}-linux-${codex_platform}"; \
    fi && \
    test "$(codex --version)" = "codex-cli ${CODEX_VERSION}" && \
    if ! id -u seven >/dev/null 2>&1; then \
      groupadd --gid 10001 seven && useradd --uid 10001 --gid seven --create-home seven; \
    fi && \
    mkdir -p /etc/codex /opt/seven-worker/contract /opt/seven-worker/assets/hooks /opt/seven-worker/assets/prompts /opt/seven-worker/assets/schemas /opt/seven-worker/result-processors && \
    true

ARG PNPM_VERSION=10.15.1
RUN npm install --global "pnpm@${PNPM_VERSION}" && \
    test "$(pnpm --version)" = "${PNPM_VERSION}"

ARG SEVEN_VERSION=0.1.0-dev
ARG SEVEN_COMMIT=0000000
ARG SEVEN_BUILD_DATE=unknown

COPY --from=build /out/seven-worker-cli /usr/local/bin/seven-worker-cli
COPY deploy/workforce/images/codex-harness-manifest.json /opt/seven-worker/contract/manifest.json
COPY deploy/workforce/images/codex-requirements.toml /etc/codex/requirements.toml
COPY deploy/workforce/result-runtime/ /opt/seven-worker/result-runtime/
COPY deploy/workforce/runtime-sdk/src/ /opt/seven-worker/result-runtime/
COPY deploy/workforce/adapters/codex/adapter.py /opt/seven/adapter/adapter.py
COPY deploy/workforce/adapters/codex/app_server.py /opt/seven/adapter/app_server.py
COPY deploy/workforce/runtime-sdk/build_image_adapter.py /opt/seven-worker/build-image-adapter.py
RUN harness_sha="$(sha256sum "$(readlink -f /usr/local/bin/codex)" | cut -d' ' -f1)" && \
    supervisor_sha="$(sha256sum /usr/local/bin/seven-worker-cli | cut -d' ' -f1)" && \
    python3 -c 'import hashlib,json,os,shutil,sys; path=os.path.realpath(shutil.which("python3")); digest=hashlib.sha256(open(path,"rb").read()).hexdigest(); open("/opt/seven-worker/result-runtime/python-runtime.json","w").write(json.dumps({"executable":path,"version":sys.version,"binarySha256":digest},sort_keys=True,separators=(",",":")))' && \
    find /opt/seven-worker/result-runtime/seven_agent /opt/seven-worker/result-runtime/seven_workforce_runtime -type f -exec sha256sum {} \; | LC_ALL=C sort > /opt/seven-worker/result-runtime/sdk-index.txt && \
    python3 -c 'import json,pkgutil; open("/opt/seven-worker/result-runtime/module-index.json","w").write(json.dumps(sorted({m.name for m in pkgutil.iter_modules()}),separators=(",",":")))' && \
    python_identity="$(sha256sum /opt/seven-worker/result-runtime/python-runtime.json | cut -d' ' -f1)" && \
    result_bootstrap_sha="$(sha256sum /opt/seven-worker/result-runtime/bootstrap.py | cut -d' ' -f1)" && \
    result_sdk_sha="$(sha256sum /opt/seven-worker/result-runtime/sdk-index.txt | cut -d' ' -f1)" && \
    result_module_index_sha="$(sha256sum /opt/seven-worker/result-runtime/module-index.json | cut -d' ' -f1)" && \
    sed -i "s/0.1.0-dev/${SEVEN_VERSION}/g; s/0000000/${SEVEN_COMMIT}/g; s/unknown/${SEVEN_BUILD_DATE}/g" /opt/seven-worker/contract/manifest.json && \
    sed -i "s/__HARNESS_SHA256__/${harness_sha}/g" /opt/seven-worker/contract/manifest.json && \
    sed -i "s/__SUPERVISOR_SHA256__/${supervisor_sha}/g" /opt/seven-worker/contract/manifest.json && \
    sed -i "s/__PYTHON_RUNTIME_IDENTITY__/${python_identity}/g; s/__RESULT_BOOTSTRAP_SHA256__/${result_bootstrap_sha}/g; s/__RESULT_SDK_SHA256__/${result_sdk_sha}/g; s/__RESULT_MODULE_INDEX_SHA256__/${result_module_index_sha}/g" /opt/seven-worker/contract/manifest.json && \
    ln -sfn /usr/local/bin/seven-worker-cli /usr/local/bin/seven-hook && \
    python3 /opt/seven-worker/build-image-adapter.py --source adapter.py && \
    chmod -R a-w /opt/seven/adapter && \
    chmod 0555 /opt/seven-worker/result-runtime/bootstrap.py && \
    chmod 0555 /usr/local/bin/seven-worker-cli && chmod -R a-w /opt/seven-worker /etc/codex

LABEL io.github.caixypromise.seven-worker.image.role="attempt" \
      io.github.caixypromise.seven-worker.image.contract="seven.worker-image/v1" \
      io.github.caixypromise.seven-worker.image.manifest="/opt/seven-worker/contract/manifest.json" \
      io.github.caixypromise.seven-worker.image.supervisor="/usr/local/bin/seven-worker-cli" \
      io.github.caixypromise.seven-worker.harness.type="codex"

USER 10001:10001
ENTRYPOINT ["/usr/local/bin/seven-worker-cli"]
CMD ["supervise"]
