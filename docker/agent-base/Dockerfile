FROM python:3.12-slim

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

RUN python -m pip install --no-cache-dir --upgrade \
        pip \
        setuptools \
        wheel \
    && python -m pip install --no-cache-dir \
        beautifulsoup4 \
        cryptography \
        dnspython \
        httpx \
        lxml \
        numpy \
        Pillow \
        pwntools \
        pycryptodome \
        python-magic \
        requests \
        scapy \
        sympy \
        z3-solver

WORKDIR /workspace

CMD ["python", "--version"]
