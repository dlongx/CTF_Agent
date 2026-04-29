FROM ctf-agent-opencode:latest

ENV PYTHONUNBUFFERED=1
ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get -o Acquire::Retries=5 update \
    && apt-get -o Acquire::Retries=5 install -y --no-install-recommends \
        build-essential \
        default-jre-headless \
        dnsutils \
        libcurl4-openssl-dev \
        libssl-dev \
        whois \
    && rm -rf /var/lib/apt/lists/*

RUN python -m pip install --no-cache-dir "pip<24.1" \
    && python -m pip install --no-cache-dir \
        flask-unsign \
        httpx \
        itsdangerous \
        jinja2 \
        PyJWT \
        python-jose \
        sqlmap \
        websockets \
        wfuzz \
        dnspython

WORKDIR /workspace

CMD ["python", "/opt/ctf_agent/agent.py"]
