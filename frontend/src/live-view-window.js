import * as API from "../bindings/github.com/TerminalAddict/SADPTools/liveviewservice.js";
import { Stream, Window } from "@wailsio/runtime";
import { CameraPlayer } from "./live-view-player.js";
import { esc, field, select, option, dialog } from "./toolkit-ui.js";
import * as Vault from "../bindings/github.com/TerminalAddict/SADPTools/toolkitservice.js";
import "./live-view.css";

export function initLiveViewWindow() {
  let panel,
    selected,
    conn,
    player,
    ticket = "",
    timer,
    generation = 0,
    startRequest = 0,
    polling = false;
  let lastFrame = 0,
    frames = 0,
    started = 0,
    statsAt = 0,
    statsFrames = 0,
    profileKey = "",
    info,
    busy = false,
    loginPrompt = null,
    authRequired = false;
  const get = (id) => panel?.querySelector(`#lv-${id}`);
  function status(text, spinning = false) {
    if (!panel) return;
    get("status").textContent = text;
    get("spinner").hidden = !spinning;
  }
  async function stop(
    message = "Stopped",
    preserveFailure = false,
    starting = false,
  ) {
    if (!starting) startRequest++;
    generation++;
    busy = false;
    clearInterval(timer);
    timer = null;
    const id = ticket;
    ticket = "";
    const old = conn;
    conn = null;
    old?.close();
    player?.close();
    player = null;
    if (panel) {
      get("play").disabled = false;
      get("play").hidden = false;
      get("play").textContent = "Resume / Retry";
      get("pause").disabled = true;
      get("stop").disabled = true;
      get("metrics").textContent = "";
    }
    if (!preserveFailure) {
      status(message);
    }
    if (id) await API.Stop(id).catch(() => {});
  }
  async function fail(message) {
    status(message);
    get("diagnostics").textContent = message;
    await stop("", true);
  }
  async function close() {
    await stop();
    await Window.Close();
  }
  function selectedCredential() {
    return selected.credentials[Number(get("credential").value)];
  }
  function askForCredentials() {
    if (loginPrompt) return;
    const saved = selectedCredential();
    const separate = get("separateONVIF").checked;
    const keepSaved = saved && separate && !authRequired;
    loginPrompt = dialog(
      "Camera sign-in",
      `<p>Enter credentials to start viewing ${esc(selected.device.ipv4)}. They stay in this viewer only and are cleared when it closes.</p>` +
        (keepSaved
          ? `<p>Video uses the selected saved credential.</p>`
          : field(
              "Username",
              "username",
              "admin",
              "text",
              'autocomplete="username" required',
            ) + field("Password", "password", "", "password")) +
        (separate
          ? field("ONVIF username", "onvifUsername", "", "text", "required") +
            field("ONVIF password", "onvifPassword", "", "password")
          : ""),
      async (values) => {
        if (!keepSaved) get("credential").value = "session";
        await start(values);
      },
      "Sign in and view",
    );
    loginPrompt.addEventListener(
      "close",
      () => {
        loginPrompt = null;
      },
      { once: true },
    );
    loginPrompt.querySelector('input[type="password"]')?.focus();
  }
  async function credentialsReady() {
    if (!selectedCredential()) return true;
    const state = await Vault.VaultStatus();
    if (state.unlocked || state.automatic) return true;
    if (loginPrompt) return false;
    loginPrompt = dialog(
      "Unlock saved credentials",
      `<p>${state.mode === "dpapi" ? "Saved credentials were locked manually. Unlock with your Windows account to view this camera." : "Enter your master passphrase to use this camera’s saved credentials."}</p>` +
        (state.mode === "dpapi"
          ? ""
          : field(
              "Master passphrase",
              "passphrase",
              "",
              "password",
              "required",
            )),
      async (v) => {
        await Vault.UnlockVault(v.passphrase || "");
        await start();
      },
      "Unlock and view",
    );
    loginPrompt.addEventListener(
      "close",
      () => {
        loginPrompt = null;
      },
      { once: true },
    );
    return false;
  }
  async function start(credentials = null) {
    const request = ++startRequest;
    await stop("", false, true);
    if (!panel || request !== startRequest) return;
    const sequence = generation;
    busy = true;
    get("play").disabled = true;
    get("stop").disabled = false;
    const values = Object.fromEntries(new FormData(get("form")));
    const credential = selected.credentials[Number(values.credential)];
    const options = {
      siteID: selected.siteID,
      deviceID: selected.id,
      method: values.method,
      profile: values.profile,
      channel: Number(values.channel),
      port: Number(values.port),
      url: values.url,
      streamPin: values.streamPin,
      onvifPin: values.onvifPin,
      clockOffset: Number(values.clockOffset),
      credentialSite: credential?.siteID || "",
      credentialRecord: credential?.recordID || "",
      separateONVIF: values.separateONVIF === "on",
    };
    status("Preparing camera preview…", true);
    try {
      if (!(await credentialsReady()) || request !== startRequest) return;
      authRequired = false;
      const pending = API.Start(
        options,
        credentials?.username || "",
        credentials?.password || "",
        credentials?.onvifUsername || "",
        credentials?.onvifPassword || "",
        credentials !== null,
      );
      get("form")
        .querySelectorAll('input[type="password"]')
        .forEach((p) => (p.value = ""));
      if (credentials) {
        credentials.password = "";
        credentials.onvifPassword = "";
      }
      const result = await pending;
      if (sequence !== generation || !panel) {
        await API.Stop(result.id);
        return;
      }
      ticket = result.id;
      get("play").hidden = true;
      started = statsAt = performance.now();
      frames = statsFrames = 0;
      lastFrame = 0;
      info = null;
      player = new CameraPlayer(
        get("video"),
        get("image"),
        () => {
          if (sequence !== generation) return;
          lastFrame = performance.now();
          frames++;
          status(
            options.method.includes("snapshot")
              ? "Snapshot refresh · not continuous video"
              : "Playing · audio off",
          );
          get("pause").disabled = false;
        },
        (message) => void fail(message),
      );
      conn = Stream("camera-preview");
      conn.onopen = () => {
        if (sequence === generation) conn.send(JSON.stringify({ id: ticket }));
      };
      conn.onmessage = (event) => {
        if (sequence !== generation) return;
        const bytes = new Uint8Array(event.data);
        if (bytes[0] === 0) {
          try {
            info = JSON.parse(new TextDecoder().decode(bytes.subarray(1)));
            player?.information(info);
          } catch {
            void fail("Invalid video metadata.");
          }
        } else player?.packet(bytes[0], bytes.subarray(1));
      };
      conn.onerror = () => {
        if (sequence === generation)
          void fail(
            "The private playback connection failed. Reopen Live View and retry.",
          );
      };
      conn.onclose = () => {
        if (sequence === generation && ticket) void poll(sequence, true);
      };
      timer = setInterval(() => {
        if (conn?.readyState === 1)
          conn.send(
            lastFrame && performance.now() - lastFrame < 3000
              ? "playing"
              : "heartbeat",
          );
        void poll(sequence);
      }, 1000);
      void poll(sequence);
    } catch (e) {
      if (sequence === generation)
        await fail(e?.message || "Could not start Live View.");
    } finally {
      if (request === startRequest) busy = false;
      if (panel && !ticket) get("play").disabled = false;
    }
  }
  async function poll(sequence, closed = false) {
    if (!ticket || polling || sequence !== generation) return;
    polling = true;
    try {
      const state = await API.Status(ticket);
      if (sequence !== generation || !panel) return;
      if (state.state === "error" || state.state === "stopped") {
        const signIn = state.errorCode === "authentication";
        await fail(state.message);
        if (signIn) {
          authRequired = true;
          askForCredentials();
        }
        return;
      }
      if (closed)
        return await fail(
          "Camera connection ended. Check the camera and use Retry.",
        );
      if (!lastFrame) status(state.message, true);
      const key = JSON.stringify(state.profiles);
      if (key !== profileKey && state.profiles?.length) {
        profileKey = key;
        const value = get("profile").value;
        get("profile").innerHTML =
          baseProfiles() +
          state.profiles
            .map((p) =>
              option(
                p.id,
                `${p.name || "Profile"} · ${p.codec} ${p.width || "?"}×${p.height || "?"}`,
                value,
              ),
            )
            .join("");
        get("profile").value = value;
      }
      const now = performance.now(),
        elapsed = (now - statsAt) / 1000;
      const fps = elapsed > 0 ? (frames - statsFrames) / elapsed : 0;
      get("metrics").textContent = info
        ? `${info.codec} · ${info.width} × ${info.height} · ${get("video").requestVideoFrameCallback || info.mime === "image/jpeg" ? `${fps.toFixed(1)} displayed frames/s` : "playback advancing"} · ${info.transport}`
        : "Waiting for stream information";
      statsAt = now;
      statsFrames = frames;
      get("diagnostics").textContent =
        `${state.credentialSource}\n${state.message}\nReceived ${(state.bytes / 1048576).toFixed(2)} MiB; ${state.fragments} media fragments.\n${frames} displayed frames in ${((now - started) / 1000).toFixed(1)} s.\nNo audio, recording or automatic retries. Camera encoding is unchanged.`;
      // Includes profile resolution time, while bounding an unsupported decoder's black screen.
      if (info && !lastFrame && now - started > 40000)
        await fail(
          "Video arrived but the system decoder displayed no frames. Try an H.264 sub stream.",
        );
      else if (lastFrame && now - lastFrame > 8000)
        await fail("Video stopped advancing. Check the network and use Retry.");
    } catch {
      if (sequence === generation)
        await fail("Could not read playback status.");
    } finally {
      polling = false;
    }
  }
  const baseProfiles = () =>
    option("sub", "Sub stream (recommended)", "sub") +
    option("main", "Main stream") +
    option("auto", "Auto · lowest resolution");
  async function open() {
    const boot = await API.Load();
    const { siteID, deviceID: id, detail } = boot;
    document.documentElement.dataset.bsTheme =
      boot.theme === "system"
        ? matchMedia("(prefers-color-scheme: dark)").matches
          ? "dark"
          : "light"
        : boot.theme || "dark";
    selected = { ...detail, siteID, id };
    profileKey = "";
    const d = detail.device,
      hasONVIF = d.xaddrs?.length > 0;
    panel = document.createElement("section");
    panel.className = "camera-preview";
    panel.innerHTML = `<form id="lv-form"><div class="dialog-heading"><div><h2>Live View</h2><div class="small text-body-secondary">${esc(d.name || d.model || "Camera")} · ${esc(d.ipv4)}</div></div><button id="lv-close" type="button" class="btn-close" aria-label="Close Live View"></button></div>
      <div id="lv-stage" class="camera-stage"><video id="lv-video" autoplay muted playsinline disablepictureinpicture></video><img id="lv-image" alt="Live camera preview" hidden></div>
      <div class="camera-status" role="status"><span id="lv-spinner" class="spinner-border spinner-border-sm" hidden></span><span id="lv-status">Opening camera preview…</span></div><div id="lv-metrics" class="small text-body-secondary mb-2"></div>
      <div class="camera-controls"><button id="lv-play" class="btn btn-primary" type="submit" hidden>Retry</button><button id="lv-pause" class="btn btn-outline-secondary" type="button" disabled>Pause</button><button id="lv-stop" class="btn btn-outline-secondary" type="button" disabled>Stop</button><button id="lv-fullscreen" class="btn btn-outline-secondary" type="button">Fullscreen</button><span class="small text-body-secondary">Audio off</span></div>
      <p class="small text-body-secondary mt-2 mb-0">Read-only preview. RTSP and HTTP may be unencrypted; use a trusted management network.</p><div class="camera-options mt-3">${select("Source", "method", option("onvif", "ONVIF video profiles", hasONVIF ? "onvif" : "hikvision") + option("hikvision", "Hikvision RTSP endpoint", hasONVIF ? "onvif" : "hikvision") + option("rtsp", "Custom RTSP / RTSPS URL") + option("mjpeg", "HTTP multipart MJPEG") + option("snapshot", "HTTP snapshot refresh (1 fps)") + option("onvif-snapshot", "ONVIF snapshot refresh (1 fps)"))}${select("Stream", "profile", baseProfiles())}</div>
      ${select("Credentials", "credential", option("session", "Enter for this session", detail.credentials.length === 1 ? "0" : "session") + detail.credentials.map((c, i) => option(String(i), c.label, detail.credentials.length === 1 ? "0" : "session")).join(""))}
<div class="mb-2"><button id="lv-login" class="btn btn-sm btn-outline-secondary" type="button">Change login</button><span class="small text-body-secondary ms-2">Session credentials clear when this window closes.</span></div>
      <details id="lv-advanced"><summary>Endpoint, channel and certificate settings</summary><div class="camera-options mt-2">${field("Hikvision channel", "channel", "1", "number", 'min="1" max="256"')}${field("Hikvision RTSP port (0 = advertised/default)", "port", d.rtspPort || "0", "number", 'min="0" max="65535"')}</div>
      ${field("Custom URL · selected camera IP only, no embedded credentials", "url", "", "text", 'autocomplete="off" placeholder="rtsp://camera-ip:554/path"')}
      <p class="small text-body-secondary">RTSP and HTTP are unencrypted. Use a trusted management network. ONVIF and streaming accounts may differ. No camera settings are changed.</p>
      <label class="form-check-label"><input name="separateONVIF" type="checkbox" class="form-check-input"> Use separate ONVIF credentials for this session</label><div class="camera-options">${field("ONVIF clock correction in seconds", "clockOffset", "0", "number", 'min="-300" max="300"')}</div>
      <div class="camera-options">${field("Approved stream certificate SHA-256 (session only)", "streamPin")}${field("Approved ONVIF certificate SHA-256 (session only)", "onvifPin")}</div><button id="lv-cert-stream" type="button" class="btn btn-sm btn-outline-secondary">Inspect custom stream certificate</button> <button id="lv-cert-onvif" type="button" class="btn btn-sm btn-outline-secondary">Inspect ONVIF certificate</button><pre id="lv-cert-result" class="tool-readable small"></pre><button id="lv-trust" type="button" class="btn btn-sm btn-outline-warning" hidden>Trust this exact fingerprint for this session</button>
      </details><details class="mt-2"><summary>Diagnostics</summary><pre id="lv-diagnostics" class="tool-readable small">Preview has not started.</pre></details></form>`;
    document.body.append(panel);
    for (const element of panel.querySelectorAll("[name]"))
      element.id = `lv-${element.name}`;
    get("close").onclick = () => void close();
    panel.addEventListener("cancel", (e) => {
      e.preventDefault();
      void close();
    });
    get("form").onsubmit = (e) => {
      e.preventDefault();
      if (!busy) {
        if (authRequired) askForCredentials();
        else void start();
      }
    };
    get("pause").onclick = () =>
      void stop("Paused · camera disconnected. Resume when ready.");
    get("stop").onclick = () => void stop();
    get("fullscreen").onclick = () => {
      get("stage")
        .requestFullscreen?.()
        .catch(() => status("Fullscreen is unavailable on this desktop."));
    };
    get("login").onclick = askForCredentials;
    get("credential").addEventListener("change", () => void start());
    get("profile").addEventListener("change", () => void start());
    get("separateONVIF").addEventListener("change", () => {
      if (get("separateONVIF").checked) askForCredentials();
    });
    get("method").addEventListener("change", () => {
      if (["rtsp", "mjpeg", "snapshot"].includes(get("method").value))
        get("advanced").open = true;
      get("streamPin").value = "";
      get("trust").hidden = true;
    });
    get("url").addEventListener("input", () => {
      get("streamPin").value = "";
      get("trust").hidden = true;
    });
    get("form").addEventListener("change", (event) => {
      if (["credential", "profile"].includes(event.target.name)) return;
      if (ticket || busy)
        void stop("Settings changed. Apply them to reconnect.");
    });
    for (const [button, onvif] of [
      ["cert-stream", false],
      ["cert-onvif", true],
    ])
      get(button).onclick = async () => {
        get("trust").hidden = true;
        get("cert-result").textContent =
          "Inspecting TLS certificate without credentials…";
        const inspectedPanel = panel,
          inspectedURL = get("url").value;
        try {
          const cert = await API.Certificate(get("url").value, onvif);
          if (
            panel !== inspectedPanel ||
            (!onvif && get("url").value !== inspectedURL)
          )
            return;
          get("cert-result").textContent =
            `${cert.authority}\n${cert.subject}\nExpires: ${cert.expires}\nSHA-256: ${cert.sha256}\nVerify this fingerprint through a trusted channel before accepting.`;
          get("trust").hidden = false;
          get("trust").onclick = () => {
            get(onvif ? "onvifPin" : "streamPin").value = cert.sha256;
            get("trust").hidden = true;
          };
        } catch (e) {
          if (panel)
            get("cert-result").textContent =
              e?.message || "Certificate inspection failed.";
        }
      };
    await start();
  }
  window.addEventListener("beforeunload", () => {
    conn?.close();
    player?.close();
  });
  return open();
}
