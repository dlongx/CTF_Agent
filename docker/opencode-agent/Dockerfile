FROM ctf-agent-base:latest

ARG NPM_REGISTRY=https://registry.npmmirror.com

RUN apt-get -o Acquire::Retries=5 update \
    && apt-get -o Acquire::Retries=5 install -y --no-install-recommends \
        nodejs \
        npm \
        ripgrep \
    && rm -rf /var/lib/apt/lists/*

RUN npm config set registry "${NPM_REGISTRY}" \
    && npm install -g \
        opencode-ai \
        @ai-sdk/openai-compatible

WORKDIR /workspace

CMD ["opencode", "--version"]
