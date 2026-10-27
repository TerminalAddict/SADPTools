// Prefer explicit imported endpoints, then advertised ONVIF services. Only
// SADP/Hikvision devices can safely assume the Hikvision RTSP path.
export function defaultPreviewMethod(detail) {
  if (detail.savedPreview) return "saved";
  const d = detail.device;
  if (d.xaddrs?.length) return "onvif";
  if (
    /hikvision/i.test(d.vendor || "") ||
    d.protocols?.some((p) => p.toLowerCase() === "sadp")
  )
    return "hikvision";
  return "rtsp";
}
