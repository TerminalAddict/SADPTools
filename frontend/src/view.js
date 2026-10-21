export const activation = (value) =>
  value === true ? "Active" : value === false ? "Inactive" : "Unknown";
export const model = (device) => device.model || device.deviceType || "";
export function visibleDevices(devices, query, key, direction) {
  const q = query.trim().toLowerCase();
  const value = (d) =>
    key === "model"
      ? model(d)
      : key === "activated"
        ? activation(d.activated)
        : (d[key] ?? "");
  return devices
    .filter((d) =>
      [
        d.ipv4,
        model(d),
        d.mac,
        d.serial,
        d.firmware,
        d.name,
        activation(d.activated),
        ...(d.interfaces ?? []).map((a) => a.name),
      ]
        .join(" ")
        .toLowerCase()
        .includes(q),
    )
    .sort(
      (a, b) =>
        direction *
          String(value(a)).localeCompare(String(value(b)), undefined, {
            numeric: true,
            sensitivity: "base",
          }) || a.id.localeCompare(b.id),
    );
}
