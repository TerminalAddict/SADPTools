import {
  dialog,
  field,
  select,
  option,
  check,
  esc,
  message,
} from "./toolkit-ui.js";

// The endpoint/settings identity deliberately excludes display-only name/location.
export function cameraCheckKey(v) {
  return JSON.stringify([
    v.site,
    v.address?.trim(),
    v.webPort,
    v.httpsPort,
    v.rtspPort,
    v.onvifURL?.trim(),
    v.method,
    v.timeout,
    v.credential,
    v.username,
  ]);
}
export function cameraSummary(result) {
  const d = result.device;
  return [
    ["Address", d.ipv4],
    ["Identity", result.identified ? "Read from camera" : "Unverified address"],
    ["Name", d.name],
    ["Manufacturer", d.vendor],
    ["Model", d.model],
    ["Serial", d.serial],
    ["MAC", d.mac],
    ["Firmware", d.firmware],
    ["Web interface", result.web?.url],
    ["RTSP port (reported / detected)", d.rtspPort],
  ];
}
export function cameraIPForm({
  API,
  sites,
  selectedSite,
  initialIP = "",
  unlock,
  afterSave,
}) {
  let checked = null,
    checkedKey = "",
    pending = null,
    stopped = false,
    closed = false,
    pollTimer,
    siteGeneration = 0;
  let pins = { webPin: "", onvifPin: "" };
  const d = dialog(
    "Add camera by IP",
    `<p>Connect to one camera through your computer’s route, including VPNs and remote subnets. No adapter selection or multicast discovery is needed.</p>` +
      `<fieldset id="camera-ip-settings">` +
      select(
        "Save to site",
        "site",
        sites
          .map((s) => option(s.id, s.name, selectedSite || sites[0]?.id))
          .join(""),
      ) +
      field(
        "Camera IPv4 address",
        "address",
        initialIP,
        "text",
        'required autocomplete="off" placeholder="Camera IP address"',
      ) +
      `<details><summary>Custom ports and identification</summary><div class="advanced-grid">` +
      field(
        "Web port (0 = default)",
        "webPort",
        "0",
        "number",
        'min="0" max="65535"',
      ) +
      field(
        "HTTPS port (0 = default)",
        "httpsPort",
        "0",
        "number",
        'min="0" max="65535"',
      ) +
      field(
        "RTSP port (0 = 554)",
        "rtspPort",
        "0",
        "number",
        'min="0" max="65535"',
      ) +
      field(
        "Check timeout (seconds)",
        "timeout",
        "30",
        "number",
        'min="5" max="60"',
      ) +
      `</div>` +
      field(
        "ONVIF service URL (optional)",
        "onvifURL",
        "",
        "text",
        'autocomplete="off" placeholder="http://camera-ip/onvif/device_service"',
      ) +
      select(
        "Read identity using",
        "method",
        option(
          "auto",
          "Automatic: ONVIF, then ISAPI when appropriate",
          "auto",
        ) +
          option("onvif", "ONVIF only") +
          option("isapi", "Hikvision-compatible ISAPI only"),
      ) +
      `</details>` +
      `<details class="my-2"><summary>Read-only sign-in (optional)</summary><p class="small">Use a saved camera account or enter a one-time login. ONVIF may require its own enabled account. No camera settings are changed. One-time passwords are cleared after the check and are not saved.</p>` +
      select(
        "Saved credential",
        "credential",
        option("", "None / one-time login", ""),
      ) +
      `<button type="button" id="camera-ip-unlock" class="btn btn-sm btn-outline-secondary mb-2">Unlock saved passwords</button>` +
      field("Username", "username", "", "text", 'autocomplete="off"') +
      field("Password", "password", "", "password") +
      `<p class="small text-body-secondary">A selected saved credential is assigned when the new camera is saved. For a one-time login, Live View may ask you to sign in again.</p></details></fieldset>` +
      `<div class="d-flex gap-2 my-3"><button id="camera-ip-check" class="btn btn-outline-primary" type="button">Check camera</button><button id="camera-ip-stop" class="btn btn-outline-secondary" type="button" hidden>Cancel check</button><span id="camera-ip-spinner" class="spinner-border spinner-border-sm align-self-center" hidden></span></div><p id="camera-ip-status" role="status" aria-live="polite">Enter an address and check its services.</p><div id="camera-ip-result"></div>` +
      field("Saved name (optional)", "name", "", "text", 'maxlength="200"') +
      field(
        "Physical location (optional)",
        "location",
        "",
        "text",
        'maxlength="1000"',
      ) +
      `<div id="camera-ip-unverified" hidden>${check("Save this address even though camera identity has not been verified", "allowUnverified")}</div>` +
      check("Open Live View after saving", "openLive", false),
    async (values) => {
      if (!checked || pending || checkedKey !== cameraCheckKey(values))
        throw Error("Check the current address and settings before saving");
      if (!checked.result.identified && !values.allowUnverified)
        throw Error(
          "Confirm saving this unverified address, or sign in and check again",
        );
      const saved = await API.SaveCameraIP({
        token: checked.token,
        siteID: values.site,
        name: values.name,
        location: values.location,
        allowUnverified: !!values.allowUnverified,
      });
      const canOpen =
        !!values.openLive &&
        !saved.record.removed &&
        saved.record.device.ipv4 === checked.result.device.ipv4;
      await afterSave(saved.record, canOpen);
      if (saved.existing)
        message(
          "Camera already saved",
          `The matching camera is already in this site at ${saved.record.device.ipv4}. Its saved address, notes and credentials were kept. Review Details if changes are needed.`,
        );
    },
    "Save camera",
  );
  const el = (id) => d.querySelector(`#camera-ip-${id}`);
  const input = (name) => d.querySelector(`[name="${name}"]`);
  const save = d.querySelector('[type="submit"]');
  const values = () =>
    Object.fromEntries(new FormData(d.querySelector("form")));
  const showError = (e) => {
    if (closed) return;
    const n = d.querySelector(".dialog-error");
    n.textContent = e?.message || String(e);
    n.hidden = false;
  };
  function invalidate(clear = true) {
    checked = null;
    save.disabled = true;
    input("allowUnverified").checked = false;
    el("unverified").hidden = true;
    if (clear) {
      el("result").replaceChildren();
      el("status").textContent = "Settings changed. Check the camera again.";
    }
  }
  function busy(active) {
    el("settings").disabled = active;
    el("check").disabled = active;
    el("stop").hidden = !active;
    el("spinner").hidden = !active;
    save.disabled = active || !checked;
  }
  async function loadProfiles() {
    const seq = ++siteGeneration;
    input("credential").innerHTML = option("", "None / one-time login", "");
    input("username").disabled = input("password").disabled = false;
    try {
      const v = await API.Site(input("site").value);
      if (closed || seq !== siteGeneration) return;
      input("credential").innerHTML += v.profiles
        .map((p) => option(p.id, `${p.label} · ${p.username}`))
        .join("");
    } catch (e) {
      showError(e);
    }
  }
  el("settings").addEventListener("input", (e) => {
    invalidate();
    if (["address", "webPort", "httpsPort", "onvifURL"].includes(e.target.name))
      pins = { webPin: "", onvifPin: "" };
  });
  input("site").onchange = () => {
    pins = { webPin: "", onvifPin: "" };
    input("password").value = "";
    invalidate();
    void loadProfiles();
  };
  input("credential").onchange = () => {
    invalidate();
    input("password").value = "";
    input("username").disabled = input("password").disabled =
      !!input("credential").value;
  };
  el("unlock").onclick = () => Promise.resolve(unlock()).catch(showError);
  el("stop").onclick = () => {
    stopped = true;
    pending?.cancel?.();
    el("status").textContent = "Cancelling camera check…";
  };
  el("check").onclick = async () => {
    if (!d.querySelector("form").reportValidity()) return;
    invalidate();
    stopped = false;
    d.querySelector(".dialog-error").hidden = true;
    const v = values(),
      key = cameraCheckKey(v);
    const request = {
      siteID: v.site,
      credentialID: v.credential || "",
      target: {
        address: v.address.trim(),
        webPort: Number(v.webPort),
        httpsPort: Number(v.httpsPort),
        rtspPort: Number(v.rtspPort),
        onvifURL: v.onvifURL.trim(),
        method: v.method,
        timeoutSeconds: Number(v.timeout),
        ...pins,
      },
    };
    busy(true);
    el("status").textContent =
      "Checking web, RTSP and camera identity through the system route…";
    pending = API.CheckCameraIP(
      request,
      v.credential ? "" : v.username || "",
      v.credential ? "" : v.password || "",
    );
    input("password").value = "";
    v.password = "";
    pollTimer = setInterval(async () => {
      try {
        const j = await API.Job();
        if (!closed && pending && j.kind === "Camera by IP" && j.running)
          el("status").textContent = j.message || "Checking camera…";
      } catch {}
    }, 500);
    try {
      const answer = await pending;
      if (closed || stopped) return;
      checked = answer;
      checkedKey = key;
      const r = answer.result;
      el("status").textContent = r.identified
        ? "Camera information received. Review and save below."
        : "Check complete; camera identity is unverified.";
      el("result").innerHTML = `<dl class="row small">${cameraSummary(r)
        .map(
          ([k, v]) =>
            `<dt class="col-sm-4">${esc(k)}</dt><dd class="col-sm-8 text-break">${esc(v ?? "") || "Unknown"}</dd>`,
        )
        .join(
          "",
        )}</dl><ul class="small">${r.evidence.map((e) => `<li><strong>${esc(e.service)} · ${esc(e.state)}</strong>: ${esc(e.message)}</li>`).join("")}</ul>`;
      const certificates = [["web", r.web]];
      if (
        r.onvif?.scheme === "https" &&
        (r.onvif.certificateSHA256 !== r.web?.certificateSHA256 ||
          new URL(r.onvif.url).origin !==
            new URL(r.web?.url || r.onvif.url).origin)
      )
        certificates.push(["onvif", r.onvif]);
      for (const [scope, ep] of certificates) {
        if (
          ep?.scheme !== "https" ||
          !ep.certificateSHA256 ||
          ep.certificateTrusted
        )
          continue;
        const block = document.createElement("div");
        const approved = pins[scope + "Pin"] === ep.certificateSHA256;
        block.className = `alert ${approved ? "alert-info" : "alert-warning"} small`;
        block.innerHTML = `<strong>${esc(scope === "web" ? "Web / camera API" : "ONVIF")} certificate</strong><p>${esc(ep.url)}</p><code class="text-break">${esc(ep.certificateSHA256)}</code><label class="d-block mt-2"><input type="checkbox" ${approved ? "checked" : ""}> I verified this fingerprint; trust this exact certificate for this check</label><p class="mb-0 mt-1">${approved ? "This exact certificate was approved for this check. Live View may ask for its own certificate approval." : "Check the camera again after approving. Credentials are never sent to an unapproved HTTPS certificate."}</p>`;
        block.querySelector("input").onchange = (e) => {
          pins[scope + "Pin"] = e.target.checked ? ep.certificateSHA256 : "";
          invalidate(false);
          el("status").textContent =
            "Certificate choice changed. Check the camera again.";
        };
        el("result").append(block);
      }
      el("unverified").hidden = r.identified;
    } catch (e) {
      if (!closed) {
        el("status").textContent = stopped
          ? "Check cancelled. Nothing was saved."
          : "Camera check could not complete.";
        if (!stopped) showError(e);
      }
    } finally {
      clearInterval(pollTimer);
      pending = null;
      if (!closed) busy(false);
    }
  };
  d.addEventListener("close", () => {
    closed = true;
    pending?.cancel?.();
    clearInterval(pollTimer);
    input("password").value = "";
    checked = null;
    pins = {};
  });
  save.disabled = true;
  void loadProfiles();
  input("address").focus();
  return d;
}
