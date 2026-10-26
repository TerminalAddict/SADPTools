import {
  esc,
  field,
  area,
  select,
  option,
  dialog,
  message,
  date,
} from "./toolkit-ui.js";
import {
  normalisePoint,
  coveragePath,
  PlanHistory,
  clamp,
} from "./plan-math.js";
export async function plansPage(c) {
  const { q, on, button, docs, parse, save, perform, API, Viewer } = c;
  let selected = docs("plan")[0] || null,
    plan = structuredClone(parse(selected)),
    history,
    zoom = 1,
    search = "",
    layer = "",
    dirty = false;
  const plans = docs("plan");
  async function open(d) {
    selected = d;
    plan = structuredClone(parse(d));
    plan.markers ||= [];
    history = new PlanHistory(plan);
    dirty = false;
    zoom = 1;
    await render();
  }
  function changed() {
    dirty = true;
    history.push(plan);
    draw();
    q("plan-state").textContent = "Unsaved placements";
  }
  async function render() {
    q("content").innerHTML =
      `<div class="advanced-actions">${select("Plan", "ad-plan-choice", option("", "Choose plan", selected?.id) + plans.map((d) => option(d.id, d.title, selected?.id)).join(""))}${button("new-plan", "Add plan image", "primary")}${button("save-plan", "Save placements", "primary")}${button("replace-plan", "Replace image")}${button("delete-plan", "Remove plan")}</div><p class="small">Offline plan images. Placement and direction cones are approximate manual annotations, not calculated optical coverage.</p><div id="ad-plan-editor"></div>`;
    on("new-plan", async () => {
      const a = await API.ImportImage(c.site, "plan");
      if (!a.id) return;
      dialog(
        "New site plan",
        field("Plan title", "title", a.title, "text", "required"),
        (v) => save("plan", v.title, { imageID: a.id, notes: "", markers: [] }),
      );
    });
    on("save-plan", async () => {
      if (!selected) throw Error("Choose a plan");
      await save("plan", selected.title, plan, selected);
    });
    on("replace-plan", () => {
      if (!selected) throw Error("Choose a plan");
      dialog(
        "Replace plan image",
        "<p>Marker coordinates are retained. Verify every placement against the replacement image before saving. The old image remains available until its references are removed.</p>",
        async () => {
          const a = await API.ImportImage(c.site, "plan");
          if (a.id) {
            plan.imageID = a.id;
            changed();
            await render();
          }
        },
        "Choose replacement",
      );
    });
    on("delete-plan", () => {
      if (!selected) throw Error("Choose a plan");
      dialog(
        "Remove site plan",
        `<p>Remove ${esc(selected.title)} and its marker placements? Referenced report snapshots and their images remain intact.</p>`,
        async () => {
          await API.DeleteAdvanced(c.site, selected.id);
          await c.refresh();
        },
        "Remove plan",
      );
    });
    q("content").querySelector('[name="ad-plan-choice"]').onchange = (e) => {
      const d = plans.find((p) => p.id === e.target.value);
      if (dirty) {
        dialog(
          "Discard unsaved plan edits?",
          "<p>Saved placements are preserved.</p>",
          () => open(d),
          "Discard edits",
        );
      } else void perform(() => open(d));
    };
    if (!selected) {
      q("plan-editor").innerHTML =
        "<p>No plan selected. Attach a local PNG, JPEG or WebP image to begin.</p>";
      return;
    }
    const image = await API.Image(c.site, plan.imageID);
    q("plan-editor").innerHTML =
      `<div class="advanced-actions">${button("undo", "Undo")}${button("redo", "Redo")}${button("zoom-out", "−")}${button("zoom-in", "+")}${button("zoom-reset", "Fit")}${button("plan-notes", "Title / notes")}<span id="ad-plan-state" class="small">${dirty ? "Unsaved placements" : "Saved placements"}</span></div><div class="plan-layout"><aside><label>Find camera<input id="ad-plan-search" class="form-control form-control-sm" placeholder="Name, IP or model" value="${esc(search)}"></label>${select("Layer", "ad-plan-layer", option("", "All layers", layer) + [...new Set(plan.markers.map((m) => m.layer).filter(Boolean))].map((l) => option(l, l, layer)).join(""))}<div id="ad-plan-devices"></div></aside><div id="ad-plan-scroll" class="plan-scroll"><div id="ad-plan-canvas" class="plan-canvas"><img id="ad-plan-image" src="${image}" alt="${esc(selected.title)}"><svg id="ad-plan-markers" viewBox="0 0 1000 1000" preserveAspectRatio="none" aria-label="Camera placements"></svg></div></div></div><p class="small">Drag a camera from the list onto the image; drag a marker to move it. Click a marker for details, direction and Live View. Drag empty background to pan; use +/− to zoom.</p><div id="ad-plan-selected"></div>`;
    q("plan-search").oninput = (e) => {
      search = e.target.value;
      draw();
    };
    q("plan-editor").querySelector('[name="ad-plan-layer"]').onchange = (e) => {
      layer = e.target.value;
      draw();
    };
    on("undo", () => {
      plan = history.undo();
      dirty = true;
      draw();
    });
    on("redo", () => {
      plan = history.redo();
      dirty = true;
      draw();
    });
    const setZoom = (value) => {
      zoom = clamp(value, 0.5, 4);
      q("plan-canvas").style.width = `${zoom * 100}%`;
    };
    on("zoom-in", () => setZoom(zoom * 1.25));
    on("zoom-out", () => setZoom(zoom / 1.25));
    on("zoom-reset", () => setZoom(1));
    on("plan-notes", () =>
      dialog(
        "Plan title and annotations",
        field("Title", "title", selected.title) +
          area("Plan notes", "notes", plan.notes),
        async (v) => {
          plan.notes = v.notes;
          await save("plan", v.title, plan, selected);
        },
      ),
    );
    const canvas = q("plan-canvas");
    canvas.ondragover = (e) => e.preventDefault();
    canvas.ondrop = (e) => {
      e.preventDefault();
      const id = e.dataTransfer.getData("text/plain");
      if (!c.records().some((r) => r.id === id)) return;
      const point = normalisePoint(
        e.clientX,
        e.clientY,
        q("plan-image").getBoundingClientRect(),
      );
      plan.markers.push({
        id: crypto.randomUUID(),
        deviceID: id,
        ...point,
        bearing: 0,
        cone: 0,
        label: c.deviceName(id),
        layer: "",
        notes: "",
      });
      changed();
    };
    let pan;
    canvas.onpointerdown = (e) => {
      if (e.target.closest("[data-marker]")) return;
      pan = {
        x: e.clientX,
        y: e.clientY,
        left: q("plan-scroll").scrollLeft,
        top: q("plan-scroll").scrollTop,
      };
      canvas.setPointerCapture(e.pointerId);
    };
    canvas.onpointermove = (e) => {
      if (!pan) return;
      q("plan-scroll").scrollLeft = pan.left + pan.x - e.clientX;
      q("plan-scroll").scrollTop = pan.top + pan.y - e.clientY;
    };
    canvas.onpointerup = canvas.onpointercancel = () => (pan = null);
    setZoom(zoom);
    draw();
  }
  function draw() {
    if (!q("plan-markers")) return;
    const meta = c.data.assets.find((a) => a.id === plan.imageID);
    const width = meta?.width || 1000,
      height = meta?.height || 1000;
    const radius = Math.min(width, height) * 0.012;
    q("plan-markers").setAttribute("viewBox", `0 0 ${width} ${height}`);
    const matches = (r) =>
      `${r.device.name} ${r.device.model} ${r.device.ipv4} ${r.device.serial}`
        .toLowerCase()
        .includes(search.toLowerCase());
    q("plan-devices").innerHTML = c
      .records()
      .filter(matches)
      .map(
        (r) =>
          `<div draggable="true" data-drag-device="${esc(r.id)}" class="plan-device"><strong>${esc(r.device.name || r.device.model || "Camera")}</strong><br><small>${esc(r.device.ipv4)} · ${plan.markers.some((m) => m.deviceID === r.id) ? "Placed" : "Unplaced"}</small></div>`,
      )
      .join("");
    for (const el of q("plan-devices").querySelectorAll("[draggable]"))
      el.ondragstart = (e) =>
        e.dataTransfer.setData("text/plain", el.dataset.dragDevice);
    q("plan-markers").innerHTML = plan.markers
      .filter((m) => !layer || m.layer === layer)
      .map((m) => {
        const r = c.view.devices.find((r) => r.id === m.deviceID),
          removed = !r || r.removed,
          dim = r && !matches(r);
        return `<g data-marker="${esc(m.id)}" tabindex="0" role="button" aria-label="${esc(m.label)}" opacity="${dim ? 0.25 : 1}"><title>${esc(m.label)} · ${removed ? "Archived / missing device" : `last observed ${date(r.device.lastSeen)}`}</title>${m.cone ? `<path d="${coveragePath(m, width, height)}" fill="#22bfa944" stroke="#17b7a5" stroke-width="2"/>` : ""}<circle cx="${m.x * width}" cy="${m.y * height}" r="${radius}" fill="${removed ? "#6b7280" : r.device.activated === false ? "#ca8420" : "#0d887d"}" stroke="white" stroke-width="3"/><text x="${m.x * width + 18}" y="${m.y * height + 5}" fill="white" stroke="#102331" paint-order="stroke" stroke-width="3" font-size="${radius * 1.6}">${esc(m.label)}</text></g>`;
      })
      .join("");
    for (const el of q("plan-markers").querySelectorAll("[data-marker]")) {
      const m = plan.markers.find((m) => m.id === el.dataset.marker);
      let dragging = false,
        start;
      el.onpointerdown = (e) => {
        e.stopPropagation();
        start = { x: e.clientX, y: e.clientY };
        el.setPointerCapture(e.pointerId);
      };
      el.onpointermove = (e) => {
        if (!start) return;
        if (Math.hypot(e.clientX - start.x, e.clientY - start.y) > 4)
          dragging = true;
        if (!dragging) return;
        const p = normalisePoint(
          e.clientX,
          e.clientY,
          q("plan-image").getBoundingClientRect(),
        );
        const dx = (p.x - m.x) * width,
          dy = (p.y - m.y) * height;
        el.setAttribute("transform", `translate(${dx} ${dy})`);
      };
      el.onpointerup = (e) => {
        e.stopPropagation();
        if (dragging) {
          Object.assign(
            m,
            normalisePoint(
              e.clientX,
              e.clientY,
              q("plan-image").getBoundingClientRect(),
            ),
          );
          changed();
        } else edit(m);
        start = null;
        dragging = false;
      };
      el.onpointercancel = () => {
        start = null;
        dragging = false;
        draw();
      };
      el.onkeydown = (e) => {
        if (e.key === "Enter") edit(m);
      };
    }
  }
  function edit(m) {
    const r = c.view.devices.find((r) => r.id === m.deviceID);
    const d = dialog(
      "Camera placement",
      `<p>${esc(c.deviceName(m.deviceID))}<br>Serial: ${esc(r?.device.serial || "Unknown")} · Last observed: ${esc(date(r?.device.lastSeen))}${r?.removed ? " · Archived" : ""}</p>` +
        field("Marker label", "label", m.label) +
        field("Layer", "layer", m.layer) +
        field(
          "Direction bearing ° (0 = up)",
          "bearing",
          m.bearing,
          "number",
          "min=0 max=360 step=any",
        ) +
        field(
          "Approximate coverage cone ° (0 = hidden)",
          "cone",
          m.cone,
          "number",
          "min=0 max=180 step=any",
        ) +
        area("Placement notes", "notes", m.notes) +
        `<div class="advanced-actions"><button type="button" class="btn btn-outline-primary marker-live">Open Live View</button><button type="button" class="btn btn-outline-danger marker-remove">Remove marker</button></div>`,
      (v) => {
        Object.assign(m, {
          label: v.label,
          layer: v.layer,
          bearing: Number(v.bearing),
          cone: Number(v.cone),
          notes: v.notes,
        });
        changed();
      },
    );
    d.querySelector(".marker-live").onclick = () =>
      void perform(() => Viewer.Open(c.site, m.deviceID));
    d.querySelector(".marker-remove").onclick = () => {
      plan.markers = plan.markers.filter((x) => x.id !== m.id);
      changed();
      d.close();
      d.remove();
    };
  }
  await open(selected);
}
