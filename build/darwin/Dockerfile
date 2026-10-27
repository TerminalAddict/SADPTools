# Adapted from Wails v3.0.0-beta.34 Dockerfile.cross (MIT; see THIRD_PARTY_NOTICES).
# This deliberately contains no Windows/Linux GUI toolchain or frontend builder.
FROM golang:1.26.10-trixie
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl xz-utils python3 \
    && rm -rf /var/lib/apt/lists/*
RUN curl -fSL --retry 3 https://ziglang.org/download/0.14.0/zig-linux-x86_64-0.14.0.tar.xz -o /tmp/zig.tar.xz \
    && echo '473ec26806133cf4d1918caf1a410f8403a13d979726a9045b421b685031a982  /tmp/zig.tar.xz' | sha256sum -c - \
    && tar -xJf /tmp/zig.tar.xz -C /opt && rm /tmp/zig.tar.xz \
    && ln -s /opt/zig-linux-x86_64-0.14.0/zig /usr/local/bin/zig
ARG SDK_SHA256
COPY MacOSX14.5.sdk.tar.xz /tmp/sdk.tar.xz
RUN test -n "$SDK_SHA256" && echo "$SDK_SHA256  /tmp/sdk.tar.xz" | sha256sum -c - \
    && tar -xJf /tmp/sdk.tar.xz -C /opt && mv /opt/MacOSX14.5.sdk /opt/macos-sdk \
    && test -f /opt/macos-sdk/SDKSettings.json && rm /tmp/sdk.tar.xz
COPY zig-cc.py /usr/local/bin/zig-cc.py
RUN chmod 755 /usr/local/bin/zig-cc.py
WORKDIR /src
