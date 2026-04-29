FROM ctf-agent-opencode:latest

ENV PYTHONUNBUFFERED=1
ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get -o Acquire::Retries=5 update \
    && apt-get -o Acquire::Retries=5 install -y --no-install-recommends \
        dnsutils \
        ffmpeg \
        nodejs \
        npm \
        qrencode \
        sox \
        zbar-tools \
    && rm -rf /var/lib/apt/lists/*

RUN python -m pip install --no-cache-dir \
        dnslib \
        numpy \
        Pillow \
        pwntools \
        pyzbar \
        qrcode \
        requests \
        scapy \
        sympy \
        z3-solver

WORKDIR /workspace

CMD ["python", "/opt/ctf_agent/agent.py"]
