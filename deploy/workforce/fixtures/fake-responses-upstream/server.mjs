import http from "node:http";

const host = process.env.SEVEN_FAKE_PROVIDER_HOST || "127.0.0.1";
const port = Number.parseInt(process.env.SEVEN_FAKE_PROVIDER_PORT || "18080", 10);
const expectedKey = process.env.SEVEN_FAKE_PROVIDER_KEY || "seven-local-fake-provider-key";
const catalogModel = process.env.SEVEN_FAKE_PROVIDER_MODEL || "seven-fake-responses";
const maxBodyBytes = 128 * 1024 * 1024;

const metrics = {
  startedAt: new Date().toISOString(),
  total: 0,
  createJson: 0,
  createSSE: 0,
  compact: 0,
  rejected: 0,
  requestedModels: {},
};

function json(response, status, body) {
  response.writeHead(status, {
    "content-type": "application/json",
    "cache-control": "no-store",
    "x-request-id": `fake_req_${metrics.total}`,
  });
  response.end(JSON.stringify(body));
}

function safeError(response, status, code) {
  metrics.rejected += 1;
  json(response, status, { error: { type: "seven_fake_provider_error", code } });
}

// Slice 2's Provider probe deliberately supplies a strict JSON schema whose
// random enum challenge cannot be satisfied by a canned response.  This local
// fixture models a provider that genuinely honours that constrained contract:
// it derives its output solely from the submitted schema, and otherwise keeps
// the normal deterministic fake Responses behaviour.  It does not expose a
// switch controlled by a Task, Worker message, or environment variable.
function structuredProbeResponse(body) {
  const format = body?.text?.format;
  if (format?.type !== "json_schema" || format?.strict !== true || format?.name !== "independent_review_probe") {
    return null;
  }
  const challenge = format?.schema?.properties?.probeChallenge?.enum;
  const reviewProbe = format?.schema?.properties?.reviewProbe?.enum;
  if (!Array.isArray(challenge) || challenge.length !== 1 || typeof challenge[0] !== "string" ||
      !Array.isArray(reviewProbe) || reviewProbe.length !== 1 || reviewProbe[0] !== true) {
    return null;
  }
  return JSON.stringify({ reviewProbe: true, probeChallenge: challenge[0] });
}

// Codex Reviewer acceptance uses the same local fake Provider, but its
// structured-output request is a normal Responses call rather than the
// Provider capability probe above.  Return one fixed, schema-valid report so
// the real codex harness can prove its report-file plumbing without letting a
// task field or environment variable choose the verdict.  This fixture never
// represents a live model response.
function structuredReviewResponse(body) {
  const format = body?.text?.format;
  if (format?.type !== "json_schema" || format?.strict !== true || format?.schema?.type !== "object") {
    return null;
  }
  return JSON.stringify({
    schemaVersion: "review-report/v1",
    verdict: "PASS",
    score: 100,
    summary: "Fixed local structured-output acceptance report.",
    findings: [],
    reviewTests: [],
    limitations: [],
  });
}

function writeStructuredReviewStream(response, report, responseId, messageId, usage, model) {
  const outputItem = {
    type: "message",
    id: messageId,
    status: "in_progress",
    role: "assistant",
    content: [],
  };
  const contentPart = { type: "output_text", annotations: [] };
  response.write("event: response.created\n");
  response.write(`data: ${JSON.stringify({ type: "response.created", response: { id: responseId, object: "response", status: "in_progress" } })}\n\n`);
  response.write("event: response.output_item.added\n");
  response.write(`data: ${JSON.stringify({ type: "response.output_item.added", output_index: 0, item: outputItem })}\n\n`);
  response.write("event: response.content_part.added\n");
  response.write(`data: ${JSON.stringify({ type: "response.content_part.added", item_id: messageId, output_index: 0, content_index: 0, part: contentPart })}\n\n`);
  response.write("event: response.output_text.delta\n");
  response.write(`data: ${JSON.stringify({ type: "response.output_text.delta", item_id: messageId, output_index: 0, content_index: 0, delta: report, logprobs: [] })}\n\n`);
  response.write("event: response.output_text.done\n");
  response.write(`data: ${JSON.stringify({ type: "response.output_text.done", item_id: messageId, output_index: 0, content_index: 0, text: report })}\n\n`);
  response.write("event: response.content_part.done\n");
  response.write(`data: ${JSON.stringify({ type: "response.content_part.done", item_id: messageId, output_index: 0, content_index: 0, part: { ...contentPart, text: report } })}\n\n`);
  response.write("event: response.output_item.done\n");
  response.write(`data: ${JSON.stringify({ type: "response.output_item.done", output_index: 0, item: { ...outputItem, status: "completed", content: [{ ...contentPart, text: report }] } })}\n\n`);
  response.end(`event: response.completed\ndata: ${JSON.stringify({ type: "response.completed", response: { id: responseId, object: "response", status: "completed", model, output: [{ ...outputItem, status: "completed", content: [{ ...contentPart, text: report }] }], output_text: report, usage } })}\n\n`);
}

const server = http.createServer((request, response) => {
  if (request.method === "GET" && request.url === "/__seven/metrics") {
    json(response, 200, metrics);
    return;
  }
  if (request.method === "GET" && request.url === "/v1/models") {
    if (request.headers.authorization !== `Bearer ${expectedKey}`) {
      safeError(response, 401, "AUTH_REJECTED");
      return;
    }
    json(response, 200, {
      object: "list",
      data: [{ id: catalogModel, object: "model", owned_by: "seven-fixture" }],
    });
    return;
  }
  if (request.method !== "POST" || (request.url !== "/v1/responses" && request.url !== "/v1/responses/compact")) {
    safeError(response, 404, "NOT_FOUND");
    return;
  }
  if (request.headers.authorization !== `Bearer ${expectedKey}`) {
    safeError(response, 401, "AUTH_REJECTED");
    return;
  }

  const chunks = [];
  let size = 0;
  request.on("data", (chunk) => {
    size += chunk.length;
    if (size > maxBodyBytes) {
      request.destroy();
      return;
    }
    chunks.push(chunk);
  });
  request.on("end", () => {
    let body;
    try {
      body = JSON.parse(Buffer.concat(chunks).toString("utf8"));
    } catch {
      safeError(response, 400, "INVALID_JSON");
      return;
    }
    if (typeof body.model !== "string" || body.model.trim() !== body.model || !body.model ||
        Buffer.byteLength(body.model, "utf8") > 256 || [...body.model].some((value) => value.charCodeAt(0) < 32 || value.charCodeAt(0) === 127)) {
      safeError(response, 400, "MODEL_INVALID");
      return;
    }

    metrics.total += 1;
    metrics.requestedModels[body.model] = (metrics.requestedModels[body.model] || 0) + 1;
    const usage = {
      input_tokens: 7,
      input_tokens_details: { cached_tokens: 2 },
      cache_write_tokens: 1,
      output_tokens: 3,
      output_tokens_details: { reasoning_tokens: 1 },
      total_tokens: 10,
    };
    if (request.url === "/v1/responses/compact") {
      metrics.compact += 1;
      json(response, 200, { id: `resp_fake_compact_${metrics.total}`, object: "response.compaction", model: body.model, usage });
      return;
    }
    const review = structuredReviewResponse(body);
    if (body.stream === true) {
      metrics.createSSE += 1;
      response.writeHead(200, {
        "content-type": "text/event-stream",
        "cache-control": "no-store",
        connection: "keep-alive",
        "x-request-id": `fake_req_${metrics.total}`,
      });
      if (review !== null) {
        writeStructuredReviewStream(response, review, `resp_fake_${metrics.total}`, `msg_fake_${metrics.total}`, usage, body.model);
        return;
      }
      response.write("event: response.created\n");
      response.write(`data: ${JSON.stringify({ type: "response.created", response: { id: `resp_fake_${metrics.total}` } })}\n\n`);
      response.write("event: response.completed\n");
      response.end(`data: ${JSON.stringify({ type: "response.completed", response: { id: `resp_fake_${metrics.total}`, model: body.model, usage } })}\n\n`);
      return;
    }
    metrics.createJson += 1;
    const structured = structuredProbeResponse(body);
    if (structured !== null) {
      json(response, 200, {
        id: `resp_fake_${metrics.total}`,
        object: "response",
        status: "completed",
        model: body.model,
        output_text: structured,
        usage,
      });
      return;
    }
    if (review !== null) {
      json(response, 200, {
        id: `resp_fake_${metrics.total}`,
        object: "response",
        status: "completed",
        model: body.model,
        output_text: review,
        output: [{
          type: "message",
          id: `msg_fake_${metrics.total}`,
          role: "assistant",
          content: [{ type: "output_text", text: review, annotations: [] }],
        }],
        usage,
      });
      return;
    }
    json(response, 200, { id: `resp_fake_${metrics.total}`, object: "response", status: "completed", model: body.model, output: [], usage });
  });
});

server.listen(port, host, () => {
  process.stdout.write(`${JSON.stringify({ event: "fake-provider.ready", host, port: server.address().port, model: catalogModel })}\n`);
});

for (const signal of ["SIGINT", "SIGTERM"]) {
  process.on(signal, () => server.close(() => process.exit(0)));
}
