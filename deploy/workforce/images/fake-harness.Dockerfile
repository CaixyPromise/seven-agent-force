ARG BUILD_IMAGE=golang:1.26.10-alpine
ARG RUNTIME_IMAGE=alpine:3.22

FROM ${BUILD_IMAGE} AS build

ARG SEVEN_VERSION=0.1.0-dev
ARG SEVEN_COMMIT=0000000
ARG SEVEN_BUILD_DATE=unknown

WORKDIR /src
COPY seven-agent-force-server ./seven-agent-force-server
RUN cd seven-agent-force-server && CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w -X github.com/CaixyPromise/seven-agent/force/internal/shared/buildinfo.Version=${SEVEN_VERSION} -X github.com/CaixyPromise/seven-agent/force/internal/shared/buildinfo.Commit=${SEVEN_COMMIT} -X github.com/CaixyPromise/seven-agent/force/internal/shared/buildinfo.BuildDate=${SEVEN_BUILD_DATE}" \
    -o /out/seven-worker-cli ./cmd/seven-worker-cli

FROM ${RUNTIME_IMAGE}

ARG PNPM_VERSION=10.15.1

USER root

RUN (command -v git >/dev/null && command -v node >/dev/null && command -v python3 >/dev/null && test -f /etc/ssl/certs/ca-certificates.crt || apk add --no-cache ca-certificates git nodejs python3) && \
    (grep -q '^seven:' /etc/group || addgroup -g 10001 seven) && \
    (grep -q '^seven:' /etc/passwd || adduser -D -u 10001 -G seven seven) && \
    mkdir -p /etc/codex /opt/seven-worker/contract /opt/seven-worker/assets/hooks /opt/seven-worker/assets/prompts /opt/seven-worker/assets/schemas /opt/seven-worker/result-processors && \
    chown -R root:root /opt/seven-worker

# Admission verifies the declared package-manager.pnpm capability.
RUN apk add --no-cache npm && npm install --global "pnpm@${PNPM_VERSION}" && \
    test "$(pnpm --version)" = "${PNPM_VERSION}"

ARG SEVEN_VERSION=0.1.0-dev
ARG SEVEN_COMMIT=0000000

COPY --from=build /out/seven-worker-cli /usr/local/bin/seven-worker-cli
ARG SEVEN_FAKE_BATCH=batch2
COPY deploy/workforce/images/fake-harness-manifest.json /opt/seven-worker/contract/manifest.json
COPY deploy/workforce/images/fake-harness-batch3-manifest.json /opt/seven-worker/contract/batch3-manifest.json
COPY deploy/workforce/result-runtime/ /opt/seven-worker/result-runtime/
COPY deploy/workforce/runtime-sdk/src/ /opt/seven-worker/result-runtime/
COPY deploy/workforce/adapters/fake/adapter.py deploy/workforce/adapters/fake/combined.py deploy/workforce/adapters/fake/batch3.py deploy/workforce/adapters/fake/interaction.py deploy/workforce/adapters/fake/tool.py deploy/workforce/adapters/fake/git_delivery_tool.py /opt/seven/adapter/
COPY deploy/workforce/adapters/fake/fixtures/ /opt/seven/adapter/fixtures/
COPY deploy/workforce/runtime-sdk/build_image_adapter.py /opt/seven-worker/build-image-adapter.py
RUN case "$SEVEN_FAKE_BATCH" in \
      batch2) adapter_source=combined.py ;; \
      batch3) cp /opt/seven-worker/contract/batch3-manifest.json /opt/seven-worker/contract/manifest.json; adapter_source=batch3.py ;; \
      batch4) adapter_source=tool.py; python3 -c 'import json; p="/opt/seven-worker/contract/manifest.json"; v=json.load(open(p)); v["adapter"]["version"]="batch4-tool"; v["adapter"]["capabilities"]=sorted(set(v["adapter"]["capabilities"]+["tool.request","tool.result","tool.received","tool.closed"])); json.dump(v,open(p,"w"),sort_keys=True,separators=(",",":"))' ;; \
      batch5) adapter_source=git_delivery_tool.py; python3 -c 'import json; p="/opt/seven-worker/contract/manifest.json"; v=json.load(open(p)); v["adapter"]["version"]="batch5-git-delivery-tool"; v["adapter"]["capabilities"]=["activity","execution.interrupt","execution.start","tool.closed","tool.received","tool.request","tool.result"]; json.dump(v,open(p,"w"),sort_keys=True,separators=(",",":"))' ;; \
      *) exit 1 ;; \
    esac && \
    ln -sfn /usr/local/bin/seven-worker-cli /usr/local/bin/seven-fake-harness && \
    ln -sfn /usr/local/bin/seven-worker-cli /usr/local/bin/seven-hook && \
    touch /etc/codex/requirements.toml && \
    python3 -c 'import hashlib,json,os,shutil,sys; path=os.path.realpath(shutil.which("python3")); digest=hashlib.sha256(open(path,"rb").read()).hexdigest(); open("/opt/seven-worker/result-runtime/python-runtime.json","w").write(json.dumps({"executable":path,"version":sys.version,"binarySha256":digest},sort_keys=True,separators=(",",":")))' && \
    find /opt/seven-worker/result-runtime/seven_agent /opt/seven-worker/result-runtime/seven_workforce_runtime -type f -exec sha256sum {} \; | LC_ALL=C sort > /opt/seven-worker/result-runtime/sdk-index.txt && \
    python3 -c 'import json,pkgutil; open("/opt/seven-worker/result-runtime/module-index.json","w").write(json.dumps(sorted({m.name for m in pkgutil.iter_modules()}),separators=(",",":")))' && \
    sed -i "s/0.1.0-dev/${SEVEN_VERSION}/g; s/0000000/${SEVEN_COMMIT}/g" /opt/seven-worker/contract/manifest.json && \
    binary_sha="$(sha256sum /usr/local/bin/seven-worker-cli | cut -d' ' -f1)" && \
    python_identity="$(sha256sum /opt/seven-worker/result-runtime/python-runtime.json | cut -d' ' -f1)" && \
    result_bootstrap_sha="$(sha256sum /opt/seven-worker/result-runtime/bootstrap.py | cut -d' ' -f1)" && \
    result_sdk_sha="$(sha256sum /opt/seven-worker/result-runtime/sdk-index.txt | cut -d' ' -f1)" && \
    result_module_index_sha="$(sha256sum /opt/seven-worker/result-runtime/module-index.json | cut -d' ' -f1)" && \
    sed -i "s/__BINARY_SHA256__/${binary_sha}/g" /opt/seven-worker/contract/manifest.json && \
    sed -i "s/__PYTHON_RUNTIME_IDENTITY__/${python_identity}/g; s/__RESULT_BOOTSTRAP_SHA256__/${result_bootstrap_sha}/g; s/__RESULT_SDK_SHA256__/${result_sdk_sha}/g; s/__RESULT_MODULE_INDEX_SHA256__/${result_module_index_sha}/g" /opt/seven-worker/contract/manifest.json && \
    python3 /opt/seven-worker/build-image-adapter.py --source "$adapter_source" && \
    chmod -R a-w /opt/seven/adapter && \
    chmod 0555 /usr/local/bin/seven-worker-cli && \
    chmod 0555 /opt/seven-worker/result-runtime/bootstrap.py && \
    chmod -R a-w /opt/seven-worker /etc/codex

LABEL io.github.caixypromise.seven-worker.image.role="attempt" \
      io.github.caixypromise.seven-worker.image.contract="seven.worker-image/v1" \
      io.github.caixypromise.seven-worker.image.manifest="/opt/seven-worker/contract/manifest.json" \
      io.github.caixypromise.seven-worker.image.supervisor="/usr/local/bin/seven-worker-cli" \
      io.github.caixypromise.seven-worker.harness.type="fake"

USER 10001:10001
ENTRYPOINT ["/usr/local/bin/seven-worker-cli"]
CMD ["supervise"]
