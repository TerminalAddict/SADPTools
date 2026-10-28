import { dialog, esc, field, date } from "./toolkit-ui.js";

export const nxSections = [
  ["system", "System", []],
  ["servers", "Servers", ["name", "url", "full_version", "operating_system"]],
  ["users", "Users", ["account", "display_name", "role", "enabled", "email"]],
  ["storage", "Storage", ["name", "server_name", "path", "recording_enabled", "backup"]],
  ["licenses", "Licences", ["name", "channels", "expiration", "deactivations_remaining"]],
];

const labels = {
  url: "Address", id: "ID", local_id: "Local system ID", cloud_id: "Cloud system ID",
  cloud_account: "Cloud account", cloud_host: "Cloud host", latest_transaction: "Latest transaction",
  public_ip: "Public IP", full_version: "Full version", version: "Version", os_info: "OS details",
  cpu: "CPU", memory_bytes: "Memory (bytes)", time_zone_offset_ms: "Time zone offset (ms)",
  max_cameras: "Maximum cameras", group_ids: "Group IDs", direct_permissions: "Direct permissions (stored bitmask)",
  key: "Licence key", serial: "Serial", hardware_id: "Hardware ID", license_block: "Licence block",
  stored_space_bytes: "Stored space (bytes)", space_limit_bytes: "Space limit (bytes)",
  deactivations_remaining: "Deactivations remaining", activations_remaining: "Activations remaining",
};
const label = key => labels[key] || key.replaceAll("_", " ").replace(/([a-z])([A-Z])/g, "$1 $2").replace(/^./, c => c.toUpperCase());
export function nxValue(value) {
  if (value == null || value === "") return "Unknown";
  if (typeof value === "boolean") return value ? "Yes" : "No";
  if (Array.isArray(value)) return value.length ? value.map(nxValue).join("\n") : "None recorded";
  if (typeof value === "object") return Object.entries(value).map(([k, v]) => `${label(k)}: ${nxValue(v)}`).join("\n");
  return String(value);
}

export function nxWitnessButton(view) {
  return view?.hasNXWitness ? '<button id="site-nx-witness" class="btn btn-outline-secondary">NX Witness</button>' : "";
}

export function nxDetails(record, omitted = []) {
  const entries = Object.entries(record || {}).filter(([key]) => !omitted.includes(key));
  if (!entries.length) return '<p class="text-body-secondary">No additional details in this export.</p>';
  return '<dl class="nx-details">' + entries.map(([key, value]) => {
    const text = esc(nxValue(value));
    const privateValue = ["key", "license_block", "fields"].includes(key) && value != null && value !== "";
    return `<dt>${esc(label(key))}</dt><dd>${privateValue ? `<details><summary>Show ${esc(label(key).toLowerCase())}</summary><pre class="nx-value">${text}</pre></details>` : `<span class="nx-value">${text}</span>`}</dd>`;
  }).join("") + '</dl>';
}

export async function openNXWitness(API, siteID, subscribe = () => () => {}) {
  const status = await API.VaultStatus();
  if (!status.unlocked && !status.automatic) {
    dialog("Unlock NX Witness information",
      '<p>This imported snapshot includes protected server, user and licence information.</p>' +
      (status.mode === "dpapi" ? '<p>Unlock it with your Windows account.</p>' : field("Saved passwords passphrase", "passphrase", "", "password", "required")),
      async values => {
        await API.UnlockVault(values.passphrase || "");
        // The unlock dialog is closed before the information dialog opens.
        setTimeout(() => { void openNXWitness(API, siteID, subscribe).catch(error => {
          dialog("NX Witness information", `<p>${esc(error?.message || "Could not open saved information.")}</p>`, async () => {}, "Close");
        }); }, 0);
      }, "Unlock and view");
    return;
  }
  let closed = false, element, info;
  const unsubscribe = subscribe(() => {
    closed = true;
    info = null;
    element?.close();
    element?.replaceChildren();
    element?.remove();
  });
  try {
    info = await API.NXWitness(siteID);
    if (closed) return;
    let section = "system", query = "", page = 0, sort = "", direction = 1;
    element = dialog("NX Witness Server info",
      `<p class="small text-body-secondary">Imported snapshot · ${esc(date(info.importedAt))}. This is saved information from NX Camera Info, not a live server query.</p>` +
      `<div class="nx-tabs" role="group" aria-label="NX Witness sections">${nxSections.map(([id, name]) => `<button type="button" class="btn btn-sm btn-outline-secondary" data-nx-section="${id}" aria-pressed="${id === section}">${name}${id === "system" ? "" : ` (${info[id]?.length || 0})`}</button>`).join("")}</div>` +
      '<div class="nx-search mt-3"><label class="form-label">Search this section<input id="nx-search" class="form-control" type="search" autocomplete="off"></label></div><div id="nx-content" class="mt-3"></div><div id="nx-pages" class="tool-actions"></div>',
      async () => {}, "Close");
    element.classList.add("nx-dialog");
    element.querySelector(".close-dialog").hidden = true;
    const get = id => element.querySelector(`#nx-${id}`);
    const render = () => {
      if (closed || !info) return;
      element.querySelector(".nx-search").hidden = section === "system";
      if (section === "system") {
        get("content").innerHTML = `<h3>System</h3>${nxDetails(info.system)}`;
        get("pages").innerHTML = "";
        return;
      }
      const [, title, columns] = nxSections.find(([id]) => id === section);
      let rows = (info[section] || []).filter(row => JSON.stringify(row).toLowerCase().includes(query.toLowerCase()));
      if (sort) rows = [...rows].sort((a, b) => direction * nxValue(a[sort]).localeCompare(nxValue(b[sort]), undefined, { numeric: true }));
      page = Math.max(0, Math.min(page, Math.ceil(rows.length / 50) - 1));
      const shown = rows.slice(page * 50, (page + 1) * 50);
      get("content").innerHTML = `<h3>${title}</h3>` + (rows.length ? `<div class="table-responsive"><table class="table table-sm align-top"><caption class="visually-hidden">Imported NX Witness ${title.toLowerCase()}</caption><thead><tr>${columns.map(key => `<th scope="col" aria-sort="${sort === key ? direction === 1 ? "ascending" : "descending" : "none"}"><button type="button" class="btn btn-sm text-nowrap" data-nx-sort="${key}">${esc(label(key))}${sort === key ? direction === 1 ? " ↑" : " ↓" : ""}</button></th>`).join("")}<th scope="col">Details</th></tr></thead><tbody>${shown.map(row => `<tr>${columns.map(key => `<td><span class="nx-value">${esc(nxValue(row[key]))}</span></td>`).join("")}<td><details><summary>Details</summary>${nxDetails(row, columns)}</details></td></tr>`).join("")}</tbody></table></div>` : `<p class="text-body-secondary">${query ? "No matching records." : `No ${title.toLowerCase()} in this export.`}</p>`);
      get("pages").innerHTML = `<button type="button" class="btn btn-sm btn-outline-secondary" id="nx-prev" ${page === 0 ? "disabled" : ""}>Previous</button><span>${rows.length ? `${page * 50 + 1}–${Math.min((page + 1) * 50, rows.length)}` : "0"} of ${rows.length}</span><button type="button" class="btn btn-sm btn-outline-secondary" id="nx-next" ${(page + 1) * 50 >= rows.length ? "disabled" : ""}>Next</button>`;
      get("prev").onclick = () => { page--; render(); };
      get("next").onclick = () => { page++; render(); };
      for (const button of element.querySelectorAll("[data-nx-sort]")) button.onclick = () => {
        direction = sort === button.dataset.nxSort ? -direction : 1;
        sort = button.dataset.nxSort;
        render();
      };
    };
    for (const button of element.querySelectorAll("[data-nx-section]")) button.onclick = () => {
      section = button.dataset.nxSection; query = ""; page = 0; sort = ""; direction = 1;
      get("search").value = "";
      for (const b of element.querySelectorAll("[data-nx-section]")) b.setAttribute("aria-pressed", String(b === button));
      render();
    };
    get("search").oninput = e => { query = e.target.value; page = 0; render(); };
    element.addEventListener("close", () => { closed = true; info = null; unsubscribe?.(); element.replaceChildren(); }, { once: true });
    render();
  } finally {
    if (!element || closed) { info = null; unsubscribe?.(); }
  }
}
