import { test } from "node:test";
import assert from "node:assert/strict";
import { activation, visibleDevices } from "./view.js";
// Synthetic data only; these records are never loaded by the application.
const devices = [
  {
    id: "a",
    ipv4: "192.0.2.100",
    model: "TEST-ONE",
    activated: null,
    interfaces: [{ name: "Ethernet 2" }],
  },
  {
    id: "b",
    ipv4: "192.0.2.9",
    model: "TEST-TWO",
    activated: false,
    interfaces: [],
  },
];
test("sorts IP octets numerically and leaves source inventory untouched", () => {
  assert.deepEqual(
    visibleDevices(devices, "", "ipv4", 1).map((d) => d.id),
    ["b", "a"],
  );
  assert.equal(devices[0].id, "a");
});
test("filters adapters and model case-insensitively", () => {
  assert.deepEqual(
    visibleDevices(devices, " ethernet 2 ", "model", 1).map((d) => d.id),
    ["a"],
  );
  assert.equal(visibleDevices(devices, "test-two", "model", 1)[0].id, "b");
});
test("unknown activation does not become inactive", () => {
  assert.equal(activation(null), "Unknown");
  assert.equal(activation(false), "Inactive");
  assert.equal(activation(true), "Active");
});
