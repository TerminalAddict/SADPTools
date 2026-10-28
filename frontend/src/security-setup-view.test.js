import test from "node:test";
import assert from "node:assert/strict";
import {
  setupReady,
  setupRequest,
  setupAfterActivation,
} from "./security-setup-view.js";

const status = (configured = false) => ({
  outcome: configured ? "questions_configured" : "questions_ready",
  questionRevision: "a".repeat(64),
  questions: Array.from({ length: 15 }, (_, i) => ({
    id: i + 1,
    text: `Synthetic question ${i + 1}`,
    configured: configured && i < 3,
  })),
});

test("activation advances only after acknowledgement; setup remains a separate explicit write", () => {
  assert.equal(
    setupAfterActivation("activate", { sent: true, outcome: "acknowledged" }),
    true,
  );
  for (const outcome of ["unconfirmed", "rejected", "not_sent"])
    assert.equal(
      setupAfterActivation("activate", { sent: true, outcome }),
      false,
    );
  assert.equal(
    setupAfterActivation("update", { sent: true, outcome: "acknowledged" }),
    false,
  );
  assert.equal(
    setupAfterActivation("activate", { sent: false, outcome: "acknowledged" }),
    false,
  );
});
test("setup requires a complete advertised profile and explicit replacement approval", () => {
  assert.equal(setupReady(status()), true);
  assert.equal(setupReady({ ...status(), outcome: "unsupported" }), false);
  assert.equal(
    setupReady({ ...status(), questions: status().questions.slice(1) }),
    false,
  );
  assert.throws(
    () =>
      setupRequest(
        status(true),
        [1, 2, 3],
        ["one", "two", "three"],
        "Synthetic<&>1",
        false,
      ),
    /replacement/,
  );
  assert.doesNotThrow(() =>
    setupRequest(
      status(true),
      [1, 2, 3],
      ["one", "two", "three"],
      "Synthetic<&>1",
      true,
    ),
  );
});
test("setup preserves exact answers and checks IDs and UTF-8 byte limits", () => {
  const answers = [" answer ", "Synthetic 漢字", "é".repeat(64)];
  const r = setupRequest(status(), [1, 2, 15], answers, "Synthetic<&>1", false);
  assert.deepEqual(
    r.answers.map((a) => a.answer),
    answers,
  );
  assert.equal(r.questionRevision, "a".repeat(64));
  assert.throws(
    () => setupRequest(status(), [1, 1, 3], answers, "Synthetic<&>1", false),
    /different/,
  );
  assert.throws(
    () => setupRequest(status(), [1, 2, 16], answers, "Synthetic<&>1", false),
    /different/,
  );
  assert.throws(
    () =>
      setupRequest(
        status(),
        [1, 2, 3],
        ["one", "two", "é".repeat(65)],
        "Synthetic<&>1",
        false,
      ),
    /128/,
  );
  assert.throws(
    () => setupRequest(status(), [1, 2, 3], answers, "é".repeat(9), false),
    /16/,
  );
});
