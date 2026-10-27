import { esc, dialog, field, option, message } from "./toolkit-ui.js";
import { sharpness, fittedRect } from "./focus-math.js";
export function initViewerTools({ panel, get, API, Vault, restart, context }) {
  let status = null,
    displayedFPS = null,
    measurement = null,
    testTimer,
    elapsed = 0,
    results = [],
    scoreTimer,
    before = null,
    after = null,
    scoreHistory = [],
    zoom = 1,
    pan = { x: 0, y: 0 },
    drag = null,
    tour = 0,
    compareGeneration = 0,
    caps = null,
    lastFrame = false,
    disposed = false;
  const tool = document.createElement("div");
  tool.className = "viewer-tools mt-3";
  tool.innerHTML = `<p id="vt-notice" class="small text-warning" role="status"></p><details><summary>Stream Info & connection tests</summary><div class="small mt-2" id="vt-summary">Waiting for stream information.</div><p class="small text-body-secondary">RTSP over TCP is supported. UDP transport is unavailable in this release. Encoded frame rate and displayed frame rate are different measurements; unavailable values stay Unknown.</p><div class="camera-controls"><button type="button" id="vt-test" class="btn btn-sm btn-primary">Run 15-second connection test</button><button type="button" id="vt-compare" class="btn btn-sm btn-outline-primary">Compare main and sub (30+ seconds)</button><button type="button" id="vt-cancel" class="btn btn-sm btn-outline-secondary">Cancel test</button><button type="button" id="vt-save" class="btn btn-sm btn-outline-secondary">Save diagnostic to site</button><button type="button" id="vt-copy" class="btn btn-sm btn-outline-secondary">Copy diagnostics</button></div><p id="vt-test-status" class="small" role="status"></p><pre id="vt-results" class="small tool-readable"></pre><details><summary>Connection timeline</summary><pre id="vt-timeline" class="small tool-readable"></pre></details></details>
 <details id="vt-focus"><summary>Focus & alignment assistant</summary><p class="small">Digital zoom changes only this preview. Sharpness is a relative guide affected by texture, light, movement and compression; it is not a calibrated focus measurement.</p><div class="camera-controls"><label>Guide <select id="vt-guide" class="form-select form-select-sm"><option value="none">None</option><option value="thirds">Rule of thirds</option><option value="crosshair">Crosshair</option><option value="grid">Alignment grid</option><option value="horizon">Horizon</option><option value="guide">Horizontal / vertical guide</option></select></label><label>Guide angle °<input id="vt-angle" class="form-control form-control-sm" type="number" min="-90" max="90" value="0"></label><label>Digital zoom <input id="vt-zoom" type="range" min="1" max="4" step=".1" value="1"></label><button id="vt-reset" type="button" class="btn btn-sm btn-outline-secondary">Reset pan / zoom</button><label><input id="vt-score" type="checkbox"> Sharpness trend</label></div><p id="vt-score-label" class="small">Sharpness is off.</p><canvas id="vt-trend" width="500" height="70" aria-label="Relative sharpness trend"></canvas><label class="form-label">Settings / lighting notes for this capture (manual)<input id="vt-setting-note" class="form-control form-control-sm" maxlength="100" placeholder="e.g. WDR ON; gain read as 20"></label><div class="camera-controls"><button id="vt-before" type="button" class="btn btn-sm btn-outline-secondary">Capture before</button><button id="vt-after" type="button" class="btn btn-sm btn-outline-secondary">Capture after</button><button id="vt-snapshot" type="button" class="btn btn-sm btn-primary">Save snapshot to site</button><button id="vt-file" type="button" class="btn btn-sm btn-outline-secondary">Save snapshot to file</button><button id="vt-save-before" type="button" class="btn btn-sm btn-outline-secondary">Save before / after to site</button></div><div id="vt-comparison" hidden><label>Before ← split → After <input id="vt-split" type="range" min="0" max="100" value="50"></label><label>Comparison zoom <input id="vt-compare-zoom" type="range" min="1" max="4" step=".1" value="1"></label><div class="comparison-scroll"><canvas id="vt-compare-image" width="800" height="450" aria-label="Before and after comparison"></canvas></div></div></details>
 <details id="vt-camera"><summary>PTZ, focus & presets</summary><p class="small">Physical controls change the selected camera. Read capabilities first, then explicitly arm controls. Each move is a bounded step; release, window blur, Stop and a watchdog attempt to stop movement.</p><button id="vt-load-controls" type="button" class="btn btn-sm btn-outline-primary">Read supported controls</button><label class="ms-2"><input id="vt-arm" type="checkbox" disabled> Arm physical controls for this camera</label><p id="vt-control-status" class="small" role="status">Controls have not been queried.</p><div id="vt-control-panel" hidden><label>Speed <input id="vt-speed" type="range" min=".1" max="1" step=".1" value=".3"></label><div class="camera-controls my-2">${[
   ["↖", -1, 1],
   ["↑", 0, 1],
   ["↗", 1, 1],
   ["←", -1, 0],
   ["→", 1, 0],
   ["↙", -1, -1],
   ["↓", 0, -1],
   ["↘", 1, -1],
 ]
   .map(
     ([label, x, y]) =>
       `<button type="button" class="btn btn-sm btn-outline-secondary" data-ptz="${x},${y},0" aria-label="Move camera ${label}">${label}</button>`,
   )
   .join(
     "",
   )}<button type="button" class="btn btn-sm btn-outline-secondary" data-ptz="0,0,1">Optical +</button><button type="button" class="btn btn-sm btn-outline-secondary" data-ptz="0,0,-1">Optical −</button><button id="vt-stop" type="button" class="btn btn-sm btn-danger">STOP</button></div><div class="camera-controls mb-2"><button type="button" data-focus="relative,-1" class="btn btn-sm btn-outline-secondary">Focus near</button><button type="button" data-focus="relative,1" class="btn btn-sm btn-outline-secondary">Focus far</button><button type="button" data-focus="auto,0" class="btn btn-sm btn-outline-secondary">Autofocus</button><button type="button" data-focus="iris,-1" class="btn btn-sm btn-outline-secondary">Iris −</button><button type="button" data-focus="iris,1" class="btn btn-sm btn-outline-secondary">Iris +</button></div><div id="vt-presets"></div><div class="camera-controls"><button id="vt-presets-load" type="button" class="btn btn-sm btn-outline-secondary">Read presets</button><button id="vt-preset-add" type="button" class="btn btn-sm btn-outline-secondary">Add preset here</button><button id="vt-tour" type="button" class="btn btn-sm btn-outline-secondary">Preview checked presets once</button></div><p class="small">Tour preview visits up to 8 selected presets once (5-second dwell); it never programs a device tour. Pure rename is unavailable: ONVIF SetPreset may overwrite a preset position.</p></div></details>`;
  get("form").append(tool);
  const el = (id) => tool.querySelector(`#vt-${id}`);
  const canvas = document.createElement("canvas");
  canvas.className = "viewer-overlay";
  get("stage").append(canvas);
  const scratch = document.createElement("canvas");
  const perform = async (fn) => {
    try {
      return await fn();
    } catch (e) {
      el("notice").textContent = e?.message || String(e);
    }
  };
  const media = () => (!get("image").hidden ? get("image") : get("video"));
  const dimensions = () => {
    const m = media();
    return {
      width: m.videoWidth || m.naturalWidth || 0,
      height: m.videoHeight || m.naturalHeight || 0,
    };
  };
  function capture() {
    const m = media(),
      { width, height } = dimensions();
    if (!lastFrame || !width || !height)
      throw Error("Wait for a displayed frame before capturing evidence");
    const scale = Math.min(1, 1920 / width, 1080 / height);
    const c = document.createElement("canvas");
    c.width = Math.round(width * scale);
    c.height = Math.round(height * scale);
    c.getContext("2d").drawImage(m, 0, 0, c.width, c.height);
    return {
      canvas: c,
      at: new Date().toISOString(),
      score: scoreHistory.at(-1) ?? null,
      codec: status?.info?.codec || "Unknown codec",
      resolution: `${width}×${height}`,
      settingsNote: el("setting-note").value,
    };
  }
  function guides() {
    const stage = get("stage"),
      rect = stage.getBoundingClientRect(),
      d = dimensions();
    canvas.width = rect.width * devicePixelRatio;
    canvas.height = rect.height * devicePixelRatio;
    const ctx = canvas.getContext("2d");
    ctx.scale(devicePixelRatio, devicePixelRatio);
    const r = fittedRect(rect.width, rect.height, d.width, d.height);
    ctx.strokeStyle = "#33f0d0";
    ctx.lineWidth = 1;
    const line = (x1, y1, x2, y2) => {
      ctx.beginPath();
      ctx.moveTo(x1, y1);
      ctx.lineTo(x2, y2);
      ctx.stroke();
    };
    const v = el("guide").value;
    if (v === "thirds" || v === "grid") {
      const n = v === "grid" ? 8 : 3;
      for (let i = 1; i < n; i++) {
        line(
          r.x + (r.width * i) / n,
          r.y,
          r.x + (r.width * i) / n,
          r.y + r.height,
        );
        line(
          r.x,
          r.y + (r.height * i) / n,
          r.x + r.width,
          r.y + (r.height * i) / n,
        );
      }
    }
    if (v === "crosshair") {
      line(r.x + r.width / 2, r.y, r.x + r.width / 2, r.y + r.height);
      line(r.x, r.y + r.height / 2, r.x + r.width, r.y + r.height / 2);
    }
    if (v === "horizon" || v === "guide") {
      ctx.translate(rect.width / 2, rect.height / 2);
      ctx.rotate((Number(el("angle").value) * Math.PI) / 180);
      line(-r.width / 2, 0, r.width / 2, 0);
      if (v === "guide") line(0, -r.height / 2, 0, r.height / 2);
    }
  }
  function transform() {
    for (const m of [get("video"), get("image")])
      m.style.transform = `translate(${pan.x}px,${pan.y}px) scale(${zoom})`;
    guides();
  }
  el("guide").onchange = guides;
  el("angle").oninput = guides;
  el("zoom").oninput = () => {
    zoom = Number(el("zoom").value);
    transform();
  };
  el("reset").onclick = () => {
    zoom = 1;
    pan = { x: 0, y: 0 };
    el("zoom").value = 1;
    transform();
  };
  get("stage").addEventListener("pointerdown", (e) => {
    if (zoom <= 1) return;
    drag = { x: e.clientX - pan.x, y: e.clientY - pan.y };
    get("stage").setPointerCapture(e.pointerId);
  });
  get("stage").addEventListener("pointermove", (e) => {
    if (!drag) return;
    const r = get("stage").getBoundingClientRect();
    pan = {
      x: Math.max(
        (-r.width * (zoom - 1)) / 2,
        Math.min((r.width * (zoom - 1)) / 2, e.clientX - drag.x),
      ),
      y: Math.max(
        (-r.height * (zoom - 1)) / 2,
        Math.min((r.height * (zoom - 1)) / 2, e.clientY - drag.y),
      ),
    };
    transform();
  });
  get("stage").addEventListener("pointerup", () => (drag = null));
  get("stage").addEventListener("pointercancel", () => (drag = null));
  const resize = new ResizeObserver(guides);
  resize.observe(get("stage"));
  function sample() {
    if (!el("score").checked || !el("focus").open || !lastFrame) return;
    const { width, height } = dimensions();
    if (!width || !height) return;
    scratch.width = 128;
    scratch.height = 72;
    const ctx = scratch.getContext("2d", { willReadFrequently: true });
    ctx.drawImage(
      media(),
      width * 0.1,
      height * 0.1,
      width * 0.8,
      height * 0.8,
      0,
      0,
      128,
      72,
    );
    const score = sharpness(ctx.getImageData(0, 0, 128, 72).data, 128, 72);
    scoreHistory.push(score);
    if (scoreHistory.length > 90) scoreHistory.shift();
    el("score-label").textContent =
      `Relative sharpness ${score.toFixed(1)} · central 80% of image · higher is usually sharper for the same scene`;
    const plot = el("trend"),
      p = plot.getContext("2d");
    p.clearRect(0, 0, plot.width, plot.height);
    p.strokeStyle = "#13bba4";
    p.beginPath();
    const max = Math.max(1, ...scoreHistory);
    scoreHistory.forEach((v, i) => {
      const x = (i / (Math.max(2, scoreHistory.length) - 1)) * plot.width,
        y = plot.height - 5 - (v / max) * (plot.height - 10);
      if (i) p.lineTo(x, y);
      else p.moveTo(x, y);
    });
    p.stroke();
  }
  scoreTimer = setInterval(() => {
    try {
      sample();
    } catch {
      el("score-label").textContent = "Sharpness is unavailable for this frame";
    }
  }, 500);
  function comparison() {
    if (!before || !after) return;
    el("comparison").hidden = false;
    const c = el("compare-image");
    c.height = Math.round((c.width * after.canvas.height) / after.canvas.width);
    const x = c.getContext("2d"),
      split = (c.width * Number(el("split").value)) / 100;
    x.drawImage(after.canvas, 0, 0, c.width, c.height);
    x.save();
    x.beginPath();
    x.rect(0, 0, split, c.height);
    x.clip();
    x.drawImage(before.canvas, 0, 0, c.width, c.height);
    x.restore();
    x.strokeStyle = "white";
    x.beginPath();
    x.moveTo(split, 0);
    x.lineTo(split, c.height);
    x.stroke();
  }
  el("before").onclick = () =>
    void perform(() => {
      before = capture();
      comparison();
    });
  el("after").onclick = () =>
    void perform(() => {
      after = capture();
      comparison();
    });
  el("split").oninput = comparison;
  const saveImage = async (image, label) =>
    API.SaveSnapshot(
      image.canvas.toDataURL("image/png"),
      `${label} · ${image.at} · ${image.codec} ${image.resolution}${image.settingsNote ? ` · Manual: ${image.settingsNote}` : " · Settings unknown"}${image.score == null ? "" : ` · relative sharpness ${image.score.toFixed(1)}`}`,
    );
  el("compare-zoom").oninput = () => {
    el("compare-image").style.width =
      `${Number(el("compare-zoom").value) * 100}%`;
  };
  el("file").onclick = () =>
    void perform(async () => {
      const image = capture();
      await API.ExportSnapshot(image.canvas.toDataURL("image/png"));
    });
  el("snapshot").onclick = () =>
    void perform(async () => {
      const image = capture();
      const saved = await saveImage(image, "Focus / alignment snapshot");
      message(
        "Snapshot saved",
        `${saved.title}\nAvailable in the site’s commissioning evidence.`,
      );
    });
  el("save-before").onclick = () =>
    void perform(async () => {
      if (!before || !after)
        throw Error("Capture both before and after frames first");
      await saveImage(before, "Before alignment");
      await saveImage(after, "After alignment");
      message(
        "Evidence saved",
        "Before and after frames were saved to this site.",
      );
    });
  const metric = (x, d = 2) => (x == null ? "Unknown" : Number(x).toFixed(d));
  function diagnosticText() {
    if (!status) return "No stream status";
    return JSON.stringify(
      {
        profile: get("profile").value,
        info: status.info,
        metrics: status.metrics,
        displayedFPS,
        errorCode: status.errorCode,
        timeline: status.timeline,
        tests: results,
      },
      null,
      2,
    );
  }
  async function saveDiagnostic(label = "Stream observation", seconds = 0) {
    if (!context().diagnosticTicket)
      throw Error("Start Live View before saving diagnostics");
    const video = get("video"),
      quality = get("image").hidden ? video.getVideoPlaybackQuality?.() : null;
    return API.SaveDiagnostic(context().diagnosticTicket, {
      displayedFPS,
      droppedFrames: quality?.droppedVideoFrames ?? null,
      testSeconds: seconds,
      label,
    });
  }
  el("save").onclick = () =>
    void perform(async () => {
      await saveDiagnostic();
      message(
        "Diagnostic saved",
        "The observation is available in the site’s report evidence.",
      );
    });
  el("copy").onclick = () =>
    void perform(() => Vault.CopyText(diagnosticText()));
  function cancelTest() {
    clearInterval(testTimer);
    testTimer = null;
    if (measurement) {
      measurement.resolve(null);
      measurement = null;
    }
    elapsed = 0;
    el("test-status").textContent =
      "Test cancelled; no reconnection or retries requested";
  }
  function connectionTest(label) {
    cancelTest();
    return new Promise((resolve) => {
      const begin = performance.now(),
        initial = status?.metrics,
        session = context().ticket;
      measurement = { resolve, session };
      testTimer = setInterval(async () => {
        elapsed = (performance.now() - begin) / 1000;
        el("test-status").textContent =
          `${label}: ${Math.min(15, elapsed).toFixed(0)} / 15 seconds`;
        if (disposed || session !== context().ticket || !session) {
          cancelTest();
          return;
        }
        if (elapsed < 15) return;
        clearInterval(testTimer);
        testTimer = null;
        const result = {
          label,
          seconds: elapsed,
          state: status?.state || "unknown",
          errorCode: status?.errorCode || "",
          codec: status?.info?.codec,
          resolution: `${status?.info?.width || "?"}×${status?.info?.height || "?"}`,
          rtpMbps:
            initial?.rtpBytes != null && status?.metrics?.rtpBytes != null
              ? (Math.max(0, status.metrics.rtpBytes - initial.rtpBytes) * 8) /
                elapsed /
                1e6
              : null,
          displayedFPS,
        };
        results.push(result);
        results = results.slice(-8);
        el("results").textContent = JSON.stringify(results, null, 2);
        el("test-status").textContent = `${label}: complete (${result.state})`;
        measurement = null;
        resolve(result);
      }, 250);
    });
  }
  el("test").onclick = () =>
    void perform(async () => {
      const result = await connectionTest(get("profile").value);
      if (result && context().siteID)
        await saveDiagnostic(result.label, result.seconds);
    });
  el("cancel").onclick = () => {
    compareGeneration++;
    cancelTest();
  };
  el("compare").onclick = () =>
    void perform(async () => {
      if (get("profile").disabled)
        throw Error(
          "Open each advertised NVR stream and run its test separately",
        );
      if (!["saved", "hikvision", "onvif"].includes(get("method").value))
        throw Error("Main/sub comparison requires imported streams, Hikvision or ONVIF video");
      const generation = ++compareGeneration;
      for (const profile of ["sub", "main"]) {
        if (generation !== compareGeneration) return;
        get("profile").value = profile;
        await restart();
        if (generation !== compareGeneration) return;
        const result = await connectionTest(profile);
        if (!result) return;
        if (context().siteID) await saveDiagnostic(profile, result.seconds);
      }
    });
  async function stopControl() {
    tour++;
    try {
      const warning = await API.StopCamera();
      if (warning) el("control-status").textContent = warning;
    } catch {}
  }
  function armed() {
    if (!el("arm").checked || !caps)
      throw Error("Read capabilities and explicitly arm controls first");
  }
  el("load-controls").onclick = () =>
    void perform(async () => {
      el("control-status").textContent = "Reading supported controls…";
      caps = await API.LoadControls();
      el("arm").disabled = false;
      el("arm").checked = false;
      el("control-panel").hidden = false;
      el("control-status").textContent =
        caps.reasons.join(" · ") ||
        "Capabilities read. Arm controls only when it is safe to move this camera.";
      for (const b of tool.querySelectorAll("[data-ptz]")) {
        const z = Number(b.dataset.ptz.split(",")[2]);
        b.disabled = z ? !caps.opticalZoom : !caps.panTilt;
      }
      for (const b of tool.querySelectorAll("[data-focus]")) {
        const mode = b.dataset.focus.split(",")[0];
        b.disabled =
          mode === "relative"
            ? !caps.relativeFocus
            : mode === "auto"
              ? !caps.autoFocus
              : !caps.iris;
      }
      for (const id of ["presets-load", "preset-add", "tour"])
        el(id).disabled = !caps.presets;
    });
  el("arm").onchange = () => {
    if (!el("arm").checked) void stopControl();
  };
  el("stop").onclick = stopControl;
  for (const b of tool.querySelectorAll("[data-ptz]")) {
    b.onpointerdown = (e) => {
      e.preventDefault();
      b.setPointerCapture(e.pointerId);
      void perform(async () => {
        armed();
        const speed = Number(el("speed").value);
        const [x, y, z] = b.dataset.ptz
          .split(",")
          .map((v) => Number(v) * speed);
        await API.MoveCamera(x, y, z);
      });
    };
    b.onpointerup = b.onpointercancel = () => void stopControl();
    b.onkeydown = (e) => {
      if (e.key === "Enter" || e.key === " ") {
        e.preventDefault();
        void perform(async () => {
          armed();
          const speed = Number(el("speed").value);
          await API.MoveCamera(
            ...b.dataset.ptz.split(",").map((v) => Number(v) * speed),
          );
        });
      }
    };
    b.onkeyup = () => void stopControl();
  }
  for (const b of tool.querySelectorAll("[data-focus]")) {
    b.onclick = () =>
      void perform(async () => {
        armed();
        const [mode, d] = b.dataset.focus.split(",");
        await API.CameraFocus(mode, Number(d));
      });
  }
  async function presets() {
    const list = await API.CameraPresets();
    el("presets").innerHTML =
      list
        .map(
          (p) =>
            `<div class="camera-controls my-1"><label><input type="checkbox" value="${esc(p.id)}" data-tour-preset> ${esc(p.name || "Unnamed preset")}</label><button type="button" data-preset-recall="${esc(p.id)}" class="btn btn-sm btn-link">Recall</button><button type="button" data-preset-delete="${esc(p.id)}" class="btn btn-sm btn-link">Delete</button></div>`,
        )
        .join("") || "No presets reported";
    for (const b of el("presets").querySelectorAll("[data-preset-recall]"))
      b.onclick = () =>
        void perform(async () => {
          armed();
          await API.CameraPreset("recall", b.dataset.presetRecall, "");
        });
    for (const b of el("presets").querySelectorAll("[data-preset-delete]"))
      b.onclick = () =>
        dialog(
          "Delete camera preset",
          "<p>This deletes the selected preset on the camera.</p>",
          async () => {
            armed();
            await API.CameraPreset("delete", b.dataset.presetDelete, "");
            await presets();
          },
          "Delete preset",
        );
  }
  el("presets-load").onclick = () => void perform(presets);
  el("preset-add").onclick = () =>
    dialog(
      "Save current camera position",
      field("Preset name", "name", "", "text", "required maxlength=100"),
      async (v) => {
        armed();
        await API.CameraPreset("add", "", v.name);
        await presets();
      },
      "Save preset on camera",
    );
  el("tour").onclick = () =>
    void perform(async () => {
      armed();
      const ids = [
        ...el("presets").querySelectorAll("[data-tour-preset]:checked"),
      ].map((x) => x.value);
      if (!ids.length || ids.length > 8)
        throw Error("Select 1–8 presets for a single preview pass");
      const sequence = ++tour;
      for (const id of ids) {
        if (sequence !== tour) return;
        armed();
        await API.CameraPreset("recall", id, "");
        await new Promise((resolve) => setTimeout(resolve, 5000));
        if (sequence !== tour) return;
        await API.StopCamera();
      }
      await stopControl();
    });
  const blur = () => {
    drag = null;
    el("arm").checked = false;
    void stopControl();
  };
  window.addEventListener("blur", blur);
  document.addEventListener("visibilitychange", () => {
    if (document.hidden) blur();
  });
  return {
    update(s, fps, hasFrame) {
      status = s;
      displayedFPS = hasFrame ? fps : null;
      lastFrame = hasFrame;
      const m = s.metrics || {},
        i = s.info || {};
      el("summary").textContent =
        `${i.codec || "Unknown codec"} · ${i.width || "?"} × ${i.height || "?"} · ${i.transport || "Unknown transport"} · RTP ${metric(m.rtpMbps)} Mb/s · Encoded ${metric(m.encodedFPS, 1)} fps · Displayed ${metric(displayedFPS, 1)} fps · First media ${metric(m.firstMediaMS, 0)} ms · Lost RTP ${m.lostPackets ?? "Unknown"} · Decode errors ${m.decodeErrors ?? "Unknown"} · ${s.errorCode || s.state}`;
      el("timeline").textContent = (s.timeline || [])
        .map((t) => `${t.elapsedMS} ms · ${t.state} · ${t.message}`)
        .join("\n");
      guides();
    },
    stopped() {
      lastFrame = false;
      // Keep the last timeline available for copying after a failed connection.
      displayedFPS = null;
      caps = null;
      el("arm").checked = false;
      el("arm").disabled = true;
      void stopControl();
    },
    dispose() {
      disposed = true;
      tour++;
      cancelTest();
      clearInterval(scoreTimer);
      resize.disconnect();
      window.removeEventListener("blur", blur);
      void stopControl();
      before = after = null;
      scoreHistory = [];
    },
  };
}
