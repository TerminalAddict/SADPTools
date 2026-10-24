import "bootstrap/dist/css/bootstrap.min.css";
import "./style.css";
import "./toolkit.css";
import { initLiveViewWindow } from "./live-view-window.js";

initLiveViewWindow().catch((error) => {
  const note = document.getElementById("viewer-error");
  note.textContent =
    error?.message ||
    "Could not open Live View. Close this window and select Live View again.";
  note.hidden = false;
});
