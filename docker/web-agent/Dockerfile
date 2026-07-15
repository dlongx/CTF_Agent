FROM ctf-agent-opencode:latest

USER root

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

COPY requirements.lock /tmp/requirements.lock

RUN PYCURL_SSL_LIBRARY=openssl python -m pip install --no-cache-dir -r /tmp/requirements.lock \
    && rm -f /tmp/requirements.lock

RUN python -m pip install --no-cache-dir pip==24.0 \
    && python -m pip install --no-cache-dir --no-deps \
        wfuzz==3.1.0 \
    && sed -i 's/pyparsing (>=2\.4\*)/pyparsing (>=2.4)/' \
        /usr/local/lib/python3.12/site-packages/wfuzz-3.1.0.dist-info/METADATA \
    && python -m pip install --no-cache-dir --upgrade \
        pip==26.1.2 \
        setuptools==83.0.0 \
        wheel==0.47.0 \
    && python -m pip check

WORKDIR /workspace

USER ctf

CMD ["python", "/opt/ctf_agent/agent.py"]
