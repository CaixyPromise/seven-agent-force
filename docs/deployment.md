# Seven Agent Force deployment

This is a self-hosted preview deployment procedure. Preparing a package does not
start services, migrate a database, publish images or qualify a production host.
Use PostgreSQL for Workforce. Redis, RabbitMQ, S3-compatible artifact storage,
and a dedicated Linux Docker Worker are required for the complete task flow.
The Framework-only MySQL support is not a Workforce deployment option.

## Build and verify

From a reviewed, committed checkout, with Go, Node/pnpm and gitleaks installed:

```sh
make check
GOOS=linux GOARCH=amd64 make release VERSION=0.1.0-preview.1
# Choose arm64 instead for an ARM Linux host.
```

The build performs Web checks, dependency audits, Go vulnerability checks,
secret scanning, exact-version license/notice checks, MPL corresponding-source
packaging and checksum verification. Python 3.11 or newer is required for the
license gate. It installs pinned govulncheck and
Syft if missing, which requires network access. Never disable a failed gate to
label a package publishable. Build on a trusted machine without private Web
build-time environment overrides: Vite embeds VITE_* values into public assets.

The archive under `dist/release/` contains:

```text
bin/seven-agent-force-server
bin/seven-agent-force-mcp-server
bin/seven-worker
bin/seven-workerctl
bin/seven-worker-cli
bin/seven-provider-proxy
configs/application.example.yaml
configs/mcp.example.yaml
migrations/{postgres,mysql}
web/dist/
deploy/nginx/seven-framework.conf.example
deploy/workforce/runtime-sdk/
docs/deployment.md
docs/workforce/mvp-runbook.md
metadata/{go.mod,go.sum,package.json,pnpm-lock.yaml}
RELEASE.txt
LICENSE, NOTICE, README.md
THIRD_PARTY_NOTICES.md, third_party/
SHA256SUMS, sbom.spdx.json
```

Verify the external archive SHA256 and then run `shasum -a 256 -c SHA256SUMS`
inside the extracted package. `RELEASE.txt` identifies version, source commit,
OS and architecture. The internal Worker CLI and Provider Proxy binaries are
image components, not public network services to start on the API host.

Runtime/Proxy images are separate artifacts. Build them from the same reviewed
source using the Makefile image targets; Dockerfiles require the full source
checkout. Record registry references and immutable digests in the deployment
record. Do not use the Makefile's development tags as production identities.
Do not claim that shipping the SDK or binaries ships a runnable Agent image.
Third-party Coding CLI installation and redistribution terms require review
before publishing images. No image push is performed by this package script.

## Configuration and isolation

Use `/opt/seven-agent-force/releases/<version>` for immutable package contents;
keep configuration and persistent data outside it. Create private configuration
from the examples, not from a developer's `.local` directory. The application
example is deliberately disabled and is not a production-ready preset.

Configure PostgreSQL, Redis, RabbitMQ, artifact endpoint/bucket/TLS and secrets;
enable Workforce and the intended Review behavior. Follow the detailed
[Worker and runtime runbook](workforce/mvp-runbook.md) for registration, admission
and model setup. Assign persistent workspace, hook cache, token and Proxy control
paths instead of the example `/tmp` paths. Keep Worker tokens readable only by
the Worker account. Configure the actual public HTTPS origin explicitly.

Use separate API and Worker accounts/configuration directories. Only the Worker
account may access its dedicated Docker Engine. API/MCP services and Attempt
containers must never receive the Docker socket. Attempts expose no ports.
Prefer a dedicated rootless Linux Worker; qualify the selected host configuration
before accepting untrusted workloads. Configure exact admitted Runtime and Proxy
digests, capacity, disk limits and allowed egress. Never expand egress merely to
make a failed task pass.

## Installation order (operator actions, not run by packaging)

1. Back up the database, artifact storage and necessary recovery/configuration
   material; verify the restore procedure. Inspect migration status against the
   intended database. Existing local unpublished schemas require the documented
   alignment path; never copy a development database into production.
2. With Worker dispatch stopped, run the selected release's migration command:

   ```sh
   /opt/seven-agent-force/current/bin/seven-agent-force-server migrate \
     --home /opt/seven-agent-force/current \
     --config-dir /etc/seven-agent-force/api status
   # After reviewing status and backup, use the same command with `up`.
   ```

3. Start the API under your process manager:

   ```sh
   /opt/seven-agent-force/current/bin/seven-agent-force-server serve \
     --home /opt/seven-agent-force/current \
     --config-dir /etc/seven-agent-force/api
   ```

4. Finish normal setup/login, provision the Worker using the supported
   registration flow, and start the Worker with its private configuration:

   ```sh
   /opt/seven-agent-force/current/bin/seven-worker \
     --config-dir /etc/seven-agent-force/worker
   ```

5. Optionally expose MCP after configuring `mcp.yaml`:

   ```sh
   /opt/seven-agent-force/current/bin/seven-agent-force-mcp-server \
     --config /etc/seven-agent-force/mcp.yaml
   ```

   Set `openAPI.baseURL` to the actual API `/api/` URL (the MCP example port is
   not the API default). Set allowed hosts/origins to the externally served
   endpoint. MCP requires the caller's Application API Key; do not embed one
   shared user key in the proxy. See the source checkout's
   `docs/workforce/protocols/` for methods, parameters and types.

6. Serve `web/dist` with the Nginx template. Its historical filename is retained.
   Replace all `__...__` placeholders, add HTTPS and configure proxy request-size
   and timeout limits to match intended uploads and streaming routes. The template
   only routes `/api/`; MCP needs an explicit separate reverse-proxy endpoint.
   Preserve Authorization, Host and Origin validation, and disable buffering for
   streaming endpoints. Do not expose development Vite directly.

## First-host acceptance and rollback

Record package SHA, image digests and configuration revision. Check normal login,
API readiness, Worker heartbeat and admitted capacity, and MCP unauthorized
rejection. With explicit Provider-cost authorization, run one isolated task
through queue, execution, Review, result download/hash verification, then verify
Worker restart/recovery and cancellation. Git delivery needs separate authorized
repository/branch credentials; never reuse the original acceptance task.

Monitor queue wait, stale Worker heartbeats, disk space, artifact failures and
unknown delivery outcomes. Resolve unknown external writes by readback before
retry. Keep logs free of credentials and raw Provider payloads.

For upgrades, drain dispatch and preserve in-flight work, install beside the
old package, inspect migrations and switch services in a controlled order.
Keep the previous package and configuration. Binary rollback is only safe when
the schema remains compatible; otherwise restore the reviewed backup under a
maintenance window. Do not rerun Agents to repair packaging/Review/delivery.
Target-host acceptance, backup restore and production load/fault qualification
remain deployment gates, not claims made by a local build.
