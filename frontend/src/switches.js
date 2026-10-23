import * as API from "../bindings/github.com/TerminalAddict/SADPTools/toolkitservice.js";
import {
  $,
  esc,
  date,
  dialog,
  field,
  area,
  select,
  option,
  check,
  table,
  message,
} from "./toolkit-ui.js";
const watts = (v) =>
  v == null ? "Unknown/unsupported" : `${Number(v).toFixed(3)} W`;
const speed = (v) => (v == null ? "Unknown" : `${v} Mbps`);
export function mappingTable(reports, mac = "") {
  return table(
    [
      "Camera",
      "Switch → candidate port",
      "Confidence",
      "VLAN / link",
      "Speed / PoE",
      "Evidence / polled",
    ],
    reports
      .flatMap((r) => r.mappings || [])
      .filter((m) => !mac || m.mac === mac)
      .map(
        (m) =>
          `<tr><td>${esc(m.ip)}<br><small>${esc(m.mac)}</small></td><td>${esc(m.switchName)} → ${esc(m.portName || "Unmapped")}${m.port ? ` <small>(ifIndex ${m.port})</small>` : ""}</td><td><strong>${esc(m.confidence)}</strong></td><td>${esc(m.vlans?.join(", ") || "Unknown")} · ${esc(m.link || "Unknown")}</td><td>${esc(speed(m.speedMbps))}<br>${esc(m.poe || "Unknown/unsupported")} · ${esc(watts(m.powerWatts))}</td><td>${esc(m.evidence?.join("; "))}<br><small>${esc(date(m.at))}</small></td></tr>`,
      ),
  );
}
export function initSwitches(kit) {
  let assets = [],
    reports = [],
    selected = "",
    lastJob = "",
    refreshing = false;
  const perform = (fn) => Promise.resolve().then(fn).catch(kit.error);
  const site = () => kit.sites.active();
  async function render() {
    if (!site()) {
      $("switches-page").innerHTML =
        '<div class="tool-empty"><h2>Switch / PoE mapping</h2><p>Select a saved site to register managed switches and securely assign read-only SNMP credentials.</p></div>';
      return;
    }
    assets = await API.SwitchAssets(site().id);
    reports = await API.SwitchReports(site().id);
    if (!assets.some((a) => a.id === selected)) selected = assets[0]?.id || "";
    $("switches-page").innerHTML =
      `<div class="tool-heading"><div><h2>Switch / PoE mapping — ${esc(site().name)}</h2><p>Read-only SNMP observations. Forwarding entries can be stale; uplink, trunk and multiple-MAC evidence prevents a direct-port claim.</p></div><button id="switch-add" class="btn btn-primary">Register switch</button></div><div class="tool-actions"><select id="switch-select" class="form-select w-auto">${assets.map((a) => option(a.id, `${a.name} · ${a.host} · SNMPv${a.version}`, selected)).join("") || option("", "No switches registered")}</select><button id="switch-edit" class="btn btn-outline-secondary" ${selected ? "" : "disabled"}>Edit asset</button><button id="switch-poll" class="btn btn-primary" ${selected ? "" : "disabled"}>Review and poll…</button><button id="switch-export" class="btn btn-outline-secondary">Export site port map</button><button id="switch-delete" class="btn btn-outline-danger" ${selected ? "" : "disabled"}>Remove asset</button></div><div id="switch-result"></div>`;
    $("switch-add").onclick = () => assetForm();
    $("switch-edit").onclick = () =>
      assetForm(assets.find((a) => a.id === selected));
    $("switch-select").onchange = (e) => {
      selected = e.target.value;
      draw();
    };
    $("switch-poll").onclick = () => approve();
    $("switch-export").onclick = () =>
      dialog(
        "Export switch evidence",
        select(
          "Format",
          "format",
          option("html", "HTML report") +
            option("csv", "CSV") +
            option("json", "JSON"),
        ),
        (v) => API.ExportSwitches(site().id, v.format),
        "Export",
      );
    $("switch-delete").onclick = () => {
      const a = assets.find((a) => a.id === selected);
      dialog(
        "Remove switch asset",
        `<p>Saved poll reports remain in site history. This removes only the asset registration; it does not change the switch.</p>` +
          field(
            `Type ${a.name} to confirm`,
            "confirmation",
            "",
            "text",
            "required",
          ),
        async (v) => {
          await API.DeleteSwitch(a.siteID, a.id, v.confirmation);
          selected = "";
          await render();
        },
        "Remove asset",
      );
    };
    draw();
  }
  function assetForm(
    a = {
      siteID: site().id,
      name: "",
      host: "",
      version: "3",
      authProtocol: "sha256",
      port: 161,
      uplinks: [],
      portLinks: [],
    },
  ) {
    const view = kit.sites.view();
    dialog(
      a.id ? "Edit switch asset" : "Register managed switch",
      field("Name", "name", a.name, "text", "required") +
        field("IPv4 address or hostname", "host", a.host, "text", "required") +
        field("Vendor", "vendor", a.vendor) +
        field("Model", "model", a.model) +
        field("Physical location", "location", a.location) +
        area("Notes", "notes", a.notes) +
        select(
          "SNMP version",
          "version",
          option("3", "SNMPv3 authPriv (recommended)", a.version) +
            option("2c", "SNMPv2c (unencrypted community)", a.version),
        ) +
        select(
          "SNMPv3 authentication / AES-128 privacy",
          "authProtocol",
          option("sha256", "SHA-256", a.authProtocol) +
            option("sha512", "SHA-512", a.authProtocol) +
            option(
              "sha1",
              "Legacy SHA-1 (only if required by switch)",
              a.authProtocol,
            ),
        ) +
        select(
          "Vault credential profile",
          "credentialID",
          option("", "Choose a profile", a.credentialID) +
            (view?.profiles || [])
              .map((p) =>
                option(p.id, `${p.label} · ${p.username}`, a.credentialID),
              )
              .join(""),
        ) +
        `<p class="small">Create profiles under Sites → Credentials. SNMPv3 uses username, authentication password and privacy password. SNMPv2c uses the password field as the community. Configure a read-only account/view on the switch.</p>` +
        field(
          "UDP port",
          "port",
          a.port,
          "number",
          "required min=1 max=65535",
        ) +
        field(
          "SNMP context name (optional, as configured on switch)",
          "contextName",
          a.contextName,
        ) +
        field(
          "Known uplink ifIndex values (comma separated)",
          "uplinks",
          a.uplinks?.join(", "),
        ) +
        check(
          "Explicitly allow SNMPv2c: community and observations travel without encryption",
          "allowV2",
          a.allowV2,
        ),
      async (v) => {
        const saved = await API.SaveSwitch({
          ...a,
          ...v,
          port: Number(v.port),
          uplinks: v.uplinks
            .split(",")
            .map((s) => s.trim())
            .filter(Boolean)
            .map(Number),
          allowV2: !!v.allowV2,
        });
        selected = saved.id;
        await render();
      },
    );
  }
  function approve() {
    const a = assets.find((a) => a.id === selected);
    dialog(
      "Approve read-only switch poll",
      `<dl><dt>Site / switch</dt><dd>${esc(site().name)} · ${esc(a.name)}</dd><dt>Target</dt><dd>${esc(a.host)} : ${a.port} / UDP</dd><dt>Security</dt><dd>SNMPv${esc(a.version)}${a.version === "3" ? ` · ${esc(a.authProtocol)} / AES-128 privacy` : " · Unencrypted community"}</dd></dl><p>Only fixed system/interface, forwarding, VLAN, LLDP and supported PoE objects are read. No SNMP SET operations are sent. Unlock the vault first.</p>` +
        check(
          "I am connected to the intended site and approve this switch target",
          "approved",
        ),
      async (v) => {
        if (!v.approved) throw Error("Confirm the target first.");
        await API.PollSwitch(a.siteID, a.id);
        await kit.poll();
      },
      "Poll switch",
    );
  }
  function draw() {
    if (!$("switch-result")) return;
    const a = assets.find((a) => a.id === selected),
      r = reports.find((r) => r.assetID === selected);
    if (!r) {
      $("switch-result").innerHTML =
        '<p class="text-body-secondary">Poll this switch to obtain timestamped observations. No switch or camera port is inferred before a response.</p>';
      return;
    }
    $("switch-result").innerHTML =
      `<p><strong>${esc(r.systemName || r.switchName)}</strong> · ${esc(r.address)} · Last polled ${esc(date(r.at))}${r.cancelled ? " · Cancelled / partial" : ""}</p><p>${esc(r.description)}</p>${r.warnings?.length ? `<div class="alert alert-warning">${r.warnings.map(esc).join("<br>")}</div>` : ""}<h3>Camera → switch → candidate port</h3>${mappingTable([r])}<h3 class="mt-4">Interfaces</h3>${table(
        [
          "ifIndex",
          "Name / label",
          "Admin / link",
          "Speed",
          "VLANs",
          "Topology / PoE",
          "Annotations",
        ],
        (r.ports || []).map(
          (p) =>
            `<tr><td>${p.index}</td><td>${esc(p.name)}<br><small>${esc(p.alias || p.description)}</small></td><td>${esc(p.admin)} / ${esc(p.operational)}</td><td>${esc(speed(p.speedMbps))}</td><td>${esc(p.vlans?.join(", ") || "Unknown")}</td><td>${p.uplink ? "Uplink/trunk evidence" : "No uplink evidence"}<br>${esc(p.poe)} · ${esc(watts(p.powerWatts))}<br><small>${esc(p.poeSource)}</small></td><td><button class="btn btn-sm btn-outline-secondary" data-port="${p.index}">Map / annotate</button></td></tr>`,
        ),
      )}<details class="mt-3"><summary>LLDP neighbours and PoE MIB indices</summary>${table(
        [
          "LLDP local port",
          "Mapped ifIndex",
          "Neighbour",
          "Chassis MAC",
          "Remote port",
          "Infrastructure",
        ],
        (r.neighbours || []).map(
          (n) =>
            `<tr><td>${n.localPort}</td><td>${n.ifIndex || "Unmapped"}</td><td>${esc(n.name)}</td><td>${esc(n.chassisMAC)}</td><td>${esc(n.portID)}</td><td>${n.infrastructure ? "Yes" : "Not advertised"}</td></tr>`,
        ),
      )}${table(
        ["PoE group.port", "Mapped ifIndex", "State", "Consumption", "Source"],
        (r.power || []).map(
          (p) =>
            `<tr><td>${p.group}.${p.port}</td><td>${p.ifIndex || "Unmapped"}</td><td>${esc(p.state)}</td><td>${esc(watts(p.watts))}</td><td>${esc(p.source)}</td></tr>`,
        ),
      )}<p class="small">PoE port indices need not equal ifIndex. Unknown values are not zero. Manual mappings take effect on the next poll.</p></details>`;
    for (const b of $("switch-result").querySelectorAll("[data-port]"))
      b.onclick = () => annotate(a, r, Number(b.dataset.port));
  }
  function annotate(a, r, index) {
    const existing = a.portLinks?.find((p) => p.ifIndex === index);
    dialog(
      `Annotate interface ${index}`,
      `<p>These annotations belong to the saved site; they do not configure the switch. Only bind MIB indices after checking the switch's documentation and physical port identity.</p>` +
        check(
          "This is a known uplink / trunk",
          "uplink",
          a.uplinks?.includes(index),
        ) +
        select(
          "PoE group.port association",
          "poe",
          option("", "No manual association") +
            (r.power || [])
              .map((p) =>
                option(
                  `${p.group}.${p.port}`,
                  `${p.group}.${p.port} · ${p.state}`,
                  existing ? `${existing.poeGroup}.${existing.poePort}` : "",
                ),
              )
              .join(""),
        ) +
        field(
          "LLDP local port index (0 = automatic mapping)",
          "lldp",
          existing?.lldpLocal || 0,
          "number",
          "min=0",
        ),
      async (v) => {
        const uplinks = new Set(a.uplinks || []);
        v.uplink ? uplinks.add(index) : uplinks.delete(index);
        const links = (a.portLinks || []).filter((p) => p.ifIndex !== index);
        const [group = 0, port = 0] = v.poe
          .split(".")
          .filter(Boolean)
          .map(Number);
        if (group || port || Number(v.lldp))
          links.push({
            ifIndex: index,
            poeGroup: group,
            poePort: port,
            lldpLocal: Number(v.lldp),
          });
        await API.SaveSwitch({ ...a, uplinks: [...uplinks], portLinks: links });
        await render();
      },
      "Save annotation",
    );
  }
  async function refresh() {
    if (refreshing || !site() || !$("switch-result")) return;
    refreshing = true;
    try {
      const job = await API.Job();
      $("switch-poll").disabled = job.running || !selected;
      if (job.completed && job.completed !== lastJob) {
        lastJob = job.completed;
        reports = await API.SwitchReports(site().id);
        draw();
      }
    } catch (e) {
      kit.error(e);
    } finally {
      refreshing = false;
    }
  }
  kit.pages.switches = render;
  setInterval(() => {
    if (!$("switches-page").hidden) void refresh();
  }, 1000);
  return { render };
}
