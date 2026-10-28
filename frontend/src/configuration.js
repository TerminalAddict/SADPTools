import * as Backend from "../bindings/github.com/TerminalAddict/SADPTools/desktopservice.js";
import {
  operationAvailability,
  operationInterfaces,
  networkFromDevice,
} from "./configuration-view.js";
import { questionsReady, questionRequest } from "./security-questions-view.js";
import {
  setupReady,
  setupRequest,
  setupAfterActivation,
} from "./security-setup-view.js";

const $ = (id) => document.getElementById(id);
const textError = (e) => e?.message || String(e);
export function initConfiguration({
  getDevice,
  getSnapshot,
  getSettings,
  refresh,
  notice,
  onWebEndpoint,
}) {
  let target,
    kind,
    busy = false,
    challenge,
    questionStatus = null,
    setupStatus = null;
  const dialog = $("operation-dialog");
  const result = (text, success = false) => {
    $("operation-result").textContent = text;
    $("operation-result").className =
      `small ${success ? "text-success" : "text-danger"}`;
  };
  function clearSecrets() {
    for (const input of $("security-questions-list").querySelectorAll("input"))
      input.value = "";
    for (const input of $("question-setup-list").querySelectorAll("input"))
      input.value = "";
    for (const id of [
      "operation-password",
      "operation-password-confirm",
      "recovery-code",
    ])
      $(id).value = "";
  }
  function setBusy(value) {
    busy = value;
    for (const el of $("operation-form").elements) el.disabled = value;
    $("operation-cancel").disabled = false;
    $("operation-cancel").hidden = !value;
    renderRecovery();
  }
  const usingQuestions = () =>
    kind === "recovery" && $("recovery-method").value === "questions";
  function renderRecovery() {
    const questions = usingQuestions();
    const setup = kind === "questions";
    const certificate =
      questions &&
      questionStatus?.state === "certificate_required" &&
      questionStatus?.endpoint?.scheme === "https" &&
      /^[a-f0-9]{64}$/i.test(questionStatus.endpoint.certificateSHA256);
    $("recovery-certificate").hidden = !certificate;
    $("recovery-fingerprint").textContent = certificate
      ? questionStatus.endpoint.certificateSHA256
      : "";
    $("trust-recovery-certificate").disabled = busy || !certificate;
    $("security-questions-fields").hidden = !questions;
    $("support-code-fields").hidden = questions;
    $("recovery-code").required = kind === "recovery" && !questions;
    for (const input of $("security-questions-list").querySelectorAll(
      "input",
    )) {
      input.required = questions && questionsReady(questionStatus);
      input.disabled = busy || !questions;
    }
    $("operation-submit").disabled =
      busy ||
      (questions && !questionsReady(questionStatus)) ||
      (setup && !setupReady(setupStatus));
    for (const input of $("question-setup-list").querySelectorAll(
      "input, select",
    )) {
      input.required = setup && setupReady(setupStatus);
      input.disabled = busy || !setup || !setupReady(setupStatus);
    }
    $("replace-questions-label").hidden =
      !setup || setupStatus?.outcome !== "questions_configured";
    $("replace-questions").required =
      setup && setupStatus?.outcome === "questions_configured";
    $("question-setup-later").disabled = busy;
    $("operation-privacy-note").textContent = setup
      ? "This saves recovery questions on this camera. It does not reset its password. The admin password and answers are encrypted and are not saved in preferences, inventory or logs."
      : questions
        ? "Answers and passwords are encrypted using the camera’s recovery protocol and are not saved in preferences or logs. Submit once; a missing reply does not prove the password stayed unchanged."
        : kind === "activate"
          ? "The activation password is encrypted using the camera’s SADP key exchange and is not saved in preferences or logs. Use a trusted management network. A missing final reply still requires rediscovery and verification."
          : kind === "update"
            ? "SADPTools checks the camera’s authentication requirements and preserves its other network settings. Your password is not sent as plaintext or saved in preferences or logs. Use a trusted management network; rediscover after applying changes."
            : "Support-code recovery uses experimental legacy formats that are not validated for modern encrypted firmware. Passwords and codes are not saved in preferences or logs. These legacy formats send them without encryption; use the camera’s supported recovery process when compatibility is unknown.";
  }
  async function checkQuestionSetup() {
    if (busy || !target || kind !== "questions") return;
    setupStatus = null;
    $("question-setup-list").replaceChildren();
    $("replace-questions").checked = false;
    $("question-setup-status").textContent =
      "Checking the camera's activation, support and configured questions…";
    result("");
    setBusy(true);
    try {
      setupStatus = await Backend.ConfigureDevice(
        target.id,
        base("getsecurityquestion"),
      );
      $("question-setup-status").textContent = setupStatus.message;
      if (setupReady(setupStatus)) {
        const selected = setupStatus.questions.filter((q) => q.configured);
        for (let i = 0; i < 3; i++) {
          const label = document.createElement("label");
          label.className = "form-label";
          label.htmlFor = `setup-question-${i}`;
          label.textContent = `Security question ${i + 1}`;
          const select = document.createElement("select");
          select.id = label.htmlFor;
          select.className = "form-select mb-2";
          select.append(
            new Option("Select a question", ""),
            ...setupStatus.questions.map(
              (q) => new Option(q.text, String(q.id)),
            ),
          );
          select.value = selected[i] ? String(selected[i].id) : "";
          const answerLabel = document.createElement("label");
          answerLabel.className = "form-label";
          answerLabel.htmlFor = `setup-answer-${i}`;
          answerLabel.textContent = `Answer ${i + 1}`;
          const input = document.createElement("input");
          input.id = answerLabel.htmlFor;
          input.type = "password";
          input.autocomplete = "off";
          input.maxLength = 128;
          input.className = "form-control mb-3";
          $("question-setup-list").append(label, select, answerLabel, input);
        }
      }
    } catch (e) {
      $("question-setup-status").textContent = textError(e);
    } finally {
      setBusy(false);
      await refresh();
    }
  }
  async function checkQuestions(certificateSHA256 = "") {
    if (busy || !target || !usingQuestions()) return;
    clearSecrets();
    questionStatus = null;
    $("security-questions-list").replaceChildren();
    $("security-questions-status").textContent =
      "Checking camera support and configured questions…";
    result("");
    setBusy(true);
    try {
      questionStatus = await Backend.CheckSecurityQuestions(
        target.id,
        $("operation-interface").value,
        certificateSHA256,
      );
      if (questionStatus.endpoint?.url)
        onWebEndpoint?.(target.id, questionStatus.endpoint);
      $("security-questions-status").textContent = questionStatus.message;
      if (questionsReady(questionStatus)) {
        for (const [index, question] of questionStatus.questions.entries()) {
          const label = document.createElement("label");
          label.htmlFor = `security-answer-${index}`;
          label.className = "form-label";
          label.textContent = question.text;
          const input = document.createElement("input");
          input.id = label.htmlFor;
          input.type = "password";
          input.autocomplete = "off";
          input.maxLength = 128;
          input.className = "form-control mb-3";
          $("security-questions-list").append(label, input);
        }
      }
    } catch (e) {
      $("security-questions-status").textContent = textError(e);
    } finally {
      setBusy(false);
      await refresh();
    }
  }
  function open(
    mode,
    selectedDevice = null,
    activationPassword = "",
    afterActivation = false,
    activationInterface = "",
  ) {
    const d = selectedDevice || getDevice();
    if (!d || busy) return;
    target = structuredClone(d);
    kind = mode;
    challenge = null;
    questionStatus = null;
    setupStatus = null;
    $("question-setup-list").replaceChildren();
    $("security-questions-list").replaceChildren();
    $("operation-form").reset();
    clearSecrets();
    result("");
    $("challenge-data").value = "";
    $("challenge-result").hidden = true;
    $("operation-rediscover").hidden = true;
    $("operation-title").textContent = {
      activate: "Activate device",
      update: "Configure network",
      recovery: "Password recovery",
      questions: "Set security questions",
    }[mode];
    $("operation-identity").textContent =
      `${d.model || d.deviceType || "Unknown model"} · ${d.ipv4 || "IP unknown"}\nMAC ${d.mac} · Serial ${d.serial || "unknown"}`;
    $("operation-description").textContent =
      mode === "activate"
        ? "Set an admin password of 8–16 bytes. SADPTools checks that the device is inactive, negotiates encryption, then sends activation once. Firmware may impose additional password rules."
        : mode === "update"
          ? "Review the IPv4 settings below, then enter the current admin password. SADPTools checks the camera again, preserves its HTTP/IPv6 settings and sends the change once. The salted SADP profile supports passwords up to 16 bytes."
          : mode === "questions"
            ? "Choose three different recovery questions and enter their answers. Saving requires the camera's current admin password."
            : "Answer the camera’s configured security questions, or use a support-provided recovery code.";
    $("activation-complete").hidden = !afterActivation;
    $("question-setup-later").hidden = !afterActivation;
    $("question-setup-later").textContent = "Set up later";
    const adapters = operationInterfaces(d);
    $("operation-connection").hidden = adapters.length <= 1;
    $("operation-connection").open = false;
    $("operation-interface").replaceChildren(
      new Option("Automatic", ""),
      ...adapters.map((name) => new Option(name, name)),
    );
    $("operation-interface").value = adapters.includes(activationInterface)
      ? activationInterface
      : "";
    $("network-fields").hidden = mode !== "update";
    $("recovery-fields").hidden = mode !== "recovery";
    $("question-setup-fields").hidden = mode !== "questions";
    $("password-confirm-field").hidden =
      mode === "update" || mode === "questions";
    $("operation-password-confirm").required =
      mode !== "update" && mode !== "questions";
    $("recovery-code").required = mode === "recovery";
    for (const id of [
      "network-ip",
      "network-mask",
      "network-gateway",
      "network-port",
      "network-dhcp",
    ])
      $(id).required = mode === "update";
    const n = networkFromDevice(d);
    $("network-ip").value = n.ipv4;
    $("network-mask").value = n.subnetMask;
    $("network-gateway").value = n.gateway;
    $("network-port").value = n.commandPort;
    $("network-dhcp").value = n.dhcp;
    $("operation-password-label").textContent =
      mode === "update" || mode === "questions"
        ? "Current admin password"
        : "New admin password";
    $("operation-password").autocomplete =
      mode === "update" || mode === "questions"
        ? "current-password"
        : "new-password";
    $("operation-password").value = activationPassword;
    $("operation-submit").textContent = {
      activate: "Activate this device",
      update: "Apply network changes",
      recovery: "Reset this device’s password",
      questions: "Save security questions",
    }[mode];
    renderRecovery();
    if (!dialog.open) dialog.showModal();
    if (mode === "recovery") void checkQuestions();
    if (mode === "questions") void checkQuestionSetup();
  }
  async function execute(request, questions = false) {
    setBusy(true);
    result(
      request.action === "activate"
        ? "Checking the device and negotiating encrypted activation…"
        : request.action === "update"
          ? "Checking the camera’s authentication and current network settings, then applying the change…"
          : request.action === "setsecurityquestion"
            ? "Rechecking the camera, encrypting and saving security questions…"
            : "Waiting for the device…",
    );
    try {
      const reply = questions
        ? await Backend.ResetWithSecurityQuestions(
            target.id,
            $("operation-interface").value,
            request,
          )
        : await Backend.ConfigureDevice(target.id, request);
      const ok = ["acknowledged", "challenge", "verified"].includes(
        reply.outcome,
      );
      result(
        `${reply.message}${reply.errorCode ? ` Device error code: ${reply.errorCode}.` : ""}`,
        ok,
      );
      if (reply.outcome === "challenge") {
        challenge = reply;
        $("challenge-data").value = reply.challenge;
        $("challenge-result").hidden = false;
      }
      $("operation-rediscover").hidden =
        !reply.sent || reply.outcome === "challenge";
      if (
        request.action === "setsecurityquestion" &&
        reply.outcome === "verified"
      ) {
        $("question-setup-status").textContent =
          "The camera reports the three selected recovery questions configured.";
        $("question-setup-later").textContent = "Done";
      }
      return reply;
    } catch (e) {
      result(textError(e));
    } finally {
      if (questions) {
        questionStatus = null;
        $("security-questions-status").textContent =
          "Check again before another submission. If the outcome is unconfirmed, verify the password through the camera login first.";
      }
      setBusy(false);
      await refresh();
    }
  }
  const base = (action) => ({
    action,
    mac: target.mac,
    interface: $("operation-interface").value,
    timeoutSeconds: getSettings()?.timeoutSeconds || 10,
    password: "",
    code: "",
    network: {
      ipv4: "",
      subnetMask: "",
      gateway: "",
      commandPort: 0,
      dhcp: false,
    },
  });
  $("operation-form").onsubmit = async (e) => {
    e.preventDefault();
    if (busy) return;
    if (
      kind === "activate" &&
      new TextEncoder().encode($("operation-password").value).length > 16
    ) {
      result(
        "SADP activation requires an 8–16 byte password. Longer passwords are not truncated.",
      );
      return;
    }
    if (
      kind !== "update" &&
      kind !== "questions" &&
      $("operation-password").value !== $("operation-password-confirm").value
    ) {
      result("The passwords do not match.");
      return;
    }
    if (kind === "questions") {
      let request;
      try {
        request = {
          ...base("setsecurityquestion"),
          ...setupRequest(
            setupStatus,
            [0, 1, 2].map((i) => Number($(`setup-question-${i}`)?.value)),
            [0, 1, 2].map((i) => $(`setup-answer-${i}`)?.value || ""),
            $("operation-password").value,
            $("replace-questions").checked,
          ),
        };
      } catch (e) {
        result(textError(e));
        return;
      }
      clearSecrets();
      setupStatus = null;
      try {
        await execute(request);
      } finally {
        request.password = "";
        for (const answer of request.answers) answer.answer = "";
        renderRecovery();
      }
      return;
    }
    if (usingQuestions()) {
      let request;
      try {
        request = questionRequest(
          questionStatus,
          [...$("security-questions-list").querySelectorAll("input")].map(
            (input) => input.value,
          ),
          $("operation-password").value,
        );
      } catch (e) {
        result(textError(e));
        return;
      }
      clearSecrets();
      try {
        await execute(request, true);
      } finally {
        request.password = "";
        for (const answer of request.answers) answer.answer = "";
      }
      return;
    }
    const request = base(kind === "recovery" ? $("reset-method").value : kind);
    request.password = $("operation-password").value;
    if (kind === "recovery") request.code = $("recovery-code").value;
    if (kind === "update")
      request.network = {
        ipv4: $("network-ip").value.trim(),
        subnetMask: $("network-mask").value.trim(),
        gateway: $("network-gateway").value.trim(),
        commandPort: Number($("network-port").value),
        dhcp: $("network-dhcp").value === "true",
      };
    clearSecrets();
    try {
      const reply = await execute(request);
      if (setupAfterActivation(request.action, reply))
        open(
          "questions",
          target,
          request.password,
          true,
          reply.interface || request.interface,
        );
    } finally {
      request.password = "";
      request.code = "";
    }
  };
  $("request-challenge").onclick = async () => {
    if (busy) return;
    challenge = null;
    $("challenge-data").value = "";
    $("challenge-result").hidden = true;
    clearSecrets();
    await execute(base($("challenge-method").value));
  };
  $("save-challenge").onclick = async () => {
    if (!challenge || busy) return;
    try {
      const path = await Backend.SaveRecoveryData(target.id, challenge);
      if (path) result("Recovery data saved. Keep this file private.", true);
    } catch (e) {
      result(textError(e));
    }
  };
  $("operation-cancel").onclick = () =>
    Backend.CancelOperation().catch((e) => result(textError(e)));
  $("operation-close").onclick = () => {
    if (!busy) {
      clearSecrets();
      dialog.close();
    }
  };
  dialog.addEventListener("cancel", (e) => {
    if (busy) e.preventDefault();
    else clearSecrets();
  });
  dialog.addEventListener("close", () => {
    clearSecrets();
    $("challenge-data").value = "";
    challenge = null;
    questionStatus = null;
    setupStatus = null;
    $("question-setup-list").replaceChildren();
    $("security-questions-list").replaceChildren();
    target = null;
  });
  $("operation-rediscover").onclick = async () => {
    if (busy) return;
    clearSecrets();
    dialog.close();
    try {
      await Backend.StartScan();
      await refresh();
    } catch (e) {
      notice(textError(e));
    }
  };
  $("activate-device").onclick = () => open("activate");
  $("network-device").onclick = () => open("update");
  $("recover-device").onclick = () => open("recovery");
  $("setup-questions-device").onclick = () => open("questions");
  $("check-question-setup").onclick = () => checkQuestionSetup();
  $("question-setup-later").onclick = () => {
    if (!busy) {
      clearSecrets();
      dialog.close();
    }
  };
  $("check-security-questions").onclick = () => checkQuestions();
  $("trust-recovery-certificate").onclick = () =>
    checkQuestions(questionStatus?.endpoint?.certificateSHA256 || "");
  $("recovery-method").onchange = () => {
    clearSecrets();
    result("");
    questionStatus = null;
    $("security-questions-list").replaceChildren();
    renderRecovery();
    if (usingQuestions()) void checkQuestions();
  };
  $("operation-interface").onchange = () => {
    if (usingQuestions()) void checkQuestions();
    if (kind === "questions") {
      clearSecrets();
      void checkQuestionSetup();
    }
  };
  return () => {
    const d = getDevice(),
      state = getSnapshot();
    const a = operationAvailability(d, state.scanning || state.configuring);
    $("activate-device").disabled = !a.activate;
    $("network-device").disabled = !a.network;
    $("recover-device").disabled = !a.recovery;
    $("setup-questions-device").disabled = !a.network;
    $("configuration-note").textContent = a.reason;
  };
}
