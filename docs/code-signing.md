# Code signing policy

## Current status

The Windows builds, including **v0.6.0-rc.2**, are unsigned. Their application,
CLI and installer have no embedded Authenticode signature. A Windows report of
**Unknown publisher** is therefore expected; the company name in executable
metadata and the release's SHA-256 checksums do not establish a trusted publisher.

[SignPath Foundation](https://signpath.org/apply) **declined the application**
because the project is too new, as reported by Paul on 27 October 2026. The earlier
receipt acknowledgement was not approval. SignPath has not signed or endorsed
this project. A different signing route, identity validation and a tested signing
workflow are required before a signed release can be published.

A valid signature establishes a publisher and detects changes to signed code.
It does not guarantee that a new download immediately avoids SmartScreen:
Microsoft also evaluates file and publisher reputation. Self-signed certificates
do not solve public distribution trust. See
[Microsoft's current SmartScreen guidance](https://learn.microsoft.com/en-us/windows/apps/package-and-deploy/smartscreen-reputation).

## Provider preference — 27 October 2026

Paul has explicitly ruled out paying Microsoft a monthly signing fee. Do not
propose or provision Azure Artifact Signing for this project. Evaluate a
non-Microsoft certificate authority instead; no provider or purchase is approved.
SignPath Foundation's rejection still stands.

Local inspection of Paul's `tmp/HiToolsDelivery.exe` and
`tmp/HiToolsDeliverySetup_EN.exe` found that both are signed by **Hangzhou Hikvision
DIGITAL Technology Co., Ltd.**, with certificates issued by **GlobalSign GCC R45
EV CodeSigning CA 2020** and **DigiCert** timestamps. Both embedded signatures in
each file passed `osslsigncode` 2.13 digest, chain, timestamp and CRL checks against
the Linux host's trust store. This is static signature verification, not a
Windows SmartScreen execution test or a malware assessment. The SADPTools beta
standalone EXE has no Authenticode signature. The supplied EXEs remain local
inspection material and must not be included in source commits or releases.

[GlobalSign](https://www.globalsign.com/en/code-signing-certificate/what-is-code-signing)
and [Sectigo](https://www.sectigo.com/ssl-certificates-tls/code-signing) offer
commercial code-signing products. Certum also lists an
[open-source product](https://shop.certum.eu/open-source-code-signing-on-simplysign.html),
but the shop currently reports it out of stock. Obtain current availability,
New Zealand eligibility, identity-validation requirements, renewal terms and the
full token/cloud cost before recommending a purchase. Provider selection must
also account for how signing will work from Sparx or an approved CI workflow.

A trusted CA signature establishes the publisher; it is not a promise of instant
SmartScreen reputation. Do not infer a vendor's billing arrangement from an
executable's certificate or timestamp, and do not buy EV solely on a claim that
it automatically bypasses SmartScreen.

## Maintainer responsibilities

- **Author/committer and reviewer:** [Paul Willard (TerminalAddict)](https://github.com/TerminalAddict).
- **Release/signing approver:** Paul Willard. Every production signing request
  requires explicit maintainer approval, including changes to build scripts.
- Contributions from others require maintainer review before signing. Signing
  credentials must not be available to pull-request builds.
- Maintainers must enable MFA on GitHub and the selected provider before using the signing
  service. Account configuration has not been verified by the application code.
- Sign only SADPTools executables built from the reviewed source commit. Keep
  third-party publisher signatures intact.

These are project responsibilities and onboarding requirements; they are not a
claim that provider roles, MFA enforcement or production signing are configured.
If a future SignPath application is accepted, add its required attribution: “Free code signing provided by
SignPath.io, certificate by SignPath Foundation”, with links to the providers.

## Privacy and eligibility disclosure

SADPTools discovers and configures devices on the local network. Device writes
are explicit actions, and password recovery requires device-validated security
answers or a valid vendor/support code. It does not implement vulnerability scanning, password cracking or a
device-security bypass. No credentials or recovery data are logged or sent to
the signing service.

The app has no telemetry or cloud account. Optional startup/manual update checks
contact GitHub's public releases API; device inventory, credentials and recovery
data are not included. Downloads require a user action. Website links open when
the user chooses them. Startup checks can be disabled in Settings.

The offline Windows installer includes Microsoft's unmodified WebView2 runtime
installer. This is proprietary third-party software with its own update,
SmartScreen and privacy behaviour, disclosed during setup and in
[third-party notices](../THIRD_PARTY_NOTICES.md). The SignPath application must
disclose it and request confirmation that it meets the System Libraries
exception. It must not claim that every bundled component is open source or
that all bundled software makes no external requests. Microsoft runtime files
must not be signed with the SADPTools signing identity.

## Release signing requirements

Any future SignPath Foundation [GitHub integration](https://docs.signpath.io/trusted-build-systems/github)
requires all jobs leading to a signing request to use GitHub-hosted runners and
the input to be a GitHub Actions artifact. Development remains on Sparx, but
uploading a locally built binary does not satisfy that source-verification
requirement. Other providers have their own integration requirements. The current
local release builder does not perform code signing.

The approved workflow must:

1. Build from the exact reviewed source commit on GitHub-hosted runners, with the
   pinned toolchain, locked dependencies, release version and required tests.
2. Sign the desktop application and Windows CLI, and arrange signing of the NSIS
   uninstaller before embedding them. Apply the required product-name/version
   metadata constraints to each project executable.
3. Package the signed payloads with the unchanged, pinned Microsoft runtime.
   Sign the finished setup executable. Signing only the outer installer leaves
   the installed application unsigned and does not fix its publisher warning.
4. Verify Authenticode trust, timestamps and the expected publisher on Windows
   for the setup, installed application, CLI and uninstaller. Check that embedded
   payloads match the signed build outputs. A certificate-table presence check
   alone is not signature/trust verification.
5. Generate the release archives, manifest and SHA-256 checksums **after** signing.
   Preserve the same asset names and updater handoff. No binary edits, resource
   changes or rebuilding may occur after signing.
6. Publish a new version through the verified release process, with signing
   evidence and remaining Windows-runtime checks recorded. Never replace the
   already-published release files or move their tags.

Use the selected provider's verified production identity and approved signing
configuration; keep signing credentials out of source and untrusted builds.
Signing actions and all other build actions must be pinned. Do not substitute test/self-signed
certificates for the production certificate, or report a signed release until
the actual signature verification has passed.

Windows acceptance testing still includes clean installation, the in-app update
shutdown/replacement/relaunch sequence, uninstallation and SmartScreen behaviour
on a normally protected machine. Disabling Windows security or asking all users
to install a custom trusted root is not the distribution solution.
