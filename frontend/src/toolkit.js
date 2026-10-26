import * as API from "../bindings/github.com/TerminalAddict/SADPTools/toolkitservice.js";
import { $, esc, dialog, field, message } from "./toolkit-ui.js";
import { initSwitches } from "./switches.js";
import { initAudit } from "./audit.js";
import { initProvision } from "./provision.js";
import { initScanner } from "./scanner.js";
import { initReconciliation } from "./reconciliation.js";
import { initAdvanced } from "./advanced.js";
import { initSites } from "./sites.js";
export function initToolkit({ getSnapshot, getSelected }) {
  let current = "live",
    lastCompletion = "";
  function error(e) {
    $("toolkit-error").textContent = e?.message || String(e);
    $("toolkit-error").hidden = false;
  }
  const sites = initSites({
    getSnapshot,
    showError: error,
    onSiteChanged: (site) => {
      $("site-context").textContent = site
        ? `Site: ${site.name}`
        : "No site selected";
    },
  });
  const pages = { sites: () => sites.refresh() };
  async function navigate(name) {
    current = name;
    $("live-view").hidden = name !== "live";
    $("toolkit-view").hidden = name === "live";
    $("toolkit-error").hidden = true;
    for (const button of document.querySelectorAll("[data-view]"))
      button.classList.toggle("active", button.dataset.view === name);
    for (const page of document.querySelectorAll(".tool-page"))
      page.hidden = page.id !== `${name}-page`;
    try {
      await pages[name]?.();
    } catch (e) {
      error(e);
    }
  }
  for (const button of document.querySelectorAll("[data-view]"))
    button.onclick = () => void navigate(button.dataset.view);
  $("toolkit-cancel").onclick = () => API.CancelJob();
  $("save-live-selected").onclick = () =>
    void sites.addLive(getSelected()).catch(error);
  $("save-detail-site").onclick = () => {
    const id = document.querySelector("tr.selected")?.dataset.id;
    if (id) void sites.addLive([id]).catch(error);
  };
  async function poll() {
    try {
      const job = await API.Job();
      $("toolkit-job").hidden = !job.running && !job.completed;
      $("toolkit-job-label").textContent = job.running
        ? `${job.kind}: ${job.message || "Working…"}`
        : job.error ||
          (job.cancelled ? "Operation cancelled" : "Operation complete");
      $("toolkit-cancel").hidden = !job.running;
      $("toolkit-job-spinner").hidden = !job.running;
      $("toolkit-job-progress").style.width =
        `${job.total ? Math.min(100, (100 * job.done) / job.total) : 0}%`;
      $("toolkit-job-count").textContent = job.total
        ? `${job.done} / ${job.total}`
        : "";
      if (job.completed && job.completed !== lastCompletion) {
        lastCompletion = job.completed;
        if (current === "sites") await sites.refresh();
        if (current === "advanced") await kit.advanced.completed();
        if (
          job.runID &&
          ["Site discovery", "ONVIF information"].includes(job.kind)
        )
          await kit.reconciliation.review(job.siteID, job.runID);
      }
    } catch (e) {
      if (current !== "live") error(e);
    }
  }
  setInterval(() => void poll(), 1000);
  const kit = {
    navigate,
    sites,
    pages,
    error,
    poll,
    addLive: (ids) => sites.addLive(ids).catch(error),
    getSnapshot,
    getSelected,
  };
  kit.scanner = initScanner(kit);
  kit.reconciliation = initReconciliation(kit);
  kit.provision = initProvision(kit);
  kit.audit = initAudit(kit);
  kit.switches = initSwitches(kit);
  kit.advanced = initAdvanced(kit);
  kit.openRun = async (site, run) => {
    if (run.kind === "audit") return kit.audit.openSaved(site, run);
    message(
      "Saved scan / report",
      `${run.at} · ${run.scope}\nOutcome: ${run.outcome}\n\n${JSON.stringify(JSON.parse(run.data), null, 2)}`,
    );
  };
  return kit;
}
