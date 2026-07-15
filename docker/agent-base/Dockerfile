FROM python:3.14.6-slim-trixie@sha256:d3400aa122fa42cf0af0dbe8ec3091b047eac5c8f7e3539f7135e86d855dc015

ENV PYTHONUNBUFFERED=1
ENV DEBIAN_FRONTEND=noninteractive

ARG APT_MIRROR=http://mirrors.ustc.edu.cn/debian
ARG APT_SECURITY_MIRROR=http://mirrors.ustc.edu.cn/debian-security

RUN sed -i "s|http://deb.debian.org/debian-security|${APT_SECURITY_MIRROR}|g; \
    s|http://deb.debian.org/debian|${APT_MIRROR}|g" /etc/apt/sources.list.d/debian.sources

RUN apt-get -o Acquire::Retries=5 update \
    && apt-get -o Acquire::Retries=5 install -y --no-install-recommends \
        bash \
        binutils \
        build-essential \
        ca-certificates \
        clang \
        cmake \
        coreutils \
        curl \
        default-jdk-headless \
        dnsutils \
        file \
        findutils \
        g++ \
        gcc \
        gdb \
        gdbserver \
        git \
        iproute2 \
        jq \
        less \
        lldb \
        ltrace \
        make \
        nano \
        net-tools \
        netcat-openbsd \
        nmap \
        p7zip-full \
        pipx \
        procps \
        python3-dev \
        python3-venv \
        ripgrep \
        socat \
        strace \
        tcpdump \
        tmux \
        unzip \
        vim-tiny \
        wget \
        xz-utils \
        xxd \
    && rm -rf /var/lib/apt/lists/*

COPY requirements.lock /tmp/requirements.lock

RUN python -m pip install --no-cache-dir --upgrade \
        pip==26.1.2 \
        setuptools==83.0.0 \
        wheel==0.47.0 \
    && python -m pip install --no-cache-dir -r /tmp/requirements.lock \
    && rm -f /tmp/requirements.lock

RUN groupadd --gid 1000 ctf \
    && useradd --uid 1000 --gid ctf --create-home --shell /bin/bash ctf \
    && mkdir -p /workspace /home/ctf/.cache /home/ctf/.local/share/opencode \
    && chown -R ctf:ctf /workspace /home/ctf \
    && printf '%s\n' 'export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin' \
        > /etc/profile.d/ctf-agent-path.sh

ENV PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin

WORKDIR /workspace

CMD ["python", "--version"]
