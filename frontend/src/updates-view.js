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
