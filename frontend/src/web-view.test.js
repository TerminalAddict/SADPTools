import test from "node:test";
import assert from "node:assert/strict";
import { createWebLookup, detectedURL } from "./web-view.js";

// Synthetic addresses only. No browser or camera requests are made here.
const camera = { id: "synthetic-a", ipv4: "192.0.2.40", httpPort: 80 };
const endpoint = {
  url: "https://192.0.2.40:8443/",
  scheme: "https",
  port: 8443,
};
const flush = () => new Promise((resolve) => setImmediate(resolve));
function deferred() {
  let resolve, reject;
  const promise = new Promise((yes, no) => {
    resolve = yes;
    reject = no;
  });
  return { promise, resolve, reject };
}

test("only detected HTTP(S) roots on the selected camera become links", () => {
  assert.equal(detectedURL(camera, endpoint), endpoint.url);
  for (const url of [
    "javascript:alert(1)",
    "https://example.invalid:8443/",
    "https://192.0.2.40:8443/action",
    "https://user:secret@192.0.2.40:8443/",
    "https://192.0.2.40:8443/?reset=true",
    "https://192.0.2.40:8443/#action",
  ]) {
    assert.equal(detectedURL(camera, { ...endpoint, url }), "");
  }
  assert.equal(detectedURL(camera, { ...endpoint, port: 443 }), "");
  assert.equal(detectedURL(camera, null), "");
});

test("late success or failure cannot replace the newly selected camera", async () => {
  for (const fails of [false, true]) {
    const a = deferred(),
      b = deferred();
    const lookup = createWebLookup(
      (id) => (id === camera.id ? a.promise : b.promise),
      () => {},
    );
    lookup.select(camera);
    await flush();
    assert.equal(lookup.get().busy, true);
    const other = { ...camera, id: "synthetic-b", ipv4: "192.0.2.41" };
    lookup.select(other);
    await flush();
    const result = {
      endpoint: { ...endpoint, url: "https://192.0.2.41:8443/" },
    };
    b.resolve(result);
    await flush();
    if (fails) a.reject(new Error("old selection failed"));
    else a.resolve({ endpoint });
    await flush();
    assert.equal(lookup.get().result, result);
    assert.equal(lookup.get().error, "");
  }
});

test("selection probes once and refreshes on address, port, scan and explicit retry", async () => {
  const calls = [];
  const lookup = createWebLookup(
    async (id, force) => {
      calls.push({ id, force });
      return { endpoint };
    },
    () => {},
  );
  lookup.select(camera);
  lookup.select({ ...camera });
  await flush();
  assert.equal(calls.length, 1);
  lookup.select({ ...camera, ipv4: "192.0.2.41" });
  await flush();
  lookup.select({ ...camera, httpPort: 8080 });
  await flush();
  lookup.select({ ...camera, httpPort: 8080, httpsPort: 8443 });
  await flush();
  lookup.select(camera, "synthetic-scan-time");
  await flush();
  await lookup.retry();
  assert.equal(calls.length, 6);
  assert.equal(calls[5].force, true);
});

test("recovery's detected endpoint supersedes an outstanding web lookup", async () => {
  const pending = deferred();
  const lookup = createWebLookup(
    () => pending.promise,
    () => {},
  );
  lookup.select(camera);
  await flush();
  lookup.adopt(camera.id, endpoint);
  pending.resolve({
    endpoint: { url: "http://192.0.2.40/", scheme: "http", port: 80 },
  });
  await flush();
  assert.equal(lookup.get().result.endpoint, endpoint);
  assert.equal(lookup.get().busy, false);
  lookup.select(null);
  lookup.adopt(camera.id, endpoint);
  assert.equal(lookup.get().result, null);
});

test("device operations defer probes until the operation ends", async () => {
  let calls = 0;
  const lookup = createWebLookup(
    async () => {
      calls++;
      return { endpoint };
    },
    () => {},
  );
  lookup.select(camera, "", true);
  await flush();
  assert.equal(calls, 0);
  lookup.select(camera, "", false);
  await flush();
  assert.equal(calls, 1);
});
