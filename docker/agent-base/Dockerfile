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
        ca-certificates \
        coreutils \
        curl \
        file \
        findutils \
        gdb \
        git \
        iproute2 \
        jq \
        less \
        ltrace \
        netcat-openbsd \
        nmap \
        procps \
        socat \
        strace \
        tcpdump \
        unzip \
        wget \
        xxd \
    && rm -rf /var/lib/apt/lists/*

RUN python -m pip install --no-cache-dir \
        beautifulsoup4 \
        cryptography \
        lxml \
        pwntools \
        pycryptodome \
        requests \
        z3-solver

WORKDIR /workspace

CMD ["python", "--version"]
