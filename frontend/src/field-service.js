import { diagnosticRows } from "./field-view.js";
import * as Desktop from "../bindings/github.com/TerminalAddict/SADPTools/desktopservice.js";
import {
  esc,
  field,
  select,
  option,
  check,
  area,
  dialog,
  message,
  date,
} from "./toolkit-ui.js";

let currentTool = "connectivity";
const drafts = new Map();
const draftNames = new Set([
  "fs-interface",
  "fs-ports",
  "fs-timeout",
  "fs-http",
  "fs-https",
  "fs-icmp",
  "fs-rtsp",
  "fs-provider",
  "fs-channel",
  "fs-start",
  "fs-end",
  "fs-profile",
]);
export function selectFieldTool(tool) {
  currentTool = tool;
}

const tools = {
  connectivity: "Connection troubleshooter",
  protocol: "ONVIF / ISAPI explorer",
  security: "Security inspector",
  recordings: "Recording & gap finder",
  imaging: "Image tuning studio",
  anpr: "ANPR commissioning",
  replacement: "Camera replacement",
  firmware: "Firmware compatibility",
  labels: "Equipment labels",
  support: "Support bundle & traces",
};
export async function fieldPage(c) {
  const {
    API,
    q,
    on,
    button,
    table,
    docs,
    parse,
    perform,
    devicePicker,
    readOptions,
  } = c;
  let disposed = false,
    timer,
    latest = null,
    preview = null,
    historyID = "",
    renderedKey = "",
    lastRendered = "";
  const root = q("content");
  const val = (n) => root.querySelector(`[name="${n}"]`)?.value || "";
  const checked = (n) => !!root.querySelector(`[name="${n}"]`)?.checked;
  const body = () => root.querySelector("#fs-body");
  const results = () => root.querySelector("#fs-results");
  const selected = () => c.records().find((r) => r.id === c.device);
  const bind = (name, fn) => {
    const el = root.querySelector(`[data-fs="${name}"]`);
    if (el) el.onclick = () => void perform(fn);
  };
  const btn = (name, title) =>
    `<button type="button" class="btn btn-sm btn-outline-primary" data-fs="${name}">${esc(title)}</button>`;
  function remember() {
    if (!renderedKey) return;
    const values = {};
    for (const el of root.querySelectorAll("[name]"))
      if (draftNames.has(el.name))
        values[el.name] = el.type === "checkbox" ? el.checked : el.value;
    const probes = [...root.querySelectorAll("[data-probe]:checked")].map(
      (el) => el.dataset.probe,
    );
    drafts.set(renderedKey, { values, probes });
    if (drafts.size > 30) drafts.delete(drafts.keys().next().value);
  }
  function restoreDraft() {
    renderedKey = `${c.site}/${c.device}/${currentTool}`;
    const draft = drafts.get(renderedKey);
    if (!draft) return;
    for (const el of root.querySelectorAll("[name]"))
      if (Object.hasOwn(draft.values, el.name)) {
        if (el.type === "checkbox") el.checked = draft.values[el.name];
        else el.value = draft.values[el.name];
      }
    for (const el of root.querySelectorAll("[data-probe]"))
      el.checked = draft.probes.includes(el.dataset.probe);
  }
  root.innerHTML =
    devicePicker() +
    select(
      "Field tool",
      "fs-tool",
      Object.entries(tools)
        .map(([id, title]) => option(id, title, currentTool))
        .join(""),
    ) +
    `<div id="fs-body"></div><div class="d-flex gap-2 my-3" id="fs-actions"></div><div role="status" aria-live="polite" id="fs-status"></div><div id="fs-results"></div>`;
  root.querySelector('[name="fs-tool"]').onchange = async (e) => {
    remember();
    currentTool = e.target.value;
    clearInterval(timer);
    preview = null;
    await perform(render);
  };
  function actions(html) {
    root.querySelector("#fs-actions").innerHTML = html;
  }
  function output(text) {
    results().innerHTML = `<pre class="tool-readable">${esc(text)}</pre>`;
  }
  const report = (f) => {
    if (
      !f?.run?.id ||
      (!historyID &&
        (f.siteID !== c.site ||
          f.deviceID !== c.device ||
          f.run.target !== selected()?.device.ipv4))
    )
      return;
    const fingerprint = JSON.stringify(f) + historyID;
    if (fingerprint === lastRendered) return;
    lastRendered = fingerprint;
    latest = f;
    let html =
      `<p class="small">${esc(f.run.kind)} · ${esc(f.run.state)} · ${esc(date(f.run.started))} · Source ${esc(f.run.source || "unknown")} / ${esc(f.run.interface || "OS route")}</p>` +
      (f.run.steps?.length
        ? table(
            ["Test", "State", "Evidence / next check", "Elapsed"],
            diagnosticRows(f.run),
          )
        : "");
    for (const finding of f.findings || [])
      html += `<div class="border rounded p-2 mb-2"><strong>${esc(finding.name)} · ${esc(finding.state)}</strong><p>${esc(finding.evidence)}</p><p>${esc(finding.recommendation)}</p><small>What could break: ${esc(finding.couldBreak)}</small></div>`;
    for (const p of f.probes || [])
      html += `<details class="my-2"><summary>${esc(p.capability.operation)} · ${esc(p.step.status)} · HTTP ${esc(p.step.code || "unknown")} · ${esc(p.contentType)}</summary><p>${esc(p.step.evidence)}</p>${(p.endpoints || []).map((e) => `<p class="small">${esc(e.namespace || "Unknown namespace")} · ${esc(e.scheme)} ${esc(e.authority)} ${esc(e.path)} · ${esc(e.state)}</p>`).join("")}<pre class="tool-readable">${esc(p.response || "No safe payload available")}</pre></details>`;
    if (f.recording) {
      const r = f.recording;
      html +=
        `<h3>Recording evidence: ${esc(r.state)}</h3><p>Replay: ${esc(r.replay)}. Metadata from ${esc(r.provider)}. Time ranges below are UTC.</p>` +
        table(
          ["Channel", "Start", "End", "Precision"],
          (r.intervals || []).map(
            (i) =>
              `<tr><td>${esc(i.channel)}</td><td>${esc(i.start)}</td><td>${esc(i.end)}</td><td>${esc(i.precision)}</td></tr>`,
          ),
        ) +
        `<h4>Metadata gaps</h4>` +
        table(
          ["Start", "End"],
          (r.gaps || []).map(
            (g) => `<tr><td>${esc(g.start)}</td><td>${esc(g.end)}</td></tr>`,
          ),
        ) +
        `<p>${(r.notes || []).map(esc).join("<br>")}</p><details><summary>Device clock / timezone evidence</summary><pre>${esc(r.clock || "Unknown — check the recorder clock before comparing timestamps")}</pre></details><p class="text-body-secondary">Replay download/decoding is unavailable in this adapter. Open recorder playback to verify the selected interval; Live View does not verify recording.</p>`;
    }
    if (f.imaging) {
      const im = f.imaging;
      const profileInput = root.querySelector('[name="fs-profile"]');
      if (!historyID && profileInput && f.profiles?.length) {
        const value = profileInput.value;
        const container = profileInput.parentElement;
        container.outerHTML = select(
          "ONVIF video profile",
          "fs-profile",
          option("", "First advertised profile", value) +
            f.profiles
              .map((p) =>
                option(
                  p.id,
                  `${p.name || p.id} · ${p.codec || "Unknown codec"} ${p.width || "?"}×${p.height || "?"}`,
                  value,
                ),
              )
              .join(""),
        );
      }
      html +=
        `<h3>Imaging capabilities</h3><p>${esc(im.privacyMasks)}</p>` +
        table(
          ["Setting", "Value", "Support", "Change"],
          im.settings.map(
            (s) =>
              `<tr><td>${esc(s.name)}</td><td>${esc(s.value || "Unknown")}</td><td>${esc(s.state)}<br><small>${esc(s.reason)}</small></td><td><button class="btn btn-sm btn-outline-primary" data-image-setting="${esc(s.name)}" ${s.state !== "supported" || historyID ? "disabled" : ""}>Preview change</button></td></tr>`,
          ),
        ) +
        `<p>Use <strong>Live View → Focus & alignment</strong> for timestamped Before / After captures and side-by-side comparisons. Overlays are previews; verify privacy masks in camera output and NVR recording.</p>`;
    }
    results().innerHTML = html;
    for (const b of results().querySelectorAll("[data-image-setting]"))
      b.onclick = () => {
        const s = f.imaging.settings.find(
            (x) => x.name === b.dataset.imageSetting,
          ),
          d = selected()?.device;
        dialog(
          `Preview ${s.name}`,
          `<p>Target ${esc(d?.model)} · ${esc(d?.ipv4)} · ${esc(d?.serial)}. Current value: <strong>${esc(s.value)}</strong>. One setting will be written; no automatic retry.</p>` +
            (s.choices?.length
              ? select(
                  "Proposed value",
                  "value",
                  s.choices.map((v) => option(v, v, s.value)).join(""),
                )
              : field(
                  `Proposed value (${s.min}–${s.max})`,
                  "value",
                  s.value,
                  "number",
                  `required step=any min="${s.min}" max="${s.max}"`,
                )) +
            field(`Type APPLY ${d?.serial}`, "confirmation") +
            `<p>Restore uses a separately confirmed change to the recorded previous value. After an uncertain result, read settings again before doing anything else.</p>`,
          async (v) => {
            await API.ApplyImage({
              ...readOptions(),
              profileID: f.imaging.profileID,
              field: s.name,
              value: v.value,
              previous: s.value,
              serial: d.serial,
              confirmation: v.confirmation,
            });
          },
          "Apply one setting",
        );
      };
  };
  async function poll() {
    if (
      disposed ||
      !root.isConnected ||
      document.getElementById("advanced-page").hidden
    ) {
      clearInterval(timer);
      return;
    }
    const job = await API.Job();
    root.querySelector("#fs-status").textContent = job.running
      ? `${job.message || "Working…"} (${job.done}/${job.total || "?"})`
      : job.error || "";
    if (currentTool === "anpr") {
      const s = await API.ANPR();
      if (s.deviceID !== c.device) return;
      latest = s;
      results().innerHTML =
        `<p>${esc(s.status)} · ${s.total} unique plate events · ${esc(s.cleanup)}</p><p>Camera detection → local receipt → <strong>NVR/VMS not verified</strong>. Confidence is shown in device-reported units.</p>` +
        table(
          ["Device / receipt time", "Plate", "Confidence", "Direction / lane"],
          s.events.map(
            (e) =>
              `<tr><td>${esc(e.deviceTime || "Unknown")}<br><small>${esc(e.received)}</small></td><td>${esc(e.plate || "Unknown")}</td><td>${esc(e.confidence ?? "Unknown")}</td><td>${esc(e.direction || "Unknown")} / ${esc(e.lane || "Unknown")}</td></tr>`,
          ),
        );
    } else if (
      [
        "connectivity",
        "protocol",
        "security",
        "recordings",
        "imaging",
      ].includes(currentTool)
    ) {
      const f = await API.Field();
      if (!historyID && f.run.kind === currentTool) report(f);
    }
  }
  async function render() {
    clearInterval(timer);
    results().innerHTML = "";
    body().innerHTML = "";
    actions("");
    latest = null;
    historyID = "";
    lastRendered = "";
    if (
      [
        "connectivity",
        "protocol",
        "security",
        "recordings",
        "imaging",
      ].includes(currentTool)
    ) {
      let html = "";
      if (["connectivity", "security"].includes(currentTool)) {
        const adapters = await Desktop.Interfaces();
        html += `<div class="advanced-grid">${select(
          "Source adapter",
          "fs-interface",
          option("", "OS-selected route", "") +
            adapters
              .filter((a) => a.unicastSuitable)
              .map((a) => option(a.name, a.name, ""))
              .join(""),
        )}${field("TCP ports (maximum 12)", "fs-ports", currentTool === "security" ? "21,23,80,443,554,8000" : "80,443,554,8000")}${field("Probe timeout (ms)", "fs-timeout", 2000, "number", "min=100 max=10000")}${field("HTTP ports", "fs-http", "80")}${field("HTTPS ports", "fs-https", "443")}</div>${check("Attempt ICMP; TCP remains useful if ping is blocked", "fs-icmp", true)}<p class="small">Only this selected device is probed. No response does not mean an address is unused. A bound source does not override the OS route table.</p>`;
      }
      if (currentTool === "connectivity")
        html += check(
          "Read ONVIF profile and test authenticated RTSP DESCRIBE (no playback)",
          "fs-rtsp",
        );
      if (currentTool === "protocol" || currentTool === "connectivity") {
        const catalogue = await API.FieldCatalogue();
        html += `<details ${currentTool === "protocol" ? "open" : ""}><summary>Allowlisted authenticated reads</summary><div class="report-evidence-grid">${catalogue.map((p) => `<label class="form-check"><input class="form-check-input" type="checkbox" data-probe="${esc(p.id)}" ${currentTool === "protocol" && ["onvif-services", "onvif-device"].includes(p.id) ? "checked" : ""}>${esc(p.protocol)} · ${esc(p.operation)}<small class="d-block">${esc(p.purpose)}</small></label>`).join("")}</div></details>`;
      }
      if (currentTool === "recordings")
        html += `<div class="advanced-grid">${select("Recording adapter", "fs-provider", option("isapi", "Hikvision ISAPI segment search", "isapi") + option("onvif", "ONVIF coarse recording bounds", ""))}${field("Advertised recording track / channel", "fs-channel", "", "text", "placeholder='e.g. track from NVR configuration'")}${field("Start (ISO 8601 with Z or UTC offset)", "fs-start", new Date(Date.now() - 3600000).toISOString())}${field("End (ISO 8601 with Z or UTC offset)", "fs-end", new Date().toISOString())}</div><p>Maximum seven days per query. Use the exact recording track from your NVR; stream IDs and recording tracks may differ. No clip is downloaded automatically.</p>`;
      if (currentTool === "imaging")
        html += `<p>Read ONVIF imaging capabilities before changing anything. The first advertised video profile is used by default.</p>${field("ONVIF profile reference (leave empty for first profile)", "fs-profile")}`;
      const saved = c.data.documents.filter(
        (d) =>
          d.kind === "field-evidence" &&
          d.deviceID === c.device &&
          (parse(d).run?.kind === currentTool ||
            (currentTool === "connectivity" &&
              parse(d).run?.kind === "remote-connectivity")),
      );
      html += `<details class="mt-3"><summary>Saved evidence (${saved.length})</summary>${select("Result", "fs-history", option("", "Current run", "") + saved.map((d) => option(d.id, `${d.title} · ${date(d.updated)}`, "")).join(""))}${btn("history", "View selected result")}${btn("history-export", "Preview export")}${btn("history-delete", "Delete saved result")}</details>`;
      if (currentTool === "connectivity")
        html += `<details class="mt-3"><summary>Compare from a trusted SSH probe</summary>${select(
          "Registered remote probe",
          "fs-remote",
          option("", "Select a registered SSH target", "") +
            docs("ssh-target")
              .map((d) => option(d.id, d.title, ""))
              .join(""),
        )}${btn("remote", "Run remote comparison")}<p class="small">Requires sadptools 0.6 on that host. Uses its saved SSH trust and credentials; checks this same camera IP.</p></details>`;
      body().innerHTML = html;
      actions(
        btn("run", "Run selected checks") +
          btn("cancel", "Cancel") +
          btn("copy", "Copy safe summary") +
          btn("live", "Open Live View"),
      );
      bind("run", async () => {
        historyID = "";
        const ports = (n) =>
          val(n)
            .split(",")
            .map((x) => Number(x.trim()))
            .filter(Boolean);
        historyID = "";
        const o = {
          ...readOptions(),
          kind: currentTool,
          rtsp: checked("fs-rtsp"),
          network: {
            target: "",
            interface: val("fs-interface"),
            timeoutMS: Number(val("fs-timeout")),
            ports: ports("fs-ports"),
            httpPorts: ports("fs-http"),
            httpsPorts: ports("fs-https"),
            icmp: checked("fs-icmp"),
            expectedNetworks: [],
          },
          operations: [...root.querySelectorAll("[data-probe]:checked")].map(
            (x) => x.dataset.probe,
          ),
          window: { start: val("fs-start"), end: val("fs-end") },
          provider: val("fs-provider"),
          channel: val("fs-channel"),
          profileID: val("fs-profile"),
        };
        if (currentTool !== "recordings")
          o.window = {
            start: "0001-01-01T00:00:00Z",
            end: "0001-01-01T00:00:00Z",
          };
        await API.StartField(o);
        await poll();
      });
      bind("history", () => {
        const d = saved.find((d) => d.id === val("fs-history"));
        historyID = d?.id || "";
        if (d) report(parse(d));
        else return poll();
      });
      bind("history-export", () => {
        const d = saved.find((d) => d.id === val("fs-history"));
        if (!d) throw Error("Select a saved result first");
        dialog(
          "Review scrubbed diagnostic export",
          `<pre class="tool-readable">${esc(JSON.stringify(d.data, null, 2))}</pre>`,
          () => API.ExportEvidenceFormat(c.site, d.id, "json"),
          "Export JSON",
        );
      });
      bind("history-delete", () => {
        const d = saved.find((d) => d.id === val("fs-history"));
        if (!d) throw Error("Select a saved result first");
        dialog(
          "Delete diagnostic evidence",
          `<p>Delete ${esc(d.title)}? Existing report references may prevent deletion.</p>`,
          async () => {
            await API.DeleteAdvanced(c.site, d.id);
            await c.refresh();
          },
          "Delete",
        );
      });
      bind("remote", async () => {
        if (!val("fs-remote")) throw Error("Choose a registered remote probe");
        await API.RemoteTroubleshoot(readOptions(), val("fs-remote"));
      });
      bind("cancel", () => API.CancelJob());
      bind("copy", () =>
        API.CopyText(
          (latest?.run?.steps || [])
            .map(
              (s) => `${s.test}: ${s.status}\n${s.evidence}\n${s.next || ""}`,
            )
            .join("\n"),
        ),
      );
      bind("live", () => c.Viewer.Open(c.site, c.device));
      await poll();
      timer = setInterval(() => void perform(poll), 700);
    }
    if (currentTool === "labels") {
      const labelAssets = await API.LabelAssets(c.site);
      body().innerHTML = `<div class="advanced-grid">${select("Saved camera, NVR or switch", "fs-label-asset", labelAssets.map((a) => option(a.id, `${a.kind} · ${a.name} · ${a.ip}`, c.device)).join(""))}${field("Site name / short code", "fs-label-site", c.view.site.name)}${select("Label size", "fs-size", option("75,35", "75 × 35 mm", "75,35") + option("62,29", "62 × 29 mm", "") + option("100,50", "100 × 50 mm", ""))}${field("Copies on one sheet", "fs-copies", 1, "number", "min=1 max=12")}${field("Location / camera number", "fs-location", selected()?.location || "")}${field("NVR/channel designation (optional)", "fs-label-channel")}</div>${check("Print IP address (discloses network information)", "fs-print-ip")}${check("Print MAC address", "fs-print-mac")}<p>QR contains only an opaque local site/asset identifier. No password, serial or authenticated link is encoded. Print the exported SVG at 100% / actual size.</p>${field("Scan with a keyboard scanner, or paste asset code", "fs-code")}`;
      const options = () => {
        const [widthMM, heightMM] = val("fs-size").split(",").map(Number);
        return {
          siteID: c.site,
          deviceID:
            labelAssets.find((a) => a.id === val("fs-label-asset"))?.kind ===
            "device"
              ? val("fs-label-asset")
              : "",
          switchID:
            labelAssets.find((a) => a.id === val("fs-label-asset"))?.kind ===
            "switch"
              ? val("fs-label-asset")
              : "",
          siteText: val("fs-label-site"),
          widthMM,
          heightMM,
          copies: Number(val("fs-copies")),
          showIP: checked("fs-print-ip"),
          showMAC: checked("fs-print-mac"),
          location: val("fs-location"),
          channel: val("fs-label-channel"),
        };
      };
      actions(
        btn("preview", "Print-safe preview") +
          btn("export", "Export SVG labels") +
          btn("locate", "Locate scanned asset"),
      );
      bind("preview", async () => {
        results().innerHTML = `<div class="bg-white p-3 rounded"><img class="img-fluid" alt="Label print preview" src="${esc(await API.PreviewLabel(options()))}"></div>`;
      });
      bind("export", async () =>
        message("Labels exported", await API.ExportLabel(options())),
      );
      bind("locate", async () => {
        const r = await API.LocateAsset(val("fs-code"));
        message(
          "Located saved asset",
          `${r.kind} · ${r.name}\n${r.location}\nCurrent IP: ${r.ip}\n${r.removed ? "Retired/replaced asset" : "Active inventory item"}`,
        );
      });
      root.querySelector('[name="fs-code"]').onkeydown = (e) => {
        if (e.key === "Enter") {
          e.preventDefault();
          root.querySelector('[data-fs="locate"]').click();
        }
      };
    }
    if (currentTool === "support") {
      const capture = await API.TraceSupport(),
        adapters = await Desktop.Interfaces();
      body().innerHTML = `<p>Preview every file before exporting. Ordinary bundles exclude raw captures, plate text, footage, credentials and configuration backups.</p>${check("Anonymise selected device IP/MAC/serial/name and local IDs", "fs-anon", true)}<div class="report-evidence-grid">${c.data.documents
        .filter(
          (d) =>
            [
              "field-evidence",
              "stream-diagnostic",
              "walk-test",
              "anpr-session",
            ].includes(d.kind) && d.deviceID === c.device,
        )
        .map(
          (d) =>
            `<label class="form-check"><input class="form-check-input" type="checkbox" data-bundle="${esc(d.id)}">${esc(d.title)} · ${esc(date(d.updated))}</label>`,
        )
        .join(
          "",
        )}</div><details class="mt-3"><summary>Optional screenshots — explicit approval required</summary><p class="text-warning">Image pixels may show faces, plates and private premises. They are NOT anonymised. Review the exact images before exporting.</p>${check("Approve exporting the selected image pixels", "fs-image-consent")}<div class="report-evidence-grid">${c.data.assets
        .filter((a) => a.kind === "snapshot" && a.deviceID === c.device)
        .map(
          (a) =>
            `<label class="form-check"><input class="form-check-input" type="checkbox" data-bundle-image="${esc(a.id)}">${esc(a.title)} · ${esc(date(a.created))}</label>`,
        )
        .join(
          "",
        )}</div></details>${field("Optional export encryption passphrase (12+ characters)", "fs-pass", "", "password")}<p class="small">Encryption produces .zip.sadpencrypted, using SADPTools' authenticated backup encryption. The unencrypted option is a standard ZIP. No upload is performed.</p>`;
      body().innerHTML += `<details class="mt-3"><summary>Optional raw packet capture</summary><p>${esc(capture.reason)}</p><p class="text-warning">Raw traces may contain passwords, tokens, plate data or video. They cannot be safely anonymised automatically. Saved separately from support bundles; delete the chosen file when no longer needed.</p>${select(
        "One capture adapter",
        "fs-capture-interface",
        adapters
          .filter((a) => a.unicastSuitable)
          .map((a) => option(a.name, a.name, ""))
          .join(""),
      )}${field("Duration (1–60 seconds)", "fs-capture-seconds", 10, "number", "min=1 max=60")}${check(`Approve raw capture for host ${selected()?.device.ipv4} on the selected adapter`, "fs-capture-consent")}<button class="btn btn-sm btn-outline-warning" data-fs="capture" ${capture.available ? "" : "disabled"}>Capture sensitive trace</button></details>`;
      for (const el of body().querySelectorAll(
        '[data-bundle],[data-bundle-image],[name="fs-anon"],[name="fs-image-consent"]',
      ))
        el.onchange = () => {
          preview = null;
          results().innerHTML =
            "<p>Selection changed. Prepare a new preview before exporting.</p>";
        };
      actions(
        btn("preview", "Prepare bundle preview") +
          btn("export", "Export reviewed bundle") +
          btn("decrypt", "Decrypt an existing bundle"),
      );
      bind("capture", () =>
        API.CaptureTrace({
          interface: val("fs-capture-interface"),
          target: selected().device.ipv4,
          seconds: Number(val("fs-capture-seconds")),
          approved: checked("fs-capture-consent"),
        }),
      );
      bind("decrypt", () =>
        dialog(
          "Decrypt support bundle",
          field("Bundle passphrase", "passphrase", "", "password"),
          async (v) =>
            message("Decrypted ZIP", await API.DecryptSupport(v.passphrase)),
          "Choose encrypted file",
        ),
      );
      bind("preview", async () => {
        preview = await API.PreviewSupport({
          siteID: c.site,
          deviceID: c.device,
          documents: [...root.querySelectorAll("[data-bundle]:checked")].map(
            (x) => x.dataset.bundle,
          ),
          anonymise: checked("fs-anon"),
          screenshots: [
            ...root.querySelectorAll("[data-bundle-image]:checked"),
          ].map((x) => x.dataset.bundleImage),
          approveImages: checked("fs-image-consent"),
        });
        results().innerHTML =
          `<p>ZIP contents: manifest.json, SHA256SUMS and the files below. Review all remaining identifiers before sharing.</p><pre class="tool-readable">${esc(JSON.stringify(preview.manifest, null, 2))}</pre>` +
          Object.entries(preview.preview)
            .map(
              ([name, text]) =>
                `<details><summary>${esc(name)}</summary><pre class="tool-readable">${esc(text)}</pre></details>`,
            )
            .join("") +
          Object.entries(preview.images || {})
            .map(
              ([name, src]) =>
                `<figure><figcaption>${esc(name)} — pixels included without redaction</figcaption><img class="img-fluid" src="${esc(src)}" alt="Explicitly selected screenshot preview"></figure>`,
            )
            .join("");
      });
      bind("export", async () => {
        if (!preview) throw Error("Prepare and review a bundle first");
        const pass = val("fs-pass");
        try {
          message(
            "Support bundle",
            await API.ExportSupport(preview.digest, pass),
          );
        } finally {
          root.querySelector('[name="fs-pass"]').value = "";
        }
      });
    }
    if (currentTool === "firmware") {
      const firmware = docs("firmware");
      body().innerHTML =
        `<p>Read-only library and compatibility planning. Firmware upload/reboot is disabled until a model-specific adapter has passed hardware validation. Imported metadata is operator-supplied, not proof of vendor authenticity.</p>` +
        table(
          ["Model / version", "Source / SHA-256", "Actions"],
          firmware.map((d) => {
            const f = parse(d);
            return `<tr><td>${esc(f.vendor)} ${esc(f.model)}<br>${esc(f.version)}<details><summary>Provenance & upgrade notes</summary><p>Hardware: ${esc(f.hardware || "Unknown")} · Platform: ${esc(f.platform || "Unknown")} · Region: ${esc(f.region || "Unknown")}</p><p>${esc(f.provenance)}</p><p>Signature: ${esc(f.signature)}</p><pre>${esc(f.releaseNotes)}</pre><p>Documented intermediate builds (operator supplied):</p><pre>${esc(f.intermediateBuilds || "None documented; no safe sequence inferred")}</pre></details></td><td>${esc(f.sourceURL)}<br><small class="text-break">${esc(f.sha256)}</small></td><td><button class="btn btn-sm btn-outline-primary" data-fw-match="${esc(d.id)}">Compare selected device</button> <button class="btn btn-sm btn-outline-secondary" data-fw-verify="${esc(d.id)}">Recheck file hash</button></td></tr>`;
          }),
        );
      actions(
        btn("import", "Import firmware provenance") +
          btn("identity", "Read device identity"),
      );
      bind("identity", () =>
        API.StartField({
          ...readOptions(),
          kind: "protocol",
          operations: ["onvif-device", "isapi-device"],
          network: {},
          window: {
            start: "0001-01-01T00:00:00Z",
            end: "0001-01-01T00:00:00Z",
          },
          channel: "",
          provider: "",
          profileID: "",
        }),
      );
      bind("import", () =>
        dialog(
          "Import firmware — no upload",
          field("Vendor", "vendor", "", "text", "required") +
            field("Exact model", "model", "", "text", "required") +
            field(
              "Official HTTPS source URL",
              "sourceURL",
              "",
              "url",
              "required",
            ) +
            field("Version/build", "version") +
            field("Hardware revision", "hardware") +
            field("Platform", "platform") +
            field("Region/language identifier", "region") +
            field("Official SHA-256 (optional)", "expectedSHA256") +
            area("Release notes", "releaseNotes") +
            area(
              "Documented intermediate builds, in order",
              "intermediateBuilds",
            ),
          async (v) => {
            await API.ImportFirmware(c.site, {
              ...v,
              filename: "",
              bytes: 0,
              sha256: "",
              signature: "",
              provenance: "",
              imported: "",
            });
            await c.refresh();
          },
          "Choose firmware file",
        ),
      );
      for (const b of root.querySelectorAll("[data-fw-match]"))
        b.onclick = () =>
          void perform(async () =>
            output(
              JSON.stringify(
                await API.MatchFirmware(c.site, b.dataset.fwMatch, c.device),
                null,
                2,
              ),
            ),
          );
      for (const b of root.querySelectorAll("[data-fw-verify]"))
        b.onclick = () =>
          void perform(async () =>
            message(
              "Firmware integrity",
              await API.VerifyFirmware(c.site, b.dataset.fwVerify),
            ),
          );
    }
    if (currentTool === "replacement") {
      const plans = docs("replacement");
      body().innerHTML =
        `<p>Plan a replacement without losing the old camera's history. Save the replacement from discovery to this site first. Activation/network changes use their existing explicit-confirmation workflows. Automatic settings cloning and NVR reassociation are not hardware validated.</p>` +
        table(
          ["Plan", "State / checks", "Actions"],
          plans.map((d) => {
            const p = parse(d);
            return `<tr><td>${esc(c.deviceName(p.oldDeviceID))} → ${esc(c.deviceName(p.newDeviceID))}</td><td>${esc(p.state)}<br>${esc(p.verification)}</td><td><button class="btn btn-sm btn-link" data-plan-show="${esc(d.id)}">Review steps</button><button class="btn btn-sm btn-outline-primary" data-plan-verify="${esc(d.id)}" ${p.state === "completed" ? "disabled" : ""}>Preflight / verify</button><button class="btn btn-sm btn-outline-secondary" data-plan-complete="${esc(d.id)}" ${p.state !== "verified" ? "disabled" : ""}>Reconcile inventory</button></td></tr>`;
          }),
        );
      actions(btn("plan", "Plan replacement"));
      bind("plan", () =>
        dialog(
          "Plan camera replacement",
          select(
            "Old saved device",
            "old",
            c
              .records()
              .map((r) => option(r.id, c.deviceName(r.id), c.device))
              .join(""),
          ) +
            select(
              "New saved replacement",
              "replacement",
              c
                .records()
                .map((r) => option(r.id, c.deviceName(r.id), ""))
                .join(""),
            ) +
            area("Operator notes", "notes"),
          async (v) => {
            await API.CreateReplacement(c.site, v.old, v.replacement, v.notes);
            await c.refresh();
          },
          "Create dry-run plan",
        ),
      );
      for (const b of root.querySelectorAll("[data-plan-show]"))
        b.onclick = () =>
          message(
            "Replacement plan",
            JSON.stringify(
              parse(plans.find((d) => d.id === b.dataset.planShow)),
              null,
              2,
            ),
          );
      for (const b of root.querySelectorAll("[data-plan-verify]"))
        b.onclick = () =>
          void perform(() =>
            API.VerifyReplacement(c.site, b.dataset.planVerify),
          );
      for (const b of root.querySelectorAll("[data-plan-complete]"))
        b.onclick = () => {
          const d = plans.find((d) => d.id === b.dataset.planComplete),
            p = parse(d),
            key = p.newIdentity.mac
              ? `mac:${p.newIdentity.mac}`
              : `serial:${p.newIdentity.serial}`;
          dialog(
            "Retire old inventory record",
            `<p>This archives the old device, preserving its history. It does not transfer credentials or rewrite historic NVR mappings. Verify Live View, NVR association and recording first.</p>` +
              field(`Type REPLACE ${key}`, "confirmation") +
              area(
                "Verification / recovery notes (required; include stream/NVR/recording checks)",
                "notes",
              ),
            async (v) => {
              await API.CompleteReplacement(
                c.site,
                d.id,
                d.updated,
                v.confirmation,
                v.notes,
              );
              await c.refresh();
            },
            "Complete replacement",
          );
        };
    }
    if (currentTool === "anpr") {
      body().innerHTML = `<div class="advanced-grid">${field("Session duration (seconds)", "fs-seconds", 60, "number", "min=10 max=300")}</div>${check("Allow plate text in this local session for up to five minutes after stopping", "fs-plates")}<p>Uses recognised ONVIF plate-event fields when the camera exposes them. Missing plate/confidence remains unknown. No VMS event is sent. Camera event receipt alone never proves NVR/VMS integration.</p>`;
      actions(
        btn("start", "Start ANPR session") +
          btn("cancel", "Stop") +
          btn("clear", "Clear plate evidence") +
          btn("grade", "Save test assessment") +
          btn("live", "Open Live View"),
      );
      bind("start", () =>
        API.StartANPR(
          readOptions(),
          Number(val("fs-seconds")),
          checked("fs-plates"),
        ),
      );
      bind("cancel", () => API.CancelJob());
      bind("clear", () => API.ClearANPR());
      bind("live", () => c.Viewer.Open(c.site, c.device));
      bind("grade", async () => {
        const s = await API.ANPR();
        if (s.running || s.deviceID !== c.device)
          throw Error("Finish this device's ANPR session first");
        dialog(
          "Grade technician-approved test cases",
          field(
            "Expected test cases",
            "expected",
            1,
            "number",
            "min=1 max=10000",
          ) +
            field("Correct recognitions", "matched", 0, "number", "min=0") +
            field(
              "Confirmed recognition failures",
              "recognitionFailures",
              0,
              "number",
              "min=0",
            ) +
            field(
              "Confirmed transport failures",
              "transportFailures",
              0,
              "number",
              "min=0",
            ) +
            area(
              "Lighting, plate size/angle, IR/white light, lane",
              "conditions",
            ) +
            area("Notes (do not include plate numbers)", "notes"),
          async (v) => {
            await API.SaveANPR(s.id, {
              ...v,
              expected: Number(v.expected),
              matched: Number(v.matched),
              recognitionFailures: Number(v.recognitionFailures),
              transportFailures: Number(v.transportFailures),
              integration: "not-verified",
            });
            await c.refresh();
          },
        );
      });
      await poll();
      timer = setInterval(() => void perform(poll), 700);
    }
    restoreDraft();
  }
  await render();
  return () => {
    remember();
    disposed = true;
    clearInterval(timer);
  };
}
