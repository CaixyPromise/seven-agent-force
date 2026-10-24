import test from "node:test";
import assert from "node:assert/strict";

test("fixed Node runtime executes without dependencies", () => {
  assert.equal(process.versions.node.split(".")[0], "22");
});
