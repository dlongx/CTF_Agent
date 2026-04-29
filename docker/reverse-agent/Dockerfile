FROM ctf-agent-opencode:latest

ENV PYTHONUNBUFFERED=1
ENV DEBIAN_FRONTEND=noninteractive

ARG APT_MIRROR=http://deb.debian.org/debian
ARG APT_SECURITY_MIRROR=http://deb.debian.org/debian-security

RUN sed -i "s|http://mirrors.ustc.edu.cn/debian-security|${APT_SECURITY_MIRROR}|g; \
    s|http://mirrors.ustc.edu.cn/debian|${APT_MIRROR}|g" /etc/apt/sources.list.d/debian.sources

RUN apt-get -o Acquire::Retries=5 update \
    && apt-get -o Acquire::Retries=5 install -y --no-install-recommends \
        apktool \
        cmake \
        default-jdk-headless \
        make \
        upx-ucl \
    && rm -rf /var/lib/apt/lists/*

RUN python -m pip install --no-cache-dir \
        angr \
        capstone \
        frida-tools \
        keystone-engine \
        lief \
        qiling \
        unicorn \
        uncompyle6 \
        z3-solver

WORKDIR /workspace

CMD ["python", "/opt/ctf_agent/agent.py"]
