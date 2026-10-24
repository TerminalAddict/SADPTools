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
  message,
  table,
} from "./toolkit-ui.js";
const identity = (d) =>
  d.mac
    ? `mac:${d.mac.toUpperCase()}`
    : d.serial
      ? `serial:${d.serial}`
      : d.uuid
        ? `uuid:${d.uuid}`
        : "";
export function initReconciliation(kit) {
  const perform = (fn) => Promise.resolve().then(fn).catch(kit.error);
  document.addEventListener(
    "toolkit:rescan",
    (e) =>
      void perform(async () => {
        const scope = await API.SiteScanScope(e.detail.id);
        dialog(
          "Review site discovery scope",
          `<p><strong>${esc(scope.siteName)}</strong></p><dl><dt>Adapter</dt><dd>${esc(scope.interface)} · ${esc(scope.addresses.join(", "))}</dd><dt>Approved device networks</dt><dd>${esc(scope.networks.join(", "))}</dd><dt>Protocols / duration</dt><dd>${esc(scope.protocols.join(" + "))} · ${scope.timeoutSeconds} seconds</dd></dl><p>${esc(scope.warning)}</p>` +
            check(
              "I am connected to the intended site and approve this scope",
              "approved",
            ),
          async (v) => {
            if (!v.approved) throw Error("Confirm the site and scope first.");
            await API.StartSiteRescan(scope.siteID, scope.token);
            await kit.poll();
          },
          "Start rescan",
        );
      }),
  );
  async function review(site, runID) {
    const run = await API.SiteRun(site, runID);
    if (!["discovery", "onvif-information"].includes(run.kind)) {
      if (kit.openRun) return kit.openRun(site, run);
      message(
        "Saved scan / report",
        `${date(run.at)} · ${run.scope}\nOutcome: ${run.outcome}\n\n${JSON.stringify(JSON.parse(run.data), null, 2)}`,
      );
      return;
    }
    const proposals = await API.ReconcileSite(site, runID);
    const d = dialog(
      "Review saved observations",
      `<p>${esc(date(run.at))} · ${esc(run.outcome)}</p><p>${esc(run.scope)}</p><p class="small">Saved notes and credentials are preserved. Not seen means this scan did not observe the device; it does not prove removal. Unchecked changes keep their saved values.</p><div class="review-proposals"></div><button type="button" class="btn btn-sm btn-outline-secondary export-history">Export change history</button>`,
      async () => {},
      "Close",
    );
    d.classList.add("tool-dialog-wide");
    d.querySelector(".close-dialog").hidden = true;
    d.querySelector(".export-history").onclick = () =>
      dialog(
        "Export site change history",
        select(
          "Format",
          "format",
          option("html", "HTML report") +
            option("csv", "CSV") +
            option("json", "JSON"),
        ),
        async (v) => API.ExportHistory(site, v.format),
        "Export",
      );
    const area = d.querySelector(".review-proposals");
    for (const p of proposals) {
      const card = document.createElement("section");
      card.className = "review-card";
      card.innerHTML = `<div class="d-flex gap-2 align-items-center"><strong>${esc(p.after.model || p.before.model || "Device")} · ${esc(p.after.ipv4 || p.before.ipv4)}</strong><span class="badge text-bg-${p.kind === "identity-review" ? "danger" : p.kind === "unchanged" ? "secondary" : "warning"}">${esc(p.kind)}</span></div><p class="small">${esc(p.message)} Identity confidence: ${esc(p.confidence)}. Last seen ${esc(date(p.before.lastSeen || p.after.lastSeen))}</p>${table(
        ["Accept", "Field", "Saved value", "Observed value"],
        p.changes.map(
          (c) =>
            `<tr><td><input type="checkbox" class="form-check-input accept-field" data-field="${esc(c.field)}" aria-label="Accept ${esc(c.field)}" ${c.manual ? "" : "checked"}></td><td>${esc(c.field)}${c.manual ? ' <small class="text-warning">Manual value — kept by default</small>' : ""}</td><td>${esc(c.before)}</td><td>${esc(c.after)}</td></tr>`,
        ),
      )}<div class="tool-actions"><button type="button" class="btn btn-sm btn-primary accept-change" ${["identity-review", "not-seen"].includes(p.kind) ? "hidden" : ""}>${p.kind === "new" ? "Add device" : p.kind === "unchanged" ? "Confirm observation" : "Accept selected changes"}</button><button type="button" class="btn btn-sm btn-outline-secondary ignore-change">Ignore observation</button><button type="button" class="btn btn-sm btn-outline-danger remove-change" ${p.kind !== "not-seen" ? "hidden" : ""}>Mark intentionally removed</button><span class="review-state small"></span></div>`;
      const done = (text) => {
        card.querySelector(".review-state").textContent = text;
        for (const b of card.querySelectorAll("button,input"))
          b.disabled = true;
      };
      card.querySelector(".accept-change").onclick = () =>
        void perform(async () => {
          const fields = [
            ...card.querySelectorAll(".accept-field:checked"),
          ].map((el) => el.dataset.field);
          await API.AcceptSiteChange(
            site,
            runID,
            p.recordID,
            identity(p.after),
            fields,
          );
          done("Accepted and recorded in history");
          await kit.sites.refresh();
        });
      card.querySelector(".ignore-change").onclick = () =>
        void perform(async () => {
          await API.IgnoreSiteChange(
            site,
            runID,
            identity(p.after) || p.recordID,
          );
          done("Ignored; saved inventory unchanged");
        });
      card.querySelector(".remove-change").onclick = () =>
        dialog(
          "Mark device intentionally removed",
          `<p>Only mark this if you know it was removed. A missing discovery reply is insufficient evidence.</p><p>${esc(p.before.model)} · ${esc(p.before.ipv4)} · ${esc(p.before.mac)}</p>`,
          async () => {
            const view = await API.Site(site);
            const r = view.devices.find((x) => x.id === p.recordID);
            if (!r) throw Error("Saved record changed; reopen review.");
            await API.SaveSiteDevice({ ...r, removed: true });
            done("Marked intentionally removed");
            await kit.sites.refresh();
          },
          "Mark removed",
        );
      area.append(card);
    }
    if (!proposals.length)
      area.textContent = "No observations or saved devices in this scope.";
  }
  document.addEventListener(
    "toolkit:run",
    (e) => void perform(() => review(e.detail.site, e.detail.run)),
  );
  async function queryONVIF(id) {
    const site = kit.sites.active();
    const view = site ? await API.Site(site.id) : null;
    const endpoint = await API.ONVIFEndpoint(site?.id || "", id);
    const untrusted =
      endpoint.scheme === "https" && !endpoint.certificateTrusted;
    dialog(
      "Read ONVIF device information",
      `<p>${esc(endpoint.url)}</p><p>This reads identity, network interfaces and advertised service ports. Device settings are not changed. Results for a selected site are saved as observations for review.</p>${endpoint.scheme === "http" ? '<p class="text-warning">HTTP has no transport encryption. Use a trusted management network.</p>' : ""}${untrusted ? `<div class="alert alert-warning">Untrusted HTTPS certificate.<br>Subject: ${esc(endpoint.subject)}<br>SHA-256: <code>${esc(endpoint.certificateSHA256)}</code></div>` + check("I verified and trust this exact device certificate for this query", "trust") : ""}` +
        select(
          "Credential profile",
          "profile",
          option("", "No credentials (partial/unauthenticated access)") +
            (view?.profiles || [])
              .map((p) =>
                option(
                  p.id,
                  `${p.label} · ${p.username}`,
                  view?.site.defaultCredential,
                ),
              )
              .join(""),
        ) +
        `<p class="small">If saved passwords are locked, unlock them in Sites to use a profile. Configure ONVIF user permissions on the camera if it uses a separate ONVIF account.</p>`,
      async (v) => {
        if (untrusted && !v.trust)
          throw Error(
            "Verify and accept the certificate before using this endpoint.",
          );
        await API.QueryONVIF(
          site?.id || "",
          id,
          v.profile,
          untrusted ? endpoint.certificateSHA256 : "",
        );
        await kit.poll();
      },
      "Read information",
    );
  }
  document.addEventListener(
    "toolkit:onvif",
    (e) => void perform(() => queryONVIF(e.detail)),
  );
  return { review, queryONVIF };
}
