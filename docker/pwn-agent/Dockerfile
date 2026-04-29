FROM ctf-agent-opencode:latest

ENV PYTHONUNBUFFERED=1
ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get -o Acquire::Retries=5 update \
    && apt-get -o Acquire::Retries=5 install -y --no-install-recommends \
        build-essential \
        gcc-multilib \
        g++-multilib \
        libc6-dbg \
        libseccomp-dev \
        patchelf \
        qemu-user \
        ruby \
        ruby-dev \
    && rm -rf /var/lib/apt/lists/*

RUN python -m pip install --no-cache-dir \
        capstone \
        keystone-engine \
        ropper \
        ROPGadget \
        unicorn

RUN gem install --no-document \
        one_gadget \
        seccomp-tools

WORKDIR /workspace

CMD ["python", "/opt/ctf_agent/agent.py"]
