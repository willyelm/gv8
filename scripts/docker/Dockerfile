FROM ubuntu:22.04

ENV DEBIAN_FRONTEND=noninteractive

# The linux/arm64 build runs inside an amd64 container because Chromium's
# Rust host toolchain is still x64-oriented. Install the target glibc runtime
# as arm64 multiarch packages so ld.lld can resolve the absolute paths used by
# the Debian arm64 linker scripts in V8's sysroot.
RUN sed -i 's/^deb /deb [arch=amd64] /' /etc/apt/sources.list && \
    printf '%s\n' \
      'deb [arch=arm64] http://ports.ubuntu.com/ubuntu-ports jammy main restricted universe multiverse' \
      'deb [arch=arm64] http://ports.ubuntu.com/ubuntu-ports jammy-updates main restricted universe multiverse' \
      'deb [arch=arm64] http://ports.ubuntu.com/ubuntu-ports jammy-security main restricted universe multiverse' \
      > /etc/apt/sources.list.d/arm64.list && \
    dpkg --add-architecture arm64

# Install the host tools needed by depot_tools, GN and Ninja.
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    build-essential \
    curl \
    git \
    libc6:arm64 \
    libc6-dev:arm64 \
    libgcc-s1:arm64 \
    libglib2.0-dev \
    lsb-release \
    python3.11 \
    python3.11-dev \
    python3-pip \
    ninja-build \
    pkg-config \
    xz-utils \
    && rm -rf /var/lib/apt/lists/*

# Make python3.11 the default python3
RUN update-alternatives --install /usr/bin/python3 python3 /usr/bin/python3.11 1

WORKDIR /workspace
