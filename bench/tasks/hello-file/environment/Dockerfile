# base images come through the wired Artifactory docker host (docker.io pulls fail on this net)
# REGISTRY: a private mirror of Docker Hub's official images; the default is Docker Hub itself
ARG REGISTRY=docker.io/library
FROM ${REGISTRY}/ubuntu:24.04
WORKDIR /app
