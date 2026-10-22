import { Browser, Events } from "@wailsio/runtime";
import * as Backend from "../bindings/github.com/TerminalAddict/SADPTools/desktopservice.js";
import { updateControls } from "./updates-view.js";

export function initUpdates({ getSnapshot, getSettings }) {
  const $ = (id) => document.getElementById(id);
  let state = { status: "idle" }, loading = false, again = false;
  const mb = (bytes) => `${(bytes / 1024 / 1024).toFixed(1)} MiB`;
  function render() {
    const controls = updateControls(state, getSnapshot().configuring);
    $("update-current").textContent = state.currentVersion || "—";
    $("update-message").textContent = state.message || "Check GitHub for a new version.";
    $("update-offer").textContent = state.version ? `Version ${state.version}` : "";
    $("update-offer").hidden = !controls.offered;
    $("update-check").disabled = !controls.check;
    $("update-download").hidden = !controls.download;
    $("update-install").hidden = !controls.install;
    $("update-install").disabled = controls.installDisabled;
    $("update-cancel").hidden = !controls.cancel;
    $("update-cancel").textContent = state.status === "ready" ? "Discard download" : "Cancel";
    $("update-progress").hidden = !controls.progress;
    $("update-progress").value = controls.percent;
    $("update-size").hidden = !controls.offered;
    $("update-size").textContent = controls.progress ? `${mb(state.downloaded)} of ${mb(state.size)}` : `Installer download: ${mb(state.size)}`;
    $("update-platform").hidden = state.supported !== false;
    $("update-install-note").hidden = !controls.install;
    $("update-device-busy").hidden = !controls.install || !controls.installDisabled;
    $("updates-open").textContent = state.status === "ready" ? "Update ready" : controls.offered ? "Update available" : "Updates";
    $("updates-open").classList.toggle("btn-outline-primary", controls.offered);
  }
  async function refresh() {
    if (loading) { again = true; return; }
    loading = true;
    try {
      do {
        again = false;
        state = await Backend.UpdateStatus();
        render();
      } while (again);
    } finally { loading = false; }
  }
  async function act(fn) {
    $("update-error").hidden = true;
    try { await fn(); }
    catch (error) {
      $("update-error").textContent = error?.message || String(error);
      $("update-error").hidden = false;
    }
    try { await refresh(); } catch { /* The app may be closing after setup starts. */ }
  }
  $("updates-open").onclick = () => {
    $("updates-dialog").showModal();
    void act(refresh);
  };
  $("updates-close").onclick = () => $("updates-dialog").close();
  $("update-check").onclick = () => void act(() => Backend.CheckForUpdates());
  $("update-download").onclick = () => void act(() => Backend.DownloadUpdate());
  $("update-cancel").onclick = () => void act(() => Backend.CancelUpdate());
  $("update-install").onclick = () => {
    $("update-install").disabled = true;
    void act(() => Backend.InstallUpdate());
  };
  $("update-release").onclick = () => void act(() => Browser.OpenURL(state.releaseURL || "https://github.com/TerminalAddict/SADPTools/releases"));
  Events.On("updates:changed", () => void act(refresh));
  return {
    render,
    async start() {
      await refresh();
      if (getSettings()?.checkUpdates && !state.currentVersion?.includes("-dev")) {
        await act(() => Backend.CheckForUpdates());
      }
    },
  };
}
