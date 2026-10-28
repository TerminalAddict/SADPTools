# Security-question password recovery

To configure the questions while you still know the admin password, use **Set
security questions** in Device details, or the follow-on step after activation.
See [security-question setup](security-question-setup.md). Setup and password
recovery use separate camera protocols and have separate compatibility limits.

Select a discovered, activated camera and open **Password recovery**. The default
**Security questions** method checks the camera's local recovery interface and
loads its three configured questions. Enter the answers exactly as originally
set, enter and confirm a new admin password, then select **Reset this device's
password**. Opening the dialog and **Check again** only read availability and
question text; they do not validate answers or reset anything.

The camera needs a usable advertised IPv4 address and web port reachable from
the computer. **It does not need to share the computer's subnet.** A working
route through a gateway is sufficient for the HTTP/HTTPS connection; firmware must
also support the recovery API.

Automatic selection uses the operating system's route to choose a current
local source address, including secondary addresses or VPN adapters. This can
differ from the adapter that received SADP discovery. An explicit adapter
override is respected. If route lookup is unavailable, selection falls back to
an observed adapter, preferring a directly connected subnet, then the selected
discovery adapter or first usable observed adapter.

Route lookup uses a temporary connected UDP socket without sending a datagram
(Go's [DialUDP](https://pkg.go.dev/net#DialUDP)); it is only a source-selection
hint. The bounded recovery HTTP requests establish actual reachability and API
support. There is no ping or same-subnet requirement. The connection binds the
chosen source address without a proxy. A connection failure reports the camera
address/port and local adapter/source, without trying other adapters. SADP
visibility alone does not establish web connectivity; routing, firewalls and
the camera's web service still matter. A browser connection through a proxy
does not prove a direct connection is available.

HTTP/HTTPS detection uses the same engine as the device-details link, with the
recovery operation's selected source address. It probes the advertised port and
standard HTTPS port, preferring HTTPS. The detected endpoint is shared with
browser opening. See [web connections](web-connections.md) for custom ports and
redirect limits. Missing advertised ports are left unknown in inventory;
detection tries the standard ports instead.

For an untrusted/self-signed HTTPS certificate, recovery stops before reading
questions or exchanging any recovery secrets. Compare the displayed SHA-256
fingerprint with the camera's certificate, then explicitly trust it for this
recovery session. The backend pins that exact certificate on every subsequent
connection. A changed certificate stops the operation; there is no HTTP fallback
or write retry. Trust is not saved in preferences. A normally trusted HTTPS
certificate uses the operating system's certificate validation.

The app distinguishes ready, unsupported, not configured, unknown and unavailable
responses. It never assumes that questions are configured because a model usually
supports them. Unknown question labels prevent submission. The existing
**Support-provided code** method remains available separately.

## Submission and results

- Answers are not trimmed or case-folded. They are limited to 128 Unicode
  characters and 512 UTF-8 bytes. New passwords use the existing validation;
  firmware may impose stricter rules.
- A GUI availability check binds one submission to the inventory device,
  address, detected protocol/port, certificate pin if accepted, adapter/source
  address and question IDs. It expires after five
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

Both commands accept `--port` (candidate web port, default 80) and `--timeout`
(default 10s, maximum 5m). HTTP/HTTPS detection runs before recovery. If an
untrusted certificate is reported, verify its fingerprint and supply
`--trust-certificate SHA256` to each command that should trust that exact
certificate. The selected adapter can reach the camera directly or through a router.
Exit codes: 0 ready/acknowledged, 1 communication failure, 2 invalid input,
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
