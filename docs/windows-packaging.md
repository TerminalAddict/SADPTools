# Windows offline installer

The Windows deliverable is a single `SADPTools-Setup-windows-amd64.exe` containing SADPTools, all licence notices and the full Microsoft WebView2 Evergreen Standalone Installer. WebView2 draws the Wails interface. Users do not need to understand or obtain it separately. This follows the [Wails v3 Windows packaging guidance](https://v3.wails.io/guides/build/windows/) and [Microsoft's offline deployment procedure](https://learn.microsoft.com/en-us/microsoft-edge/webview2/concepts/distribution#offline-deployment).

## Build on Sparx

```sh
make tools                  # Pinned Wails CLI, once per toolchain update
make windows-installer      # Also available as make installer
```

Output:

- `bin/SADPTools-Setup-windows-amd64.exe`
- `bin/SADPTools-Setup-windows-amd64.exe.sha256`

The installer includes all notices; distribute this one EXE. The checksum is optional for users and useful for release verification. Both the application and installer are currently unsigned. Signing is a future release step; an unsigned build can trigger Windows reputation/publisher warnings.

The [code signing policy](code-signing.md) describes the selected SignPath
Foundation route, eligibility disclosures and required signing order. Signing
only the setup executable does not sign the application it installs.

The root Wails `package` task builds the frontend and Windows binary, then calls `build/windows/package.py`. Python 3.11+ is required only on the build host. On Linux AMD64, the helper uses an installed NSIS 3.10 if present, otherwise downloads two pinned Ubuntu packages and extracts them with `dpkg-deb` into `.cache/windows-installer/nsis/`. It never uses sudo, registers packages or runs the Windows redistributable on the build host. This bootstrap is intended for Ubuntu 24.04+ AMD64 (including Sparx); other hosts should supply NSIS 3.10 on PATH or through `MAKENSIS`, with `NSISDIR` if necessary.

All downloaded input files are verified against committed SHA-256 hashes and sizes, including cached files. Partial, oversized or corrupt downloads cannot become build inputs. Microsoft URLs are pinned to a specific payload, not the moving “latest” redirect. `SADPTOOLS_BUILD_CACHE` can relocate the cache. Do not commit the 204 MiB Microsoft redistributable or generated installers to Git.

To prepare inputs in advance or package an already-built application:

```sh
python3 build/windows/package.py --prepare
python3 build/windows/package.py
```

For a versioned public build use the [release process](releases.md). Direct builds
accept `make windows-installer VERSION=0.2.0-beta.1`; when packaging an existing
binary, pass the same version to `package.py --version 0.2.0-beta.1`.

Those commands can run without network access once inputs are cached. A full `make windows-installer` still runs `npm ci` and may need registry access. Version and payload pinning make the inputs repeatable; the PE/NSIS toolchain may embed timestamps, so byte-for-byte reproducibility is not claimed.

## Setup behaviour

1. Reject unsupported Windows versions and CPU architectures. Current target: Windows 10/11 on AMD64.
2. Show the included Microsoft terms and required privacy notice, with ordinary setup acceptance. SADPTools' own source remains MIT-licensed.
3. Check Microsoft's per-machine and per-user runtime registration. Empty and `0.0.0.0` versions are treated as missing.
4. If needed, extract the bundled standalone installer and invoke `/silent /install`. The full runtime is included, so this does not require a download. Verify registration after it exits; if preparation failed, stop before installing/updating SADPTools and show a useful error.
5. Install into `%LOCALAPPDATA%\Programs\SADPTools`, create Start menu shortcuts and register an uninstaller for the current user. Normal setup requests no administrator elevation. Device/organisation policies can still prevent software installation.

Microsoft controls the installed Evergreen runtime's updates. An existing runtime is reused, not downgraded. The runtime can be shared by other applications, so SADPTools' uninstaller leaves it in place. Preferences and exports are preserved. Close the app before upgrading or uninstalling.

The small raw application EXE remains available for development; it does not carry the runtime itself. End users should receive the Setup EXE.

In-app updates use the same installer with `/WAITPID=<PID>`. Setup waits up to
30 seconds for that process to exit, stages the new executable beside the old
one, then replaces it without truncating the old executable during extraction.
The Finish page offers to launch the installed app. Invalid PID input exits 70;
failure to wait exits 71 before any application replacement. See the
[update lifecycle and Windows acceptance checks](updates.md).

Managed deployment can run `SADPTools-Setup-windows-amd64.exe /S /ACCEPTLICENSE=yes` after reviewing the bundled terms. This installs for the account running setup; it is not an all-users deployment. Silent mode without explicit acceptance exits 66. Other setup-specific errors: 64 unsupported Windows, 65 unsupported architecture, 67 runtime preparation failed, 68 uninstaller could not remove the running app, 69 application-file/shortcut/registration failure. NSIS may also return its own failure/cancellation codes. Interactive setup displays details; silent callers must inspect the process exit status.

## Maintaining the runtime

The pinned payload is WebView2 **155.0.4283.62**, retrieved directly from Microsoft on 21 October 2026. Before each release:

- Obtain the latest x64 **Evergreen Standalone Installer** from the [official Microsoft page](https://developer.microsoft.com/en-us/microsoft-edge/webview2/). Keep the standalone installer, not the small online bootstrapper.
- Verify its Microsoft Authenticode signature on a Windows build/release host (`Get-AuthenticodeSignature`), record the resolved versioned URL, version, size and SHA-256 in `installer-inputs.json`, and review any changed Microsoft terms. Preserve the unmodified redistributable. The initial payload was fetched directly over HTTPS and hashed on Linux; Authenticode verification remains a Windows release check.
- Rebuild, inspect archive integrity and test clean offline installation on Windows 10 and 11, including a PC without a runtime, a per-user runtime, a machine-wide runtime and a newer runtime.
- Check upgrade, uninstall, paths with spaces, standard-user accounts, runtime-installation failure, silent deployment, application launch and discovery/firewall behaviour. Check signature/reputation handling before public release.

Do not edit a checksum merely to silence a download error. If Microsoft retires a pinned URL, refresh the payload deliberately or restore the verified cached file from an approved internal build archive. End-user internet access is unnecessary for initial installation; future Microsoft security updates require access to Microsoft's update mechanism.

GitHub Actions builds the installer and checksum as CI artifacts. Public GitHub
releases are separate and use the explicit [release commands](releases.md).
The Wails CLI is installed with `CGO_ENABLED=0`, so the Windows cross-build job
does not need Linux GUI development libraries.
