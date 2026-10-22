import test from "node:test";
import assert from "node:assert/strict";
import { questionsReady, questionRequest } from "./security-questions-view.js";

const checked = () => ({
  state: "ready",
  checkID: "synthetic-check",
  questions: [
    { id: 2, text: "Synthetic question A?" },
    { id: 7, text: "Synthetic question B?" },
    { id: 9, text: "Synthetic question C?" },
  ],
});
test("only complete checked question sets permit submission", () => {
  assert.equal(questionsReady(checked()), true);
  for (const state of [
    "unknown",
    "unsupported",
    "not_configured",
    "unavailable",
  ])
    assert.equal(questionsReady({ ...checked(), state }), false);
  assert.equal(questionsReady({ ...checked(), checkID: "" }), false);
  const duplicate = checked();
  duplicate.questions[1].id = 2;
  assert.equal(questionsReady(duplicate), false);
  const missing = checked();
  missing.questions[1].text = "";
  assert.equal(questionsReady(missing), false);
  assert.throws(() =>
    questionRequest(null, ["a", "b", "c"], "Synthetic-pass1"),
  );
});
test("answers retain whitespace and map to device IDs, with no optimistic result", () => {
  const status = checked();
  const answers = ["  synthetic a  ", "synthetic Māori", "<&synthetic>"];
  const request = questionRequest(status, answers, "Synthetic-pass1");
  assert.deepEqual(request.answers, [
    { id: 2, answer: answers[0] },
    { id: 7, answer: answers[1] },
    { id: 9, answer: answers[2] },
  ]);
  assert.equal(request.checkID, status.checkID);
  assert.equal(request.outcome, undefined);
  assert.throws(() =>
    questionRequest(status, ["a", "", "b"], "Synthetic-pass1"),
  );
});
