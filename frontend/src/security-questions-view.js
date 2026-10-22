// Availability is based on a fresh backend check, never on model-name guesses.
export function questionsReady(status) {
  const questions = status?.questions;
  return (
    status?.state === "ready" &&
    !!status.checkID &&
    Array.isArray(questions) &&
    questions.length === 3 &&
    new Set(questions.map((q) => q.id)).size === 3 &&
    questions.every(
      (q) =>
        Number.isInteger(q.id) &&
        q.id > 0 &&
        typeof q.text === "string" &&
        q.text.length > 0,
    )
  );
}

export function questionRequest(status, answers, password) {
  if (
    !questionsReady(status) ||
    answers.length !== 3 ||
    answers.some((a) => !a)
  ) {
    throw new Error(
      "Check availability and answer all three configured questions.",
    );
  }
  return {
    checkID: status.checkID,
    answers: status.questions.map((q, i) => ({ id: q.id, answer: answers[i] })),
    password,
  };
}
