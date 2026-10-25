# Running a containerized Linux Worker

The Worker is a trusted data-plane process. It needs Git, an SSH client and the Docker CLI as well as the Worker binary. The supplied `deploy/workforce/images/worker.Dockerfile` includes these tools and runs as non-root UID/GID `10001:10001`, matching the standard Runtime images' private-file ownership. Do not make private output or token files world-readable to work around a UID mismatch.

## Filesystem and daemon access

A Docker bind-mount source is resolved by the daemon, not by the Docker client. When the Worker itself runs in a container, its workspace, temporary files, token and Proxy control paths must exist at the same absolute paths in both the Worker and daemon filesystem views. An ordinary container-private `/tmp` does not satisfy that requirement: image admission also bind-mounts a temporary Processor fixture.

For example, prepare a private directory on a dedicated Linux Worker host:

```sh
sudo install -d -o 10001 -g 10001 -m 0700 \
  /srv/seven-agent-force/worker/config \
  /srv/seven-agent-force/worker/workspaces \
  /srv/seven-agent-force/worker/tmp \
  /srv/seven-agent-force/worker/proxy
```

Provision the Worker through the operator console or authenticated `POST /api/workforce/workers`. Save the returned token once, mode `0600`, owned by UID 10001. Write a separate Worker `application.yaml` containing only its Worker, queue, artifact and admitted-image configuration. Do not copy the API datasource, browser-signing keys or model credentials into the Worker.

Match these configuration paths to the shared directory:

```yaml
workforce:
  enabled: true
  worker:
    tokenFile: /srv/seven-agent-force/worker/worker.token
    workspaceRoot: /srv/seven-agent-force/worker/workspaces
    dockerHost: unix:///var/run/docker.sock
  providerProxy:
    controlRoot: /srv/seven-agent-force/worker/proxy
```

The snippet is a path example, not a complete configuration. Set the provisioned numeric Worker ID, reachable API, RabbitMQ and artifact endpoints, container budget and verified image identities as well.

A container launch then uses the same paths and the actual socket group:

```sh
SOCKET_GID="$(stat -c '%g' /var/run/docker.sock)"
docker run --name seven-force-worker \
  --group-add "$SOCKET_GID" \
  -e DOCKER_HOST=unix:///var/run/docker.sock \
  -e TMPDIR=/srv/seven-agent-force/worker/tmp \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v /srv/seven-agent-force/worker:/srv/seven-agent-force/worker \
  YOUR_REVIEWED_WORKER_IMAGE \
  --config-dir /srv/seven-agent-force/worker/config
```

Only the trusted Worker receives access to its dedicated daemon. Never mount that socket into the API, MCP or Attempt containers. Match socket group access; do not use `chmod 666` or run the Worker as root. A Docker-in-Docker test environment additionally needs shared paths between the Worker and that daemon container; the host socket example alone does not create such isolation.

## Image identities

OCI registry manifest digests and Docker local image/config IDs are different identities. After pulling a reviewed immutable image on the actual Worker engine:

```sh
docker image inspect --format '{{.Id}}' YOUR_IMAGE_AT_MANIFEST_DIGEST
```

Use the immutable registry reference for `providerProxy.image`, and the inspected Docker config ID for `providerProxy.imageDigest`. Worker managed-image allowlists also use those inspected IDs. Admission verifies that the reference resolves to the expected config ID before use. A bare exact `sha256:<config-id>` is also an immutable local reference; floating tags remain rejected for Provider Proxy configuration.

Obtain these values from the engine that executes tasks. An ID reported by a separate development image store is not sufficient evidence about the Worker engine. Keep the control plane's expected Proxy identity consistent with the admitted Worker inventory.

## Synthetic acceptance scope

Default Fake images are `TEST_ONLY` and intentionally excluded from normal project task selection. They also differ by the SDK capabilities their fixture implements. A disposable end-to-end fixture must satisfy the complete current task capability contract and be explicitly made schedulable inside its isolated environment. Do not advertise missing capabilities, weaken the scheduler or rewrite database admission records to make it selectable. A synthetic success is not real Provider or production acceptance.

First verify normal owner setup, organization membership, Worker heartbeat and image admission. Then submit a deterministic task, verify platform completion checks and Review, and compare the downloaded result against known bytes and hashes. Keep failed attempts as evidence and preserve frozen Runtime selections when preparing a corrected fixture.
