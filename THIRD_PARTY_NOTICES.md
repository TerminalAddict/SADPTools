# Third-party notices

SADP inquiry and device-management command wire formats and response field mappings in `internal/protocol/sadp/` are adapted from [cameronnewman/hikvision-tooling](https://github.com/cameronnewman/hikvision-tooling), commit `afa00e3db11bb47914e1f0d724c6975e0a41e218` (MIT, Copyright (c) 2025 Cameron Newman). Its full licence is retained in `LICENSES/hikvision-tooling-MIT.txt`. The scanner lifecycle and transport are implemented here; no upstream executable or `internal` package is imported.

The application uses Wails v3 (MIT), Bootstrap 5 (MIT), Go's x/net and x/sys packages (BSD-3-Clause), and their transitive dependencies. Frontend assets retain upstream licence comments; dependency versions are recorded in `go.sum` and `frontend/package-lock.json`. Bundled dependency licence texts are collected in `LICENSES/dependencies.txt` for distribution. Build tooling includes Vite (MIT); its licence is also included there.

Wails generated the initial desktop scaffold and Windows manifest/resource configuration. The application icon and interface are SADPTools assets.

## Windows installer

The Windows Setup executable redistributes the **unmodified Microsoft Edge WebView2 Runtime Evergreen Standalone Installer (x64)**. This Microsoft component is separately licensed, not covered by SADPTools' MIT licence. The full Microsoft terms, obtained on 21 October 2026 from the [official licence endpoint](https://developer.microsoft.com/microsoft-edge/api/eula/webview2?locale=en-us), are retained in `LICENSES/Microsoft-WebView2.txt` and displayed during setup. The payload comes directly from Microsoft's download service; its version, URL, size and SHA-256 are pinned in `build/windows/installer-inputs.json`.

WebView2 includes Microsoft Defender SmartScreen, which collects and sends information to Microsoft as disclosed in [Microsoft's Privacy Statement](https://aka.ms/privacy) and the [Microsoft Edge Privacy Whitepaper](https://learn.microsoft.com/en-us/microsoft-edge/privacy-whitepaper#smartscreen). Microsoft supplies runtime updates separately. SADPTools itself does not implement telemetry. Uninstalling SADPTools leaves this shared Microsoft component installed.

The installer is compiled with **NSIS 3.10**, whose installer stub, Modern UI and standard plugins carry their respective open-source notices. The full copyright/licence file from the pinned Ubuntu NSIS packages is retained in `LICENSES/NSIS.txt`. No Wails NSIS helper code is copied; the installer script is maintained in this repository.
