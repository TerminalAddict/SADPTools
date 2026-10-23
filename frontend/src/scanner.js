import * as API from "../bindings/github.com/TerminalAddict/SADPTools/toolkitservice.js";
import * as Desktop from "../bindings/github.com/TerminalAddict/SADPTools/desktopservice.js";
import {
  $,
  esc,
  option,
  dialog,
  field,
  select,
  check,
  message,
  contextMenu,
  table,
  date,
} from "./toolkit-ui.js";
export function initScanner(kit) {
  let adapters = [],
    options = {
      interface: "",
      cidr: "",
      start: "",
      end: "",
      timeoutMS: 500,
      workers: 24,
      rate: 60,
      tcpPorts: [],
      icmp: true,
      confirmScope: false,
    },
    page = 0,
    mode = "grid",
    filter = "",
    sort = "ip",
    descending = false,
    result = null,
    initialized = false,
    polling = false;
  const perform = async (fn) => {
    try {
      return await fn();
    } catch (e) {
      kit.error(e);
    }
  };
  function read() {
    return {
      ...options,
      interface: $("ip-adapter").value,
      cidr: $("ip-cidr").value.trim(),
      start: $("ip-cidr").value.trim() ? "" : $("ip-start").value.trim(),
      end: $("ip-cidr").value.trim() ? "" : $("ip-end").value.trim(),
      timeoutMS: Number($("ip-timeout").value),
      workers: Number($("ip-workers").value),
      rate: Number($("ip-rate").value),
      icmp: $("ip-icmp").checked,
      tcpPorts: $("ip-ports")
        .value.split(",")
        .map((s) => s.trim())
        .filter(Boolean)
        .map(Number),
      confirmScope: false,
    };
  }
  async function scope() {
    options = read();
    const s = await API.IPScope(options);
    $("ip-start").value = s.start;
    $("ip-end").value = s.end;
    $("ip-scope").textContent =
      `${s.start} – ${s.end} · ${s.total} addresses · Source ${s.source} · Gateway ${s.gateway || "unknown"}${s.routed ? " · Routed / outside adapter subnet" : " · On selected adapter subnet"}${s.public ? " · Public address space" : ""}`;
    return s;
  }
  async function start() {
    const s = await scope();
    const launch = async () => {
      await API.StartIPScan(kit.sites.active()?.id || "", {
        ...options,
        confirmScope: true,
      });
      page = 0;
      await refresh();
    };
    dialog(
      "Confirm IP investigation",
      `<p><strong>${esc(s.start)} – ${esc(s.end)}</strong> (${s.total} addresses) through ${esc(s.interface)}.</p><p>${s.needsConfirmation ? "This scope is public, routed or larger than 1024 addresses. Confirm that it is authorised before proceeding." : "Only scan a network you are authorised to manage."}</p><p>ICMP: ${options.icmp ? "enabled" : "disabled"}; TCP ports: ${esc(options.tcpPorts.join(", ") || "disabled")}. No response does not mean an address is free.</p>` +
        check("I am authorised to investigate this exact range", "approved"),
      async (v) => {
        if (!v.approved) throw Error("Confirm the scan scope first.");
        await launch();
      },
      "Start scan",
    );
  }
  function details(h) {
    const ev = h.evidence
      .map(
        (e) =>
          `${e.probe} · ${e.state}${e.latencyMS != null ? ` · ${e.latencyMS.toFixed(2)} ms` : ""}\n${e.detail || ""} · ${date(e.at)}`,
      )
      .join("\n\n");
    const d = dialog(
      `IP ${h.ip}`,
      `<p><strong>${esc(h.status)}</strong> · ${h.known ? "Known assignment/evidence" : "No saved assignment"}${h.responding ? " · Responding" : ""}</p><p>${esc(h.reserved)} ${h.conflict ? "Contradictory MAC evidence — review ownership." : ""}</p><p>MAC evidence: ${esc(h.macs?.join(", ") || "Unknown")}<br>Device matches: ${esc(h.matches?.join(", ") || "None")}<br>Hostname: ${esc(h.hostname || "Not queried")}</p><pre class="tool-readable">${esc(ev || "Not yet probed")}</pre><p class="small">No response is inconclusive. Verify again before assigning an address.</p><div class="tool-actions"><button type="button" data-host-action="copy" class="btn btn-outline-secondary">Copy IP</button><button type="button" data-host-action="ping" class="btn btn-outline-secondary">Ping again</button><button type="button" data-host-action="web" class="btn btn-outline-secondary">Open web interface</button><button type="button" data-host-action="ports" class="btn btn-outline-secondary">Inspect services</button><button type="button" data-host-action="dns" class="btn btn-outline-secondary">Resolve hostname</button><button type="button" data-host-action="reserve" class="btn btn-outline-secondary">Reserve IP</button><button type="button" data-host-action="associate" class="btn btn-outline-secondary">Associate saved device</button><button type="button" data-host-action="assign" class="btn btn-outline-secondary">Plan assignment</button></div>`,
      async () => {},
      "Close",
    );
    d.querySelector(".close-dialog").hidden = true;
    for (const b of d.querySelectorAll("[data-host-action]"))
      b.onclick = () =>
        perform(async () => {
          const site = kit.sites.active()?.id || "";
          switch (b.dataset.hostAction) {
            case "copy":
              await API.CopyText(h.ip);
              break;
            case "ping":
              await API.ReprobeIP(site, h.ip, []);
              d.close();
              d.remove();
              break;
            case "web":
              await API.OpenIP(h.ip, 0);
              break;
            case "dns":
              message("Reverse DNS", (await API.ResolveIP(h.ip)).join("\n"));
              break;
            case "ports":
              dialog(
                "Inspect TCP services",
                `<p>One TCP connect per listed port on ${esc(h.ip)}; no authentication or device configuration.</p>` +
                  field("Ports (maximum 8)", "ports", "80,443,554,8000"),
                async (v) => {
                  await API.ReprobeIP(
                    site,
                    h.ip,
                    v.ports.split(",").map(Number),
                  );
                  d.close();
                  d.remove();
                },
                "Probe ports",
              );
              break;
            case "reserve":
              if (!site) throw Error("Select a saved site first.");
              dialog(
                "Reserve address",
                field("IP", "ip", h.ip, "text", "readonly") +
                  field("Reason / intended use", "note") +
                  check("Remove existing reservation", "remove"),
                async (v) => {
                  await API.ReserveIP(site, h.ip, v.note, !!v.remove);
                  await kit.sites.refresh();
                  await refresh();
                },
                "Save reservation",
              );
              break;
            case "associate":
              if (!site) throw Error("Select a saved site first.");
              {
                const v = await API.Site(site);
                dialog(
                  "Associate saved inventory address",
                  `<p>This updates the saved inventory only. It does not change the camera. Confirm the device identity and address ownership.</p>` +
                    select(
                      "Saved device",
                      "record",
                      v.devices
                        .map((r) =>
                          option(
                            r.id,
                            `${r.device.model} · ${r.device.mac || r.device.serial} · ${r.device.ipv4}`,
                          ),
                        )
                        .join(""),
                    ),
                  async (values) => {
                    const r = v.devices.find((r) => r.id === values.record);
                    await API.SaveSiteDevice({
                      ...r,
                      device: { ...r.device, ipv4: h.ip },
                      manualFields: [
                        ...new Set([...(r.manualFields || []), "ipv4"]),
                      ],
                    });
                    await kit.sites.refresh();
                  },
                  "Associate address",
                );
              }
              break;
            case "assign":
              d.close();
              d.remove();
              document.dispatchEvent(
                new CustomEvent("toolkit:assign", { detail: h.ip }),
              );
              break;
          }
        });
  }
  async function refresh() {
    if (polling || !$("ip-results")) return;
    polling = true;
    try {
      const size = mode === "grid" ? 200 : 100;
      result = await API.IPScanPage(
        page * size,
        size,
        filter,
        sort,
        descending,
      );
      $("ip-progress").style.width =
        `${result.scope.total ? (result.completed / result.scope.total) * 100 : 0}%`;
      $("ip-count").textContent =
        `${result.completed} / ${result.scope.total || 0} processed${result.finished ? ` · ${result.cancelled ? "Cancelled" : "Complete"}` : ""}`;
      $("ip-duration").textContent = result.started
        ? `${Math.max(0, ((result.finished ? Date.parse(result.finished) : Date.now()) - Date.parse(result.started)) / 1000).toFixed(1)} s`
        : "";
      $("ip-warnings").textContent = (result.warnings || []).join(" ");
      const hosts = result.hosts || [];
      const compact = result.scope.cidr?.endsWith("/24");
      $("ip-results").innerHTML =
        mode === "grid"
          ? `<div class="ip-grid">${hosts.map((h) => `<button class="ip-cell ip-${esc(h.status)}" data-ip="${esc(h.ip)}" title="${esc(h.ip)} · ${esc(h.status)}${h.known ? " · Known" : ""}${h.responding ? " · Responding" : ""}"><strong>${esc(compact ? h.ip.split(".").at(-1) : h.ip)}</strong><small>${esc(h.status)}${h.known && h.status !== "known" ? " · saved/cache" : ""}</small></button>`).join("")}</div>`
          : table(
              [
                "IP",
                "Status",
                "Known",
                "Responding",
                "MAC evidence",
                "Hostname",
                "Probe evidence",
              ],
              hosts.map(
                (h) =>
                  `<tr data-ip="${esc(h.ip)}" tabindex="0"><td>${esc(h.ip)}</td><td>${esc(h.status)}</td><td>${h.known ? "Yes" : "—"}</td><td>${h.responding ? "Yes" : "—"}</td><td>${esc(h.macs.join(", "))}</td><td>${esc(h.hostname)}</td><td>${esc(h.evidence.map((e) => e.probe + ": " + e.state).join("; "))}</td></tr>`,
              ),
            );
      for (const b of $("ip-results").querySelectorAll("[data-ip]")) {
        const h = hosts.find((h) => h.ip === b.dataset.ip);
        b.onclick = () => details(h);
        b.onkeydown = (e) => {
          if (e.key === "Enter") details(h);
        };
        b.oncontextmenu = (e) =>
          contextMenu(e, [
            ["IP details", () => details(h)],
            ["Copy IP", () => perform(() => API.CopyText(h.ip))],
            [
              "Ping again",
              () =>
                perform(() =>
                  API.ReprobeIP(kit.sites.active()?.id || "", h.ip, []),
                ),
            ],
          ]);
      }
      $("ip-page-label").textContent =
        `${result.matching ? `${page * size + 1}–${Math.min((page + 1) * size, result.matching)}` : "0"} of ${result.matching} matching addresses`;
      $("ip-prev").disabled = page === 0;
      $("ip-next").disabled = (page + 1) * size >= result.matching;
      const job = await API.Job();
      $("ip-start-scan").disabled = job.running;
      $("ip-rescan").disabled = job.running;
      $("ip-stop").disabled = !job.running;
      $("ip-reset").disabled = job.running;
      $("ip-save").disabled =
        job.running || !result.started || !kit.sites.active();
      $("ip-export").disabled = !result.started;
    } catch (e) {
      kit.error(e);
    } finally {
      polling = false;
    }
  }
  async function render() {
    if (!initialized) {
      adapters = await Desktop.Interfaces();
      const settings = await Desktop.GetSettings();
      const saved = (await API.Preferences()).scanner;
      if (
        saved?.interface &&
        adapters.some((a) => a.name === saved.interface && a.unicastSuitable)
      )
        options = { ...options, ...saved, confirmScope: false };
      options.interface =
        options.interface ||
        settings.interface ||
        adapters.find((a) => a.unicastSuitable)?.name ||
        "";
      if (!options.cidr && !options.start && !options.end)
        options.cidr =
          adapters.find((a) => a.name === options.interface)?.addresses[0] ||
          "";
      initialized = true;
    }
    $("scanner-page").innerHTML =
      `<div class="tool-heading"><div><h2>IP scanner</h2><p>Investigate a defined IPv4 range. No response is not a guarantee that an IP is free.</p></div></div><div class="ip-controls"><label>Network adapter<select id="ip-adapter" class="form-select">${adapters
        .filter((a) => a.unicastSuitable)
        .map((a) =>
          option(
            a.name,
            `${a.name} · ${a.addresses.join(", ")}`,
            options.interface,
          ),
        )
        .join(
          "",
        )}</select></label><label>CIDR<input id="ip-cidr" class="form-control" value="${esc(options.cidr)}"></label><label>Start IP<input id="ip-start" class="form-control" value="${esc(options.start)}"></label><label>End IP<input id="ip-end" class="form-control" value="${esc(options.end)}"></label><button id="ip-calculate" class="btn btn-outline-secondary">Calculate scope</button></div><p id="ip-scope" class="small text-body-secondary"></p><details class="mb-3"><summary>Probe settings</summary><div class="ip-controls mt-2"><label><input id="ip-icmp" type="checkbox" ${options.icmp ? "checked" : ""}> ICMP echo</label><label>TCP ports (empty = off)<input id="ip-ports" class="form-control" value="${esc(options.tcpPorts.join(","))}"></label><label>Timeout ms<input id="ip-timeout" class="form-control" type="number" min="100" max="3000" value="${options.timeoutMS}"></label><label>Workers<input id="ip-workers" class="form-control" type="number" min="1" max="64" value="${options.workers}"></label><label>Hosts / second<input id="ip-rate" class="form-control" type="number" min="1" max="200" value="${options.rate}"></label></div></details><div class="tool-actions"><button id="ip-start-scan" class="btn btn-primary">Start scan</button><button id="ip-stop" class="btn btn-outline-secondary">Stop / Cancel</button><button id="ip-rescan" class="btn btn-outline-secondary">Rescan</button><button id="ip-reset" class="btn btn-outline-secondary">Reset</button><button id="ip-save" class="btn btn-outline-secondary">Save scan to site</button><button id="ip-export" class="btn btn-outline-secondary">Export</button><select id="ip-mode" class="form-select w-auto">${option("grid", "Grid", mode) + option("table", "Table", mode)}</select><select id="ip-filter" class="form-select w-auto">${["", "responding", "no-response", "known", "conflict", "reserved", "unscanned"].map((x) => option(x, x || "All addresses", filter)).join("")}</select><select id="ip-sort" class="form-select w-auto">${["ip", "status", "hostname"].map((x) => option(x, `Sort: ${x}`, sort)).join("")}</select><button id="ip-direction" class="btn btn-outline-secondary">${descending ? "↓" : "↑"}</button></div><div class="d-flex gap-3 small"><span id="ip-count"></span><span id="ip-duration"></span></div><div class="progress my-2" style="height:6px"><div id="ip-progress" class="progress-bar"></div></div><p id="ip-warnings" class="small text-warning"></p><div class="ip-legend">${["responding", "known", "no-response", "reserved", "unscanned", "conflict"].map((s) => `<span class="ip-${s}">${s}</span>`).join("")}</div><div id="ip-results"></div><div class="tool-actions mt-3"><button id="ip-prev" class="btn btn-sm btn-outline-secondary">Previous</button><span id="ip-page-label"></span><button id="ip-next" class="btn btn-sm btn-outline-secondary">Next</button></div>`;
    $("ip-adapter").onchange = () => {
      options.interface = $("ip-adapter").value;
      $("ip-cidr").value =
        adapters.find((a) => a.name === options.interface)?.addresses[0] || "";
      void perform(scope);
    };
    $("ip-start").oninput = $("ip-end").oninput = () => {
      $("ip-cidr").value = "";
    };
    $("ip-calculate").onclick = () => perform(scope);
    $("ip-start-scan").onclick = $("ip-rescan").onclick = () => perform(start);
    $("ip-stop").onclick = () => API.CancelJob();
    $("ip-reset").onclick = () =>
      perform(async () => {
        await API.ResetIPScan();
        page = 0;
        await refresh();
      });
    $("ip-save").onclick = () =>
      perform(async () => {
        await API.SaveIPScan(kit.sites.active().id);
        await kit.sites.refresh();
        message(
          "Scan saved",
          "Probe evidence and timestamps were saved to the selected site.",
        );
      });
    $("ip-export").onclick = () =>
      dialog(
        "Export scan evidence",
        select(
          "Format",
          "format",
          option("csv", "CSV") + option("json", "JSON"),
        ),
        (v) => API.ExportIPScan(v.format),
        "Export",
      );
    $("ip-mode").onchange = (e) => {
      mode = e.target.value;
      page = 0;
      void refresh();
    };
    $("ip-filter").onchange = (e) => {
      filter = e.target.value;
      page = 0;
      void refresh();
    };
    $("ip-sort").onchange = (e) => {
      sort = e.target.value;
      page = 0;
      void refresh();
    };
    $("ip-direction").onclick = () => {
      descending = !descending;
      void refresh();
    };
    $("ip-prev").onclick = () => {
      page--;
      void refresh();
    };
    $("ip-next").onclick = () => {
      page++;
      void refresh();
    };
    await perform(scope);
    await refresh();
  }
  kit.pages.scanner = render;
  setInterval(() => {
    if (!$("scanner-page").hidden) void refresh();
  }, 1000);
  return { refresh, render };
}
