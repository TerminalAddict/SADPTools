import * as API from "../bindings/github.com/TerminalAddict/SADPTools/toolkitservice.js";
import * as Backend from "../bindings/github.com/TerminalAddict/SADPTools/desktopservice.js";
import {
  $,
  esc,
  option,
  dialog,
  field,
  select,
  check,
  table,
  date,
} from "./toolkit-ui.js";
export function initProvision(kit) {
  let picked = new Set(),
    plan = null,
    adapters = [],
    candidate = "",
    source = "live";
  const perform = (fn) => Promise.resolve().then(fn).catch(kit.error);
  function devices() {
    return kit.getSnapshot().devices || [];
  }
  function selectedSaved() {
    const view = kit.sites.view();
    const ids = kit.sites.selected();
    if (!view || !ids.length) throw Error("Select saved site devices first.");
    const live = devices();
    picked.clear();
    for (const r of view.devices.filter((r) => ids.includes(r.id))) {
      const matches = live.filter(
        (d) =>
          d.mac &&
          d.mac === r.device.mac &&
          (!d.serial || !r.device.serial || d.serial === r.device.serial) &&
          !d.identityConflict,
      );
      if (matches.length !== 1)
        throw Error(
          `Rediscover and resolve the live identity for ${r.device.model || r.device.mac || r.id} before provisioning.`,
        );
      picked.add(matches[0].id);
    }
  }
  function read() {
    const f = $("provision-form");
    const v = Object.fromEntries(new FormData(f));
    return {
      ids: [...picked],
      interface: v.adapter,
      activate: !!v.activate,
      configureNetwork: !!v.network,
      dhcp: v.addressing === "dhcp",
      start: v.start,
      end: v.end,
      subnetMask: v.mask,
      gateway: v.gateway,
      commandPort: Number(v.port),
      timeoutSeconds: Number(v.timeout),
      saveVerified: !!v.save,
    };
  }
  async function render() {
    adapters = (await Backend.Interfaces()).filter((a) => a.suitable);
    const d = devices().find((d) => picked.has(d.id)) || devices()[0];
    const view = kit.sites.view();
    $("provision-page").innerHTML =
      `<div class="tool-heading"><div><h2>Bulk provisioning</h2><p>Preview assignments, approve the complete plan, then verify each device by SADP rediscovery. There is no batch rollback.</p></div></div><div class="tool-actions"><button id="provision-live-selection" class="btn btn-outline-primary">Use live selection</button><button id="provision-site-selection" class="btn btn-outline-primary">Use saved site selection</button><span>Site: ${esc(view?.site.name || "None — results will stay in this session")}</span></div><form id="provision-form" class="provision-settings"><div>${select("Send through adapter", "adapter", adapters.map((a) => option(a.name, `${a.name} · ${a.addresses.join(", ")}`, view?.site.interface || d?.interfaces?.[0]?.name)).join(""))}${check("Activate inactive cameras first", "activate", false)}${check("Configure network settings", "network", true)}${select("Address mode", "addressing", option("static", "Static IPv4") + option("dhcp", "DHCP (static fallback addresses are still required)"))}${field("First candidate IPv4", "start", candidate || "", "text")}${field("Last candidate IPv4", "end", candidate || "", "text")}</div><div>${field("Subnet mask", "mask", d?.subnetMask || "")}${field("Gateway (0.0.0.0 for none)", "gateway", d?.gateway || "")}${field("SDK port (0 = preserve each device's advertised port)", "port", 0, "number", "min=0 max=65535")}${field("Timeout per write (seconds)", "timeout", 10, "number", "required min=1 max=300")}${check("Save independently verified settings to the selected site", "save", false)}<p class="small text-body-secondary">Reserved, assigned, responding, local, gateway and subnet-boundary addresses are excluded. A failed ping is not proof of availability.</p></div></form><div id="provision-picks"></div><div class="tool-actions mt-3"><button id="provision-preview" class="btn btn-primary">Create dry-run preview</button><span id="provision-count"></span></div><div id="provision-plan"></div>`;
    $("provision-live-selection").onclick = () =>
      perform(async () => {
        picked = new Set(kit.getSelected());
        renderPicks();
      });
    $("provision-site-selection").onclick = () =>
      perform(async () => {
        selectedSaved();
        renderPicks();
      });
    $("provision-preview").onclick = () =>
      perform(async () => {
        plan = await API.PreviewProvision(kit.sites.active()?.id || "", read());
        renderPlan();
      });
    renderPicks();
    plan = await API.ProvisionStatus();
    renderPlan();
  }
  function renderPicks() {
    const rows = devices();
    $("provision-picks").innerHTML = table(
      [
        "Select",
        "Current IPv4",
        "Model",
        "MAC / serial",
        "Activation",
        "Compatibility",
      ],
      rows.map((d) => {
        const supported =
          d.protocols?.includes("sadp") && !d.identityConflict && !!d.mac;
        return `<tr data-device="${esc(d.id)}"><td><input class="form-check-input" type="checkbox" aria-label="Select ${esc(d.ipv4)}" ${picked.has(d.id) ? "checked" : ""} ${supported ? "" : "disabled"}></td><td>${esc(d.ipv4)}</td><td>${esc(d.model)}</td><td>${esc(d.mac)}<br><small>${esc(d.serial)}</small></td><td>${d.activated == null ? "Unknown" : d.activated ? "Active" : "Inactive"}</td><td>${supported ? "SADP; firmware support varies" : "Requires unambiguous SADP discovery"}</td></tr>`;
      }),
    );
    for (const tr of $("provision-picks").querySelectorAll("[data-device]"))
      tr.querySelector("input").onchange = (e) => {
        e.target.checked
          ? picked.add(tr.dataset.device)
          : picked.delete(tr.dataset.device);
        $("provision-count").textContent = `${picked.size} selected`;
      };
    $("provision-count").textContent = `${picked.size} selected`;
  }
  function renderPlan() {
    if (!$("provision-plan")) return;
    if (!plan?.id) {
      $("provision-plan").innerHTML = "";
      return;
    }
    const ready =
      !plan.blocked &&
      Date.parse(plan.expires) > Date.now() &&
      plan.items.some((r) => r.state === "pending");
    $("provision-plan").innerHTML =
      `<h3>Reviewed write plan</h3><p class="small">Adapter: ${esc(plan.options.interface)} · Created ${esc(date(plan.created))} · Preview expires ${esc(date(plan.expires))}</p><div class="alert alert-warning">${plan.warnings.map(esc).join("<br>")}</div>${table(
        [
          "Device",
          "Current → proposed",
          "Action order",
          "State",
          "Evidence / result",
        ],
        plan.items.map(
          (r) =>
            `<tr><td>${esc(r.before.model)}<br>${esc(r.before.mac)}</td><td>${esc(r.before.ipv4)} → ${esc(r.proposed.ipv4 || "unchanged")}<br><small>${esc(r.proposed.subnetMask)} ${esc(r.proposed.gateway)}${r.actions.includes("update") ? ` · DHCP ${r.proposed.dhcp ? "on" : "off"}` : ""}</small></td><td>${esc(r.actions.join(" → "))}</td><td><span class="queue-state queue-${esc(r.state)}">${esc(r.state === "unconfirmed" ? "Unconfirmed — manual verification required" : r.state)}</span></td><td>${esc(r.message)}${r.steps?.map((s) => `<br><small>${esc(s.action)}: ${esc(s.outcome)} · ${esc(s.message)}</small>`).join("") || ""}</td></tr>`,
        ),
      )}<div class="tool-actions mt-3"><button id="provision-approve" class="btn btn-danger" ${ready ? "" : "disabled"}>Review credentials and apply…</button><button id="provision-export" class="btn btn-outline-secondary">Export plan / result</button></div>`;
    $("provision-approve").onclick = approve;
    $("provision-export").onclick = () =>
      dialog(
        "Export provisioning report",
        select(
          "Format",
          "format",
          option("html", "HTML") +
            option("csv", "CSV") +
            option("json", "JSON"),
        ),
        (v) => API.ExportProvision(v.format),
        "Export",
      );
  }
  function approve() {
    const activate = plan.items.some((r) => r.actions.includes("activate"));
    dialog(
      "Approve explicit camera writes",
      `<p>${plan.items.length} selected cameras via <strong>${esc(plan.options.interface)}</strong>. Review the per-device assignments before proceeding.</p><p class="text-warning">Activation negotiates encryption; network changes use each camera's advertised authentication profile and preserve unrelated settings. Use a trusted management network. A timeout can mean a change happened; uncertain writes will not be retried.</p>${field("Current admin password (blank = saved per-device/site credentials)", "current", "", "password")}${activate ? field("New admin password for inactive cameras (8–16 bytes)", "password", "", "password", "required minlength=8 maxlength=16") + field("Repeat new password", "repeat", "", "password", "required") : ""}${field("Type APPLY to approve this exact plan", "confirmation", "", "text", "required")}${check("I am authorised to configure these selected cameras and reviewed all assignments", "approved")}`,
      async (v) => {
        if (!v.approved) throw Error("Confirm authorisation and review first.");
        if (activate && v.password !== v.repeat)
          throw Error("New passwords do not match.");
        await API.StartProvision(
          plan.id,
          v.confirmation,
          v.current,
          v.password || "",
        );
        await kit.poll();
        plan = await API.ProvisionStatus();
        renderPlan();
      },
      "Apply write plan once",
    );
  }
  document.addEventListener("toolkit:assign", (e) => {
    candidate = e.detail;
    void kit.navigate("provision");
  });
  kit.pages.provision = render;
  setInterval(() => {
    if (!$("provision-page").hidden && $("provision-plan"))
      void perform(async () => {
        plan = await API.ProvisionStatus();
        renderPlan();
      });
  }, 1200);
  return { render, renderPlan };
}
