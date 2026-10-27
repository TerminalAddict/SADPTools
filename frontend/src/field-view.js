import { esc } from "./toolkit-ui.js";

export function diagnosticRows(run) {
  return (run?.steps || []).map(
    (s) =>
      `<tr><td>${esc(s.test)}<br><small>${esc(s.protocol)}</small></td><td>${esc(s.status)}</td><td>${esc(s.evidence)}${s.next ? `<br><small class="text-body-secondary">${esc(s.next)}</small>` : ""}</td><td>${esc(s.durationMS)} ms</td></tr>`,
  );
}
