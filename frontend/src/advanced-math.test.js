import test from "node:test";
import assert from "node:assert/strict";
import { normalisePoint, PlanHistory, coveragePath } from "./plan-math.js";
import { sharpness, fittedRect } from "./focus-math.js";
test("synthetic plan coordinates remain stable at resized and zoomed rectangles", () => {
  assert.deepEqual(
    normalisePoint(150, 75, { left: 50, top: 25, width: 200, height: 100 }),
    { x: 0.5, y: 0.5 },
  );
  assert.deepEqual(
    normalisePoint(250, 125, { left: 50, top: 25, width: 400, height: 200 }),
    { x: 0.5, y: 0.5 },
  );
  assert.deepEqual(
    normalisePoint(-2, 300, { left: 0, top: 0, width: 100, height: 100 }),
    { x: 0, y: 1 },
  );
  assert.throws(() => normalisePoint(0, 0, { width: 0, height: 0 }));
  assert.ok(
    !coveragePath({ x: 0.5, y: 0.5, bearing: 90, cone: 60 }).includes("NaN"),
  );
});
test("plan undo/redo does not share mutable marker state and branches correctly", () => {
  const h = new PlanHistory({ markers: [] });
  h.push({ markers: [{ x: 0.2 }] });
  const edited = h.value();
  edited.markers[0].x = 0.8;
  assert.equal(h.value().markers[0].x, 0.2);
  assert.equal(h.undo().markers.length, 0);
  assert.equal(h.redo().markers.length, 1);
  h.undo();
  h.push({ markers: [{ x: 0.9 }] });
  assert.equal(h.redo().markers[0].x, 0.9);
});
test("synthetic focus score is relative and flat frames score zero", () => {
  const w = 16,
    h = 16,
    flat = new Uint8ClampedArray(w * h * 4);
  flat.fill(120);
  assert.equal(sharpness(flat, w, h), 0);
  const edge = new Uint8ClampedArray(w * h * 4);
  for (let y = 0; y < h; y++)
    for (let x = 0; x < w; x++) {
      const i = (y * w + x) * 4;
      edge[i] = edge[i + 1] = edge[i + 2] = ((x + y) % 2) * 255;
      edge[i + 3] = 255;
    }
  assert.ok(sharpness(edge, w, h) > 1000);
  assert.equal(sharpness([], 0, 0), null);
});
test("alignment guides fit letterboxed landscape and portrait media", () => {
  const r = fittedRect(1000, 1000, 1920, 1080);
  for (const [k, v] of Object.entries({
    x: 0,
    y: 218.75,
    width: 1000,
    height: 562.5,
  }))
    assert.ok(Math.abs(r[k] - v) < 1e-8);
  assert.deepEqual(fittedRect(1000, 500, 100, 200), {
    x: 375,
    y: 0,
    width: 250,
    height: 500,
  });
});

import { checklistRows } from "./commissioning.js";
test("saved checklist with absent evidence loads as manual/not-tested without fabricating evidence", () => {
  const rows = checklistRows(
    [{ id: "fixture" }],
    ["live-video", "focus-aim"],
    [
      {
        deviceID: "fixture",
        test: "live-video",
        result: "not-tested",
        source: "manual",
        evidenceIDs: null,
      },
    ],
  );
  assert.deepEqual(rows[0].evidenceIDs, []);
  assert.equal(rows[1].result, "not-tested");
  rows[0].evidenceIDs.push("test");
  assert.deepEqual(rows[1].evidenceIDs, []);
});
