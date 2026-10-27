import { dialog, esc, field, message } from "./toolkit-ui.js";

export function importSummary(preview) {
  return `<p>${esc(preview.format)} · ${esc(preview.customer)}</p><table class="table table-sm"><caption class="visually-hidden">Site import contents</caption><thead><tr><th scope="col">Included</th><th scope="col">Count</th></tr></thead><tbody>` +
    [["Cameras / devices", preview.devices],
      ["Devices with saved credentials", preview.devicesWithCredentials],
      ["Credential profiles", preview.credentials], ["Switches", preview.switches],
      ["Documents / plans", preview.documents], ["Attachments", preview.attachments],
      ["History entries", preview.history], ["Saved scans / reports", preview.runs]]
      .map(([label, count]) => `<tr><td>${esc(label)}</td><td>${esc(count)}</td></tr>`).join("") +
    `</tbody></table><ul>${(preview.warnings || []).map(text => `<li>${esc(text)}</li>`).join("")}</ul>` +
    "<p>Creates a separate site on this computer. Existing sites are preserved. Cameras will not be contacted or reconfigured.</p>";
}

function protectionFields(status) {
  if (status.unlocked || status.automatic || status.mode === "dpapi") return "";
  return `<p>${status.initialized ? "Unlock saved passwords on this computer." : "Choose a password to protect saved camera credentials on this computer."} This can differ from the site file password.</p>` +
    field(status.initialized ? "Saved passwords passphrase" : "New saved passwords passphrase", "localPassphrase", "", "password", status.initialized ? "required" : "required minlength=12") +
    (status.initialized ? "" : field("Repeat saved passwords passphrase", "localRepeat", "", "password", "required minlength=12"));
}

async function unlockForTransfer(API, values, status) {
  if (status.unlocked || status.automatic) return;
  if (!status.initialized && status.mode !== "dpapi" && values.localPassphrase !== values.localRepeat)
    throw Error("Saved passwords passphrases do not match.");
  await API.UnlockVault(values.localPassphrase || "");
}

export async function exportSiteDialog(API, siteID) {

  const needsVault = await API.SiteExportNeedsVault(siteID);
  const status = await API.VaultStatus();
  dialog("Export entire site",
    "<p>Includes all saved devices, known camera passwords, shared credentials, notes, switches, history, reports, plans and attachments. Save the encrypted <strong>.sadpsite</strong> file, then use <strong>Import site</strong> on your other laptop.</p>" +
    field("Site file password", "passphrase", "", "password", "required minlength=12") +
    field("Repeat site file password", "repeat", "", "password", "required minlength=12") +
    `<div class="transfer-protection">${needsVault ? protectionFields(status) : ""}</div>`,
    async (values, element) => {
      if (values.passphrase !== values.repeat) throw Error("Site file passwords do not match.");
      if (needsVault) {
        const current = await API.VaultStatus();
        if (!current.unlocked && !current.automatic && current.mode !== "dpapi" && !element.querySelector('[name="localPassphrase"]')) {
          element.querySelector(".transfer-protection").innerHTML = protectionFields(current);
          throw Error("Saved passwords locked while reviewing. Enter the passphrase to continue.");
        }
        await unlockForTransfer(API, values, current);
      }
      const path = await API.BackupSite(siteID, values.passphrase);
      if (path) setTimeout(() => message("Site exported", `Saved encrypted site to ${path}. Use Import site on your other laptop and enter the site file password.`), 0);
    }, "Save site file");
}

export function importSiteDialog(API, onImported) {
  dialog("Import site",
    "<p>Choose an encrypted <strong>.sadpsite</strong> file, an <strong>NX Camera Info JSON</strong> export, or SADPTools site JSON. You will review the contents before importing.</p><p>For NX Camera Info, export JSON with passwords revealed to include known passwords. A masked export imports inventory only. JSON files containing passwords should be kept private.</p>" +
    field("Site file password (.sadpsite only; leave blank for JSON)", "passphrase", "", "password"),
    async values => {
      const preview = await API.PrepareSiteImport(values.passphrase);
      if (!preview.token) return;
      let status;
      try { status = await API.VaultStatus(); }
      catch (error) { await API.CancelSiteImport(preview.token); throw error; }
      setTimeout(() => reviewImport(API, preview, status, onImported), 0);
    }, "Choose site file");
}

function reviewImport(API, preview, status, onImported) {
  const d = dialog("Review site import",
    field("Imported site name", "name", preview.name, "text", "required maxlength=200") +
    importSummary(preview) + `<div class="transfer-protection">${preview.needsVault ? protectionFields(status) : ""}</div>`,
    async (values, element) => {
      const current = await API.VaultStatus();
      if (preview.needsVault) {
        if (!current.unlocked && !current.automatic && current.mode !== "dpapi" && !element.querySelector('[name="localPassphrase"]')) {
          element.querySelector(".transfer-protection").innerHTML = protectionFields(current);
          throw Error("Saved passwords locked while reviewing. Enter the passphrase to continue.");
        }
        await unlockForTransfer(API, values, current);
      }
      const site = await API.CommitSiteImport(preview.token, values.name);
      await onImported(site);
    }, "Import as new site");
  d.addEventListener("close", () => { void API.CancelSiteImport(preview.token).catch(() => {}); }, { once: true });
}
