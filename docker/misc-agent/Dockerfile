FROM ctf-agent-opencode:latest

ENV PYTHONUNBUFFERED=1
ENV DEBIAN_FRONTEND=noninteractive

ARG APT_MIRROR=http://deb.debian.org/debian
ARG APT_SECURITY_MIRROR=http://deb.debian.org/debian-security

RUN sed -i "s|http://mirrors.ustc.edu.cn/debian-security|${APT_SECURITY_MIRROR}|g; \
    s|http://mirrors.ustc.edu.cn/debian|${APT_MIRROR}|g" /etc/apt/sources.list.d/debian.sources

RUN apt-get -o Acquire::Retries=5 update \
    && apt-get -o Acquire::Retries=5 install -y --no-install-recommends \
        dnsutils \
        ffmpeg \
        imagemagick \
        libimage-exiftool-perl \
        nodejs \
        npm \
        outguess \
        p7zip-full \
        pngcheck \
        qrencode \
        sox \
        steghide \
        tshark \
        unrar-free \
        zbar-tools \
    && rm -rf /var/lib/apt/lists/*

RUN python -m pip install --no-cache-dir \
        dnslib \
        pymodbus \
        pyzbar \
        qrcode \
        stegpy

WORKDIR /workspace

CMD ["python", "/opt/ctf_agent/agent.py"]
