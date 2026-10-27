import test from "node:test";
import assert from "node:assert/strict";
import { diagnosticRows } from "./field-view.js";

test("diagnostic timelines preserve uncertainty, timing and escape untrusted evidence", () => {
  const rows = diagnosticRows({
    steps: [
      {
        test: "<img src=x onerror=alert(1)>",
        protocol: "https",
        status: "unknown",
        evidence: "<script>secret()</script>",
        next: 'Check "route" & VLAN',
        durationMS: 25,
      },
    ],
  });
  assert.equal(rows.length, 1);
  assert.ok(rows[0].includes("unknown"));
  assert.ok(rows[0].includes("25 ms"));
  assert.ok(rows[0].includes("&lt;script&gt;"));
  assert.ok(!rows[0].includes("<script>"));
  assert.ok(!rows[0].includes("<img"));
  assert.deepEqual(diagnosticRows(null), []);
});
