import test from "node:test";
import assert from "node:assert/strict";
import { cameraCheckKey, cameraSummary } from "./camera-ip.js";

test("changing the target, credential or site invalidates the reviewed camera; display notes do not", () => {
  const v = {
    site: "synthetic",
    address: "192.0.2.10",
    webPort: "80",
    httpsPort: "443",
    rtspPort: "554",
    onvifURL: "",
    method: "auto",
    timeout: "30",
    credential: "",
    username: "",
  };
  const key = cameraCheckKey(v);
  for (const field of Object.keys(v))
    assert.notEqual(cameraCheckKey({ ...v, [field]: "changed" }), key, field);
  assert.equal(
    cameraCheckKey({ ...v, name: "Saved name", location: "Fixture bench" }),
    key,
  );
  assert.equal(cameraCheckKey({ ...v, password: "SYNTHETIC SECRET" }), key);
  assert.equal(cameraCheckKey({ ...v, address: " 192.0.2.10 " }), key);
});
test("review keeps missing identity and services unknown", () => {
  const rows = Object.fromEntries(
    cameraSummary({
      identified: false,
      device: {
        ipv4: "192.0.2.10",
        model: "",
        mac: "",
        serial: "",
        rtspPort: null,
      },
      web: { url: "" },
    }),
  );
  assert.equal(rows.Identity, "Unverified address");
  assert.equal(rows.MAC, "");
  assert.equal(rows.Manufacturer, undefined);
  assert.equal(rows["RTSP port (reported / detected)"], null);
  const read = Object.fromEntries(
    cameraSummary({
      identified: true,
      device: { ipv4: "192.0.2.10", model: "SYNTHETIC" },
      web: { url: "https://192.0.2.10/" },
    }),
  );
  assert.equal(read.Identity, "Read from camera");
  assert.equal(read["Web interface"], "https://192.0.2.10/");
});
