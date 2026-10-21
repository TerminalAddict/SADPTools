import * as Backend from "../bindings/github.com/TerminalAddict/SADPTools/desktopservice.js";
import {
  operationAvailability,
  networkFromDevice,
} from "./configuration-view.js";

const $ = (id) => document.getElementById(id);
const textError = (e) => e?.message || String(e);
export function initConfiguration({
  getDevice,
  getSnapshot,
  getSettings,
  refresh,
  notice,
}) {
  let target,
    kind,
    busy = false,
    challenge;
  const dialog = $("operation-dialog");
  const result = (text, success = false) => {
    $("operation-result").textContent = text;
    $("operation-result").className =
      `small ${success ? "text-success" : "text-danger"}`;
  };
  function clearSecrets() {
    for (const id of [
      "operation-password",
      "operation-password-confirm",
      "recovery-code",
    ])
      $(id).value = "";
  }
  function setBusy(value) {
    busy = value;
    for (const el of $("operation-form").elements) el.disabled = value;
    $("operation-cancel").disabled = false;
    $("operation-cancel").hidden = !value;
  }
  function open(mode) {
    const d = getDevice();
    if (!d || busy) return;
    target = structuredClone(d);
    kind = mode;
    challenge = null;
    $("operation-form").reset();
    clearSecrets();
    result("");
    $("challenge-data").value = "";
    $("challenge-result").hidden = true;
    $("operation-rediscover").hidden = true;
    $("operation-title").textContent = {
      activate: "Activate device",
      update: "Configure network",
      recovery: "Password recovery",
    }[mode];
    $("operation-identity").textContent =
      `${d.model || d.deviceType || "Unknown model"} · ${d.ipv4 || "IP unknown"}\nMAC ${d.mac} · Serial ${d.serial || "unknown"}`;
    $("operation-description").textContent =
      mode === "activate"
        ? "Set the admin password for this inactive device. Firmware may impose its own password rules."
        : mode === "update"
          ? "Review the complete network configuration below, then enter the current admin password to apply it."
          : "Request recovery data or submit a support-provided code with a new admin password.";
    $("operation-interface").replaceChildren(
      ...[...new Set(d.interfaces.map((a) => a.name))].map(
        (name) => new Option(name, name),
      ),
    );
    const preferred = getSettings()?.interface;
    if (d.interfaces.some((a) => a.name === preferred))
      $("operation-interface").value = preferred;
    $("network-fields").hidden = mode !== "update";
    $("recovery-fields").hidden = mode !== "recovery";
    $("password-confirm-field").hidden = mode === "update";
    $("operation-password-confirm").required = mode !== "update";
    $("recovery-code").required = mode === "recovery";
    for (const id of [
      "network-ip",
      "network-mask",
      "network-gateway",
      "network-port",
      "network-dhcp",
    ])
      $(id).required = mode === "update";
    const n = networkFromDevice(d);
    $("network-ip").value = n.ipv4;
    $("network-mask").value = n.subnetMask;
    $("network-gateway").value = n.gateway;
    $("network-port").value = n.commandPort;
    $("network-dhcp").value = n.dhcp;
    $("operation-password-label").textContent =
      mode === "update" ? "Current admin password" : "New admin password";
    $("operation-password").autocomplete =
      mode === "update" ? "current-password" : "new-password";
    $("operation-submit").textContent = {
      activate: "Activate this device",
      update: "Apply network changes",
      recovery: "Reset this device’s password",
    }[mode];
    dialog.showModal();
  }
  async function execute(request) {
    setBusy(true);
    result("Waiting for the device…");
    try {
      const reply = await Backend.ConfigureDevice(target.id, request);
      const ok = ["acknowledged", "challenge"].includes(reply.outcome);
      result(
        `${reply.message}${reply.errorCode ? ` Device error code: ${reply.errorCode}.` : ""}`,
        ok,
      );
      if (reply.outcome === "challenge") {
        challenge = reply;
        $("challenge-data").value = reply.challenge;
        $("challenge-result").hidden = false;
      }
      $("operation-rediscover").hidden =
        !reply.sent || reply.outcome === "challenge";
    } catch (e) {
      result(textError(e));
    } finally {
      setBusy(false);
      await refresh();
    }
  }
  const base = (action) => ({
    action,
    mac: target.mac,
    interface: $("operation-interface").value,
    timeoutSeconds: getSettings()?.timeoutSeconds || 10,
    password: "",
    code: "",
    network: {
      ipv4: "",
      subnetMask: "",
      gateway: "",
      commandPort: 0,
      dhcp: false,
    },
  });
  $("operation-form").onsubmit = async (e) => {
    e.preventDefault();
    if (busy) return;
    if (
      kind !== "update" &&
      $("operation-password").value !== $("operation-password-confirm").value
    ) {
      result("The passwords do not match.");
      return;
    }
    const request = base(kind === "recovery" ? $("reset-method").value : kind);
    request.password = $("operation-password").value;
    if (kind === "recovery") request.code = $("recovery-code").value;
    if (kind === "update")
      request.network = {
        ipv4: $("network-ip").value.trim(),
        subnetMask: $("network-mask").value.trim(),
        gateway: $("network-gateway").value.trim(),
        commandPort: Number($("network-port").value),
        dhcp: $("network-dhcp").value === "true",
      };
    clearSecrets();
    await execute(request);
    request.password = "";
    request.code = "";
  };
  $("request-challenge").onclick = async () => {
    if (busy) return;
    challenge = null;
    $("challenge-data").value = "";
    $("challenge-result").hidden = true;
    clearSecrets();
    await execute(base($("challenge-method").value));
  };
  $("save-challenge").onclick = async () => {
    if (!challenge || busy) return;
    try {
      const path = await Backend.SaveRecoveryData(target.id, challenge);
      if (path) result("Recovery data saved. Keep this file private.", true);
    } catch (e) {
      result(textError(e));
    }
  };
  $("operation-cancel").onclick = () =>
    Backend.CancelOperation().catch((e) => result(textError(e)));
  $("operation-close").onclick = () => {
    if (!busy) dialog.close();
  };
  dialog.addEventListener("cancel", (e) => {
    if (busy) e.preventDefault();
  });
  dialog.addEventListener("close", () => {
    clearSecrets();
    $("challenge-data").value = "";
    challenge = null;
    target = null;
  });
  $("operation-rediscover").onclick = async () => {
    if (busy) return;
    dialog.close();
    try {
      await Backend.StartScan();
      await refresh();
    } catch (e) {
      notice(textError(e));
    }
  };
  $("activate-device").onclick = () => open("activate");
  $("network-device").onclick = () => open("update");
  $("recover-device").onclick = () => open("recovery");
  return () => {
    const d = getDevice(),
      state = getSnapshot();
    const a = operationAvailability(d, state.scanning || state.configuring);
    $("activate-device").disabled = !a.activate;
    $("network-device").disabled = !a.network;
    $("recover-device").disabled = !a.recovery;
    $("configuration-note").textContent = a.reason;
  };
}
