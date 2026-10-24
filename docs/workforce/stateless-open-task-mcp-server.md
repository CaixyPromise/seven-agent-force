# Stateless Open Task MCP Server

## Purpose

Seven Agent Force exposes a remote MCP adapter for the existing Open Task REST
contract. The adapter is an independent, stateless process. It does not own Task
creation rules, credentials, Worker dispatch, result storage, or Webhook
delivery.

```text
remote MCP client
  -> POST /mcp with an Integration Application API Key
  -> seven-agent-force-mcp-server
  -> authenticated Open REST
  -> TaskSubmission / ResultOutputAccess
```

The authoritative task and output contracts remain under
`/api/open/workforce/v1`. MCP is a protocol adapter over those contracts, not a
second task implementation.

## Integration Application credentials

An operator creates an application on `/workforce/open-integrations`. Every
application receives a one-time `swk_...` API Key. The caller stores that Key in
its own Secret Manager and sends it on every MCP POST:

```http
Authorization: Bearer swk_<locator>.<secret>
```

The MCP Server has no global customer Key and does not persist, cache, or log a
request Key. It forwards the same Bearer credential to Open REST. Open REST
resolves the tenant and Integration Application and enforces Task and Output
ownership.

Webhook is optional. Leaving the Webhook address empty creates an MCP/Open REST
application with an API Credential only. Supplying an admitted HTTPS Webhook
address additionally creates a Signing Credential and enables terminal-event
delivery. A `whsec_...` Signing Secret is used only by an external Webhook
receiver; it is never an MCP credential.

API Key rotation remains application-scoped. The new Key is current
immediately, the previous Key remains in grace for 15 minutes, and every MCP
request is reauthenticated. Updating the client Secret therefore needs no MCP
session reset or MCP Server restart.

## Protocol

The endpoint implements MCP `2026-07-28` over Stateless Streamable HTTP with
the pinned official Go SDK `github.com/modelcontextprotocol/go-sdk v1.7.0`.

- The only endpoint is `POST /mcp`.
- Responses are JSON; SSE is disabled.
- No `Mcp-Session-Id`, Redis session, GET stream, DELETE session, resumability,
  Resource, Prompt, Sampling, or Elicitation surface is exposed.
- The 2026 protocol uses `server/discover` and per-request MCP `_meta`. It does
  not use the legacy `initialize` / `notifications/initialized` handshake.
- Requests are authenticated before MCP dispatch by calling
  `GET /api/open/workforce/v1/capabilities` with the request Bearer Key.
- Authentication results are request-scoped and are not cached.

The public reverse proxy terminates TLS and forwards `/mcp` to the MCP process,
which listens on loopback by default. Machine clients may omit `Origin`; when an
Origin is present it must exactly match the configured allowlist.

## Tools

The server publishes exactly six Tools:

| Tool | Behavior |
|---|---|
| `list_model_options` | Reads configured model credential references for an authenticated active Project, without upstream discovery. |
| `list_runtime_options` | Reads compatible published RuntimeRelease choices for the Project and model credential. |
| `create_task` | Sends an application-scoped idempotent Open Task request and returns immediately after `202 Accepted`. |
| `get_task` | Returns the bounded public Task state and safe reason/summary. |
| `list_task_outputs` | Lists at most 64 declared `IMMEDIATE + AVAILABLE` outputs. |
| `read_task_output` | Reads one verified UTF-8 `text/*` or `application/json` output up to 256 KiB. |

Task and Output identities are decimal strings in Tool input and output. They
are parsed to Go `int64` only inside the adapter and remain PostgreSQL `BIGINT`
identities in the authoritative service.

`create_task` requires `projectId`, `instruction`, `externalCorrelationId`,
`modelSelection: {"credentialId":"88","model":"configured-model"}`,
`runtimeReleaseId` and an explicit `idempotencyKey`. `name` is optional.
Credential and runtime IDs are positive decimal int64 strings, never JavaScript
numbers. `credentialId` references a tenant-authorized Provider credential;
raw Provider secrets are never accepted. Optional fields mirror Open REST:
`tags`, `referenceAssetIds`, `additionalCompletionCheck`, `reviewEnabled`,
`deliveryPreference` and `executionTimeoutSeconds`.

Capabilities declare `requiresModelSelection`, `requiresRuntimeReleaseId` and
`supportsSelectionDiscovery`. For a known Project ID, call:

```http
GET /api/open/workforce/v1/projects/77/model-options
GET /api/open/workforce/v1/projects/77/runtime-options?modelCredentialId=88
```

The MCP equivalents are `list_model_options({"projectId":"77"})` and
`list_runtime_options({"projectId":"77","modelCredentialId":"88"})`.
Model items contain only `credentialId`, `model`, `protocol`, `label`.
Runtime items contain only `runtimeReleaseId`, `name`, `recommended`, with
`projectVersionId` on the response. Each query authenticates the Application
and validates the same tenant-level active Project visibility as creation;
it does not inherit the owner's operator RBAC. Queries reuse the configured
Provider catalog and existing Runtime compatibility rules. They do not call
upstream model discovery, reserve capacity or authorize later delivery.
Creation revalidates the Application, model and Runtime selections.
Project listing/provisioning remains in the operator console; callers need a
known Project ID. Provider secrets, image references, Worker topology, adapter
internals and capacity are omitted from public discovery responses.

Public state maps automatic `REVIEWING` to `RUNNING`, `INTERACTION_SUSPENDED`
to `ACTION_REQUIRED` with `TASK_INTERACTION_REQUIRED`, and `MANUAL_REVIEW`
to `ACTION_REQUIRED`. ReviewDelivery decisions remain separate from Task state.
Open/MCP expose selection discovery, creation, polling and outputs. Answer/supplement,
cancel, independent review decisions and delivery decisions exist in operator
REST with their existing permission checks; exposing them to an Integration
Application requires shared ownership/authorization contracts before adapters
can reuse them. Task responses provide a relative `links.operator` such as
`/workforce/tasks/77` without credentials. This phase is not a fully external
interaction loop.

A client that loses the
HTTP response retries with the same Key and input; Open REST returns the
original Task rather than starting another Worker Attempt. Open creation now
uses the transaction-owned Application guard and writes Application principal,
`OPEN_REST` channel and correlation with the Task graph. The application-scoped
key is unchanged, but its request hash includes Application and correlation.
Historical Open records written with the Web hash can therefore reject the
same-key replay with `IDEMPOTENCY_CONFLICT`; they are not migrated or retried
under a fresh key automatically. This preserves refusal rather than duplicate
creation/execution. PostgreSQL concurrent replay/disable behavior requires
separate isolated integration evidence; local DTO tests alone do not prove it.

`read_task_output` first verifies that the output appears in the authenticated
Task output list, then streams the public download. Before returning text it
verifies the declared size, SHA-256 and UTF-8 encoding. Binary, oversized,
cross-Task, cross-application and integrity-drifted content fails closed.

## Configuration

The process configuration contains network and resource limits only. It never
contains a customer API Key:

```yaml
server:
  listenAddress: "127.0.0.1:18081"
  endpointPath: "/mcp"
  allowedOrigins: []
  allowedHosts:
    - "127.0.0.1:18081"
    - "localhost:18081"
  readTimeoutSeconds: 10
  writeTimeoutSeconds: 60

protocol:
  version: "2026-07-28"
  maxRequestBodyBytes: 262144
  jsonResponseOnly: true

openAPI:
  baseURL: "http://127.0.0.1:9277/api"
  timeoutSeconds: 15

limits:
  ingressConcurrency: 64
  globalConcurrency: 64
  perApplicationConcurrency: 8
  outputTextLimitBytes: 262144
```

Client configuration must inject the API Key from a protected value:

```yaml
mcpServers:
  seven-workforce:
    url: https://workforce.example.com/mcp
    headers:
      Authorization: Bearer ${SEVEN_WORKFORCE_API_KEY}
```

Whether a specific client can securely inject a static Authorization Header is
a client compatibility requirement. This release does not put the Key in a Tool
argument, URL query, checked-in file, browser localStorage, or shared MCP Server
configuration.

Every HTTP `Host` is checked independently of `Origin`. A reverse proxy that
publishes another hostname must add that exact public hostname (and port, when
the port should be pinned) to `allowedHosts`. Browser-style requests with a
non-empty `Origin` must also match an exact entry in `allowedOrigins`; machine
clients may omit `Origin`, but they do not bypass the Host allowlist.

## Security and evidence boundary

The MCP process has no database, RabbitMQ, Docker, Worker, Provider, object
store, or Signing Secret access. Its Open REST client disables environment
proxies and redirects and uses bounded timeouts and response reads. The ingress
limit bounds concurrent unauthenticated body reads separately from global and
per-application execution limits; these in-process limits are not a distributed
tenant quota.

Logs allow only request identity, authenticated Application ID, MCP method or
Tool name, safe result code, duration, status and response size. Authorization,
request bodies, instructions, output text, object keys and raw upstream errors
are excluded.

The repeatable local acceptance extends the existing Open Task vertical slice:

```text
MCP-only application
  -> server/discover and tools/list
  -> create_task
  -> Dispatcher / Worker / FakeHarness / Finalizer
  -> get_task SUCCEEDED
  -> list_task_outputs
  -> read_task_output with size and SHA-256 verification
  -> idempotent replay
  -> exactly one Worker Attempt
  -> zero Public Events and Webhook Deliveries for the MCP-only application
```

That evidence proves the local deterministic adapter and execution path. It is
not public-TLS deployment, real Provider, distributed load, production, or
business acceptance.
