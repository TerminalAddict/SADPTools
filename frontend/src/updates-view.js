export function updateControls(state, configuring = false) {
  const busy = ["checking", "downloading", "installing"].includes(state.status);
  return {
    check: !busy,
    download: state.status === "available" && state.supported,
    install: state.status === "ready" && state.supported,
    installDisabled: configuring,
    cancel: ["checking", "downloading", "ready"].includes(state.status),
    progress: state.status === "downloading",
    offered: Boolean(state.version),
    percent: state.size > 0 ? Math.min(100, Math.max(0, state.downloaded / state.size * 100)) : 0,
  };
}

// About shares the same updater state; installation still uses its explicit
// confirmation screen, including the active-device-operation guard.
export function aboutUpdateView(state) {
  let action = "check", label = "Check for updates";
  if (["available", "ready"].includes(state.status) && state.supported) {
    action = state.status === "available" ? "download" : "details";
    label = "Update now";
  } else if (state.status === "available") {
    action = "release";
    label = "View release ↗";
  } else if (state.status === "downloading") {
    action = "details";
    label = "View download";
  } else if (state.status === "checking") {
    label = "Checking…";
  } else if (state.status === "installing") {
    label = "Opening setup…";
  }
  return {
    action,
    label,
    disabled: ["checking", "installing"].includes(state.status),
    primary: ["download", "details"].includes(action),
    message: `${state.version ? `Version ${state.version} · ` : ""}${state.message || "Check GitHub for a new version."}`,
  };
}
