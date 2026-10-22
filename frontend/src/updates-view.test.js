import test from "node:test";
import assert from "node:assert/strict";
import { aboutUpdateView, updateControls } from "./updates-view.js";

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

test("About offers supported updates and sends verified downloads to installation confirmation", () => {
  // Synthetic update states, never published releases or device results.
  const available = { status: "available", supported: true, version: "1.2.0-beta.3", message: "A new version is available." };
  const offer = aboutUpdateView(available);
  assert.equal(offer.label, "Update now");
  assert.equal(offer.action, "download");
  assert.match(offer.message, /1\.2\.0-beta\.3/);
  assert.equal(aboutUpdateView({ ...available, status: "ready" }).action, "details");
  const linux = aboutUpdateView({ ...available, supported: false });
  assert.equal(linux.action, "release");
  assert.equal(linux.label, "View release ↗");
});

test("About exposes check failures and progress without starting duplicate work", () => {
  for (const status of ["idle", "current", "error"]) {
    const view = aboutUpdateView({ status, message: "Synthetic status message" });
    assert.equal(view.action, "check");
    assert.equal(view.disabled, false);
    assert.equal(view.message, "Synthetic status message");
  }
  for (const status of ["checking", "installing"]) {
    assert.equal(aboutUpdateView({ status }).disabled, true);
  }
  assert.equal(aboutUpdateView({ status: "downloading", supported: true }).action, "details");
  assert.match(aboutUpdateView({ status: "available", supported: true, message: "Download failed: offline" }).message, /Download failed: offline/);
});
