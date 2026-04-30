FROM ctf-agent-opencode:latest

ENV PYTHONUNBUFFERED=1
ENV DEBIAN_FRONTEND=noninteractive
ENV TERM=xterm

ARG APT_MIRROR=http://deb.debian.org/debian
ARG APT_SECURITY_MIRROR=http://deb.debian.org/debian-security

RUN sed -i "s|http://mirrors.ustc.edu.cn/debian-security|${APT_SECURITY_MIRROR}|g; \
    s|http://mirrors.ustc.edu.cn/debian|${APT_MIRROR}|g" /etc/apt/sources.list.d/debian.sources

RUN dpkg --add-architecture i386 \
    && apt-get -o Acquire::Retries=5 update \
    && apt-get -o Acquire::Retries=5 install -y --no-install-recommends \
        bc \
        bison \
        busybox-static \
        build-essential \
        checksec \
        cpio \
        elfutils \
        flex \
        gcc-multilib \
        g++-multilib \
        gdb-multiarch \
        gdbserver \
        libc6-i386 \
        libc6-dbg \
        libc6-dbg:i386 \
        libelf-dev \
        libgcc-s1:i386 \
        libncurses-dev \
        lib32stdc++6 \
        libseccomp-dev \
        libssl-dev \
        libstdc++6:i386 \
        kmod \
        ncurses-term \
        patchelf \
        qemu-system-x86 \
        qemu-user \
        qemu-user-binfmt \
        qemu-utils \
        rsync \
        ruby \
        ruby-dev \
    && rm -rf /var/lib/apt/lists/*

RUN python -m pip install --no-cache-dir \
        capstone \
        keystone-engine \
        LibcSearcher \
        ropper \
        ROPGadget \
        unicorn

RUN gem install --no-document \
        one_gadget \
        seccomp-tools

WORKDIR /workspace

CMD ["python", "/opt/ctf_agent/agent.py"]
