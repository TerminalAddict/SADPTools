import test from "node:test";
import assert from "node:assert/strict";
import { scanPresentation } from "./scan-view.js";

const started = Date.parse("2026-01-01T00:00:00Z");
const scan = {
  scanning: true,
  scanStarted: new Date(started).toISOString(),
  scanSeconds: 10,
  scanSeen: 3,
  scanNew: 1,
};

test("scan feedback reports the listening window and this scan's real counts", () => {
  assert.equal(scanPresentation({}).visible, false);
  const view = scanPresentation(
    { ...scan, devices: new Array(20) },
    started + 2500,
  );
  assert.equal(view.active, true);
  assert.equal(view.percent, 25);
  assert.equal(view.timing, "2.5 / 10 s");
  assert.equal(view.counts, "3 seen this scan · 1 new");
  assert.equal(view.label, "Listening for cameras…");
});

test("progress is clamped and never implies discovery has finished before the engine", () => {
  assert.equal(scanPresentation(scan, started - 1000).percent, 0);
  const finishing = scanPresentation(scan, started + 12000);
  assert.equal(finishing.percent, 100);
  assert.equal(finishing.label, "Finishing scan…");
  assert.equal(finishing.active, true);
  for (const [scanOutcome, label] of [
    ["completed", "Scan complete"],
    ["cancelled", "Scan cancelled"],
    ["failed", "Scan failed"],
  ]) {
    const view = scanPresentation(
      { ...scan, scanning: false, scanOutcome },
      started + 12000,
    );
    assert.equal(view.label, label);
    assert.equal(view.active, false);
    assert.equal(view.visible, true);
    assert.equal(view.timing, "");
  }
});
