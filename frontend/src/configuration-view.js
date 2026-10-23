export function operationInterfaces(device) {
  return [
    ...new Set((device?.interfaces || []).map((a) => a.name).filter(Boolean)),
  ];
}

export function operationAvailability(device, busy) {
  const supported =
    !device?.identityConflict &&
    (!device?.protocols?.length || device.protocols.includes("sadp"));
  const ready =
    supported &&
    !!device?.mac &&
    operationInterfaces(device).length > 0 &&
    !busy;
  return {
    activate: ready && device.activated !== true,
    network: ready && device.activated !== false,
    recovery: ready && device.activated !== false,
    reason: busy
      ? "Wait for discovery or the current operation to finish."
      : !supported
        ? "Hikvision writes require an unambiguous SADP-discovered device; ONVIF support alone is insufficient."
        : !ready
          ? "Select a device with a MAC address and discovered adapter."
          : "Changes apply only to the selected device. Firmware support varies.",
  };
}
export function networkFromDevice(device) {
  return {
    ipv4: device.ipv4 || "",
    subnetMask: device.subnetMask || "",
    gateway: device.gateway || "",
    commandPort: device.commandPort ?? "",
    dhcp: device.dhcp == null ? "" : String(device.dhcp),
  };
}
