import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { once } from "node:events";
import { createInterface } from "node:readline";
import test from "node:test";
import { fileURLToPath } from "node:url";

async function fixture(path, env) {
  const child = spawn(process.execPath, [fileURLToPath(path)], { env: { ...process.env, ...env }, stdio: ["ignore", "pipe", "pipe"] });
  const lines = createInterface({ input: child.stdout });
  const [line] = await once(lines, "line");
  const ready = JSON.parse(line);
  return {
    child,
    baseUrl: `http://${ready.host}:${ready.port}`,
    async close() {
      const exited = once(child, "exit");
      child.kill("SIGTERM");
      await exited;
    },
  };
}

test("OpenAI catalog is not a runtime model allowlist", async () => {
  const upstream = await fixture(new URL("./fake-responses-upstream/server.mjs", import.meta.url), {
    SEVEN_FAKE_PROVIDER_PORT: "0",
  });
  try {
    const headers = { authorization: "Bearer seven-local-fake-provider-key", "content-type": "application/json" };
    const models = await fetch(`${upstream.baseUrl}/v1/models`, { headers });
    assert.equal(models.status, 200);
    assert.deepEqual((await models.json()).data.map((model) => model.id), ["seven-fake-responses"]);

    for (const model of ["seven-fake-responses", "codex-autoreview-not-in-catalog"]) {
      const response = await fetch(`${upstream.baseUrl}/v1/responses`, {
        method: "POST", headers, body: JSON.stringify({ model, input: "test" }),
      });
      assert.equal(response.status, 200);
      assert.equal((await response.json()).model, model);
    }
    const metrics = await (await fetch(`${upstream.baseUrl}/__seven/metrics`)).json();
    assert.equal(metrics.requestedModels["codex-autoreview-not-in-catalog"], 1);
  } finally {
    await upstream.close();
  }
});

test("Anthropic discovers pages and accepts an auxiliary model outside the catalog", async () => {
  const upstream = await fixture(new URL("./fake-anthropic-upstream/server.mjs", import.meta.url), {
    SEVEN_FAKE_ANTHROPIC_PORT: "0",
  });
  try {
    const headers = {
      "x-api-key": "seven-local-fake-anthropic-key",
      "anthropic-version": "2023-06-01",
      "content-type": "application/json",
    };
    const first = await (await fetch(`${upstream.baseUrl}/v1/models`, { headers })).json();
    assert.equal(first.has_more, true);
    const second = await (await fetch(`${upstream.baseUrl}/v1/models?after_id=${first.last_id}`, { headers })).json();
    assert.equal(second.has_more, false);

    for (const model of ["claude-seven-primary", "claude-autoreview-not-in-catalog"]) {
      const response = await fetch(`${upstream.baseUrl}/v1/messages`, {
        method: "POST", headers, body: JSON.stringify({ model, max_tokens: 32, messages: [], stream: false }),
      });
      assert.equal(response.status, 200);
      const body = await response.json();
      assert.equal(body.model, model);
      assert.equal(body.usage.cache_read_input_tokens, 4);
      assert.equal(body.usage.cache_creation_input_tokens, 2);
    }
    const stream = await fetch(`${upstream.baseUrl}/v1/messages`, {
      method: "POST", headers,
      body: JSON.stringify({ model: "claude-seven-primary", max_tokens: 32, messages: [], stream: true }),
    });
    assert.match(await stream.text(), /event: message_delta/);
    const count = await fetch(`${upstream.baseUrl}/v1/messages/count_tokens`, {
      method: "POST", headers,
      body: JSON.stringify({ model: "claude-seven-primary", messages: [] }),
    });
    assert.equal((await count.json()).input_tokens, 11);
  } finally {
    await upstream.close();
  }
});
