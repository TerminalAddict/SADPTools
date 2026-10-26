# Add a camera by IP

Available from **0.5.0-beta.2**. This connects directly through the
computer's routing table, including routed subnets and VPNs. The camera does not
need to appear in SADP multicast discovery or share your computer's subnet.

## Known camera address

1. Open **Sites → Add camera by IP**, or **IP scanner → Add camera by IP**.
   An IP scanner result's details also offer this action with its address filled in.
2. Choose the destination site and enter one numeric IPv4 address. If there are
   no active sites, the app first offers to create one. No adapter selection is needed.
3. Leave the defaults for normal ports. Under **Custom ports and identification**,
   enter a nonstandard web/HTTPS/RTSP port or an exact ONVIF device-service URL
   when required. An ONVIF URL must use this same IP and an `/onvif/` path.
4. Optionally expand **Read-only sign-in**. Choose a saved credential belonging
   to this site, or enter a one-time username/password. ONVIF may require an
   enabled ONVIF service and a separate ONVIF account on the camera.
5. Click **Check camera**. A spinner and activity message show progress.
   **Cancel check**, closing the dialog or closing the app cancels the operation.
6. For an untrusted HTTPS certificate, compare its SHA-256 fingerprint with the
   camera's certificate through a trusted channel, approve it, then check again.
   One-time passwords are cleared after each check; re-enter them when needed.
7. Review the returned identity and service information, optionally add a saved
   name/location, then click **Save camera**. Select **Open Live View after saving**
   to open its existing Live View workflow. You can also use the saved row's
   **Live View** button later.

The check detects HTTP/HTTPS and sends a small RTSP OPTIONS request. It does not
start video. It reads identity using ONVIF and, when appropriate, the read-only
Hikvision-compatible ISAPI endpoint. It never activates or reconfigures the camera.
A password rejected by ONVIF is not automatically tried against ISAPI; select
**ISAPI only** if that is the intended account/service.

HTTPS certificate approval is limited to this dialog/check. Live View has its
own certificate approval. Credentials are not sent to unapproved HTTPS endpoints,
and a failed HTTPS check does not downgrade an authenticated attempt to HTTP.

## Saving and unknown values

Checking alone does not change the saved inventory. Unknown model, serial, MAC,
activation state and other absent properties stay unknown. An open web or RTSP
service alone is not proof that the address belongs to a camera. If identity
cannot be read, an explicit checkbox allows saving it as an **unverified address**.
That does not confirm connectivity or compatibility.

A saved credential selected for the check is assigned to a newly saved camera.
One-time credentials are not saved. Live View can ask for them again. An existing
site default is not silently assigned by this workflow.

Stable identity matches (MAC/serial/UUID, when available) return the existing
record unchanged. A matching IP with a different or unknown identity is blocked
for review. Existing addresses, notes and credential assignments are never
silently overwritten. Records are kept in the chosen site, separately from the
local broadcast discovery list. No database schema migration is required.

## Finding an unknown address on a routed network

Use the existing **IP scanner** with the authorised remote CIDR/range and the
adapter through which that route is reachable. Review the scope before scanning.
Open a responding address's details and choose **Add camera by IP**. Port responses
are evidence of a service, not proof of a camera or its manufacturer. No response
is inconclusive; routing, firewall rules, custom ports or disabled services can
prevent detection.

SADP/ONVIF multicast discovery remains local to the broadcast/multicast network.
For discovery from inside another network, **Commissioning → Remote discovery**
uses an explicitly selected SSH host running the SADPTools CLI there. Routed IP
access alone does not make local SADP activation/network/password commands work
across routers. This addition does not change those command requirements.

## Validation on Sparx — 27 October 2026

- `make check`: Go tests, vet and race checks passed; **33 frontend tests** and
  **34 packaging tests** passed.
- New generated/synthetic cases cover target validation, ONVIF/ISAPI identity,
  unknown fields, custom ports, certificate approval/change/downgrade, credential
  handling, bounded RTSP OPTIONS parsing, timeout/cancellation and socket cleanup.
- Saved-site tests cover stable duplicates, IP collisions, site isolation,
  credential assignment, unverified consent, expiring/single-use review tokens,
  cancellation/shutdown and no merging into local discovery.
- A native Linux Wails/WebKit session under Xvfb, with an isolated test profile,
  detected synthetic HTTPS/ONVIF/RTSP services hosted on Sparx. The certificate
  fingerprint matched the generated fixture certificate. Approval, identity
  reading, cancellation, rechecking and saving a named site record passed.
  Automatic discovery/update checks were disabled; no physical camera was used.
- Windows AMD64 standalone and offline Setup, and Linux GUI/CLI/DEB/RPM/manual
  packages compile/package successfully. Linux and Windows desktop-tagged vet
  and Linux desktop-tagged race tests passed. Release downloads include
  `SHA256SUMS` and a manifest identifying their source commit.

Windows execution and a real routed/VPN camera, its credentials, custom ports,
firewall behaviour and Live View still need operator testing. Synthetic local
service tests do not establish physical-camera or remote-network compatibility.
