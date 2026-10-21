# SADPTools development

- Use Go and Wails **v3**, Bootstrap 5 and vanilla JavaScript. Consult the pinned Wails source and current v3 docs for desktop APIs.
- Keep protocol, platform sockets, discovery, inventory and application logic independent of Wails. Desktop entry files use the `desktop` build tag so headless Go checks need no GUI libraries.
- Only read-only SADP inquiry is implemented. Device writes need an explicitly scoped milestone and protocol validation.
- Preserve unknown values; never substitute sample devices for real results. Label synthetic fixtures. Keep credentials and real network captures out of Git.
- Preserve upstream attribution in `LICENSES` and `THIRD_PARTY_NOTICES.md` when adapting code.
- Use `go test ./...`, `go vet ./...`, `go test -race ./...` for engine changes. Use `make frontend`, `make windows`, and `make linux-gui` for desktop changes as appropriate.
- Pin dependencies and build tooling. Commit generated bindings; regenerate with `make bindings` after changing desktop service methods.
- Report physical-device, Windows-runtime and graphical-session checks separately from compilation and synthetic tests.
- Do not install privileged system packages or push Git changes without the user's instruction.
