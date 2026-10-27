import { test } from "node:test";
import assert from "node:assert/strict";
import { defaultPreviewMethod } from "./live-view-source.js";

test("imported stream addresses take priority over vendor guesses and ONVIF", () => {
  assert.equal(defaultPreviewMethod({ savedPreview: true, device: { vendor: "Hikvision", xaddrs: ["synthetic"] } }), "saved");
});
test("use ONVIF advertisements or an identified Hikvision camera", () => {
  assert.equal(defaultPreviewMethod({ device: { xaddrs: ["synthetic"] } }), "onvif");
  assert.equal(defaultPreviewMethod({ device: { vendor: "HIKVISION" } }), "hikvision");
  assert.equal(defaultPreviewMethod({ device: { protocols: ["sadp"] } }), "hikvision");
});
test("other imported cameras never default to a Hikvision path", () => {
  for (const vendor of ["GENERIC_RTSP", "Milesight", "TP-Link", "", "Grandstream"])
    assert.equal(defaultPreviewMethod({ device: { vendor, protocols: ["imported"] } }), "rtsp");
});
