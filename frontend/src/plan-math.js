export const clamp = (v, a = 0, b = 1) => Math.max(a, Math.min(b, v));
export function normalisePoint(x, y, rect) {
  if (!(rect.width > 0 && rect.height > 0))
    throw Error("Plan image is not loaded");
  return {
    x: clamp((x - rect.left) / rect.width),
    y: clamp((y - rect.top) / rect.height),
  };
}
export function coveragePath(m, width = 1000, height = 1000) {
  const a = ((m.bearing - 90 - m.cone / 2) * Math.PI) / 180,
    b = ((m.bearing - 90 + m.cone / 2) * Math.PI) / 180,
    x = m.x * width,
    y = m.y * height,
    r = Math.min(width, height) * 0.11;
  return `M ${x} ${y} L ${x + Math.cos(a) * r} ${y + Math.sin(a) * r} A ${r} ${r} 0 0 1 ${x + Math.cos(b) * r} ${y + Math.sin(b) * r} Z`;
}
export class PlanHistory {
  constructor(value) {
    this.values = [structuredClone(value)];
    this.index = 0;
  }
  push(v) {
    this.values = this.values.slice(0, this.index + 1);
    this.values.push(structuredClone(v));
    if (this.values.length > 80) this.values.shift();
    this.index = this.values.length - 1;
  }
  undo() {
    this.index = Math.max(0, this.index - 1);
    return this.value();
  }
  redo() {
    this.index = Math.min(this.values.length - 1, this.index + 1);
    return this.value();
  }
  value() {
    return structuredClone(this.values[this.index]);
  }
}
