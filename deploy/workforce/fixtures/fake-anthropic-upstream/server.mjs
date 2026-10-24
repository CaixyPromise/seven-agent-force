import http from "node:http";

const host = process.env.SEVEN_FAKE_ANTHROPIC_HOST || "127.0.0.1";
const port = Number.parseInt(process.env.SEVEN_FAKE_ANTHROPIC_PORT || "18081", 10);
const expectedKey = process.env.SEVEN_FAKE_ANTHROPIC_KEY || "seven-local-fake-anthropic-key";
const primaryModel = process.env.SEVEN_FAKE_ANTHROPIC_MODEL || "claude-seven-primary";
const maxBodyBytes = 1 << 20;

const metrics = {
  startedAt: new Date().toISOString(),
  total: 0,
  messagesJson: 0,
  messagesSSE: 0,
  countTokens: 0,
  rejected: 0,
  requestedModels: {},
};

function json(response, status, body) {
  response.writeHead(status, {
    "content-type": "application/json",
    "cache-control": "no-store",
    "request-id": `anthropic_fake_${metrics.total}`,
  });
  response.end(JSON.stringify(body));
}

function safeError(response, status, code) {
  metrics.rejected += 1;
  json(response, status, { type: "error", error: { type: "seven_fake_anthropic_error", message: code } });
}

function authenticated(request) {
  return request.headers["x-api-key"] === expectedKey &&
    typeof request.headers["anthropic-version"] === "string" && request.headers["anthropic-version"].length > 0;
}

function validModel(model) {
  return typeof model === "string" && model.trim() === model && model.length > 0 &&
    Buffer.byteLength(model, "utf8") <= 256 &&
    ![...model].some((value) => value.charCodeAt(0) < 32 || value.charCodeAt(0) === 127);
}

function usage() {
  return {
    input_tokens: 11,
    cache_read_input_tokens: 4,
    cache_creation_input_tokens: 2,
    output_tokens: 5,
  };
}

function readBody(request, response, callback) {
  const chunks = [];
  let size = 0;
  request.on("data", (chunk) => {
    size += chunk.length;
    if (size > maxBodyBytes) {
      safeError(response, 413, "REQUEST_TOO_LARGE");
      request.destroy();
      return;
    }
    chunks.push(chunk);
  });
  request.on("end", () => {
    try {
      callback(JSON.parse(Buffer.concat(chunks).toString("utf8")));
    } catch {
      safeError(response, 400, "INVALID_JSON");
    }
  });
}

function writeStream(response, model) {
  const id = `msg_fake_${metrics.total}`;
  const initial = usage();
  response.writeHead(200, {
    "content-type": "text/event-stream",
    "cache-control": "no-store",
    connection: "keep-alive",
    "request-id": `anthropic_fake_${metrics.total}`,
  });
  response.write(`event: message_start\ndata: ${JSON.stringify({
    type: "message_start",
    message: { id, type: "message", role: "assistant", model, content: [], stop_reason: null,
      usage: { ...initial, output_tokens: 0 } },
  })}\n\n`);
  response.write(`event: content_block_start\ndata: ${JSON.stringify({
    type: "content_block_start", index: 0, content_block: { type: "text", text: "" },
  })}\n\n`);
  response.write(`event: content_block_delta\ndata: ${JSON.stringify({
    type: "content_block_delta", index: 0, delta: { type: "text_delta", text: "done" },
  })}\n\n`);
  response.write(`event: content_block_stop\ndata: ${JSON.stringify({ type: "content_block_stop", index: 0 })}\n\n`);
  response.write(`event: message_delta\ndata: ${JSON.stringify({
    type: "message_delta", delta: { stop_reason: "end_turn", stop_sequence: null },
    usage: { output_tokens: initial.output_tokens },
  })}\n\n`);
  response.end(`event: message_stop\ndata: ${JSON.stringify({ type: "message_stop" })}\n\n`);
}

const server = http.createServer((request, response) => {
  if (request.method === "GET" && request.url === "/__seven/metrics") {
    json(response, 200, metrics);
    return;
  }
  if (!authenticated(request)) {
    safeError(response, 401, "AUTH_REJECTED");
    return;
  }
  if (request.method === "GET" && request.url?.startsWith("/v1/models")) {
    const url = new URL(request.url, `http://${host}`);
    if (!url.searchParams.get("after_id")) {
      json(response, 200, {
        data: [{ id: primaryModel, type: "model", display_name: "Seven Primary" }],
        has_more: true,
        first_id: primaryModel,
        last_id: primaryModel,
      });
    } else {
      json(response, 200, {
        data: [{ id: "claude-seven-secondary", type: "model", display_name: "Seven Secondary" }],
        has_more: false,
        first_id: "claude-seven-secondary",
        last_id: "claude-seven-secondary",
      });
    }
    return;
  }
  if (request.method !== "POST" || (request.url !== "/v1/messages" && request.url !== "/v1/messages/count_tokens")) {
    safeError(response, 404, "NOT_FOUND");
    return;
  }
  readBody(request, response, (body) => {
    if (!validModel(body.model)) {
      safeError(response, 400, "MODEL_INVALID");
      return;
    }
    metrics.total += 1;
    metrics.requestedModels[body.model] = (metrics.requestedModels[body.model] || 0) + 1;
    if (request.url === "/v1/messages/count_tokens") {
      metrics.countTokens += 1;
      json(response, 200, { input_tokens: usage().input_tokens });
      return;
    }
    if (body.stream === true) {
      metrics.messagesSSE += 1;
      writeStream(response, body.model);
      return;
    }
    metrics.messagesJson += 1;
    json(response, 200, {
      id: `msg_fake_${metrics.total}`,
      type: "message",
      role: "assistant",
      model: body.model,
      content: [{ type: "text", text: "done" }],
      stop_reason: "end_turn",
      stop_sequence: null,
      usage: usage(),
    });
  });
});

server.listen(port, host, () => {
  process.stdout.write(`${JSON.stringify({ event: "fake-anthropic.ready", host, port: server.address().port, model: primaryModel })}\n`);
});

for (const signal of ["SIGINT", "SIGTERM"]) {
  process.on(signal, () => server.close(() => process.exit(0)));
}
