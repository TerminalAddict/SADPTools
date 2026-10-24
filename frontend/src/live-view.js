import * as API from "../bindings/github.com/TerminalAddict/SADPTools/liveviewservice.js";
import { message } from "./toolkit-ui.js";

export function initLiveView() {
  async function open(siteID, deviceID) {
    try {
      await API.Open(siteID, deviceID);
    } catch (e) {
      message(
        "Live View unavailable",
        e?.message || "Could not open the camera window.",
      );
    }
  }
  document.addEventListener(
    "camera:preview",
    (e) => void open(e.detail.siteID || "", e.detail.deviceID),
  );
  return { open };
}
