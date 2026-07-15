FROM ctf-agent-opencode:latest

USER root

ENV PYTHONUNBUFFERED=1
ENV DEBIAN_FRONTEND=noninteractive

ARG APT_MIRROR=http://deb.debian.org/debian
ARG APT_SECURITY_MIRROR=http://deb.debian.org/debian-security

RUN sed -i "s|http://mirrors.ustc.edu.cn/debian-security|${APT_SECURITY_MIRROR}|g; \
    s|http://mirrors.ustc.edu.cn/debian|${APT_MIRROR}|g" /etc/apt/sources.list.d/debian.sources

RUN apt-get -o Acquire::Retries=5 update \
    && apt-get -o Acquire::Retries=5 install -y --no-install-recommends \
        dnsutils \
        binwalk \
        ffmpeg \
        imagemagick \
        iputils-ping \
        libimage-exiftool-perl \
        dmtx-utils \
        outguess \
        p7zip-full \
        poppler-utils \
        pngcheck \
        qrencode \
        sox \
        steghide \
        tesseract-ocr \
        traceroute \
        tshark \
        unar \
        zbar-tools \
    && rm -rf /var/lib/apt/lists/*

RUN set -eux; \
    printf '%s\n' \
        '#!/bin/sh' \
        'set -eu' \
        'overwrite=' \
        'while [ "$#" -gt 0 ]; do' \
        '  case "$1" in' \
        '    x|e) shift ;;' \
        '    -o+) overwrite=1; shift ;;' \
        '    -o-) shift ;;' \
        '    --) shift; break ;;' \
        '    -*) shift ;;' \
        '    *) break ;;' \
        '  esac' \
        'done' \
        'archive="${1:-}"' \
        '[ -n "$archive" ] || { echo "usage: unrar x <archive> [dest]" >&2; exit 2; }' \
        'shift || true' \
        'dest="${1:-.}"' \
        'if [ -n "$overwrite" ]; then' \
        '  exec unar -f -o "$dest" "$archive"' \
        'fi' \
        'exec unar -o "$dest" "$archive"' \
        > /usr/local/bin/unrar; \
    chmod +x /usr/local/bin/unrar

COPY requirements.lock /tmp/requirements.lock

RUN python -m pip install --no-cache-dir -r /tmp/requirements.lock \
    && rm -f /tmp/requirements.lock

WORKDIR /workspace

USER ctf

CMD ["python", "/opt/ctf_agent/agent.py"]
