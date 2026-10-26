import * as Desktop from "../bindings/github.com/TerminalAddict/SADPTools/desktopservice.js";
import { mappingTable } from "./switches.js";
import { cameraIPForm } from "./camera-ip.js";
import * as API from "../bindings/github.com/TerminalAddict/SADPTools/toolkitservice.js";
import { Events } from "@wailsio/runtime";
import {
  $,
  esc,
  option,
  list,
  date,
  dialog,
  field,
  area,
  select,
  check,
  message,
  contextMenu,
  table,
} from "./toolkit-ui.js";
export function initSites({ getSnapshot, showError, onSiteChanged }) {
  let sites = [],
    selected = "",
    view = null,
    query = "",
    allSearch = false,
    selectedRecords = new Set(),
    sort = "ipv4",
    direction = 1,
    page = 0,
    vault = {},
    searchGeneration = 0,
    preferencesLoaded = false;
  const perform = async (fn) => {
    try {
      return await fn();
    } catch (e) {
      showError(e);
    }
  };
  const active = () => view?.site;
  Events.On("vault:changed", () => void perform(refresh));
  async function refresh() {
    sites = await API.ListSites();
    if (!preferencesLoaded) {
      selected = (await API.Preferences()).siteID || "";
      preferencesLoaded = true;
    }
    vault = await API.VaultStatus();
    if (selected && !sites.some((s) => s.id === selected)) selected = "";
    if (selected) view = await API.Site(selected);
    else view = null;
    render();
    onSiteChanged?.(active());
  }
  async function choose(id) {
    selected = id;
    page = 0;
    selectedRecords.clear();
    if (id) await API.VisitSite(id);
    await refresh();
  }
  async function siteForm(
    site = { name: "", networks: [], tags: [], protocols: ["sadp"] },
    afterSave,
  ) {
    const adapters = await Desktop.Interfaces();
    const profiles =
      view && site.id && view.site.id === site.id ? view.profiles : [];
    dialog(
      site.id ? "Edit site" : "New site",
      field("Site name", "name", site.name, "text", "required maxlength=200") +
        field("Customer", "customer", site.customer) +
        field("Address / label", "address", site.address) +
        area("Notes", "notes", site.notes) +
        field("Tags (comma separated)", "tags", site.tags?.join(", ")) +
        area(
          "Approved IPv4 networks (one CIDR per line)",
          "networks",
          site.networks?.join("\n"),
        ) +
        select(
          "Discovery adapter (required before a site rescan)",
          "interface",
          option("", "Choose an adapter", site.interface) +
            adapters
              .filter((a) => a.suitable)
              .map((a) =>
                option(
                  a.name,
                  `${a.name} · ${a.addresses.join(", ")}`,
                  site.interface,
                ),
              )
              .join("") +
            (site.interface &&
            !adapters.some((a) => a.name === site.interface && a.suitable)
              ? option(
                  site.interface,
                  `${site.interface} (currently unavailable)`,
                  site.interface,
                )
              : ""),
        ) +
        check("Use SADP", "sadp", site.protocols?.includes("sadp")) +
        check(
          "Use ONVIF WS-Discovery",
          "onvif",
          site.protocols?.includes("onvif"),
        ) +
        select(
          "Site default credential",
          "defaultCredential",
          option("", "None", site.defaultCredential) +
            profiles
              .map((p) =>
                option(
                  p.id,
                  `${p.label} · ${p.username}`,
                  site.defaultCredential,
                ),
              )
              .join(""),
        ) +
        check("Archived", "archived", site.archived),
      async (values) => {
        const saved = await API.SaveSite({
          ...site,
          ...values,
          tags: list(values.tags),
          networks: list(values.networks),
          protocols: [
            ...(values.sadp ? ["sadp"] : []),
            ...(values.onvif ? ["onvif"] : []),
          ],
          archived: !!values.archived,
        });
        selected = saved.id;
        preferencesLoaded = true;
        await API.VisitSite(saved.id);
        await refresh();
        await afterSave?.(saved);
      },
    );
  }
  function credentialSource(r) {
    if (r.credentialMode === "none") return "None";
    const id =
      r.credentialMode === "profile"
        ? r.credentialID
        : view?.site.defaultCredential;
    const p = view?.profiles.find((p) => p.id === id);
    return p
      ? `${r.credentialMode === "profile" ? "Device" : "Site default"}: ${p.label}`
      : "None (no site default)";
  }
  function deviceForm(r) {
    const d = r.device;
    const recorded = [
      ["Local record ID", r.id],
      ["Device UUID", d.uuid],
      [
        "Activation",
        d.activated == null ? "Unknown" : d.activated ? "Active" : "Inactive",
      ],
      ["DHCP", d.dhcp == null ? "Unknown" : d.dhcp ? "Enabled" : "Disabled"],
      [
        "HTTP / HTTPS ports",
        `${d.httpPort ?? "Unknown"} / ${d.httpsPort ?? "Unknown"}`,
      ],
      [
        "SDK / RTSP ports",
        `${d.commandPort ?? "Unknown"} / ${d.rtspPort ?? "Unknown"}`,
      ],
      [
        "Discovered adapters",
        d.interfaces?.map((a) => `${a.name} (${a.address})`).join(", "),
      ],
      ["ONVIF service URLs", d.xaddrs?.join(", ")],
    ];
    const details = dialog(
      "Saved device — edit approved inventory",
      `<p class="small">First saved ${esc(date(r.firstSaved))} · Updated ${esc(date(r.updated))} · Last seen ${esc(date(d.lastSeen))}<br>Protocols: ${esc(d.protocols?.join(", ") || "Unknown")} · Credentials: ${esc(credentialSource(r))}</p><details class="mb-3"><summary>Recorded identity and network services</summary><dl class="small mt-2">${recorded.map(([label, value]) => `<dt>${esc(label)}</dt><dd>${esc(value || "Unknown")}</dd>`).join("")}</dl></details>` +
        field("IP address", "ipv4", d.ipv4) +
        field("Name", "name", d.name) +
        field("Vendor", "vendor", d.vendor) +
        field("Model", "model", d.model) +
        field("MAC", "mac", d.mac) +
        field("Serial", "serial", d.serial) +
        field("Firmware", "firmware", d.firmware) +
        field("Subnet mask", "subnetMask", d.subnetMask) +
        field("Gateway", "gateway", d.gateway) +
        field("Physical location", "location", r.location) +
        area("Installer notes", "notes", r.notes) +
        field("Tags", "tags", r.tags?.join(", ")) +
        select(
          "Credential assignment",
          "credential",
          option("inherit", "Use site default", r.credentialMode) +
            option("none", "None — do not inherit", r.credentialMode) +
            view.profiles
              .map((p) =>
                option(p.id, `${p.label} · ${p.username}`, r.credentialID),
              )
              .join(""),
        ) +
        check("Intentionally removed from site", "removed", r.removed),
      async (v) => {
        const next = { ...d };
        const manual = new Set(r.manualFields || []);
        for (const k of [
          "ipv4",
          "name",
          "vendor",
          "model",
          "mac",
          "serial",
          "firmware",
          "subnetMask",
          "gateway",
        ]) {
          if (v[k] !== d[k]) manual.add(k);
          next[k] = v[k];
        }
        const mode = ["none", "inherit"].includes(v.credential)
          ? v.credential
          : "profile";
        await API.SaveSiteDevice({
          ...r,
          device: next,
          location: v.location,
          notes: v.notes,
          tags: list(v.tags),
          manualFields: [...manual],
          credentialMode: mode,
          credentialID: mode === "profile" ? v.credential : "",
          removed: !!v.removed,
        });
        await refresh();
      },
    );
    const mapping = document.createElement("details");
    mapping.innerHTML =
      "<summary>Saved switch / PoE observations</summary><div>Loading saved observations…</div>";
    details.querySelector("form").append(mapping);
    void API.SwitchReports(r.siteID)
      .then((reports) => {
        mapping.querySelector("div").innerHTML = r.device.mac
          ? mappingTable(reports, r.device.mac)
          : "No reliable MAC address is recorded for this device.";
      })
      .catch((e) => {
        mapping.querySelector("div").textContent = e.message || String(e);
      });
  }
  async function history(r) {
    const events = await API.SiteHistory(r?.siteID || selected, r?.id || "");
    message(
      r ? "Device history" : "Site history",
      events
        .map(
          (e) =>
            `${date(e.at)} · ${e.source} · ${e.kind}\nBefore: ${e.before || "—"}\nAfter: ${e.after || "—"}`,
        )
        .join("\n\n") || "No history yet.",
    );
  }
  function move(r) {
    dialog(
      "Move saved device",
      `<p>Credential assignment will become None. Existing history stays in the original site.</p>` +
        select(
          "Destination site",
          "site",
          sites
            .filter((s) => s.id !== r.siteID && !s.archived)
            .map((s) => option(s.id, s.name))
            .join(""),
        ),
      async (v) => {
        await API.MoveSiteDevice(r.siteID, r.id, v.site);
        await refresh();
      },
      "Move device",
    );
  }
  function merge() {
    const ids = [...selectedRecords];
    if (ids.length !== 2)
      throw Error("Select exactly two saved records to merge.");
    dialog(
      "Merge duplicate records",
      `<p>The first selected record is retained, including its credentials and manual fields. Missing notes/location can be filled from the second. Review identities carefully.</p><pre>${esc(ids.join("\n"))}</pre>` +
        field("Type MERGE to confirm", "confirmation", "", "text", "required"),
      async (v) => {
        await API.MergeSiteDevices(selected, ids[0], ids[1], v.confirmation);
        selectedRecords.clear();
        await refresh();
      },
      "Merge records",
    );
  }
  function profileForm(p = { siteID: selected, label: "", username: "" }) {
    dialog(
      p.id ? "Replace credential" : "New reusable credential",
      field("Label", "label", p.label, "text", "required") +
        field("Username / SNMPv3 security name", "username", p.username) +
        field(
          "Password / SNMP authentication password / v2 community",
          "password",
          "",
          "password",
          "required",
        ) +
        field(
          "SNMPv3 privacy password (optional for camera credentials)",
          "privacy",
          "",
          "password",
        ) +
        `<p class="small text-body-secondary">Secrets are protected in the local vault. This profile can be shared by multiple devices; exports omit its secrets.</p>`,
      async (v) => {
        await API.SaveCredential(
          { ...p, label: v.label },
          v.username,
          v.password,
          v.privacy,
        );
        await refresh();
      },
    );
  }
  async function unlock() {
    vault = await API.VaultStatus();
    if (vault.mode === "dpapi") {
      await API.UnlockVault("");
      await refresh();
      return;
    }
    const info = vault.initialized
      ? "Enter your master passphrase. It is never saved."
      : "Create a master passphrase of at least 12 characters. Keep it safe; it cannot be recovered.";
    dialog(
      "Unlock saved passwords",
      `<p>${esc(info)}</p>` +
        field("Master passphrase", "passphrase", "", "password", "required") +
        (!vault.initialized
          ? field(
              "Repeat master passphrase",
              "repeat",
              "",
              "password",
              "required minlength=12",
            )
          : ""),
      async (v) => {
        if (
          !vault.initialized &&
          vault.mode !== "dpapi" &&
          v.passphrase !== v.repeat
        )
          throw Error("Master passphrases do not match.");
        await API.UnlockVault(v.passphrase || "");
        await refresh();
      },
      vault.initialized ? "Unlock" : "Protect saved passwords",
    );
  }
  async function reveal(p) {
    const secret = await API.RevealPassword(selected, p.id);
    const d = message("Password — clears after 15 seconds", secret);
    setTimeout(() => {
      d.querySelector("pre")?.replaceChildren();
      d.close();
      d.remove();
    }, 15000);
  }
  function copyPassword(p) {
    dialog(
      "Copy protected password",
      `<p>The OS clipboard and clipboard history may retain copied passwords. Only copy when needed.</p>`,
      async () => API.CopyPassword(selected, p.id),
      "Copy password",
    );
  }
  function backup() {
    dialog(
      "Encrypted site backup",
      `<p>Includes saved inventory and credentials. Choose a separate passphrase to restore on another computer. ${vault.unlocked || vault.automatic ? "" : "Unlock saved passwords first if this site contains credentials."}</p>` +
        field(
          "Backup passphrase",
          "passphrase",
          "",
          "password",
          "required minlength=12",
        ) +
        field(
          "Repeat passphrase",
          "repeat",
          "",
          "password",
          "required minlength=12",
        ),
      async (v) => {
        if (v.passphrase !== v.repeat) throw Error("Passphrases do not match.");
        await API.BackupSite(selected, v.passphrase);
      },
      "Choose backup file",
    );
  }
  function restore() {
    dialog(
      "Restore encrypted site",
      `<p>Imports a new site; existing sites are preserved. ${vault.unlocked || vault.automatic ? "" : "Unlock or create password protection before restoring credentials."}</p>` +
        field("Backup passphrase", "passphrase", "", "password", "required"),
      async (v) => {
        const site = await API.RestoreSite(v.passphrase);
        if (site.id) selected = site.id;
        await refresh();
      },
      "Choose backup file",
    );
  }
  async function addByIP(address = "") {
    sites = await API.ListSites();
    const available = sites.filter((s) => !s.archived);
    if (!available.length) {
      await siteForm(undefined, () => addByIP(address));
      return;
    }
    return cameraIPForm({
      API,
      sites: available,
      selectedSite: selected,
      initialIP: address,
      unlock,
      afterSave: async (record, openLive) => {
        await choose(record.siteID);
        if (openLive)
          document.dispatchEvent(
            new CustomEvent("camera:preview", {
              detail: { siteID: record.siteID, deviceID: record.id },
            }),
          );
      },
    });
  }
  async function addLive(ids) {
    if (!ids.length) throw Error("Select at least one discovered device.");
    sites = await API.ListSites();
    if (!sites.length) {
      await siteForm(undefined, async (site) => {
        const result = await API.SaveLiveToSite(site.id, ids);
        await refresh();
        if (result.review.length)
          message("Identity review required", result.review.join("\n"));
      });
      return;
    }
    dialog(
      "Save discovered devices to site",
      select(
        "Site",
        "site",
        sites
          .filter((s) => !s.archived)
          .map((s) => option(s.id, s.name, selected))
          .join(""),
      ) +
        `<p>${ids.length} selected device(s). Existing matches will be kept; conflicting identities require review.</p>`,
      async (v) => {
        const result = await API.SaveLiveToSite(v.site, ids);
        selected = v.site;
        await refresh();
        if (result.review.length)
          message("Identity review required", result.review.join("\n"));
      },
      "Save devices",
    );
  }
  function exportSite() {
    dialog(
      "Export site without credentials",
      select(
        "Format",
        "format",
        option("csv", "CSV") +
          option("json", "JSON") +
          option("html", "HTML report"),
      ),
      async (v) => API.ExportSite(selected, v.format),
      "Export",
    );
  }
  async function renderRecords() {
    if (!$("site-records")) return;
    const generation = ++searchGeneration;
    let records = allSearch
      ? await API.SearchSites(query)
      : (view?.devices || []).filter((r) =>
          [
            r.device.ipv4,
            r.device.mac,
            r.device.serial,
            r.device.model,
            r.device.name,
            r.notes,
            r.location,
            ...(r.tags || []),
          ]
            .join(" ")
            .toLowerCase()
            .includes(query.toLowerCase()),
        );
    if (generation !== searchGeneration || !$("site-records")) return;
    records.sort(
      (a, b) =>
        direction *
        String(a.device[sort] || "").localeCompare(
          String(b.device[sort] || ""),
          undefined,
          { numeric: true },
        ),
    );
    page = Math.min(page, Math.max(0, Math.ceil(records.length / 100) - 1));
    const shown = records.slice(page * 100, page * 100 + 100);
    $("site-records").innerHTML = table(
      [
        "Select",
        "IP address",
        "Model / name",
        "MAC / serial",
        "Firmware",
        "Credential source",
        "Location / notes",
        "Actions",
      ],
      shown.map(
        (r) =>
          `<tr data-id="${esc(r.id)}"><td><input type="checkbox" class="form-check-input site-pick" aria-label="Select ${esc(r.device.ipv4)}" ${selectedRecords.has(r.id) ? "checked" : ""}></td><td>${esc(r.device.ipv4)}${r.removed ? '<br><span class="badge text-bg-secondary">Intentionally removed</span>' : ""}${allSearch ? `<br><small>${esc(sites.find((s) => s.id === r.siteID)?.name)}</small>` : ""}</td><td>${esc(r.device.model)}<br><small>${esc(r.device.name)}</small></td><td>${esc(r.device.mac)}<br><small>${esc(r.device.serial)}</small></td><td>${esc(r.device.firmware)}</td><td>${esc(allSearch ? "Open site for credentials" : credentialSource(r))}</td><td>${esc(r.location)}<br><small>${esc(r.notes)}</small></td><td><button class="btn btn-sm btn-outline-primary preview-record">Live View</button> <button class="btn btn-sm btn-outline-secondary edit-record">Details</button> <button class="btn btn-sm btn-outline-secondary record-history">History</button></td></tr>`,
      ),
    );
    $("site-pagination").innerHTML =
      `<button id="site-prev" class="btn btn-sm btn-outline-secondary" ${page === 0 ? "disabled" : ""}>Previous</button> <span>${records.length ? `${page * 100 + 1}–${Math.min(records.length, (page + 1) * 100)}` : "0"} of ${records.length}</span> <button id="site-next" class="btn btn-sm btn-outline-secondary" ${(page + 1) * 100 >= records.length ? "disabled" : ""}>Next</button>`;
    $("site-prev").onclick = () => {
      page--;
      void renderRecords();
    };
    $("site-next").onclick = () => {
      page++;
      void renderRecords();
    };
    for (const tr of $("site-records").querySelectorAll("tbody tr")) {
      const r = shown.find((r) => r.id === tr.dataset.id);
      tr.querySelector(".site-pick").onchange = (e) =>
        e.target.checked
          ? selectedRecords.add(r.id)
          : selectedRecords.delete(r.id);
      const preview = () =>
        document.dispatchEvent(
          new CustomEvent("camera:preview", {
            detail: { siteID: r.siteID, deviceID: r.id },
          }),
        );
      tr.querySelector(".preview-record").onclick = preview;
      tr.querySelector(".edit-record").onclick = () =>
        perform(async () => {
          if (r.siteID !== selected) await choose(r.siteID);
          deviceForm(r);
        });
      tr.querySelector(".record-history").onclick = () =>
        perform(async () => {
          if (r.siteID !== selected) await choose(r.siteID);
          await history(r);
        });
      tr.oncontextmenu = (e) =>
        contextMenu(e, [
          ["Live View", preview],
          ["Copy IP", () => perform(() => API.CopyText(r.device.ipv4))],
          [
            "Read ONVIF information",
            () =>
              perform(async () => {
                if (r.siteID !== selected) await choose(r.siteID);
                document.dispatchEvent(
                  new CustomEvent("toolkit:onvif", { detail: r.id }),
                );
              }),
          ],
          ["View history", () => perform(() => history(r))],
          ["Move to site", () => move(r)],
          [
            "Apply site credentials",
            () =>
              perform(async () => {
                await API.SaveSiteDevice({
                  ...r,
                  credentialMode: "inherit",
                  credentialID: "",
                });
                await refresh();
              }),
          ],
        ]);
    }
  }
  function render() {
    if (!$("sites-page")) return;
    $("sites-page").innerHTML =
      `<aside class="site-sidebar"><div class="d-flex gap-2"><h2 class="me-auto">Sites</h2><button id="new-site" class="btn btn-sm btn-primary">New</button></div><input id="site-filter" class="form-control my-2" placeholder="Find a site" aria-label="Find a site"><div id="site-list">${sites.map((s) => `<button class="site-choice ${s.id === selected ? "active" : ""}" data-site="${esc(s.id)}">${esc(s.name)}${s.archived ? " · Archived" : ""}<small>${esc(s.customer)}</small></button>`).join("") || '<p class="text-body-secondary">Create a site to save an inventory.</p>'}</div><button id="site-add-ip" class="btn btn-sm btn-outline-primary mt-3">Add camera by IP</button><button id="restore-site" class="btn btn-sm btn-outline-secondary mt-3">Restore encrypted backup</button><hr><p class="small">Saved passwords: <strong>${vault.automatic ? "Protected by Windows" : vault.unlocked ? "Available" : "Locked"}</strong></p><button id="vault-action" class="btn btn-sm btn-outline-secondary">${vault.unlocked || vault.automatic ? "Lock now" : "Unlock saved passwords"}</button></aside><section class="site-content">${view ? `<div class="tool-heading"><div><h2>${esc(view.site.name)}</h2><p>${esc(view.site.customer)} · ${view.devices.filter((r) => !r.removed).length} saved devices · Last visited ${esc(date(view.site.lastVisited))}</p><p class="small">Networks: ${esc(view.site.networks?.join(", ") || "Not configured")} · Last scan ${esc(date(view.site.lastScan))}</p></div><button id="edit-site" class="btn btn-sm btn-outline-secondary">Edit site</button></div><div class="tool-actions"><button id="site-save-all" class="btn btn-primary">Save all discovered</button><button id="site-rescan" class="btn btn-outline-primary">Rescan site</button><button id="site-credentials" class="btn btn-outline-secondary">Credentials</button><button id="site-export" class="btn btn-outline-secondary">Export</button><button id="site-backup" class="btn btn-outline-secondary">Backup</button><button id="site-history" class="btn btn-outline-secondary">History</button><button id="site-merge" class="btn btn-outline-secondary">Merge selected</button><button id="site-delete" class="btn btn-outline-danger">Delete site</button></div><div class="tool-actions"><input id="saved-search" class="form-control" placeholder="Search IP, MAC, serial, model, name or notes" value="${esc(query)}"><label class="text-nowrap"><input id="all-sites-search" type="checkbox" ${allSearch ? "checked" : ""}> Across sites</label><select id="site-sort" class="form-select w-auto">${["ipv4", "model", "mac", "serial", "firmware", "name"].map((k) => option(k, `Sort: ${k}`, sort)).join("")}</select><button id="site-direction" class="btn btn-outline-secondary">${direction === 1 ? "↑" : "↓"}</button></div><div id="site-records"></div><div id="site-pagination" class="tool-actions"></div><div id="site-runs" class="mt-3"><h3>Saved scans and reports</h3>${view.runs.map((r) => `<button class="btn btn-sm btn-outline-secondary me-2 mb-2" data-run="${esc(r.id)}">${esc(r.kind)} · ${esc(date(r.at))} · ${esc(r.outcome)}</button>`).join("") || "No saved scans yet."}</div>` : '<div class="tool-empty"><h2>Persistent site inventories</h2><p>Select or create a site. Live discovery stays separate until you explicitly save observations.</p></div>'}</section>`;
    $("new-site").onclick = () => perform(() => siteForm());
    $("restore-site").onclick = restore;
    $("site-add-ip").onclick = () => perform(() => addByIP());
    $("vault-action").onclick = () =>
      perform(async () => {
        if (vault.unlocked || vault.automatic) {
          await API.LockVault();
          await refresh();
        } else await unlock();
      });
    $("site-filter").oninput = (e) => {
      for (const b of $("site-list").children)
        b.hidden = !b.textContent
          .toLowerCase()
          .includes(e.target.value.toLowerCase());
    };
    for (const b of document.querySelectorAll("[data-site]"))
      b.onclick = () => perform(() => choose(b.dataset.site));
    if (!view) return;
    $("edit-site").onclick = () => perform(() => siteForm(view.site));
    $("site-save-all").onclick = () =>
      perform(() => addLive(getSnapshot().devices.map((d) => d.id)));
    $("site-export").onclick = exportSite;
    $("site-backup").onclick = backup;
    $("site-history").onclick = () => perform(() => history());
    $("site-merge").onclick = () => perform(merge);
    $("site-delete").onclick = () =>
      dialog(
        "Delete site permanently",
        `<p>Make an encrypted backup first. This deletes this site's inventory, credential profiles and history.</p>` +
          field(
            `Type ${view.site.name} to confirm`,
            "confirmation",
            "",
            "text",
            "required",
          ),
        async (v) => {
          await API.DeleteSite(selected, v.confirmation);
          selected = "";
          await refresh();
        },
        "Delete site",
      );
    $("site-credentials").onclick = () => credentials();
    $("saved-search").oninput = (e) => {
      query = e.target.value;
      page = 0;
      void perform(renderRecords);
    };
    $("all-sites-search").onchange = (e) => {
      allSearch = e.target.checked;
      page = 0;
      void perform(renderRecords);
    };
    $("site-sort").onchange = (e) => {
      sort = e.target.value;
      void renderRecords();
    };
    $("site-direction").onclick = () => {
      direction *= -1;
      void renderRecords();
    };
    $("site-rescan").onclick = () =>
      document.dispatchEvent(
        new CustomEvent("toolkit:rescan", { detail: view.site }),
      );
    for (const b of $("site-runs").querySelectorAll("[data-run]"))
      b.onclick = () =>
        document.dispatchEvent(
          new CustomEvent("toolkit:run", {
            detail: { site: selected, run: b.dataset.run },
          }),
        );
    void perform(renderRecords);
  }
  function credentials() {
    const d = dialog(
      "Reusable credentials",
      `<p>Saved passwords are ${vault.automatic ? "protected by your Windows account and available automatically" : vault.unlocked ? "available" : "locked"}. Profiles can be assigned as the site default or to individual devices.</p><div>${view.profiles.map((p) => `<div class="credential-row" data-profile="${esc(p.id)}"><strong>${esc(p.label)}</strong> · ${esc(p.username)}<div><button type="button" class="btn btn-sm btn-outline-secondary edit-profile">Replace</button> <button type="button" class="btn btn-sm btn-outline-secondary reveal-profile">Reveal</button> <button type="button" class="btn btn-sm btn-outline-secondary copy-profile">Copy password</button> <button type="button" class="btn btn-sm btn-outline-danger delete-profile">Remove</button></div></div>`).join("") || "No credentials saved."}</div>`,
      async () => {},
      "Close",
    );
    const add = document.createElement("button");
    add.type = "button";
    add.className = "btn btn-primary mt-3";
    add.textContent = "New credential profile";
    add.onclick = () => {
      d.close();
      d.remove();
      profileForm();
    };
    d.querySelector("form").append(add);
    for (const row of d.querySelectorAll("[data-profile]")) {
      const p = view.profiles.find((x) => x.id === row.dataset.profile);
      row.querySelector(".edit-profile").onclick = () => {
        d.close();
        d.remove();
        profileForm(p);
      };
      row.querySelector(".reveal-profile").onclick = () =>
        perform(() => reveal(p));
      row.querySelector(".copy-profile").onclick = () => copyPassword(p);
      row.querySelector(".delete-profile").onclick = () =>
        dialog(
          "Remove credential profile",
          `<p>Remove ${esc(p.label)}? Assignments must be removed first.</p>`,
          async () => {
            await API.DeleteCredential(selected, p.id);
            d.close();
            d.remove();
            await refresh();
          },
          "Remove profile",
        );
    }
  }
  return {
    refresh,
    choose,
    addLive,
    addByIP,
    active,
    view: () => view,
    selected: () => [...selectedRecords],
    unlock,
    render,
  };
}
