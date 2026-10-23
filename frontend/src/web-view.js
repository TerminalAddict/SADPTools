export function detectedURL(device, endpoint) {
  if (!device?.ipv4 || !endpoint?.url) return "";
  try {
    const url = new URL(endpoint.url);
    const port = Number(url.port || (url.protocol === "https:" ? 443 : 80));
    if (
      !["http:", "https:"].includes(url.protocol) ||
      url.protocol !== `${endpoint.scheme}:` ||
      url.hostname !== device.ipv4 ||
      port !== endpoint.port ||
      url.username ||
      url.password ||
      url.pathname !== "/" ||
      url.search ||
      url.hash
    )
      return "";
    return url.href;
  } catch {
    return "";
  }
}

// Ignore late replies after selection/address changes. The backend separately
// validates the current inventory identity before opening any URL.
export function createWebLookup(check, changed) {
  let key = "",
    device = null,
    sequence = 0,
    state = { busy: false, result: null, error: "" };
  async function start(force) {
    if (!device) return;
    const current = ++sequence,
      id = device.id;
    state = { busy: true, result: null, error: "" };
    changed();
    try {
      const result = await check(id, force);
      if (current === sequence) state = { busy: false, result, error: "" };
    } catch (error) {
      if (current === sequence)
        state = {
          busy: false,
          result: null,
          error: error?.message || String(error),
        };
    } finally {
      if (current === sequence) changed();
    }
  }
  return {
    select(next, scan = "", blocked = false) {
      const nextKey = next
        ? JSON.stringify([next.id, next.ipv4, next.httpPort, scan])
        : "";
      if (nextKey === key) return;
      const selection = ++sequence;
      key = nextKey;
      device = next;
      state = { busy: false, result: null, error: "" };
      if (!next) return;
      if (blocked) {
        key = "";
        state.error = "Finish the device operation to check its web interface.";
        return;
      }
      queueMicrotask(() => {
        if (sequence === selection) void start(false);
      });
    },
    retry() {
      return start(true);
    },
    adopt(id, endpoint) {
      if (device?.id !== id || !detectedURL(device, endpoint)) return;
      ++sequence;
      state = {
        busy: false,
        result: {
          endpoint,
          message: `${endpoint.scheme.toUpperCase()} web interface detected.`,
        },
        error: "",
      };
      changed();
    },
    get() {
      return state;
    },
  };
}
