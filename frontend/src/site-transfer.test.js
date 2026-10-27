import test from "node:test";
import assert from "node:assert/strict";
import { importSummary } from "./site-transfer.js";

test("site import review escapes metadata and displays portable contents without secrets", () => {
  const html = importSummary({format:"NX Camera Info JSON", customer:'<img src=x onerror="synthetic">', devices:5,
    devicesWithCredentials:3, credentials:2, switches:1, documents:4, attachments:2, history:7, runs:3,
    warnings:["<script>synthetic</script>"], password:"SYNTHETIC-secret-not-a-preview-field"});
  assert.ok(html.includes("Devices with saved credentials"));
  assert.ok(html.includes("Attachments"));
  assert.ok(html.includes("Existing sites are preserved"));
  assert.ok(!html.includes("<img"));
  assert.ok(!html.includes("<script>"));
  assert.ok(!html.includes("SYNTHETIC-secret-not-a-preview-field"));
});
