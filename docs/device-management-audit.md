# Device-management audit — 28 October 2026

This review followed successful activation and a failed DHCP attempt on an
IPC-B261H-MU running V5.7.1 build 220610. It covers the shared management engine,
desktop/CLI entry points, provisioning, sockets and the other camera-write paths.
It is not a certification of every feature on every camera. Synthetic success
replies test application behaviour; independent protocol evidence and real device
state are needed to establish compatibility.

## Findings and corrections

| Area | Finding / action | Evidence and remaining test |
| --- | --- | --- |
| Activation | v1.0.2 added RSA exchange/AES encoding and bounded MAC-less reply handling. | User reports successful Windows activation on this model. Read-only inquiry now reports active. Independent password-login and other firmware tests remain separate. |
| DHCP and static IPv4 | Previous code sent a plaintext password, omitted unrelated ports/IPv6 settings, and rejected MAC-less acknowledgements. Replaced with fresh inquiry, advertised authentication, field preservation and address-change-aware reply matching. | Independent synthetic crypto vectors match archived SDK behaviour. This camera advertises the supported salted/encrypted profile. Corrected DHCP/static writes still need hardware testing. |
| Error reporting | Network `PasswordError`, `locked`, and `notActivated` were not recognised. | Added explicit safe messages; unknown responses remain unconfirmed. No deliberately incorrect password or lockout test was sent to hardware. |
| GUI / CLI | Both use the shared configuration engine; GUI resolves one observed adapter and excludes simultaneous discovery. | Automated controller/CLI tests; UI explains fresh authentication and preserves the existing rediscovery workflow. Windows compilation is separate from Windows runtime testing. |
| Batch provisioning | Uses the same configuration engine, so it receives the network fix. Requires matching MAC/identity rediscovery before dependent writes; unconfirmed outcomes stop that camera's sequence. | Existing plan/runner tests. Multi-camera and DHCP lease behaviour need hardware tests. |
| SADP support-code recovery | Copied upstream formats remain unverified. The examined SDK uses `reset` and an encrypted recovery flow; the existing `resetPassword` / `securityCode` templates do not establish support for that flow. An empty `exchangecode` is not a complete key exchange. | App and CLI now identify these as experimental legacy formats. Modern support-code recovery is **not validated or implemented** by those templates. No reset test was attempted. Use the camera/manufacturer workflow for this case. |
| Security-question setup | Native encrypted SADP setup, offered after acknowledged activation and for existing active cameras. Fresh support/profile/revision checks, explicit replacement consent, one write and configured-ID readback. | Read-only IPC-B261H-MU reply passed the strict 15-question parser. Independent synthetic crypto vectors, transaction tests and mocked browser checks pass. No setup write or answer-based reset has been tested on hardware. See [setup](security-question-setup.md). |
| Security-question recovery | Separate HTTP(S) implementation checks capability and configured question IDs, negotiates encryption, then sends one explicit reset. Connection pinning and unconfirmed outcomes are retained. | Synthetic crypto/HTTP/TLS tests and earlier browser-profile research. No successful reset on this camera has been established. |
| ONVIF imaging | Rechecks serial, advertised setting, permitted range and previous value; changes one field and reads back before reporting verified. | Synthetic tests. Requires credentials and model-specific hardware tests; no image changes made in this review. |
| PTZ / focus / presets | Advertised capability checks, device identity verification and explicit controls; movement is bounded. Failed writes are not automatically retried. | Synthetic tests; this camera's advertised capabilities and physical behaviour still need a credentialed check. |
| Backups / firmware / replacements | Backup restore is gated; firmware and replacement workflows do not silently install firmware or clone device settings. | Existing tests preserve these limits. They are not evidence of working vendor restore/upgrade support. |
| Discovery / platform sockets | Wildcard UDP/37020, one selected multicast adapter per write, receiving interface metadata, timeout/cancellation cleanup. Windows uses WSARecvMsg/IP_PKTINFO, Linux/macOS use x/net control messages. | Real read-only discovery on Sparx plus synthetic tests. User's Windows activation is useful evidence; Windows DHCP and multi-adapter runtime checks remain. |
| Credentials / logs | The new authentication profile is ephemeral and redacted for formatting/logging. No salt, password, raw reply or recovery challenge is added to inventory/preferences. | Serialization/logging regression tests. No live credential was supplied or stored during this review. |

See [device management](device-management.md) for the network authentication
algorithm and primary protocol evidence. The implementation uses Go's standard
cryptographic libraries; no proprietary SDK is distributed.

## Hardware test sequence

Use the identified test camera, leave recorders/production cameras out of the
test, and record its current state. Do not treat a send or acknowledgement as
proof of the resulting setting.

1. Run the new Windows test build, rediscover, select the camera by MAC and
   confirm active status. Open Configure network and enable DHCP with its current
   admin password, preserving the displayed static fields. Submit once.
2. Close and rediscover. Check that the same MAC reports DHCP enabled and inspect
   its IPv4 address. Separately check the router's lease/request logs. Open the
   detected web interface and verify login.
3. If the result is unconfirmed, rediscover and inspect the router before another
   write. Record only safe diagnostics, model/firmware and outcomes; do not share
   passwords, salts or raw protocol captures.
4. After the DHCP result is understood, explicitly choose any further writes:
   disable DHCP / restore a chosen static address, change a port, image setting,
   preset or password. Check readback and connectivity after each action.
5. Read-only checks can then cover HTTP/HTTPS, ISAPI/ONVIF information, advertised
   stream profiles/live view and security-question availability. A password reset
   or lockout test requires its own approval and recovery plan; a generic audit
   does not authorise either operation.

No camera write was performed by the development agent during this audit. The
corrected DHCP transaction must not be described as hardware-verified until that
test succeeds. Windows execution and actual Mac/Linux graphical sessions are
separate from cross-compilation and headless tests.
