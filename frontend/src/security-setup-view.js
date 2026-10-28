export function setupReady(status) {
  return (
    ["questions_ready", "questions_configured"].includes(status?.outcome) &&
    /^[a-f0-9]{64}$/.test(status?.questionRevision || "") &&
    status.questions?.length === 15 &&
    new Set(status.questions.map((q) => q.id)).size === 15 &&
    status.questions.every(
      (q) => Number.isInteger(q.id) && q.id >= 1 && q.id <= 15 && q.text,
    )
  );
}

export function setupAfterActivation(action, reply) {
  return (
    action === "activate" &&
    reply?.sent === true &&
    reply.outcome === "acknowledged"
  );
}

export function setupRequest(status, ids, answers, password, replaceQuestions) {
  if (!setupReady(status))
    throw new Error("Check the camera's security questions before saving.");
  if (status.outcome === "questions_configured" && !replaceQuestions)
    throw new Error("Confirm replacement of the camera's existing questions.");
  if (
    ids.length !== 3 ||
    answers.length !== 3 ||
    new Set(ids).size !== 3 ||
    ids.some((id) => !status.questions.some((q) => q.id === id))
  )
    throw new Error("Select three different security questions.");
  const length = (s) => new TextEncoder().encode(s).length;
  if (!password || length(password) > 16)
    throw new Error(
      "Enter the current admin password (up to 16 UTF-8 bytes for this SADP profile).",
    );
  if (
    answers.some(
      (a) => !a || length(a) > 128 || /[\p{Cc}\uFFFE\uFFFF]/u.test(a),
    )
  )
    throw new Error(
      "Each answer must contain 1–128 UTF-8 bytes, without control characters.",
    );
  return {
    password,
    questionRevision: status.questionRevision,
    replaceQuestions: Boolean(replaceQuestions),
    answers: ids.map((id, i) => ({ id, answer: answers[i] })),
  };
}
