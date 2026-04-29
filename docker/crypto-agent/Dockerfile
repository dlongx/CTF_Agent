FROM ctf-agent-opencode:latest

ENV PYTHONUNBUFFERED=1
ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get -o Acquire::Retries=5 update \
    && apt-get -o Acquire::Retries=5 install -y --no-install-recommends \
        g++ \
        hashcat \
        libgmp-dev \
        libmpc-dev \
        libmpfr-dev \
        pari-gp \
    && rm -rf /var/lib/apt/lists/*

RUN python -m pip install --no-cache-dir \
        cysignals \
        ecdsa \
        fpylll \
        gmpy2 \
        hashpumpy \
        pycryptodome \
        py_ecc \
        sympy \
        z3-solver

WORKDIR /workspace

CMD ["python", "/opt/ctf_agent/agent.py"]
