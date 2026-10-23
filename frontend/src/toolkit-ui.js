// Shared DOM helpers. All device/site strings are escaped before HTML insertion.
export const esc = (value) =>
  String(value ?? "").replace(
    /[&<>"']/g,
    (c) =>
      ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[
        c
      ],
  );
export const $ = (id) => document.getElementById(id);
export const option = (value, label, selected) =>
  `<option value="${esc(value)}" ${value === selected ? "selected" : ""}>${esc(label)}</option>`;
export const list = (value) =>
  String(value || "")
    .split(/[\n,]+/)
    .map((x) => x.trim())
    .filter(Boolean);
export const date = (value) =>
  value && !String(value).startsWith("0001-")
    ? new Date(value).toLocaleString()
    : "—";
export function dialog(title, html, submit, label = "Save") {
  const d = document.createElement("dialog");
  d.className = "tool-dialog";
  d.innerHTML = `<form><div class="dialog-heading"><h2>${esc(title)}</h2><button type="button" class="btn-close" aria-label="Close"></button></div><div class="dialog-error alert alert-danger" hidden></div>${html}<div class="d-flex gap-2 mt-3"><button class="btn btn-primary" type="submit">${esc(label)}</button><button class="btn btn-outline-secondary close-dialog" type="button">Cancel</button></div></form>`;
  const close = () => {
    d.querySelector("form").reset();
    d.close();
    d.remove();
  };
  d.querySelector(".btn-close").onclick = close;
  d.querySelector(".close-dialog").onclick = close;
  d.addEventListener("cancel", (e) => {
    e.preventDefault();
    close();
  });
  d.querySelector("form").onsubmit = async (e) => {
    e.preventDefault();
    const button = d.querySelector('[type="submit"]');
    button.disabled = true;
    const data = Object.fromEntries(new FormData(e.target));
    try {
      await submit(data, d);
      close();
    } catch (error) {
      const note = d.querySelector(".dialog-error");
      note.textContent = error?.message || String(error);
      note.hidden = false;
    } finally {
      button.disabled = false;
      for (const input of d.querySelectorAll('input[type="password"]'))
        input.value = "";
    }
  };
  document.body.append(d);
  d.showModal();
  return d;
}
export function field(label, name, value = "", type = "text", extra = "") {
  return `<label class="form-label d-block">${esc(label)}<input class="form-control mt-1" name="${esc(name)}" type="${esc(type)}" value="${esc(value)}" ${extra} ${type === "password" ? 'autocomplete="new-password"' : ""}></label>`;
}
export function area(label, name, value = "") {
  return `<label class="form-label d-block">${esc(label)}<textarea class="form-control mt-1" name="${esc(name)}" rows="3">${esc(value)}</textarea></label>`;
}
export function select(label, name, options) {
  return `<label class="form-label d-block">${esc(label)}<select class="form-select mt-1" name="${esc(name)}">${options}</select></label>`;
}
export function check(label, name, checked = false) {
  return `<label class="form-check-label d-block my-2"><input class="form-check-input me-2" type="checkbox" name="${esc(name)}" ${checked ? "checked" : ""}>${esc(label)}</label>`;
}
export function message(title, text) {
  const d = dialog(
    title,
    `<pre class="tool-readable">${esc(text)}</pre>`,
    async () => {},
    "Close",
  );
  d.querySelector(".close-dialog").hidden = true;
  return d;
}
export function contextMenu(event, actions) {
  event.preventDefault();
  document.querySelector(".tool-context")?.remove();
  const menu = document.createElement("div");
  menu.className = "tool-context";
  menu.style.left = `${Math.min(event.clientX, innerWidth - 230)}px`;
  menu.style.top = `${Math.min(event.clientY, innerHeight - actions.length * 36 - 12)}px`;
  for (const [label, fn] of actions) {
    const b = document.createElement("button");
    b.className = "btn btn-sm text-start";
    b.textContent = label;
    b.onclick = () => {
      menu.remove();
      fn();
    };
    menu.append(b);
  }
  document.body.append(menu);
  setTimeout(
    () =>
      document.addEventListener("click", () => menu.remove(), { once: true }),
    0,
  );
}
export function table(headers, rows) {
  return `<div class="tool-table-wrap"><table class="table table-sm table-hover"><thead><tr>${headers.map((h) => `<th>${esc(h)}</th>`).join("")}</tr></thead><tbody>${rows.join("")}</tbody></table></div>`;
}
