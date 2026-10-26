import {
  esc,
  field,
  area,
  select,
  option,
  dialog,
  message,
} from "./toolkit-ui.js";
const number = (v) => (v === "" || v == null ? null : Number(v));
export const defaultChannel = () => ({
  label: "Channel group",
  count: 1,
  mbps: null,
  lowMbps: null,
  highMbps: null,
  audioMbps: 0,
  hoursPerDay: 24,
  calendar: [],
  dutyCycle: 1,
  source: "estimate",
});
export function scenarioFrom(values, channels) {
  return {
    name: values.name,
    channels,
    capacity: Number(values.capacity),
    unit: values.unit,
    reserve: Number(values.reserve) / 100,
    overhead: Number(values.overhead) / 100,
    safetyMargin: Number(values.safetyMargin) / 100,
    targetDays: Number(values.targetDays),
  };
}
const fmt = (v, d = 2) => (v == null ? "Unknown" : Number(v).toFixed(d));
export async function retentionPage(c) {
  const { q, on, button, table, docs, parse, save, perform, API } = c;
  const scenarios = docs("retention");
  let old = null,
    channels = [defaultChannel()],
    draft = {
      name: "Storage scenario",
      capacity: 0,
      unit: "TB",
      reserve: 0.05,
      overhead: 0.05,
      safetyMargin: 0.1,
      targetDays: 30,
    };
  function render() {
    q("content").innerHTML =
      `<p>Plan recording capacity without changing any device settings. Use capacity available after RAID. Unknown bitrates produce no complete estimate.</p><form id="ad-retention-form"><div class="advanced-grid">${field("Scenario name", "name", draft.name)}${field("Recording capacity", "capacity", draft.capacity, "number", "min=0 step=any")}${select("Capacity unit", "unit", ["TB", "TiB", "GB", "GiB"].map((u) => option(u, u, draft.unit)).join(""))}${field("Filesystem / storage reserve (%)", "reserve", draft.reserve * 100, "number", "min=0 max=95 step=any")}${field("Container / recording overhead (%)", "overhead", draft.overhead * 100, "number", "min=0 max=200 step=any")}${field("Safety margin (%)", "safetyMargin", draft.safetyMargin * 100, "number", "min=0 max=200 step=any")}${field("Target recording days", "targetDays", draft.targetDays, "number", "min=0 max=36600 step=any")}</div><h3>Channels / groups</h3>${table(
        [
          "Group / source",
          "Count",
          "Low / typical / high Mb/s",
          "Schedule / audio",
          "Actions",
        ],
        channels.map(
          (x, i) =>
            `<tr><td>${esc(x.label)}<br><small>${esc(x.source)}</small></td><td>${x.count}</td><td>${fmt(x.lowMbps)} / ${fmt(x.mbps)} / ${fmt(x.highMbps)}</td><td>${x.calendar?.length ? "Weekly calendar" : `${x.hoursPerDay} h/day`} · ${fmt(x.dutyCycle * 100, 0)}% recording fraction<br>${x.audioMbps} Mb/s audio</td><td><button type="button" class="btn btn-sm btn-link" data-edit-channel="${i}">Edit</button> <button type="button" class="btn btn-sm btn-link" data-remove-channel="${i}">Remove</button></td></tr>`,
        ),
      )}<div class="advanced-actions">${button("add-bitrate", "Add channel group")}${button("prefill-bitrate", "Use verified NVR targets / stream measurements")}<button class="btn btn-primary" type="submit">Calculate</button>${button("save-scenario", "Save scenario to site")}</div><div id="ad-retention-result" class="mt-3"></div></form><h3 class="mt-4">Saved scenarios — compare</h3><div id="ad-scenario-comparison"></div>`;
    q("retention-form").onsubmit = (e) => {
      e.preventDefault();
      void perform(calculate);
    };
    on("add-bitrate", () => edit(-1));
    on("prefill-bitrate", prefill);
    on("save-scenario", async () => {
      const scenario = current();
      await API.CalculateRetention(scenario);
      await save("retention", scenario.name, scenario, old);
    });
    for (const b of q("content").querySelectorAll("[data-edit-channel]"))
      b.onclick = () => edit(Number(b.dataset.editChannel));
    for (const b of q("content").querySelectorAll("[data-remove-channel]"))
      b.onclick = () => {
        draft = current();
        channels.splice(Number(b.dataset.removeChannel), 1);
        render();
      };
    void perform(compare);
  }
  function current() {
    return scenarioFrom(
      Object.fromEntries(new FormData(q("retention-form"))),
      channels,
    );
  }
  function edit(i) {
    draft = current();
    const x = i >= 0 ? channels[i] : defaultChannel();
    dialog(
      "Recording channel / group",
      field("Label", "label", x.label) +
        field(
          "Number of identical channels",
          "count",
          x.count,
          "number",
          "min=1 max=100000 required",
        ) +
        select(
          "Bitrate provenance",
          "source",
          ["estimate", "configured", "measured"]
            .map((s) => option(s, s, x.source))
            .join(""),
        ) +
        field(
          "Average video bitrate Mb/s (blank = unknown)",
          "mbps",
          x.mbps ?? "",
          "number",
          "min=0 max=10000 step=any",
        ) +
        field(
          "Low video bitrate Mb/s (optional)",
          "lowMbps",
          x.lowMbps ?? "",
          "number",
          "min=0 max=10000 step=any",
        ) +
        field(
          "High video bitrate Mb/s (optional)",
          "highMbps",
          x.highMbps ?? "",
          "number",
          "min=0 max=10000 step=any",
        ) +
        field(
          "Additional audio Mb/s",
          "audioMbps",
          x.audioMbps,
          "number",
          "min=0 max=100 step=any",
        ) +
        field(
          "Scheduled hours per day",
          "hoursPerDay",
          x.hoursPerDay,
          "number",
          "min=0 max=24 step=any",
        ) +
        field(
          "Recorded fraction of scheduled time (%)",
          "duty",
          x.dutyCycle * 100,
          "number",
          "min=0 max=100 step=any",
        ) +
        field(
          "Optional Mon–Sun hours, seven comma-separated values",
          "calendar",
          (x.calendar || []).join(", "),
        ) +
        `<p class="small">For event recording, the recorded fraction is your activity estimate. Include pre/post-event buffers. Resolution alone does not determine bitrate.</p>`,
      async (v) => {
        const next = {
          label: v.label,
          count: Number(v.count),
          source: v.source,
          mbps: number(v.mbps),
          lowMbps: number(v.lowMbps),
          highMbps: number(v.highMbps),
          audioMbps: Number(v.audioMbps),
          hoursPerDay: Number(v.hoursPerDay),
          dutyCycle: Number(v.duty) / 100,
          calendar: v.calendar.trim() ? v.calendar.split(",").map(Number) : [],
        };
        const proposed = [...channels];
        if (i < 0) proposed.push(next);
        else proposed[i] = next;
        await API.CalculateRetention({ ...draft, channels: proposed });
        channels = proposed;
        render();
      },
    );
  }
  async function calculate() {
    const r = await API.CalculateRetention(current());
    q("retention-result").innerHTML =
      `<div class="advanced-stat-grid"><div><strong>${fmt(r.retentionDays)} days</strong><small>Typical estimated retention</small></div><div><strong>${fmt(r.lowRetentionDays)}–${fmt(r.highRetentionDays)} days</strong><small>Range from high/low VBR</small></div><div><strong>${fmt(r.bytesPerDay == null ? null : r.bytesPerDay / 1e9)} GB/day</strong><small>Including overhead and safety margin</small></div><div><strong>${fmt(r.requiredBytes == null ? null : r.requiredBytes / 1e12)} TB</strong><small>Required recording space for target</small></div></div><p class="small">Usable recording space: ${(r.usableBytes / 1e12).toFixed(3)} TB. Mb/s = million bits/s, MB/s = million bytes/s. TB is decimal; TiB is binary.</p><ul class="small">${r.warnings.map((w) => `<li>${esc(w)}</li>`).join("")}</ul>`;
  }
  async function compare() {
    const results = await Promise.all(
      scenarios.map(async (d) => {
        try {
          return { d, r: await API.CalculateRetention(parse(d)) };
        } catch {
          return { d, r: {} };
        }
      }),
    );
    q("scenario-comparison").innerHTML = table(
      ["Scenario", "Capacity", "GB/day", "Typical / range days", "Actions"],
      results.map(
        ({ d, r }) =>
          `<tr><td>${esc(d.title)}</td><td>${parse(d).capacity} ${esc(parse(d).unit)}</td><td>${fmt(r.bytesPerDay == null ? null : r.bytesPerDay / 1e9)}</td><td>${fmt(r.retentionDays)} / ${fmt(r.lowRetentionDays)}–${fmt(r.highRetentionDays)}</td><td><button class="btn btn-sm btn-link" data-load-scenario="${esc(d.id)}">Edit</button><button class="btn btn-sm btn-link" data-copy-scenario="${esc(d.id)}">Copy scenario</button></td></tr>`,
      ),
    );
    for (const b of q("scenario-comparison").querySelectorAll("button"))
      b.onclick = () => {
        const d = scenarios.find(
          (d) => d.id === (b.dataset.loadScenario || b.dataset.copyScenario),
        );
        old = b.dataset.loadScenario ? d : null;
        draft = structuredClone(parse(d));
        if (!old) draft.name += " copy";
        channels = draft.channels;
        render();
      };
  }
  function prefill() {
    const candidates = [];
    const seen = new Set();
    for (const d of docs("nvr-channels")) {
      if (seen.has(d.deviceID)) continue;
      seen.add(d.deviceID);
      for (const ch of parse(d).channels || [])
        for (const stream of ch.streams || [])
          if (stream.configuredKbps > 0)
            candidates.push({
              label: `${c.deviceName(d.deviceID)} · ch ${ch.id} stream ${stream.id}`,
              mbps: stream.configuredKbps / 1000,
              source: "configured",
            });
    }
    for (const d of docs("stream-diagnostic")) {
      const x = parse(d);
      if (x.status?.metrics?.rtpMbps > 0)
        candidates.push({
          label: `${c.deviceName(d.deviceID)} · ${x.profile} · measured ${x.at}`,
          mbps: x.status.metrics.rtpMbps,
          source: "measured",
        });
    }
    if (!candidates.length) {
      message(
        "No bitrate evidence",
        "Read NVR channels with advertised target bitrates, or save a stream diagnostic first. You can enter an estimate manually.",
      );
      return;
    }
    dialog(
      "Select bitrate evidence",
      `<p>Select the stream that will actually be recorded. RTP measurements include RTP headers; configured targets are not measured averages. Add additional groups as needed.</p>` +
        select(
          "Evidence",
          "index",
          candidates
            .map((x, i) =>
              option(
                String(i),
                `${x.label} · ${x.mbps.toFixed(2)} Mb/s · ${x.source}`,
              ),
            )
            .join(""),
        ),
      (v) => {
        draft = current();
        channels.push({ ...defaultChannel(), ...candidates[Number(v.index)] });
        render();
      },
    );
  }
  render();
}
