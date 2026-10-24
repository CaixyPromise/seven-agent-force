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
ARG CLAUDE_CODE_VERSION=2.1.273
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates git ripgrep patch diffutils coreutils python3 && \
    rm -rf /var/lib/apt/lists/* && \
    npm install --global "@anthropic-ai/claude-code@${CLAUDE_CODE_VERSION}" && \
    claude --version | grep -F "${CLAUDE_CODE_VERSION}" && \
    groupadd --gid 10001 seven && useradd --uid 10001 --gid seven --create-home seven && \
    mkdir -p /opt/seven/adapter /opt/seven-worker/contract /opt/seven-worker/result-runtime /opt/seven-worker/assets/hooks

ARG PNPM_VERSION=10.15.1
RUN npm install --global "pnpm@${PNPM_VERSION}" && test "$(pnpm --version)" = "${PNPM_VERSION}"

ARG SEVEN_VERSION=0.1.0-dev
ARG SEVEN_COMMIT=0000000
ARG SEVEN_BUILD_DATE=unknown
COPY --from=build /out/seven-worker-cli /usr/local/bin/seven-worker-cli
COPY deploy/workforce/images/claude-harness-manifest.json /opt/seven-worker/contract/manifest.json
COPY deploy/workforce/result-runtime/ /opt/seven-worker/result-runtime/
COPY deploy/workforce/runtime-sdk/src/ /opt/seven-worker/result-runtime/
COPY deploy/workforce/adapters/claude/adapter.py /opt/seven/adapter/adapter.py
COPY deploy/workforce/adapters/claude/mcp_bridge.py /opt/seven/adapter/mcp_bridge.py
COPY deploy/workforce/runtime-sdk/build_image_adapter.py /opt/seven-worker/build-image-adapter.py
RUN harness_sha="$(sha256sum "$(readlink -f /usr/local/bin/claude)" | cut -d' ' -f1)" && \
    supervisor_sha="$(sha256sum /usr/local/bin/seven-worker-cli | cut -d' ' -f1)" && \
    python3 -c 'import hashlib,json,os,shutil,sys; p=os.path.realpath(shutil.which("python3")); open("/opt/seven-worker/result-runtime/python-runtime.json","w").write(json.dumps({"executable":p,"version":sys.version,"binarySha256":hashlib.sha256(open(p,"rb").read()).hexdigest()},sort_keys=True,separators=(",",":")))' && \
    find /opt/seven-worker/result-runtime/seven_agent /opt/seven-worker/result-runtime/seven_workforce_runtime -type f -exec sha256sum {} \; | LC_ALL=C sort > /opt/seven-worker/result-runtime/sdk-index.txt && \
    python3 -c 'import json,pkgutil; open("/opt/seven-worker/result-runtime/module-index.json","w").write(json.dumps(sorted({m.name for m in pkgutil.iter_modules()}),separators=(",",":")))' && \
    python_identity="$(sha256sum /opt/seven-worker/result-runtime/python-runtime.json | cut -d' ' -f1)" && \
    result_bootstrap_sha="$(sha256sum /opt/seven-worker/result-runtime/bootstrap.py | cut -d' ' -f1)" && \
    result_sdk_sha="$(sha256sum /opt/seven-worker/result-runtime/sdk-index.txt | cut -d' ' -f1)" && \
    result_module_index_sha="$(sha256sum /opt/seven-worker/result-runtime/module-index.json | cut -d' ' -f1)" && \
    sed -i "s/0.1.0-dev/${SEVEN_VERSION}/g; s/0000000/${SEVEN_COMMIT}/g; s/unknown/${SEVEN_BUILD_DATE}/g; s/__HARNESS_SHA256__/${harness_sha}/g; s/__SUPERVISOR_SHA256__/${supervisor_sha}/g; s/__PYTHON_RUNTIME_IDENTITY__/${python_identity}/g; s/__RESULT_BOOTSTRAP_SHA256__/${result_bootstrap_sha}/g; s/__RESULT_SDK_SHA256__/${result_sdk_sha}/g; s/__RESULT_MODULE_INDEX_SHA256__/${result_module_index_sha}/g" /opt/seven-worker/contract/manifest.json && \
    ln -sfn /usr/local/bin/seven-worker-cli /usr/local/bin/seven-hook && \
    python3 /opt/seven-worker/build-image-adapter.py --source adapter.py && \
    chmod -R a-w /opt/seven/adapter /opt/seven-worker && chmod 0555 /usr/local/bin/seven-worker-cli

LABEL io.github.caixypromise.seven-worker.image.role="attempt" \
      io.github.caixypromise.seven-worker.image.contract="seven.worker-image/v1" \
      io.github.caixypromise.seven-worker.image.manifest="/opt/seven-worker/contract/manifest.json" \
      io.github.caixypromise.seven-worker.image.supervisor="/usr/local/bin/seven-worker-cli" \
      io.github.caixypromise.seven-worker.harness.type="claude-code"
USER 10001:10001
ENTRYPOINT ["/usr/local/bin/seven-worker-cli"]
CMD ["supervise"]
