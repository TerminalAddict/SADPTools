import test from "node:test";
import assert from "node:assert/strict";
import { esc, table, field, option } from "./toolkit-ui.js";
import { operationAvailability } from "./configuration-view.js";
test("untrusted site/device strings stay text in dialogs and table headers", () => {
  const input = '\"><img src=x onerror="synthetic">';
  for (const html of [
    esc(input),
    table([input], []),
    field(input, "name", input),
    option(input, input),
  ]) {
    assert.ok(!html.includes("<img"));
    assert.ok(html.includes("&lt;"));
  }
});
test("ONVIF-only and contradictory identities cannot use Hikvision writes", () => {
  const d = {
    mac: "02:00:00:00:00:01",
    interfaces: [{ name: "Synthetic" }],
    activated: true,
    protocols: ["onvif"],
  };
  assert.equal(operationAvailability(d, false).network, false);
  d.protocols.push("sadp");
  assert.equal(operationAvailability(d, false).network, true);
  d.identityConflict = true;
  assert.equal(operationAvailability(d, false).network, false);
});
