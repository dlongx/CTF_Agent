FROM ctf-agent-opencode:latest

ENV PYTHONUNBUFFERED=1
ENV DEBIAN_FRONTEND=noninteractive

ARG APT_MIRROR=http://deb.debian.org/debian
ARG APT_SECURITY_MIRROR=http://deb.debian.org/debian-security

RUN sed -i "s|http://mirrors.ustc.edu.cn/debian-security|${APT_SECURITY_MIRROR}|g; \
    s|http://mirrors.ustc.edu.cn/debian|${APT_MIRROR}|g" /etc/apt/sources.list.d/debian.sources

RUN apt-get -o Acquire::Retries=5 update \
    && apt-get -o Acquire::Retries=5 install -y --no-install-recommends \
        binwalk \
        ffmpeg \
        foremost \
        imagemagick \
        john \
        libimage-exiftool-perl \
        outguess \
        p7zip-full \
        pcapfix \
        pngcheck \
        poppler-utils \
        qrencode \
        sleuthkit \
        sox \
        steghide \
        testdisk \
        tshark \
        unrar-free \
        zbar-tools \
        ruby \
        ruby-dev \
    && rm -rf /var/lib/apt/lists/*

RUN python -m pip install --no-cache-dir \
        construct \
        hachoir \
        matplotlib \
        oletools \
        pyshark \
        stegpy \
        volatility3

RUN gem install --no-document zsteg

WORKDIR /workspace

CMD ["python", "/opt/ctf_agent/agent.py"]
