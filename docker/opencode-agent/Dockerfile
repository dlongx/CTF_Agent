FROM node:26-bookworm-slim@sha256:2d49d876e96237d76de412761cf05dbfe5aee325cc4406a4d41d5824c5bb8beb AS node-runtime

FROM ctf-agent-base:latest

USER root

ARG NPM_REGISTRY=https://registry.npmmirror.com
ARG OPENCODE_AI_VERSION=1.17.18
ARG AI_SDK_OPENAI_COMPATIBLE_VERSION=3.0.7
ARG AI_SDK_ANTHROPIC_VERSION=4.0.12

COPY --from=node-runtime /usr/local/bin/node /usr/local/bin/node
COPY --from=node-runtime /usr/local/lib/node_modules /usr/local/lib/node_modules

RUN ln -s ../lib/node_modules/npm/bin/npm-cli.js /usr/local/bin/npm \
    && ln -s ../lib/node_modules/npm/bin/npx-cli.js /usr/local/bin/npx

RUN npm config set registry "${NPM_REGISTRY}" \
    && npm install -g \
        "opencode-ai@${OPENCODE_AI_VERSION}" \
        "@ai-sdk/openai-compatible@${AI_SDK_OPENAI_COMPATIBLE_VERSION}" \
        "@ai-sdk/anthropic@${AI_SDK_ANTHROPIC_VERSION}"

ENV HOME=/home/ctf
ENV XDG_CACHE_HOME=/home/ctf/.cache
ENV XDG_DATA_HOME=/home/ctf/.local/share

RUN chown -R ctf:ctf /workspace /home/ctf

WORKDIR /workspace

USER ctf

CMD ["opencode", "--version"]
