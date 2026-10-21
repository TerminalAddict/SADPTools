# Third-party notices

SADP inquiry XML templates and response field mappings in `internal/protocol/sadp/sadp.go` are adapted from [cameronnewman/hikvision-tooling](https://github.com/cameronnewman/hikvision-tooling), commit `afa00e3db11bb47914e1f0d724c6975e0a41e218` (MIT, Copyright (c) 2025 Cameron Newman). Its full licence is retained in `LICENSES/hikvision-tooling-MIT.txt`. The scanner lifecycle and transport are implemented here; no upstream executable or `internal` package is imported.

The application uses Wails v3 (MIT), Bootstrap 5 (MIT), Go's x/net and x/sys packages (BSD-3-Clause), and their transitive dependencies. Frontend assets retain upstream licence comments; dependency versions are recorded in `go.sum` and `frontend/package-lock.json`. Bundled dependency licence texts are collected in `LICENSES/dependencies.txt` for distribution. Build tooling includes Vite (MIT); its licence is also included there.

Wails generated the initial desktop scaffold and Windows manifest/resource configuration. The application icon and interface are SADPTools assets.
