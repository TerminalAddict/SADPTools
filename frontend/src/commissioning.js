import {
  esc,
  field,
  area,
  select,
  option,
  check,
  dialog,
  message,
  date,
} from "./toolkit-ui.js";
const choices = ["not-tested", "pass", "fail", "not-applicable"];
export function checklistRows(records, tests, old = []) {
  const by = new Map(old.map((x) => [`${x.deviceID}/${x.test}`, x]));
  return records.flatMap((r) =>
    tests.map((test) => {
      const saved = by.get(`${r.id}/${test}`) || {};
      return structuredClone({
        deviceID: r.id,
        test,
        result: "not-tested",
        source: "manual",
        at: "",
        notes: "",
        ...saved,
        evidenceIDs: saved.evidenceIDs || [],
      });
    }),
  );
}
export function commissioningPage(c) {
  const { q, on, button, table, docs, parse, save, perform, API } = c;
  const old = docs("commissioning")[0];
  const form = parse(old);
  let checks = checklistRows(
    c.records(),
    c.data.checks || [],
    form.checks || [],
  );
  const attachments = c.data.documents.filter((d) =>
    [
      "nvr-channels",
      "stream-diagnostic",
      "walk-test",
      "retention",
      "plan",
    ].includes(d.kind),
  );
  q("content").innerHTML =
    `<p>Build a dated handover from saved evidence and technician checks. A generated report is an immutable snapshot; subsequent scans do not change it.</p><form id="ad-checklist"><div class="advanced-grid">${field("Report title", "title", form.title || "Commissioning and handover", "text", "required maxlength=200")}${field("Technician", "technician", form.technician || "")}${field("Commissioning date", "date", form.date || new Date().toISOString().slice(0, 10), "date")}${field("Project reference", "project", form.project || "")}${select(
      "Branding template",
      "brandingID",
      option("", "None", form.brandingID) +
        docs("branding")
          .map((d) => option(d.id, d.title, form.brandingID))
          .join(""),
    )}</div><div class="advanced-actions">${button("branding", "Create branding template")}${button("evidence-checks", "Fill checks from saved evidence")}</div>${area("Executive summary", "summary", form.summary || "")}<div id="ad-checks"></div>${area("Acceptance / signoff notes", "signoff", form.signoff || "")}<button class="btn btn-primary" type="submit">Save commissioning checklist</button></form><h3 class="mt-4">Generate a report snapshot</h3><p class="small">Save your checklist first. Select supporting evidence and images below. Credentials and authenticated stream URLs are always excluded. Text redaction does not alter text visible inside screenshots or plan images; review selected images before client handover.</p><div class="report-evidence-grid">${attachments.map((d) => `<label class="form-check"><input class="form-check-input" type="checkbox" data-report-document="${esc(d.id)}"><span>${esc(d.title)} · ${esc(date(d.updated))}</span></label>`).join("")}</div><details class="mt-2"><summary>Selected image evidence</summary><div class="report-evidence-grid">${
      c.data.assets
        .filter((a) => a.kind === "snapshot")
        .map(
          (a) =>
            `<label class="form-check"><input class="form-check-input" type="checkbox" data-report-image="${esc(a.id)}">${esc(a.title)} · ${esc(date(a.created))}</label>`,
        )
        .join("") || "No saved snapshots. Capture them explicitly in Live View."
    }</div></details>${check("Redact customer address, IP/MAC/serial and sensitive network details", "ad-redact", true)}${button("generate-report", "Generate immutable report snapshot", "primary")}<h3 class="mt-4">Saved report snapshots</h3>${table(
      ["Title", "Generated", "Export"],
      docs("report").map(
        (d) =>
          `<tr><td>${esc(d.title)}</td><td>${esc(date(d.updated))}</td><td>${["pdf", "html", "csv", "json"].map((f) => `<button class="btn btn-sm btn-outline-secondary m-1" data-report="${esc(d.id)}" data-format="${f}">${f.toUpperCase()}</button>`).join("")}</td></tr>`,
      ),
    )}`;
  function draw() {
    q("checks").innerHTML = table(
      ["Device", "Check", "Result", "Source / evidence", "Notes"],
      checks.map(
        (x, i) =>
          `<tr><td>${esc(c.deviceName(x.deviceID))}</td><td>${esc(x.test)}</td><td><select class="form-select form-select-sm" aria-label="${esc(x.test)} result" data-check-result="${i}">${choices.map((v) => option(v, v, x.result)).join("")}</select></td><td><small>${esc(x.source)}${x.evidenceIDs.length ? ` · ${x.evidenceIDs.length} saved reference(s)` : ""}</small></td><td><input class="form-control form-control-sm" aria-label="Check notes" maxlength="4096" data-check-note="${i}" value="${esc(x.notes)}"></td></tr>`,
      ),
    );
    for (const el of q("checks").querySelectorAll("[data-check-result]"))
      el.onchange = () => {
        const x = checks[Number(el.dataset.checkResult)];
        x.result = el.value;
        x.source = "manual";
        x.at = new Date().toISOString();
        x.evidenceIDs = [];
        el.closest("tr").querySelector("small").textContent = "manual";
      };
    for (const el of q("checks").querySelectorAll("[data-check-note]"))
      el.oninput = () =>
        (checks[Number(el.dataset.checkNote)].notes = el.value);
  }
  draw();
  q("checklist").onsubmit = (e) => {
    e.preventDefault();
    const v = Object.fromEntries(new FormData(e.target));
    void perform(() => save("commissioning", v.title, { ...v, checks }, old));
  };
  on("evidence-checks", () => {
    for (const x of checks) {
      if (x.result !== "not-tested") continue;
      const match =
        x.test === "live-video"
          ? docs("stream-diagnostic").find(
              (d) =>
                d.deviceID === x.deviceID &&
                parse(d).status?.metrics?.firstPlayingMS != null,
            )
          : x.test === "motion-alarm"
            ? docs("walk-test").find(
                (d) => d.deviceID === x.deviceID && parse(d).result === "pass",
              )
            : null;
      if (match) {
        x.result = "pass";
        x.source = `saved ${match.kind}`;
        x.at = match.updated;
        x.evidenceIDs = [match.id];
      }
    }
    draw();
    message(
      "Evidence linked",
      "Only saved playback evidence and explicitly graded walk-tests were used. Focus, recording, night/IR, PTZ and credential handover remain manual checks.",
    );
  });
  on("branding", () =>
    dialog(
      "Reusable company branding",
      field("Template name", "title", "Company branding") +
        field("Company name", "company") +
        area("Business details", "details") +
        check("Choose a logo image after saving this form", "logo", false),
      async (v) => {
        let logoID = "";
        if (v.logo === "on") {
          const a = await API.ImportImage(c.site, "branding");
          logoID = a.id || "";
        }
        await save("branding", v.title, {
          company: v.company,
          details: v.details,
          logoID,
        });
      },
    ),
  );
  on("generate-report", async () => {
    if (!old) throw Error("Save the commissioning checklist first");
    const documentIDs = [
        ...q("content").querySelectorAll("[data-report-document]:checked"),
      ].map((x) => x.dataset.reportDocument),
      imageIDs = [
        ...q("content").querySelectorAll("[data-report-image]:checked"),
      ].map((x) => x.dataset.reportImage);
    await API.CreateReport({
      siteID: c.site,
      checklistID: old.id,
      documentIDs,
      imageIDs,
      redact: q("content").querySelector('[name="ad-redact"]').checked,
    });
    await c.refresh();
  });
  for (const b of q("content").querySelectorAll("[data-report]"))
    b.onclick = () =>
      void perform(async () => {
        const path = await API.ExportReport(
          c.site,
          b.dataset.report,
          b.dataset.format,
        );
        if (path) message("Report exported", path);
      });
}
