# Camera web connections

Selecting a camera checks its web connection without credentials. The IP address
in **Device details** becomes a link to the detected HTTP or HTTPS endpoint, and
**Open web interface** opens the same endpoint in the system browser. HTTPS is
preferred when both protocols respond. **Check web interface again** forces a
fresh check. Unreachable cameras remain in inventory with an explanation and no
guessed clickable address.

Detection probes HTTP and HTTPS on the advertised web port, plus HTTPS on 443.
If discovery did not supply a port, it tries HTTP/HTTPS on 80 and HTTPS on 443
without filling the unknown inventory field. A redirect to another port on the
same numeric camera IP can identify a custom HTTPS port. Other custom ports
cannot be inferred when neither advertised nor reachable through a redirect;
this is not a port scanner. The CLI recovery commands accept `--port` to supply
a candidate explicitly.

Probes send only `GET /`, with no credentials, cookies or proxy. They discard
response bodies, bound headers, use short connect/request timeouts and join
their work on cancellation. Redirects to another host, credential-bearing URLs,
non-web schemes and HTTPS downgrades are rejected. Redirect paths and queries
are never requested: only the destination origin is probed. A same-origin login
redirect or authentication challenge still identifies a responding web service.

Detection inspects HTTPS even when the camera uses a self-signed certificate.
This does **not** trust that certificate for recovery or change browser trust.
The browser applies its own checks. Security-question recovery uses normal TLS
validation or an explicitly confirmed SHA-256 certificate pin, with no silent
downgrade after a certificate failure. Its recheck uses the chosen adapter/source
address, and its detected endpoint updates the details/browser cache. See
[security-question recovery](security-question-recovery.md).

The details link is checked against the current inventory device. Results are
cached for one minute and invalidated on address/advertised-port changes; late
selection responses cannot replace the current camera's link. Detection proves
that a web server answered at that moment, not that login or a recovery API will
work. Routed cameras are supported; discovery itself still needs its multicast
network access.
