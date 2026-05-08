FROM ctf-agent-opencode:latest

ENV PYTHONUNBUFFERED=1
ENV DEBIAN_FRONTEND=noninteractive

ARG APT_MIRROR=http://deb.debian.org/debian
ARG APT_SECURITY_MIRROR=http://deb.debian.org/debian-security

RUN sed -i "s|http://mirrors.ustc.edu.cn/debian-security|${APT_SECURITY_MIRROR}|g; \
    s|http://mirrors.ustc.edu.cn/debian|${APT_MIRROR}|g" /etc/apt/sources.list.d/debian.sources

RUN apt-get -o Acquire::Retries=10 -o Acquire::http::Pipeline-Depth=0 -o Acquire::http::No-Cache=true update \
    && apt-get -o Acquire::Retries=10 -o Acquire::http::Pipeline-Depth=0 -o Acquire::http::No-Cache=true install -y --no-install-recommends \
        binwalk \
        foremost \
        john \
        libimage-exiftool-perl \
        outguess \
        p7zip-full \
        pcapfix \
        pngcheck \
        poppler-utils \
        qrencode \
        sleuthkit \
        steghide \
        testdisk \
        unrar-free \
    && rm -rf /var/lib/apt/lists/*

RUN apt-get -o Acquire::Retries=10 -o Acquire::http::Pipeline-Depth=0 -o Acquire::http::No-Cache=true update \
    && apt-get -o Acquire::Retries=10 -o Acquire::http::Pipeline-Depth=0 -o Acquire::http::No-Cache=true install -y --no-install-recommends \
        dmtx-utils \
        ffmpeg \
        imagemagick \
        iputils-ping \
        sox \
        tesseract-ocr \
        traceroute \
        zbar-tools \
    && rm -rf /var/lib/apt/lists/*

RUN apt-get -o Acquire::Retries=10 -o Acquire::http::Pipeline-Depth=0 -o Acquire::http::No-Cache=true update \
    && apt-get -o Acquire::Retries=10 -o Acquire::http::Pipeline-Depth=0 -o Acquire::http::No-Cache=true install -y --no-install-recommends \
        ruby \
        ruby-dev \
        tshark \
    && rm -rf /var/lib/apt/lists/*

RUN python -m pip install --no-cache-dir \
        construct \
        hachoir \
        matplotlib \
        oletools \
        opencv-python-headless \
        pylibdmtx \
        pysstv \
        pytesseract \
        pyshark \
        scipy \
        stegpy \
        volatility3

RUN gem install --no-document zsteg

WORKDIR /workspace

CMD ["python", "/opt/ctf_agent/agent.py"]
