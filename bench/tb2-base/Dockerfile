# A tb2 prebuilt task image, raised with node+npm+opencode (staged from the local bench base by
# build.sh into .stage/ — Docker Desktop's BuildKit store cannot see classic-store local images,
# and this bench net routes through Artifactory only, so a second FROM against docker.io is a dead
# end) and with the verifier toolchain baked below. ARG TASK_IMAGE names the alexgshaw image on the
# proxy host. Harbor's OpenCode installer never touches the net.
ARG TASK_IMAGE
FROM ${TASK_IMAGE}
ARG EXTRA_PIP=""
ARG EXTRA_APT=""
COPY .stage/node /usr/local/bin/node
COPY .stage/node_modules/ /usr/local/lib/node_modules/
# The bench net routes through Artifactory only: apt goes to the `debian`/`ubuntu` remotes, pip to the
# pypi remote. Artifactory answers 401 without an identity, so the same identity pip.conf carries
# rides in as /etc/apt/auth.conf via a BuildKit secret — installed for the RUN, removed before the
# layer closes. TLS peer checks are off for apt (the corp CA is not in the task images) — payload
# signatures still ride the distro-signed InRelease against the image's own keyring.
RUN --mount=type=secret,id=aptconf,target=/run/secrets/apt.conf \
    set -eu; . /etc/os-release; host=artifactory.example.com; \
    case "$ID" in \
      ubuntu) echo "deb https://$host/artifactory/ubuntu $VERSION_CODENAME main universe" > /etc/apt/sources.list ;; \
      debian) echo "deb https://$host/artifactory/debian $VERSION_CODENAME main" > /etc/apt/sources.list ;; \
      *) echo "tb2-base: unsupported base distro '$ID'" >&2; exit 1 ;; \
    esac \
 && rm -rf /etc/apt/sources.list.d \
 && install -m 600 /run/secrets/apt.conf /etc/apt/auth.conf \
 && printf 'Acquire::https::Verify-Peer "false";\nAcquire::https::Verify-Host "false";\n' > /etc/apt/apt.conf.d/99artifactory \
 && apt-get update \
 && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends curl $EXTRA_APT \
 && rm -f /etc/apt/auth.conf \
 && rm -rf /var/lib/apt/lists/*
# The verifier toolchain, baked from the pypi remote at build time: the machine's pip.conf (which
# carries the Artifactory index and its inline credentials) rides a BuildKit secret, never a layer.
# tests/test.sh then needs no network at all.
RUN --mount=type=secret,id=pipconf,target=/run/secrets/pip.conf \
    PIP_CONFIG_FILE=/run/secrets/pip.conf \
    python3 -m pip install --break-system-packages --no-cache-dir \
      uv pytest==8.4.1 pytest-json-ctrf==0.3.5 $EXTRA_PIP
# Task payloads staged by build.sh (empty for most). filter-js-from-html gets the XSS testbed zip
# its verifier otherwise downloads from github — unreachable on this bench net.
COPY .stage-payload/ /opt/tb2-payload-in/
RUN if [ -f /opt/tb2-payload-in/testbed.zip ]; then mkdir -p /opt/tb2-xss-testbed \
  && mv /opt/tb2-payload-in/testbed.zip /opt/tb2-xss-testbed/testbed.zip; fi \
 && rm -rf /opt/tb2-payload-in
# Per-task offline bakes staged by build.sh (tb2-base/task-setup/<task>.sh, most tasks ship none):
# the setup runs with both secrets mounted and is removed before the layer closes.
COPY .stage-task/ /opt/tb2-task-in/
RUN --mount=type=secret,id=aptconf,target=/run/secrets/apt.conf \
    --mount=type=secret,id=pipconf,target=/run/secrets/pip.conf \
    if [ -f /opt/tb2-task-in/setup.sh ]; then bash /opt/tb2-task-in/setup.sh; fi \
 && rm -rf /opt/tb2-task-in
RUN ln -sf ../lib/node_modules/npm/bin/npm-cli.js /usr/local/bin/npm \
 && ln -sf ../lib/node_modules/opencode-ai/bin/opencode.exe /usr/local/bin/opencode \
 && printf 'registry=https://artifactory.example.com/artifactory/api/npm/npm\nstrict-ssl=false\n//artifactory.example.com/artifactory/api/npm/npm/:_authToken=${NPM_AUTH_TOKEN}\n' > /root/.npmrc
# Harbor's installer drives nvm on glibc images; an accepting shim stands in.
RUN printf '#!/bin/bash\nexit 0\n' > /usr/local/bin/nvm && chmod +x /usr/local/bin/nvm

