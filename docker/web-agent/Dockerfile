FROM ctf-agent-opencode:latest

ENV PYTHONUNBUFFERED=1
ENV DEBIAN_FRONTEND=noninteractive

ARG APT_MIRROR=http://deb.debian.org/debian
ARG APT_SECURITY_MIRROR=http://deb.debian.org/debian-security
ARG NIKTO_COMMIT=c8fcc2940d65482b3ac5dc9a295e90d9c877d068

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
        iputils-ping \
        libcurl4-openssl-dev \
        libjson-perl \
        libnet-ssleay-perl \
        libssl-dev \
        libxml-writer-perl \
        perl \
        tshark \
        traceroute \
        wafw00f \
        whatweb \
        whois \
    && rm -rf /var/lib/apt/lists/*

RUN git clone --filter=blob:none https://github.com/sullo/nikto.git /opt/nikto \
    && git -C /opt/nikto checkout --detach "${NIKTO_COMMIT}" \
    && rm -rf /opt/nikto/.git \
    && ln -sf /opt/nikto/program/nikto.pl /usr/local/bin/nikto \
    && chmod +x /usr/local/bin/nikto

RUN PYCURL_SSL_LIBRARY=openssl python -m pip install --no-cache-dir \
        chardet==7.4.3 \
        aiohttp==3.13.5 \
        flask-unsign==1.2.1 \
        itsdangerous==2.2.0 \
        Jinja2==3.1.6 \
        pycurl==7.46.0 \
        pyparsing==3.3.2 \
        mitmproxy==12.2.2 \
        PyJWT==2.12.1 \
        python-jose==3.5.0 \
        requests-toolbelt==1.0.0 \
        six==1.17.0 \
        sqlmap==1.10.3 \
        websocket-client==1.9.0 \
        websockets==16.0

RUN python -m pip install --no-cache-dir "pip<24.1" \
    && python -m pip install --no-cache-dir --no-deps \
        wfuzz==3.1.0 \
    && python -m pip install --no-cache-dir --upgrade \
        pip \
        setuptools \
        wheel

WORKDIR /workspace

CMD ["python", "/opt/ctf_agent/agent.py"]
