FROM ctf-agent-opencode:latest

ENV PYTHONUNBUFFERED=1
ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get -o Acquire::Retries=5 update \
    && apt-get -o Acquire::Retries=5 install -y --no-install-recommends \
        binwalk \
        ffmpeg \
        foremost \
        john \
        libimage-exiftool-perl \
        p7zip-full \
        pcapfix \
        poppler-utils \
        sleuthkit \
        sox \
        steghide \
        testdisk \
        tshark \
        zbar-tools \
        ruby \
        ruby-dev \
    && rm -rf /var/lib/apt/lists/*

RUN python -m pip install --no-cache-dir \
        construct \
        matplotlib \
        numpy \
        Pillow \
        pyshark \
        python-magic \
        scapy \
        volatility3

RUN gem install --no-document zsteg

WORKDIR /workspace

CMD ["python", "/opt/ctf_agent/agent.py"]
