import * as API from "../bindings/github.com/TerminalAddict/SADPTools/toolkitservice.js";
import {
  $,
  esc,
  date,
  dialog,
  field,
  select,
  option,
  check,
  table,
  message,
} from "./toolkit-ui.js";
export function auditTable(report, filter = "") {
  return table(
    ["Device", "Check", "Status", "Evidence", "Source / time"],
    (report?.findings || [])
      .filter((f) => !filter || f.status === filter)
      .map(
        (f) =>
          `<tr><td>${esc(f.ip)}<br><small>${esc(f.model)} · ${esc(f.mac)}</small></td><td>${esc(f.check)}</td><td><span class="audit-status audit-${esc(f.status)}">${esc(f.status)}</span></td><td>${esc(f.evidence)}</td><td>${esc(f.source)}<br><small>${esc(date(f.at))}</small></td></tr>`,
      ),
  );
}
export function initAudit(kit) {
  let picked = new Set(),
    report = null,
    filter = "",
    polling = false;
  const perform = (fn) => Promise.resolve().then(fn).catch(kit.error);
  async function render() {
    const view = kit.sites.view();
    if (!view) {
      $("audit-page").innerHTML =
        '<div class="tool-empty"><h2>Health audit</h2><p>Select a saved site to run read-only health checks.</p></div>';
      return;
    }
    const rows = view.devices.filter((r) => !r.removed);
    picked = new Set(rows.map((r) => r.id));
    $("audit-page").innerHTML =
      `<div class="tool-heading"><div><h2>Health audit — ${esc(view.site.name)}</h2><p>Read-only checks of saved network consistency, duplicates, advertised service ports, HTTP/HTTPS and available authenticated ONVIF health data.</p></div></div><p class="small">Unlock the vault to use each device's assigned credentials. Unsupported APIs, missing credentials and failed connections remain Unknown / Not checked. Firmware upgrade suitability is not inferred from version strings.</p><details><summary>Device scope (${rows.length} saved devices)</summary><div id="audit-picks">${rows.map((r) => `<label class="d-block"><input type="checkbox" class="form-check-input me-2" data-record="${esc(r.id)}" checked>${esc(r.device.ipv4)} · ${esc(r.device.model)} · ${esc(r.device.mac)}</label>`).join("")}</div></details><div class="tool-actions mt-3"><button id="audit-start" class="btn btn-primary">Review and run audit…</button><button id="audit-selected" class="btn btn-outline-secondary">Use saved table selection</button><button id="audit-export" class="btn btn-outline-secondary">Export latest report</button><select id="audit-filter" class="form-select w-auto">${["", "pass", "warning", "fail", "unknown", "not-checked"].map((x) => option(x, x || "All results", filter)).join("")}</select><span id="audit-summary"></span></div><div id="audit-results"></div>`;
    for (const input of $("audit-picks").querySelectorAll("input"))
      input.onchange = (e) =>
        e.target.checked
          ? picked.add(input.dataset.record)
          : picked.delete(input.dataset.record);
    $("audit-selected").onclick = () => {
      picked = new Set(kit.sites.selected());
      for (const input of $("audit-picks").querySelectorAll("input"))
        input.checked = picked.has(input.dataset.record);
    };
    $("audit-filter").onchange = (e) => {
      filter = e.target.value;
      draw();
    };
    $("audit-export").onclick = () => exportReport("", "");
    $("audit-start").onclick = () => perform(() => approve(view));
    await refresh();
  }
  async function approve(view) {
    if (!picked.size) throw Error("Select at least one device.");
    const selected = view.devices.filter((r) => picked.has(r.id));
    const pins = {};
    const d = dialog(
      "Approve read-only audit scope",
      `<p>Site: <strong>${esc(view.site.name)}</strong></p><pre>${esc(selected.map((r) => `${r.device.ipv4} · ${r.device.model} · ${r.device.mac}`).join("\n"))}</pre><p>This will connect to the selected saved addresses. Check that you are connected to the intended customer network. Credentials will be used only for discovered, same-device ONVIF endpoints.</p><button type="button" class="btn btn-outline-secondary inspect-audit-tls">Inspect ONVIF HTTPS certificates (optional)</button><div class="audit-certificates mt-2"></div>` +
        check("I approve these devices and this site scope", "approved"),
      async (v) => {
        if (!v.approved) throw Error("Confirm the target scope first.");
        for (const input of d.querySelectorAll("[data-cert-record]"))
          if (input.checked) pins[input.dataset.certRecord] = input.dataset.pin;
        await API.StartAudit(
          view.site.id,
          selected.map((r) => r.id),
          pins,
        );
        await kit.poll();
        await refresh();
      },
      "Run read-only audit",
    );
    d.querySelector(".inspect-audit-tls").onclick = async (e) => {
      e.target.disabled = true;
      const list = d.querySelector(".audit-certificates");
      list.textContent = "Inspecting without credentials…";
      const output = [];
      for (const r of selected.filter((r) => r.device.xaddrs?.length)) {
        try {
          const endpoint = await API.ONVIFEndpoint(view.site.id, r.id);
          if (endpoint.scheme === "https" && !endpoint.certificateTrusted)
            output.push(
              `<label class="d-block my-2"><input type="checkbox" class="form-check-input me-2" data-cert-record="${esc(r.id)}" data-pin="${esc(endpoint.certificateSHA256)}">I verified ${esc(r.device.ipv4)}: ${esc(endpoint.subject)}<br><code>${esc(endpoint.certificateSHA256)}</code></label>`,
            );
        } catch (error) {
          output.push(
            `<p>${esc(r.device.ipv4)}: ${esc(error.message || error)}</p>`,
          );
        }
      }
      list.innerHTML =
        output.join("") ||
        "No untrusted ONVIF HTTPS certificates found. Unchecked certificates are not trusted for credential use.";
      e.target.disabled = false;
    };
  }
  function draw() {
    if (!$("audit-results")) return;
    $("audit-results").innerHTML = auditTable(report, filter);
    const counts = {};
    for (const f of report?.findings || [])
      counts[f.status] = (counts[f.status] || 0) + 1;
    $("audit-summary").textContent =
      `${report?.done || 0} / ${report?.total || 0} devices · ${Object.entries(
        counts,
      )
        .map(([k, v]) => `${v} ${k}`)
        .join(
          " · ",
        )}${report?.cancelled ? " · Cancelled; partial report" : ""}`;
  }
  async function refresh() {
    if (polling || !$("audit-results")) return;
    polling = true;
    try {
      report = await API.AuditStatus();
      if (report.siteID !== kit.sites.active()?.id) report = null;
      draw();
      const job = await API.Job();
      $("audit-start").disabled = job.running;
      $("audit-export").disabled = !report?.started;
    } catch (e) {
      kit.error(e);
    } finally {
      polling = false;
    }
  }
  function exportReport(site, run) {
    dialog(
      "Export health audit",
      select(
        "Format",
        "format",
        option("html", "Customer HTML report") +
          option("csv", "CSV") +
          option("json", "JSON"),
      ),
      (v) => API.ExportAudit(site, run, v.format),
      "Export",
    );
  }
  async function openSaved(site, run) {
    const report = JSON.parse(run.data);
    const d = dialog(
      `Health audit — ${report.siteName}`,
      `<p>${esc(date(report.started))} · ${esc(run.outcome)}</p>${auditTable(report)}`,
      async () => {},
      "Close",
    );
    const b = document.createElement("button");
    d.classList.add("tool-dialog-wide");
    d.querySelector(".close-dialog").hidden = true;
    b.type = "button";
    b.className = "btn btn-outline-secondary mt-2";
    b.textContent = "Export this report";
    b.onclick = () => exportReport(site, run.id);
    d.querySelector("form").append(b);
  }
  kit.pages.audit = render;
  setInterval(() => {
    if (!$("audit-page").hidden) void refresh();
  }, 1000);
  return { render, openSaved };
}
