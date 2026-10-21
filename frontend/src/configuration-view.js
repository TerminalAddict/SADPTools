export function operationAvailability(device, busy) {
  const ready = !!device?.mac && !!device?.interfaces?.length && !busy;
  return {
    activate: ready && device.activated !== true,
    network: ready && device.activated !== false,
    recovery: ready && device.activated !== false,
    reason: busy
      ? "Wait for discovery or the current operation to finish."
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
