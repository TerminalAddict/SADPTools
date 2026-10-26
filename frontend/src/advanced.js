import * as API from "../bindings/github.com/TerminalAddict/SADPTools/toolkitservice.js";
import * as Viewer from "../bindings/github.com/TerminalAddict/SADPTools/liveviewservice.js";
import {
  $,
  esc,
  field,
  area,
  select,
  option,
  check,
  dialog,
  message,
  date,
} from "./toolkit-ui.js";
import { retentionPage } from "./retention.js";
import { plansPage } from "./plans.js";
import { commissioningPage } from "./commissioning.js";
import "./advanced.css";
export function initAdvanced(kit) {
  let tab = "nvr",
    site = "",
    view = null,
    data = { documents: [], assets: [] },
    device = "",
    nvrSort = "id",
    nvrDescending = false,
    timer,
    lastEvent = 0,
    walkID = "",
    grades = {};
  const root = $("advanced-page"),
    tabs = {
      nvr: "NVR channels",
      events: "Event walk-test",
      retention: "Storage planner",
      plans: "Site plans",
      reports: "Checklist & reports",
      config: "Configuration",
      remote: "Remote discovery",
    };
  const docs = (kind) => data.documents.filter((d) => d.kind === kind);
  const parse = (d) => d?.data || {};
  const records = () => view?.devices.filter((r) => !r.removed) || [];
  const deviceName = (id) => {
    const r = view?.devices.find((r) => r.id === id);
    return r
      ? `${r.device.name || r.device.model || "Device"} · ${r.device.ipv4}`
      : id || "Unlinked";
  };
  const q = (id) => root.querySelector(`#ad-${id}`);
  const perform = async (fn) => {
    try {
      return await fn();
    } catch (e) {
      kit.error(e);
    }
  };
  const on = (id, fn) => {
    const el = q(id);
    if (el) el.onclick = () => void perform(fn);
  };
  const table = (headers, rows) =>
    `<div class="table-responsive"><table class="table table-sm align-middle"><thead><tr>${headers.map((x) => `<th>${esc(x)}</th>`).join("")}</tr></thead><tbody>${rows.join("") || `<tr><td colspan="${headers.length}" class="text-body-secondary">No saved results. Start an explicit read to collect evidence.</td></tr>`}</tbody></table></div>`;
  const button = (id, label, style = "outline-secondary") =>
    `<button id="ad-${id}" type="button" class="btn btn-sm btn-${style}">${esc(label)}</button>`;
  const requireSite = () => {
    if (!site) throw Error("Choose a saved site first.");
  };
  async function unlock() {
    const v = await API.VaultStatus();
    if (v.unlocked || v.automatic) return true;
    await kit.sites.unlock();
    return false;
  }
  async function refresh() {
    $("toolkit-error").hidden = true;
    await kit.sites.refresh();
    site = kit.sites.active()?.id || "";
    view = kit.sites.view();
    data = site
      ? await API.Advanced(site)
      : { documents: [], assets: [], checks: [] };
    if (!records().some((r) => r.id === device))
      device = kit.sites.selected()[0] || records()[0]?.id || "";
    await render();
  }
  async function save(kind, title, value, old = null, deviceID = "") {
    requireSite();
    const d = await API.SaveAdvanced({
      id: old?.id || "",
      siteID: site,
      deviceID,
      kind,
      title,
      version: 1,
      data: value,
      updated: old?.updated || "",
    });
    await refresh();
    return d;
  }
  function readOptions() {
    requireSite();
    if (!device) throw Error("Choose a saved device.");
    return {
      siteID: site,
      deviceID: device,
      pin: q("pin")?.value.trim() || "",
    };
  }
  function devicePicker() {
    return `<div class="advanced-device">${select(
      "Saved device",
      "ad-device",
      records()
        .map((r) => option(r.id, deviceName(r.id), device))
        .join(""),
    )}${field("Approved certificate SHA-256 (only if required)", "ad-pin", "", "text", "autocomplete=off")}</div><p class="small text-body-secondary">Authenticated reads use this device’s saved credential. Configure accounts in Sites. No credentials are sent to untrusted certificates.</p>`;
  }
  function wirePicker() {
    const el = root.querySelector('[name="ad-device"]');
    if (el) {
      el.id = "ad-device";
      el.onchange = () => {
        device = el.value;
        void perform(render);
      };
    }
    const pin = root.querySelector('[name="ad-pin"]');
    if (pin) pin.id = "ad-pin";
  }
  const ctx = {
    API,
    Viewer,
    kit,
    q,
    on,
    table,
    button,
    docs,
    parse,
    records,
    deviceName,
    perform,
    refresh,
    save,
    requireSite,
    unlock,
    get site() {
      return site;
    },
    get view() {
      return view;
    },
    get data() {
      return data;
    },
    get device() {
      return device;
    },
  };
  async function render() {
    clearInterval(timer);
    root.innerHTML = `<div class="d-flex justify-content-between flex-wrap gap-2"><h2>Commissioning tools</h2><div>${button("refresh", "Reload saved evidence")}</div></div><div class="d-flex align-items-end gap-3 mb-3">${select(
      "Site",
      "ad-site",
      option("", "Standalone storage planner", site) +
        (await API.ListSites())
          .filter((s) => !s.archived)
          .map((s) => option(s.id, s.name, site))
          .join(""),
    )}${button("credentials", "Saved credentials")}</div><nav class="advanced-tabs mb-3" aria-label="Commissioning tools">${Object.entries(
      tabs,
    )
      .map(
        ([id, name]) =>
          `<button type="button" class="btn btn-sm ${id === tab ? "btn-primary" : "btn-outline-secondary"}" data-tool="${id}">${name}</button>`,
      )
      .join("")}</nav><div id="ad-content"></div>`;
    root.querySelector('[name="ad-site"]').onchange = async (e) => {
      await kit.sites.choose(e.target.value);
      await refresh();
    };
    on("refresh", refresh);
    on("credentials", () => kit.navigate("sites"));
    for (const b of root.querySelectorAll("[data-tool]"))
      b.onclick = () => {
        tab = b.dataset.tool;
        void perform(render);
      };
    if (!site && tab !== "retention") {
      q("content").innerHTML =
        '<p class="text-body-secondary">Choose a saved site to collect and organise device evidence. Storage planning is also available without a site.</p>';
      return;
    }
    if (tab === "nvr") nvrPage();
    if (tab === "events") await eventPage();
    if (tab === "retention") await retentionPage(ctx);
    if (tab === "plans") await plansPage(ctx);
    if (tab === "reports") commissioningPage(ctx);
    if (tab === "config") configPage();
    if (tab === "remote") remotePage();
    wirePicker();
  }
  function nvrPage() {
    const invDoc = docs("nvr-channels").find((d) => d.deviceID === device),
      inv = parse(invDoc),
      channels = [...(inv.channels || [])].sort(
        (a, b) =>
          String(a[nvrSort] || "").localeCompare(
            String(b[nvrSort] || ""),
            undefined,
            { numeric: true },
          ) * (nvrDescending ? -1 : 1),
      );
    q("content").innerHTML =
      devicePicker() +
      `<div class="advanced-actions">${button("channels", "Read NVR channels", "primary")}${button("export", "Export saved channels")}${select("Sort", "ad-nvr-sort", ["id", "name", "address", "status", "model"].map((k) => option(k, k, nvrSort)).join(""))}${button("nvr-sort-direction", nvrDescending ? "Descending" : "Ascending")}</div><p class="small">${inv.at ? `Last read ${esc(date(inv.at))} · ${esc(inv.provider)}` : "No channel inventory read yet."} Internal PoE camera addresses are recorded observations; direct access is not assumed. Recording state is Unknown unless verified.</p>${(inv.warnings || []).map((w) => `<p class="text-warning">${esc(w)}</p>`).join("")}` +
      table(
        [
          "Channel",
          "Name / model",
          "Reported address",
          "Connection / recording",
          "Linked site camera",
          "Streams",
        ],
        channels.map(
          (ch) =>
            `<tr><td>${esc(ch.id)}</td><td>${esc(ch.name || "Unnamed")}<br><small>${esc(ch.model || "Unknown model")}</small></td><td>${esc(ch.address || "Unknown")}</td><td>${esc(ch.status)} / ${esc(ch.recording)}</td><td>${esc(deviceName(ch.linkedDeviceID))}<br><small>${esc(ch.confidence)}</small><br><button class="btn btn-sm btn-link" data-link="${esc(ch.id)}">Set link</button></td><td>${(ch.streams || []).map((st) => `<button class="btn btn-sm btn-outline-primary m-1" data-channel="${esc(ch.id)}" data-stream="${esc(st.id)}">Live ${esc(st.id)} · ${esc(st.codec || "?")}</button>`).join("") || "Mapping unavailable"}</td></tr>`,
        ),
      );
    on("channels", async () => {
      if (await unlock()) await API.StartNVR(readOptions());
    });
    on("export", () => {
      if (!invDoc) throw Error("Read channels first");
      dialog(
        "Export channel inventory",
        select(
          "Format",
          "format",
          ["json", "csv", "text"]
            .map((f) => option(f, f.toUpperCase()))
            .join(""),
        ),
        (v) => API.ExportEvidenceFormat(site, invDoc.id, v.format),
        "Export",
      );
    });
    q("content").querySelector('[name="ad-nvr-sort"]').onchange = (e) => {
      nvrSort = e.target.value;
      void perform(render);
    };
    on("nvr-sort-direction", () => {
      nvrDescending = !nvrDescending;
      return render();
    });
    for (const b of root.querySelectorAll("[data-stream]"))
      b.onclick = () =>
        void perform(() =>
          Viewer.OpenChannel(site, device, b.dataset.channel, b.dataset.stream),
        );
    for (const b of root.querySelectorAll("[data-link]"))
      b.onclick = () => {
        const old = docs("channel-links").find((d) => d.deviceID === device),
          links = { ...(parse(old).links || {}) };
        dialog(
          "Link NVR channel",
          `<p>This manual link is retained across channel scans. Match the serial/MAC where available; an IP address alone is not identity.</p>` +
            select(
              "Site camera",
              "linked",
              option("", "Unlinked", links[b.dataset.link]) +
                records()
                  .filter((r) => r.id !== device)
                  .map((r) =>
                    option(r.id, deviceName(r.id), links[b.dataset.link]),
                  )
                  .join(""),
            ),
          (v) => {
            links[b.dataset.link] = v.linked;
            return save(
              "channel-links",
              "NVR manual channel links",
              { links },
              old,
              device,
            );
          },
        );
      };
  }
  async function eventPage() {
    q("content").innerHTML =
      devicePicker() +
      `<div class="advanced-actions">${field("Duration (seconds)", "ad-duration", 60, "number", 'min="10" max="300"')}${button("walk", "Start walk-test", "primary")}${button("stop-walk", "Stop")}${check("Notify inside this app for new events", "ad-notify", false)}</div><div id="ad-walk-status" role="status"></div><div id="ad-event-list"></div><div id="ad-walk-save"></div><details class="mt-3"><summary>Saved walk-tests</summary>${
        docs("walk-test")
          .map(
            (d) =>
              `<p>${esc(date(d.updated))} · ${esc(deviceName(d.deviceID))} · ${esc(parse(d).result)} <button class="btn btn-sm btn-link" data-evidence="${esc(d.id)}">Export JSON</button></p>`,
          )
          .join("") || "No saved tests"
      }</details>`;
    on("walk", async () => {
      if (await unlock())
        await API.StartWalk(
          readOptions(),
          Number(root.querySelector('[name="ad-duration"]').value),
        );
    });
    on("stop-walk", async () => {
      if ((await API.Job()).kind === "walk-test") await API.CancelJob();
    });
    for (const b of root.querySelectorAll("[data-evidence]"))
      b.onclick = () =>
        void perform(() => API.ExportEvidence(site, b.dataset.evidence));
    const poll = async () => {
      if (root.hidden || tab !== "events") return;
      const w = await API.Walk();
      if (w.siteID !== site || !w.id) return;
      if (walkID !== w.id) {
        walkID = w.id;
        grades = {};
      }
      q("walk-status").textContent =
        `${w.running ? "Listening…" : "Stopped"} · ${w.total ? w.total + " events received" : "No event observed"} · ${w.error || w.cleanup || "Subscription active"}`;
      q("event-list").innerHTML =
        `<p class="small">${esc(w.capabilities.reason || "")} ${esc((w.capabilities.topics || []).join(" · "))}</p>` +
        table(
          [
            "Received / device time",
            "Type / original topic",
            "State",
            "Channel",
            "Clock",
            "Grade",
          ],
          (w.events || [])
            .slice(-100)
            .reverse()
            .map(
              (e) =>
                `<tr><td>${esc(date(e.received))}<br><small>${esc(date(e.deviceTime))}</small></td><td>${esc(e.type)}<br><small>${esc(e.topic)}</small></td><td>${esc(e.state)}</td><td>${esc(e.channel || "Unknown")}</td><td>${e.clockOffsetSeconds == null ? "Unknown" : `${Number(e.clockOffsetSeconds).toFixed(1)} s`}${e.clockWarning ? " ⚠ clock skew" : ""}</td><td><button type="button" class="btn btn-sm btn-link" data-event-grade="${esc(e.eventID)}" ${w.running ? "disabled" : ""}>${esc(grades[e.eventID]?.result || "Not tested")}</button></td></tr>`,
            ),
        );
      for (const b of q("event-list").querySelectorAll("[data-event-grade]"))
        b.onclick = () => {
          const old = grades[b.dataset.eventGrade] || {};
          dialog(
            "Grade observed event",
            select(
              "Result",
              "result",
              ["not-tested", "pass", "fail"]
                .map((x) => option(x, x, old.result))
                .join(""),
            ) +
              area("Expected behaviour / notes", "notes", old.notes || "") +
              select(
                "Optional snapshot",
                "snapshotID",
                option("", "None", old.snapshotID) +
                  data.assets
                    .filter(
                      (a) => a.kind === "snapshot" && a.deviceID === w.deviceID,
                    )
                    .map((a) => option(a.id, a.title, old.snapshotID))
                    .join(""),
              ),
            (v) => {
              grades[b.dataset.eventGrade] = {
                eventID: b.dataset.eventGrade,
                ...v,
              };
            },
          );
        };
      if (
        w.total > lastEvent &&
        root.querySelector('[name="ad-notify"]')?.checked
      ) {
        q("walk-status").classList.add("event-flash");
        setTimeout(
          () => q("walk-status")?.classList.remove("event-flash"),
          900,
        );
      }
      lastEvent = w.total;
      if (!w.running && !q("save-walk")) {
        q("walk-save").innerHTML = button(
          "save-walk",
          "Save graded summary",
          "primary",
        );
        on("save-walk", () =>
          dialog(
            "Save walk-test evidence",
            select(
              "Verdict",
              "result",
              ["not-tested", "pass", "fail"].map((x) => option(x, x)).join(""),
            ) +
              area("Test notes / expected action", "notes") +
              select(
                "Optional saved snapshot",
                "snapshot",
                option("", "None") +
                  data.assets
                    .filter(
                      (a) => a.kind === "snapshot" && a.deviceID === w.deviceID,
                    )
                    .map((a) => option(a.id, a.title))
                    .join(""),
              ),
            async (v) => {
              await API.SaveWalk(
                w.id,
                v.result,
                v.notes,
                v.snapshot ? [v.snapshot] : [],
                Object.values(grades),
              );
              await refresh();
            },
          ),
        );
      }
    };
    await poll();
    timer = setInterval(() => void perform(poll), 1000);
  }
  function configPage() {
    const backups = docs("configuration");
    q("content").innerHTML =
      devicePicker() +
      `<div class="advanced-actions">${button("capture-config", "Capture encrypted settings", "primary")}${button("import-config", "Import encrypted package")}</div><p class="small">Documented ONVIF settings are captured with unsupported/unreadable sections identified. Full vendor export is not assumed. Configuration restore is disabled pending approved model/firmware hardware tests.</p>` +
      table(
        ["Date / device", "Provider / model", "Sections", "Actions"],
        backups.map(
          (d) =>
            `<tr><td>${esc(date(d.updated))}<br>${esc(deviceName(d.deviceID))}</td><td>${esc(parse(d).provider)}<br>${esc(parse(d).model)} ${esc(parse(d).firmware)}</td><td>${(parse(d).sections || []).map((s) => `${esc(s.key)}: ${esc(s.state)}`).join("<br>")}</td><td><button class="btn btn-sm btn-link" data-config-export="${esc(d.id)}">Encrypted export</button><button class="btn btn-sm btn-link" data-config-plan="${esc(d.id)}">Preview restore / clone</button></td></tr>`,
        ),
      ) +
      `<div class="advanced-device">${select("Earlier backup", "ad-left", backups.map((d) => option(d.id, `${date(d.updated)} · ${deviceName(d.deviceID)}`)).join(""))}${select("Later / freshly captured backup", "ad-right", backups.map((d) => option(d.id, `${date(d.updated)} · ${deviceName(d.deviceID)}`)).join(""))}</div>${button("compare-config", "Compare settings")}<div id="ad-config-diff"></div>`;
    on("capture-config", async () => {
      if (await unlock()) await API.StartConfiguration(readOptions());
    });
    const pass = () =>
      field(
        "Backup passphrase (12+ characters)",
        "passphrase",
        "",
        "password",
        "required minlength=12",
      );
    on("import-config", () =>
      dialog("Import encrypted configuration", pass(), async (v) => {
        if (!device)
          throw Error("Choose a target device for this imported backup");
        await API.ImportConfiguration(site, device, v.passphrase);
        await refresh();
      }),
    );
    for (const b of root.querySelectorAll("[data-config-export]"))
      b.onclick = () =>
        dialog("Export encrypted configuration", pass(), (v) =>
          API.ExportConfiguration(site, b.dataset.configExport, v.passphrase),
        );
    for (const b of root.querySelectorAll("[data-config-plan]"))
      b.onclick = () =>
        void perform(async () => {
          const p = await API.ConfigurationPlan(
            site,
            b.dataset.configPlan,
            device,
          );
          message(
            "Restore / clone preview — no writes",
            `Target: ${deviceName(device)}\nCompatible: ${p.compatible ? "same model and firmware" : "No"}\n${p.reasons.join("\n")}\n\nPortable candidate settings:\n${p.settings.map((x) => `${x.key}: ${x.value}`).join("\n")}\n\nExcluded:\n${p.excluded.map((x) => x.key).join("\n")}`,
          );
        });
    on("compare-config", async () => {
      const diffs = await API.CompareConfiguration(
        site,
        root.querySelector('[name="ad-left"]').value,
        root.querySelector('[name="ad-right"]').value,
      );
      q("config-diff").innerHTML = table(
        ["Setting", "Change", "Before", "After"],
        diffs.map(
          (x) =>
            `<tr>${[x.key, x.kind, x.before, x.after].map((v) => `<td>${esc(v)}</td>`).join("")}</tr>`,
        ),
      );
    });
  }
  function remotePage() {
    const targets = docs("ssh-target"),
      observations = docs("remote-observation");
    q("content").innerHTML =
      `<p>Run the SADPTools CLI on an explicitly trusted SSH host at this site. Discovery runs on that host’s adapters. No remote device writes or permanent service.</p>${button("new-ssh", "Add SSH target", "primary")}` +
      table(
        ["Host / CLI", "Scope / trust", "Actions"],
        targets.map((d) => {
          const t = parse(d);
          return `<tr><td>${esc(t.username)}@${esc(t.host)}:${esc(t.port)}<br>${esc(t.executable)}</td><td>${esc(t.interface || "All suitable remote adapters")} · ${esc(t.protocols)}<br><small>${esc(t.fingerprint || t.knownHosts)}</small></td><td><button class="btn btn-sm btn-link" data-ssh-edit="${esc(d.id)}">Edit</button><button class="btn btn-sm btn-outline-primary" data-ssh-run="${esc(d.id)}">Discover remotely</button></td></tr>`;
        }),
      ) +
      `<h3>Remote observations</h3>` +
      observations
        .map((doc) => {
          const ob = parse(doc);
          return `<details class="mb-3"><summary>${esc(ob.target?.host)} · ${esc(date(ob.at))} · ${ob.report?.devices?.length || 0} devices · SSH ${esc(ob.latencyMS)} ms${ob.report?.partial ? " · partial scan" : ""}</summary><p>${esc(ob.diagnostic || "")} ${esc((ob.report?.warnings || []).join(" · "))}</p>${table(
            ["Import", "Device", "Identity", "Remote adapter"],
            (ob.report?.devices || []).map(
              (d) =>
                `<tr><td><input type="checkbox" data-observation="${esc(doc.id)}" value="${esc(d.id)}" aria-label="Select remote device"></td><td>${esc(d.ipv4)} · ${esc(d.model)}</td><td>${esc(d.mac)}<br>${esc(d.serial)}</td><td>${esc((d.interfaces || []).map((a) => a.name).join(", "))}</td></tr>`,
            ),
          )}<button class="btn btn-sm btn-primary" data-import="${esc(doc.id)}">Import selected into ${esc(view.site.name)}</button></details>`;
        })
        .join("");
    const edit = (old) => {
      const t = parse(old);
      dialog(
        "Trusted SSH discovery target",
        field("Label", "title", old?.title || "Remote discovery") +
          field(
            "Host / VPN address",
            "host",
            t.host || "",
            "text",
            "required",
          ) +
          field(
            "SSH port",
            "port",
            t.port || 22,
            "number",
            "min=1 max=65535 required",
          ) +
          field(
            "SSH username",
            "username",
            t.username || "",
            "text",
            "required",
          ) +
          field(
            "Approved SHA256 host-key fingerprint",
            "fingerprint",
            t.fingerprint || "",
          ) +
          field(
            "Or known_hosts file on this PC",
            "knownHosts",
            t.knownHosts || "",
          ) +
          `<p class="small">Verify the host key through a trusted channel. Keys are never accepted silently.</p>` +
          field(
            "Identity file on this PC (optional)",
            "keyPath",
            t.keyPath || "",
          ) +
          check("Use SSH_AUTH_SOCK agent when available", "agent", t.agent) +
          select(
            "Vault password / key passphrase profile",
            "credentialID",
            option("", "None", t.credentialID) +
              (view.profiles || [])
                .map((p) => option(p.id, p.label, t.credentialID))
                .join(""),
          ) +
          `<p class="small">Private keys stay under your control. An encrypted key uses the profile’s privacy-password field as its passphrase; a login password uses its password field.</p>` +
          field(
            "Remote CLI executable",
            "executable",
            t.executable || "sadptools",
            "text",
            "required",
          ) +
          field(
            "Remote adapter (blank = all suitable)",
            "interface",
            t.interface || "",
          ) +
          select(
            "Discovery protocols",
            "protocols",
            ["sadp", "onvif", "sadp,onvif"]
              .map((p) => option(p, p, t.protocols || "sadp,onvif"))
              .join(""),
          ) +
          field(
            "Timeout seconds",
            "timeout",
            t.timeout || 10,
            "number",
            "min=1 max=300",
          ),
        (v) =>
          save(
            "ssh-target",
            v.title,
            {
              ...v,
              port: Number(v.port),
              timeout: Number(v.timeout),
              agent: v.agent === "on",
            },
            old,
          ),
      );
    };
    on("new-ssh", () => edit());
    for (const b of root.querySelectorAll("[data-ssh-edit]"))
      b.onclick = () => edit(targets.find((d) => d.id === b.dataset.sshEdit));
    for (const b of root.querySelectorAll("[data-ssh-run]"))
      b.onclick = () =>
        void perform(async () => {
          const d = targets.find((d) => d.id === b.dataset.sshRun);
          if (parse(d).credentialID && !(await unlock())) return;
          await API.StartRemote(site, d.id);
        });
    for (const b of root.querySelectorAll("[data-import]"))
      b.onclick = () => {
        const ids = [
          ...root.querySelectorAll(
            `input[data-observation="${b.dataset.import}"]:checked`,
          ),
        ].map((x) => x.value);
        dialog(
          "Import remote observations",
          `<p>Import ${ids.length} selected devices into <strong>${esc(view.site.name)}</strong>? Stable identifiers are reconciled within this site only. Conflicting identities require review.</p>`,
          async () => {
            const r = await API.ImportRemote(site, b.dataset.import, ids);
            await refresh();
            message("Remote import", JSON.stringify(r, null, 2));
          },
          "Import selected",
        );
      };
  }
  kit.pages.advanced = refresh;
  return {
    refresh,
    completed: async () => {
      if (!root.hidden) await refresh();
    },
  };
}
