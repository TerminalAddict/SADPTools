# Setting up security questions

After the camera acknowledges activation, the single-device desktop dialog opens
**Set security questions** as the next step. SADPTools checks the camera's current
activation state and question support before enabling Save. Choose three different
questions, enter their answers, then click **Save security questions**. This is a
separate, explicit write; activation never silently supplies answers.

The activation password stays in the dialog temporarily for this step, and the
same adapter is used. **Set up later** skips setup and clears the password. Skipping
or failing question setup does not ask the camera to undo activation. If activation
is rejected or unconfirmed, the dialog does not advance automatically. If the
camera has not yet reported active, wait and use **Check again**.

For an already active camera, use **Set security questions** in Device details and
enter its current admin password. Existing configured questions are selected, but
their answers cannot be read back. Replacing them requires all three new answers
and the **Replace this camera's existing security questions and answers** checkbox.
Batch provisioning does not configure recovery questions automatically.

## Compatibility and privacy

This uses native SADP multicast through one discovered adapter. The camera can be
on a different IP subnet if it is still reachable on the same Layer 2 network; its
web interface need not be reachable. It does not provide routed remote question
setup. [Password recovery](security-question-recovery.md) uses a separate HTTP(S)
protocol and can follow an IP route.

The supported setup profile advertises `SupportSecurityQuestion=true` and the
original complete set of question IDs 1–15. Labels express the meanings of that
profile's device-supplied English catalog. Unknown, incomplete, duplicate or custom
profiles stop before a write instead of guessing question meanings. The current
admin password must fit 1–16 UTF-8 bytes. Answers must each contain 1–128 UTF-8
bytes, without control characters. Case and spaces are preserved; firmware can
impose stricter limits. No password or answer is truncated.

Passwords and answers are encrypted for the camera. They do not enter preferences,
inventory, exports or logs. Form fields clear on submission, close or skip; the
temporary activation password is discarded when the follow-on step ends. Go and
JavaScript strings cannot promise complete memory erasure. Use a trusted management
network: SADP's legacy encryption is not authenticated TLS.

## Transaction and results

1. Send a fresh inquiry, requiring the target MAC, UUID, receiving adapter and a
   valid matching source IPv4. Require active status and advertised support.
2. Read `getsecurityquestion`, matching that source, adapter, UUID and type. A MAC
   may be absent, but a conflicting MAC is rejected. Validate the complete question
   list and configured flags. Bind the reviewed list to the MAC, serial, model,
   firmware and adapter using a revision hash.
3. On Save, repeat those read-only checks. Changed identity/settings or missing
   replacement confirmation stop before encryption negotiation or a write.
4. Exchange a fresh RSA-1024 PKCS#1 public key and decrypt the 32-byte hexadecimal
   ASCII challenge. Use its first 16 ASCII bytes as the AES-128 key. The admin
   password uses the activation format: challenge prefix plus password zero-filled
   to 16 bytes. Each answer uses raw UTF-8 zero-padded to a 16-byte boundary, without
   the challenge prefix or an extra padding block. Base64 the raw AES-ECB ciphertext.
5. Send `setsecurityquestion` once, with three `Question` entries containing `ID`,
   encrypted `Answer` and `mark=true`, plus the encrypted admin `Password`.
6. Require a correlated explicit success reply, then read the question list once
   more. **Verified** means the camera acknowledged setup and reports exactly the
   three selected question IDs configured. It does not mean that a reset or login
   was attempted to test the answers.

Cancellation, send errors, missing acknowledgements and failed readback after a
write remain **unconfirmed**. Check status before submitting again. Status readback
cannot verify the contents of answers, particularly when replacing answers for the
same question IDs. There are no automatic write retries, plaintext fallbacks or
password-reset tests. Password rejection and lockout receive explicit messages.

## Evidence and test checklist

Hikvision documents setting recovery questions after activation in its
[activation guide](https://enpinfo.hikvision.com/hkwsen/unzip/20230913171744_32005_doc/GUID-4AE35502-F0B0-48E6-A989-284CFE251449.html).
Wire behaviour was examined in the same publicly archived
[SADP library](https://github.com/themactep/hiktools/blob/4ba69e93ed68f0b3a0b5839a01bb0580bb8bfa17/gists/libsadp.so)
used for the activation/network investigation (SHA-256
`52bbb97c9445f4fc7aa4933b0a7619b2d75ec59f54583cf1e6907bf3afb0ad87`). Synthetic ASCII,
metacharacter and UTF-8 answer vectors were independently checked against its
encryption routine under isolated emulation. No proprietary SDK implementation or
binary is incorporated or distributed; the native Go implementation uses standard
library cryptography.

On 28 October 2026, a read-only check of an IPC-B261H-MU, V5.7.1 build 220610,
after the user reset it, confirmed inactive status, advertised question support and
the complete 15-question list with no configured questions. Its actual reply passed
the production question parser. No activation, password, question-setup or network
write was sent by the development agent. Synthetic tests cover encryption,
correlation, validation, replacement checks, cancellation, uncertainty, readback,
secret redaction and the desktop flow. Compilation and mocked browser checks do not
establish camera or Windows runtime compatibility.

For the user's physical test:

1. Run the new Windows test build and discover the reset camera. Select it by MAC.
2. Activate with the intended password. Confirm the next step opens and shows three
   question selectors. Enter the answers locally; do not send them in diagnostics.
3. Save once. Record the displayed outcome. If verified, close and reopen **Set
   security questions** to confirm the chosen IDs are still selected. Check the
   camera's own web interface where reachable.
4. If unconfirmed, use **Check again** before deciding whether another write is
   necessary. Preserve activation/setup outcomes separately.
5. Test skip and later setup as separate cases if desired. A password-reset test is
   a separate explicit action; this setup workflow never performs one itself.
