import test from "node:test";
import assert from "node:assert/strict";
import {
  operationAvailability,
  operationInterfaces,
  networkFromDevice,
} from "./configuration-view.js";

test("operation controls follow device state and prevent overlapping scans", () => {
  const d = {
    mac: "02:00:00:00:00:01",
    interfaces: [{ name: "Synthetic adapter" }],
    activated: false,
  };
  assert.equal(operationAvailability(d, false).activate, true);
  assert.equal(operationAvailability(d, false).network, false);
  d.activated = true;
  assert.equal(operationAvailability(d, false).activate, false);
  assert.equal(operationAvailability(d, false).recovery, true);
  assert.equal(operationAvailability(d, true).network, false);
  assert.equal(operationAvailability(null, false).activate, false);
});
test("connection options only count distinct discovered adapters", () => {
  assert.deepEqual(operationInterfaces(null), []);
  assert.deepEqual(operationInterfaces({}), []);
  const device = {
    mac: "02:00:00:00:00:01",
    interfaces: [
      { name: "Ethernet Test", address: "192.0.2.1" },
      { name: "Ethernet Test", address: "192.0.2.2" },
      { name: "" },
    ],
  };
  assert.deepEqual(operationInterfaces(device), ["Ethernet Test"]);
  device.interfaces.push({ name: "Wi-Fi Test" });
  assert.deepEqual(operationInterfaces(device), ["Ethernet Test", "Wi-Fi Test"]);
  device.interfaces = [{ name: "" }];
  assert.equal(operationAvailability(device, false).activate, false);
});
test("network form preserves advertised values without inventing address or SDK port", () => {
  assert.deepEqual(networkFromDevice({}), {
    ipv4: "",
    subnetMask: "",
    gateway: "",
    commandPort: "",
    dhcp: "",
  });
  assert.equal(networkFromDevice({ commandPort: 8123 }).commandPort, 8123);
});
