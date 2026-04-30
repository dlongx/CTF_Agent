FROM ctf-agent-opencode:latest

ENV PYTHONUNBUFFERED=1
ENV DEBIAN_FRONTEND=noninteractive

ARG APT_MIRROR=http://deb.debian.org/debian
ARG APT_SECURITY_MIRROR=http://deb.debian.org/debian-security

RUN sed -i "s|http://mirrors.ustc.edu.cn/debian-security|${APT_SECURITY_MIRROR}|g; \
    s|http://mirrors.ustc.edu.cn/debian|${APT_MIRROR}|g" /etc/apt/sources.list.d/debian.sources

RUN apt-get -o Acquire::Retries=5 update \
    && apt-get -o Acquire::Retries=5 install -y --no-install-recommends \
        build-essential \
        default-jre-headless \
        dnsutils \
        dirb \
        git \
        gobuster \
        golang-go \
        libcurl4-openssl-dev \
        libnet-ssleay-perl \
        libssl-dev \
        perl \
        tshark \
        wafw00f \
        whatweb \
        whois \
    && rm -rf /var/lib/apt/lists/*

RUN git clone --depth 1 https://github.com/sullo/nikto.git /opt/nikto \
    && ln -sf /opt/nikto/program/nikto.pl /usr/local/bin/nikto \
    && chmod +x /usr/local/bin/nikto

RUN PYCURL_SSL_LIBRARY=openssl python -m pip install --no-cache-dir \
        chardet \
        aiohttp \
        flask-unsign \
        itsdangerous \
        jinja2 \
        "pycurl>=7.45.6" \
        "pyparsing>=2.4" \
        mitmproxy \
        PyJWT \
        python-jose \
        requests-toolbelt \
        six \
        sqlmap \
        websocket-client \
        websockets

RUN python -m pip install --no-cache-dir "pip<24.1" \
    && python -m pip install --no-cache-dir --no-deps \
        wfuzz==3.1.0 \
    && python -m pip install --no-cache-dir --upgrade \
        pip \
        setuptools \
        wheel

WORKDIR /workspace

CMD ["python", "/opt/ctf_agent/agent.py"]
