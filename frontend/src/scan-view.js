export function scanPresentation(snapshot, now = Date.now()) {
  const active = !!snapshot.scanning;
  const started = Date.parse(snapshot.scanStarted || "");
  const duration = Math.max(1, Number(snapshot.scanSeconds) || 1);
  const elapsed = Number.isFinite(started)
    ? Math.max(0, (now - started) / 1000)
    : 0;
  const counts = `${snapshot.scanSeen || 0} seen this scan · ${snapshot.scanNew || 0} new`;
  const outcome =
    snapshot.scanOutcome === "cancelled"
      ? "Scan cancelled"
      : snapshot.scanOutcome === "failed"
        ? "Scan failed"
        : "Scan complete";
  return {
    active,
    visible: active || !!snapshot.scanStarted,
    percent: Math.min(100, (elapsed / duration) * 100),
    label: active
      ? elapsed < duration
        ? "Listening for cameras…"
        : "Finishing scan…"
      : outcome,
    counts,
    timing: active
      ? `${Math.min(duration, elapsed).toFixed(1)} / ${duration} s`
      : "",
  };
}
