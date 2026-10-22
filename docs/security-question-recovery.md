# Security-question password recovery

Select a discovered, activated camera and open **Password recovery**. The default
**Security questions** method checks the camera's local recovery interface and
loads its three configured questions. Enter the answers exactly as originally
set, enter and confirm a new admin password, then select **Reset this device's
password**. Opening the dialog and **Check again** only read availability and
question text; they do not validate answers or reset anything.

The camera needs a usable advertised IPv4 address and HTTP port, plus an active
discovered adapter on that IPv4 subnet. The application chooses that adapter
automatically, including a suitable secondary IPv4 address when available.
The connection uses that source address without a proxy. SADP visibility across
subnets alone does not establish IP connectivity. Firewall rules and network
isolation can still prevent the HTTP connection.

The app distinguishes ready, unsupported, not configured, unknown and unavailable
responses. It never assumes that questions are configured because a model usually
supports them. Unknown question labels prevent submission. The existing
**Support-provided code** method remains available separately.

## Submission and results

- Answers are not trimmed or case-folded. They are limited to 128 Unicode
  characters and 512 UTF-8 bytes. New passwords use the existing validation;
  firmware may impose stricter rules.
- A GUI availability check binds one submission to the inventory device,
  address, port, adapter/source address and question IDs. It expires after five
  minutes. The backend consumes it before submission and checks the camera's
  selected questions again. Changing connections requires another check.
- The encrypted reset is sent once, without retries. Cancelling or timing out
  after submission leaves the outcome **unconfirmed**. Verify through the camera
  login before trying again; cancellation cannot undo a reset.
- A recognised success response is reported as **acknowledged**, not proof that
  a subsequent login works. Recognised rejection/lockout responses show only
  safe status information and reported attempts/time remaining. Raw device
  replies are never displayed or logged.
- Answers, passwords, RSA keys and recovery challenges are not stored in
  preferences, inventory, exports or logs. Input fields clear on submission,
  method changes and close. Go/JavaScript cannot guarantee complete erasure of
  all temporary string copies.
- Discovery, other configuration operations and updater installation cannot
  overlap the recovery transaction. Shutdown/cancellation joins ongoing work.

## CLI

Read-only availability check (replace placeholders with the actual address and
adapter name):

```sh
./bin/sadptools recovery questions CAMERA_IP --interface 'ADAPTER_NAME' --json
```

An explicit reset reads a private JSON object from stdin, containing `answers`
(three objects with numeric `id` and string `answer`) and `password`. Use the
question IDs returned by the check. Keep that input out of Git and shell history.

```sh
./bin/sadptools recovery reset CAMERA_IP --interface 'ADAPTER_NAME' \
  --secrets-stdin --confirm-reset --json < /path/to/private-recovery-input.json
```

Both commands accept `--port` (default 80) and `--timeout` (default 10s, maximum
5m). Exit codes: 0 ready/acknowledged, 1 communication failure, 2 invalid input,
3 recovery unavailable, 4 rejected, 5 unconfirmed, 130 cancellation before a
reset was submitted. No administrator credentials or answers are sent by the
`questions` command.

## Protocol and compatibility

The Go implementation in `internal/recovery` is independent of Wails and uses
only Go's standard library plus the existing project packages. Its flow is:

1. GET `/ISAPI/Security/extern/capabilities`: require
   `externSecurityCap/RestAdminPassWord/isSupportWithSecurityQuestion=true`.
2. GET `/ISAPI/Security/questionConfiguration`: require exactly three distinct
   questions with `mark=true`. Missing or ambiguous flags remain unknown.
3. GET `/doc/i18n/en/Common.json`: resolve each selected ID through this
   firmware's `questionList.number<ID>` text. No hardcoded question meanings.
4. On explicit reset only, generate an ephemeral RSA-1024 key (exponent 65537)
   and POST `/ISAPI/Security/challenge`. `PublicKey/key` contains base64 of the
   ASCII hexadecimal modulus. Decode the returned `Challenge/key` and decrypt
   its RSA PKCS#1 v1.5 ciphertext; require a 32-character hexadecimal challenge.
5. Encrypt each UTF-8 answer and new password using the browser recovery
   profile: base64 of ASCII hex of AES-128-ECB blocks, with zero padding and a
   challenge-derived prefix. POST challenge and PUT reset share one exchange.
   The answer/password values themselves are base64-encoded before encryption.
6. PUT `/ISAPI/Security/questionCertification` with `SecurityQuestion`,
   `QuestionList/Question/id`, encrypted `answer`, and encrypted `Resetpassword`.
   Only a recognised successful response establishes acknowledgement.

This recovery profile differs from authenticated ISAPI's password-derived CBC
encryption. An unsupported or malformed challenge stops before reset; there is
no plaintext fallback. HTTP does not authenticate the camera itself: use the
intended camera on a trusted management network. Requests do not follow
redirects, reuse connections or automatically retry a reset.

Research used Hikvision's [network/security API documentation](https://open.hikvision.com/hardware/v2/08%E5%8D%8F%E8%AE%AE%E9%80%8F%E4%BC%A0/%E7%BD%91%E7%BB%9C%E5%92%8C%E5%AE%89%E5%85%A8.html),
the manufacturer-authored [ISAPI developer guide](https://github.com/loozhengyuan/hikvision-sdk/blob/master/resources/isapi.pdf),
and read-only inspection of the web recovery client served by local cameras.
The implementation was written independently; proprietary browser source and
real device captures are not redistributed. Encryption fixtures use synthetic
inputs and were compared with the camera browser's AES primitive offline.

The legacy web profile was inspected on V5.5.61 and V5.6.6 firmware. Other web
layouts, question catalogs and encryption variants are not assumed compatible.
Neither the reference project's SADP command existence nor a positive capability
check proves that an actual reset will succeed on every firmware.

## Hardware acceptance still required

Read-only checks and synthetic tests do not establish successful physical-device
recovery. On a specifically authorised spare camera, verify correct answers and
a new password, successful subsequent login, unchanged device configuration,
and relevant password/Unicode limits. Test rejection and lockouts only with
explicit authorisation. Windows execution and process/network behaviour require
separate validation from cross-compilation. No real-device reset is performed
automatically during builds or tests.
