# syntax=docker/dockerfile:1.2

# Copyright 2021 Authors of Cilium
# SPDX-License-Identifier: Apache-2.0

# BUILDPLATFORM is an automatic platform ARG enabled by Docker BuildKit.
# Represents the plataform where the build is happening, do not mix with
# TARGETARCH
# skopeo inspect --override-os linux --override-arch amd64 docker://docker.io/library/node:24.14.1-alpine3.23 | jq -r '.Digest'
FROM --platform=${BUILDPLATFORM} docker.io/library/node:26.5.1-alpine3.23@sha256:2a633e101381371ba148c7c212bf447c00cd267d814b708a9fe52c4984204729 as stage1
RUN apk add make git bash
WORKDIR /app

COPY package.json package.json
COPY package-lock.json package-lock.json
COPY .npmrc .npmrc
COPY scripts/ scripts/
COPY patches/ patches/

# TARGETOS is an automatic platform ARG enabled by Docker BuildKit.
ARG TARGETOS
# TARGETARCH is an automatic platform ARG enabled by Docker BuildKit.
ARG TARGETARCH
RUN npm --target_arch=${TARGETARCH} install && npm run postinstall

COPY . .

ARG NODE_ENV=production
RUN npm run build

# skopeo inspect --override-os linux --override-arch amd64 docker://docker.io/nginxinc/nginx-unprivileged:1.29.8-alpine3.23-slim | jq -r '.Digest'
FROM docker.io/nginxinc/nginx-unprivileged:1.31.2-alpine3.23-slim@sha256:eed3e4c2f3f9285e80610238e0b18fe1bf311c50559434db1969b1cfabd5518b AS release
USER root
RUN apk upgrade --no-cache
USER 101
COPY --from=stage1 /app/server/public /app
