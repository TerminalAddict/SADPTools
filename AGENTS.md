# SADPTools development

- Use Go and Wails **v3**, Bootstrap 5 and vanilla JavaScript. Consult the pinned Wails source and current v3 docs for desktop APIs.
- Keep protocol, platform sockets, discovery, inventory and application logic independent of Wails. Desktop entry files use the `desktop` build tag so headless Go checks need no GUI libraries.
- Milestone two implements single-device activation, network updates and code-based password recovery. Keep writes explicit, on one selected adapter, with no automatic retries or live-device test writes unless the user identifies and authorises the device/action. Do not interpret a timeout as proof that no change occurred.
- Never log credentials, raw command XML/replies or recovery challenges; do not store secrets in preferences or inventory. Preserve unconfirmed outcomes and firmware compatibility limits.
- Preserve unknown values; never substitute sample devices for real results. Label synthetic fixtures. Keep credentials and real network captures out of Git.
- Preserve upstream attribution in `LICENSES` and `THIRD_PARTY_NOTICES.md` when adapting code.
- Use `go test ./...`, `go vet ./...`, `go test -race ./...` for engine changes. Use `make frontend`, `make windows`, and `make linux-gui` for desktop changes as appropriate.
- Pin dependencies and build tooling. Commit generated bindings; regenerate with `make bindings` after changing desktop service methods.
- Report physical-device, Windows-runtime and graphical-session checks separately from compilation and synthetic tests.
- Do not install privileged system packages or push Git changes without the user's instruction.
