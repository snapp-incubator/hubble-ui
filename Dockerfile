# syntax=docker/dockerfile:1.2

# Copyright 2021 Authors of Cilium
# SPDX-License-Identifier: Apache-2.0

# BUILDPLATFORM is an automatic platform ARG enabled by Docker BuildKit.
# Represents the plataform where the build is happening, do not mix with
# TARGETARCH
# skopeo inspect --override-os linux --override-arch amd64 docker://docker.io/library/node:26.11.1-alpine3.24 | jq -r '.Digest'
FROM --platform=${BUILDPLATFORM} docker.io/library/node:26.12.0-alpine3.24@sha256:6f08a523443ddedf52941874c203475b0ad503e859ad70cb26ab8be9a517dbd5 as stage1
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

# skopeo inspect --override-os linux --override-arch amd64 docker://docker.io/nginxinc/nginx-unprivileged:1.31.6-alpine3.24-slim | jq -r '.Digest'
FROM docker.io/nginxinc/nginx-unprivileged:1.31.6-alpine3.24-slim@sha256:ac4d9c7e457c6510b95ac72dd84b27f7cc68ecb9a83fbd44f05a6460cf08aba4 AS release
USER root
RUN apk upgrade --no-cache
USER 101
COPY --from=stage1 /app/server/public /app
