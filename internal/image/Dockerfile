# Embedded by yb. A Yocto/OpenEmbedded *host build environment* aligned to a
# release: the base Ubuntu and any extra packages come from build ARGs that yb
# fills from the version profile (internal/image/image.go). No kas is installed
# — yb drives checkout, conf generation, and bitbake itself.

ARG UBUNTU_VERSION=22.04
FROM ubuntu:${UBUNTU_VERSION}

ARG DEBIAN_FRONTEND=noninteractive
ARG EXTRA_PACKAGES=""
ARG UID=1000
ARG GID=1000

# Yocto "Required Packages for the Build Host" (valid zeus..scarthgap on Ubuntu),
# plus openssh-client for bitbake's git fetcher. EXTRA_PACKAGES carries per-release
# additions (e.g. python2 for zeus BSP layers).
RUN apt-get update && apt-get install -y --no-install-recommends \
        gawk wget git diffstat unzip texinfo gcc build-essential chrpath \
        socat cpio python3 python3-pip python3-pexpect xz-utils debianutils \
        iputils-ping python3-git python3-jinja2 libegl1-mesa libsdl1.2-dev \
        python3-subunit mesa-common-dev zstd liblz4-tool file locales sudo \
        ca-certificates curl tar bzip2 openssh-client \
        ${EXTRA_PACKAGES} \
    && rm -rf /var/lib/apt/lists/*

# bitbake requires a UTF-8 locale.
RUN locale-gen en_US.UTF-8 && update-locale LC_ALL=en_US.UTF-8 LANG=en_US.UTF-8
ENV LANG=en_US.UTF-8 \
    LC_ALL=en_US.UTF-8

# Non-root build user (bitbake refuses to run as root). UID/GID match the host so
# bind-mounted output stays owned by the caller. Ships an ssh config that uses the
# key yb mounts at /home/builder/.ssh/iri for any private git fetches.
RUN groupadd -g ${GID} builder \
    && useradd -m -u ${UID} -g ${GID} -s /bin/bash builder \
    && echo "builder ALL=(ALL) NOPASSWD: ALL" > /etc/sudoers.d/builder \
    && chmod 0440 /etc/sudoers.d/builder \
    && install -d -m 700 -o ${UID} -g ${GID} /home/builder/.ssh \
    && printf 'Host *\n    IdentityFile ~/.ssh/iri\n    IdentitiesOnly yes\n    StrictHostKeyChecking accept-new\n    UserKnownHostsFile ~/.ssh/known_hosts\n' \
        > /home/builder/.ssh/config \
    && chmod 600 /home/builder/.ssh/config \
    && chown ${UID}:${GID} /home/builder/.ssh/config

USER builder
WORKDIR /work
