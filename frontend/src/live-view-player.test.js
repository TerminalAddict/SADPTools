import test from "node:test";
import assert from "node:assert/strict";
import { CameraPlayer } from "./live-view-player.js";

function fixture(Media) {
  const messages = [];
  const video = {
    pause() {
      this.paused = true;
    },
    removeAttribute() {},
    load() {
      this.unloaded = true;
    },
  };
  const image = { removeAttribute() {} };
  return {
    video,
    image,
    messages,
    player: new CameraPlayer(
      video,
      image,
      () => {},
      (m) => messages.push(m),
      Media,
    ),
  };
}
test("unsupported HEVC provides an actionable error and disposes resources", () => {
  const f = fixture({ isTypeSupported: () => false });
  f.player.information({
    mime: 'video/mp4; codecs="hvc1.1.6.L120.B0"',
    codec: "H.265",
  });
  assert.equal(f.player.closed, true);
  assert.equal(f.video.unloaded, true);
  assert.match(f.messages[0], /H\.264/);
});
test("packets cannot grow the pending MSE queue without bound", () => {
  const f = fixture();
  f.player.info = { mime: "video/mp4" };
  for (let i = 0; i < 200; i++) f.player.packet(1, new Uint8Array(128 * 1024));
  assert.equal(f.player.closed, true);
  assert.equal(f.player.bytes, 0);
  assert.equal(f.player.queue.length, 0);
  assert.equal(f.messages.length, 1);
  assert.match(f.messages[0], /cannot keep up/);
});
test("close cancels rendering and rejects subsequent video", () => {
  const f = fixture();
  f.player.info = { mime: "video/mp4" };
  f.player.packet(1, new Uint8Array(5));
  f.player.close();
  f.player.close();
  f.player.packet(1, new Uint8Array(10));
  assert.equal(f.player.queue.length, 0);
  assert.equal(f.messages.length, 0);
});
test("oversized JPEG and unexpected media are rejected", () => {
  const f = fixture();
  f.player.info = { mime: "image/jpeg" };
  f.player.packet(2, new Uint8Array(4 * 1024 * 1024 + 1));
  assert.equal(f.player.closed, true);
  const g = fixture();
  g.player.packet(1, new Uint8Array(10));
  assert.equal(g.player.closed, true);
});

test("advancing playback remains live when WebKit never delivers frame callbacks", () => {
  const f = fixture();
  f.player.close();
  let frame;
  const observed = [];
  f.video.requestVideoFrameCallback = (callback) => {
    frame = callback;
    return 7;
  };
  f.video.cancelVideoFrameCallback = (id) => assert.equal(id, 7);
  const player = new CameraPlayer(
    f.video,
    f.image,
    (counted) => observed.push(counted),
    assert.fail,
  );
  f.video.paused = false;
  f.video.currentTime = 1;
  f.video.ontimeupdate();
  f.video.ontimeupdate(); // Duplicate timestamps do not establish progress.
  f.video.currentTime = 2;
  f.video.ontimeupdate();
  assert.deepEqual(observed, [false, false]);
  frame();
  assert.deepEqual(observed, [false, false, true]);
  f.video.paused = true;
  f.video.currentTime = 3;
  f.video.ontimeupdate();
  assert.equal(observed.length, 3);
  player.close();
  frame();
  assert.equal(observed.length, 3);
  assert.equal(f.video.ontimeupdate, null);
});
