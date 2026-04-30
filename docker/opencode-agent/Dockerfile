FROM ctf-agent-base:latest

ARG NPM_REGISTRY=https://registry.npmmirror.com
ARG OPENCODE_AI_VERSION=1.14.30
ARG AI_SDK_OPENAI_COMPATIBLE_VERSION=2.0.42
ARG AI_SDK_ANTHROPIC_VERSION=3.0.72

RUN apt-get -o Acquire::Retries=5 update \
    && apt-get -o Acquire::Retries=5 install -y --no-install-recommends \
        nodejs \
        npm \
        ripgrep \
    && rm -rf /var/lib/apt/lists/*

RUN npm config set registry "${NPM_REGISTRY}" \
    && npm install -g \
        "opencode-ai@${OPENCODE_AI_VERSION}" \
        "@ai-sdk/openai-compatible@${AI_SDK_OPENAI_COMPATIBLE_VERSION}" \
        "@ai-sdk/anthropic@${AI_SDK_ANTHROPIC_VERSION}"

WORKDIR /workspace

CMD ["opencode", "--version"]
