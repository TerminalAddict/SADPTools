import "bootstrap/dist/css/bootstrap.min.css";
import "./style.css";
import "./toolkit.css";
import { initLiveView } from "./live-view.js";
import { initToolkit } from "./toolkit.js";
import { contextMenu } from "./toolkit-ui.js";
import { Browser, Events } from "@wailsio/runtime";
import * as Backend from "../bindings/github.com/TerminalAddict/SADPTools/desktopservice.js";
import { initConfiguration } from "./configuration.js";
import { initUpdates } from "./updates.js";
import { activation, model, visibleDevices } from "./view.js";
import { createWebLookup, detectedURL } from "./web-view.js";
import { scanPresentation } from "./scan-view.js";

const multiSelected = new Set();
const $ = (id) => document.getElementById(id);
let settings,
  snapshot = { devices: [], logs: [], warnings: [], scanning: false },
  selected = "",
  sortKey = "ipv4",
  sortDirection = 1,
  refreshTimer,
  refreshing = false,
  refreshAgain = false;
const dash = (value) =>
  value === null || value === undefined || value === "" ? "—" : String(value);
const date = (value) => (value ? new Date(value).toLocaleString() : "—");
const errorText = (error) => error?.message || String(error);
function notice(text, success = false) {
  $("notice").textContent = text;
  $("notice").hidden = !text;
  $("notice").className = `alert alert-${success ? "success" : "danger"}`;
}
async function action(fn) {
  try {
    notice("");
    await fn();
  } catch (error) {
    notice(errorText(error));
  }
}
function visible() {
  return visibleDevices(
    snapshot.devices,
    $("search").value,
    sortKey,
    sortDirection,
  );
}
function badge(value) {
  const span = document.createElement("span");
  span.className = `badge-state badge-${activation(value).toLowerCase()}`;
  span.textContent = activation(value);
  return span;
}
function render() {
  const list = visible(),
    body = $("devices"),
    fragment = document.createDocumentFragment();
  // Restore keyboard focus when a live inventory update replaces table rows.
  const focused = document.activeElement?.closest("tr")?.dataset.id;
  for (const d of list) {
    const row = document.createElement("tr");
    row.dataset.id = d.id;
    row.tabIndex = 0;
    row.classList.toggle("selected", d.id === selected);
    row.setAttribute("aria-selected", String(d.id === selected));
    const pickCell = document.createElement("td"),
      pick = document.createElement("input");
    pick.type = "checkbox";
    pick.className = "form-check-input";
    pick.checked = multiSelected.has(d.id);
    pick.setAttribute("aria-label", `Select ${d.ipv4 || d.id}`);
    pick.onclick = (e) => e.stopPropagation();
    pick.onchange = () => {
      if (pick.checked) multiSelected.add(d.id);
      else multiSelected.delete(d.id);
    };
    pickCell.append(pick);
    row.append(pickCell);
    row.oncontextmenu = (e) =>
      contextMenu(e, [
        [
          "Live View",
          () => {
            selected = d.id;
            render();
            void liveView.open("", d.id);
          },
        ],
        ["Add to site", () => toolkit.addLive([d.id])],
        ["Save selected to site", () => toolkit.addLive([...multiSelected])],
      ]);
    for (const val of [d.ipv4, model(d), d.mac, d.firmware, d.serial]) {
      const cell = document.createElement("td");
      cell.textContent = dash(val);
      cell.title = dash(val);
      row.append(cell);
    }
    const cell = document.createElement("td");
    cell.append(badge(d.activated));
    row.append(cell);
    const select = () => {
      selected = d.id;
      render();
    };
    row.addEventListener("click", select);
    row.addEventListener("keydown", (e) => {
      if (e.key === "Enter" || e.key === " ") {
        e.preventDefault();
        select();
      }
    });
    fragment.append(row);
  }
  body.replaceChildren(fragment);
  if (focused) {
    [...body.rows]
      .find((r) => r.dataset.id === focused)
      ?.focus({ preventScroll: true });
  }
  $("count").textContent = snapshot.devices.length;
  $("visible-count").textContent =
    `${list.length} of ${snapshot.devices.length} devices`;
  $("inventory-caption").textContent = snapshot.devices.length
    ? "Live inventory · Existing devices stay visible during discovery."
    : "Discover devices on your local network to get started.";
  $("empty").hidden = list.length > 0;
  $("empty").querySelector("h3").textContent = snapshot.devices.length
    ? "No matching devices"
    : snapshot.scanning
      ? "Listening for device replies…"
      : snapshot.lastScan
        ? "No devices found"
        : "No devices discovered yet";
  $("empty").querySelector("p").textContent = snapshot.devices.length
    ? "Try a different IP address, model, MAC address or serial number."
    : snapshot.lastScan
      ? "Check the selected adapter, local network and firewall, then try again."
      : "Choose a network adapter, then select Discover devices. Devices appear as they respond.";
  $("discover").disabled = snapshot.scanning || snapshot.configuring;
  $("cancel").hidden = !snapshot.scanning;
  $("adapter").disabled = snapshot.scanning || snapshot.configuring;
  $("scan-status").textContent = snapshot.configuring
    ? "Device operation…"
    : snapshot.scanning
      ? "Discovering…"
      : snapshot.lastError
        ? "Scan failed"
        : snapshot.warnings.length
          ? "Completed with warnings"
          : snapshot.lastScan
            ? "Scan complete"
            : "Ready to discover";
  $("scan-dot").classList.toggle("scanning", snapshot.scanning);
  if (!snapshot.scanning && snapshot.scanOutcome === "cancelled")
    $("scan-status").textContent = "Scan cancelled";
  renderScanActivity();
  $("export").disabled = list.length === 0;
  $("last-scan").textContent = snapshot.lastScan
    ? `Last scan ${date(snapshot.lastScan)}`
    : "No scan completed";
  updateConfiguration();
  updates.render();
  renderDetails();
  renderLogs();
}
function renderDetails() {
  const d = snapshot.devices.find((d) => d.id === selected);
  webLookup.select(d, snapshot.lastScan, snapshot.configuring);
  $("detail-empty").hidden = !!d;
  $("detail-content").hidden = !d;
  $("detail-status").textContent = d ? activation(d.activated) : "No selection";
  if (!d) return;
  const web = webLookup.get(),
    endpoint = web.result?.endpoint;
  const url = detectedURL(d, endpoint);
  $("detail-status").className =
    `badge-state badge-${activation(d.activated).toLowerCase()}`;
  $("detail-model").textContent = model(d) || "Unknown model";
  $("detail-name").textContent = d.name || "";
  const fields = [
    ["IPv4 address", d.ipv4],
    ["Subnet mask", d.subnetMask],
    ["Gateway", d.gateway],
    ["MAC address", d.mac],
    ["Serial number", d.serial],
    ["Firmware", d.firmware],
    ["Device type", d.deviceType],
    ["Vendor", d.vendor],
    ["Discovery protocols", d.protocols?.join(", ")],
    ["ONVIF service addresses", d.xaddrs?.join("\n")],
    ["HTTPS port", d.httpsPort],
    ["RTSP port", d.rtspPort],
    [
      "Identity",
      d.identityConflict
        ? "Conflicting identifiers — review before management"
        : "",
    ],
    ["DHCP", d.dhcp === null ? "Unknown" : d.dhcp ? "Enabled" : "Disabled"],
    ["HTTP port", d.httpPort],
    ["SDK command port", d.commandPort],
    ["Recovery ability (advertised)", d.passwordResetAbility],
    ["Recovery mode (advertised)", d.passwordResetMode],
    [
      "Discovered on",
      d.interfaces.map((a) => `${a.name} · ${a.address}`).join("\n"),
    ],
    ["First seen", date(d.firstSeen)],
    ["Last seen", date(d.lastSeen)],
  ];
  const fragment = document.createDocumentFragment();
  for (const [label, value] of fields) {
    const dt = document.createElement("dt"),
      dd = document.createElement("dd");
    dt.textContent = label;
    dd.textContent = dash(value);
    if (label === "IPv4 address" && url) {
      const link = document.createElement("a");
      link.href = url;
      link.textContent = d.ipv4;
      link.title = `Open ${url}`;
      link.target = "_blank";
      link.rel = "noopener noreferrer";
      link.addEventListener("click", (event) => {
        event.preventDefault();
        void action(() => Backend.OpenDevice(d.id));
      });
      dd.replaceChildren(
        link,
        document.createTextNode(` · ${endpoint.scheme.toUpperCase()}`),
      );
    }
    fragment.append(dt, dd);
  }
  $("detail-fields").replaceChildren(fragment);
  $("onvif-info").hidden = !d.xaddrs?.length;
  $("onvif-info").disabled =
    snapshot.scanning || snapshot.configuring || d.identityConflict;
  $("onvif-info").onclick = () =>
    document.dispatchEvent(new CustomEvent("toolkit:onvif", { detail: d.id }));
  $("open-web").disabled = !url || web.busy || snapshot.configuring;
  $("open-web").textContent = web.busy
    ? "Checking HTTP / HTTPS…"
    : url
      ? `Open ${endpoint.scheme.toUpperCase()} web interface ↗`
      : "Open web interface ↗";
  $("check-web").disabled = web.busy || snapshot.configuring;
  $("web-note").textContent = web.busy
    ? "Checking the camera’s web connection…"
    : web.error || web.result?.message || "Checking web interface…";
  if (url && endpoint.scheme === "https" && !endpoint.certificateTrusted)
    $("web-note").textContent =
      "HTTPS detected. Your browser may ask you to confirm this camera’s certificate.";
}
function renderScanActivity() {
  const scan = scanPresentation(snapshot);
  $("scan-activity").hidden = !scan.visible;
  $("scan-spinner").hidden = !scan.active;
  $("discover-spinner").hidden = !scan.active;
  $("discover-label").textContent = scan.active
    ? "Discovering…"
    : "Discover devices";
  if ($("scan-activity-label").textContent !== scan.label)
    $("scan-activity-label").textContent = scan.label;
  $("scan-counts").textContent = scan.counts;
  $("scan-timing").textContent = scan.timing;
  $("scan-progress-track").hidden = !scan.active;
  $("scan-progress-track").setAttribute(
    "aria-valuenow",
    String(Math.round(scan.percent)),
  );
  $("scan-progress").style.width = `${scan.percent}%`;
}
function renderLogs() {
  const logs = snapshot.logs.filter(
    (l) => $("debug-logs").checked || l.level !== "DEBUG",
  );
  $("log-count").textContent = `${snapshot.logs.length} entries`;
  $("logs").textContent =
    logs
      .map(
        (l) =>
          `${new Date(l.time).toLocaleTimeString()}  ${l.level.padEnd(5)}  ${l.message}${l.details ? "  " + l.details : ""}`,
      )
      .join("\n") || "No discovery activity yet.";
}
async function refresh() {
  if (refreshing) {
    refreshAgain = true;
    return;
  }
  refreshing = true;
  try {
    snapshot = await Backend.Snapshot();
    render();
    if (snapshot.lastError) notice(snapshot.lastError);
    else if (snapshot.warnings.length) notice(snapshot.warnings.join("\n"));
  } catch (error) {
    notice(`Desktop connection: ${errorText(error)}`);
  } finally {
    refreshing = false;
    if (refreshAgain) {
      refreshAgain = false;
      queueRefresh();
    }
  }
}
function queueRefresh() {
  if (!refreshTimer)
    refreshTimer = setTimeout(() => {
      refreshTimer = null;
      void refresh();
    }, 100);
}
async function adapters() {
  const items = await Backend.Interfaces();
  $("adapter").replaceChildren(new Option("All suitable interfaces", ""));
  for (const a of items) {
    const option = new Option(
      `${a.name} · ${a.addresses.join(", ") || a.reason}`,
      a.name,
    );
    option.disabled = !a.suitable;
    $("adapter").append(option);
  }
  if (settings.interface && !items.some((a) => a.name === settings.interface)) {
    $("adapter").append(
      new Option(`${settings.interface} · unavailable`, settings.interface),
    );
  }
  $("adapter").value = settings.interface;
}
async function save(next) {
  await Backend.SaveSettings(next);
  settings = next;
  document.documentElement.dataset.bsTheme = settings.theme;
  $("automatic").checked = settings.automatic;
}
$("discover").onclick = () =>
  action(async () => {
    await Backend.StartScan();
    await refresh();
  });
$("cancel").onclick = () =>
  action(async () => {
    await Backend.CancelScan();
    await refresh();
  });
$("search").oninput = render;
$("adapters-refresh").onclick = () => action(adapters);
$("adapter").onchange = () =>
  action(async () => {
    try {
      await save({ ...settings, interface: $("adapter").value });
    } catch (e) {
      $("adapter").value = settings.interface;
      throw e;
    }
  });
$("automatic").onchange = () =>
  action(async () => {
    try {
      await save({ ...settings, automatic: $("automatic").checked });
    } catch (e) {
      $("automatic").checked = settings.automatic;
      throw e;
    }
  });
$("debug-logs").onchange = renderLogs;
$("preview-camera").onclick = () => liveView.open("", selected);
$("open-web").onclick = () => action(() => Backend.OpenDevice(selected));
$("check-web").onclick = () => webLookup.retry();
$("export").onclick = () =>
  action(async () => {
    const path = await Backend.ExportDevices(
      visible().map((d) => d.id),
      $("export-format").value,
    );
    if (path) notice(`Exported to ${path}`, true);
  });
for (const th of document.querySelectorAll("th[data-key]")) {
  th.querySelector("button").onclick = () => {
    sortDirection = sortKey === th.dataset.key ? -sortDirection : 1;
    sortKey = th.dataset.key;
    for (const item of document.querySelectorAll("th[data-key]")) {
      item.setAttribute(
        "aria-sort",
        item === th
          ? sortDirection === 1
            ? "ascending"
            : "descending"
          : "none",
      );
      item.querySelector("span").textContent =
        item === th ? (sortDirection === 1 ? "↑" : "↓") : "";
    }
    render();
  };
}
$("about-open").onclick = () => {
  $("about-error").hidden = true;
  $("about-credits").open = false;
  $("about-dialog").showModal();
  void updates.refreshAbout();
};
$("about-close").onclick = () => $("about-dialog").close();
for (const link of document.querySelectorAll("#about-dialog a")) {
  link.onclick = async (event) => {
    event.preventDefault();
    $("about-error").hidden = true;
    try {
      await Browser.OpenURL(link.href);
    } catch (error) {
      $("about-error").textContent =
        `Could not open your browser. ${errorText(error)}`;
      $("about-error").hidden = false;
    }
  };
}
$("settings-open").onclick = () => {
  if (!settings) return;
  $("theme").value = settings.theme;
  $("timeout").value = settings.timeoutSeconds;
  $("protocol-sadp").checked =
    !settings.protocols?.length || settings.protocols.includes("sadp");
  $("protocol-onvif").checked = settings.protocols?.includes("onvif") || false;
  $("interval").value = settings.intervalSeconds;
  $("check-updates").checked = settings.checkUpdates;
  $("update-channel").value = settings.updateChannel;
  $("settings-error").textContent = "";
  $("settings-dialog").showModal();
};
$("settings-close").onclick = () => $("settings-dialog").close();
$("settings-form").onsubmit = async (e) => {
  e.preventDefault();
  try {
    if (!$("protocol-sadp").checked && !$("protocol-onvif").checked)
      throw new Error("Select at least one discovery protocol");
    await save({
      ...settings,
      theme: $("theme").value,
      protocols: [
        $("protocol-sadp").checked ? "sadp" : "",
        $("protocol-onvif").checked ? "onvif" : "",
      ].filter(Boolean),
      timeoutSeconds: Number($("timeout").value),
      intervalSeconds: Number($("interval").value),
      checkUpdates: $("check-updates").checked,
      updateChannel: $("update-channel").value,
    });
    $("settings-dialog").close();
  } catch (error) {
    $("settings-error").textContent = errorText(error);
  }
};
async function init() {
  // No mock backend or fixture inventory is used when the Wails bridge is absent.
  try {
    settings = await Backend.GetSettings();
    document.documentElement.dataset.bsTheme = settings.theme;
    $("automatic").checked = settings.automatic;
    $("version").textContent = await Backend.Version();
    $("about-version").textContent = $("version").textContent;
    Events.On("inventory:changed", queueRefresh);
    await adapters();
    await refresh();
    await updates.start();
    setInterval(() => {
      if (snapshot.scanning) queueRefresh();
    }, 1000);
    setInterval(() => {
      if (snapshot.scanning) renderScanActivity();
    }, 200);
  } catch (error) {
    notice(
      `Could not connect to the desktop backend. Launch SADPTools as a desktop application. ${errorText(error)}`,
    );
    $("discover").disabled = true;
    $("automatic").disabled = true;
  }
}
$("live-select-all").onchange = (e) => {
  for (const d of visible())
    if (e.target.checked) multiSelected.add(d.id);
    else multiSelected.delete(d.id);
  render();
};
const liveView = initLiveView();
const toolkit = initToolkit({
  getSnapshot: () => snapshot,
  getSelected: () =>
    multiSelected.size ? [...multiSelected] : selected ? [selected] : [],
});
const webLookup = createWebLookup(Backend.CheckDeviceWeb, renderDetails);
const updateConfiguration = initConfiguration({
  getDevice: () => snapshot.devices.find((d) => d.id === selected),
  getSnapshot: () => snapshot,
  getSettings: () => settings,
  refresh,
  notice,
  onWebEndpoint: (id, endpoint) => webLookup.adopt(id, endpoint),
});
const updates = initUpdates({
  getSnapshot: () => snapshot,
  getSettings: () => settings,
});
void init();
