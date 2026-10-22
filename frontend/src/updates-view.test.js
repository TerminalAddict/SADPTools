import test from "node:test";
import assert from "node:assert/strict";
import { updateControls } from "./updates-view.js";

test("only a verified Windows download can be installed, and device writes block it", () => {
  for (const status of ["idle", "checking", "available", "downloading", "current", "error", "installing"]) {
    assert.equal(updateControls({ status, supported: true }).install, false);
  }
  assert.equal(updateControls({ status: "ready", supported: false }).install, false);
  const ready = updateControls({ status: "ready", supported: true }, true);
  assert.equal(ready.install, true);
  assert.equal(ready.installDisabled, true);
  assert.equal(ready.cancel, true);
});

test("download state disables duplicate requests and bounds progress", () => {
  const active = updateControls({ status: "downloading", supported: true, downloaded: 11, size: 10 });
  assert.equal(active.download, false);
  assert.equal(active.check, false);
  assert.equal(active.cancel, true);
  assert.equal(active.percent, 100);
  assert.equal(updateControls({ status: "available", supported: false }).download, false);
});
