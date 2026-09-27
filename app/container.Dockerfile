# The runtime a containered session runs in (--containered; canon/binary.md). The binary is not in
# the image: it is mounted read-only from the host, so the image never goes stale with it. The
# agent's bash sees these tools; name a richer image with --image <ref> or registry.containers.image.
# BASE and APT come from registry.containers: a private registry's mirror of the base image, and an
# apt mirror (an Artifactory debian remote) for the packages.
ARG BASE=debian:bookworm-slim
FROM ${BASE}
ARG APT=
RUN if [ -n "$APT" ]; then \
      sed -i "s#http://deb.debian.org/debian#${APT}#g" /etc/apt/sources.list.d/debian.sources 2>/dev/null || true; \
      sed -i "s#http://deb.debian.org/debian#${APT}#g" /etc/apt/sources.list 2>/dev/null || true; \
    fi \
 && apt-get update \
 && apt-get install -y --no-install-recommends \
      bash ca-certificates coreutils curl diffutils file findutils gawk git grep jq less make \
      patch procps python3 ripgrep sed tar unzip xz-utils \
 && rm -rf /var/lib/apt/lists/*
ENV LANG=C.UTF-8
