FROM ctf-agent-opencode:latest

ENV PYTHONUNBUFFERED=1
ENV DEBIAN_FRONTEND=noninteractive
ENV ANDROID_STUBS_ARM64=/opt/android-stubs/arm64-v8a
ENV ANDROID_STUBS_X86_64=/opt/android-stubs/x86_64
ENV LD_LIBRARY_PATH=/opt/android-stubs/x86_64:/opt/android-stubs/arm64-v8a

ARG APT_MIRROR=http://deb.debian.org/debian
ARG APT_SECURITY_MIRROR=http://deb.debian.org/debian-security
ARG JADX_VERSION=1.5.2
ARG JADX_SHA256=5a8b480839c9c61527895d81d5572182279d973abe112047417f237df958a3aa

RUN sed -i "s|http://mirrors.ustc.edu.cn/debian-security|${APT_SECURITY_MIRROR}|g; \
    s|http://mirrors.ustc.edu.cn/debian|${APT_MIRROR}|g" /etc/apt/sources.list.d/debian.sources

RUN apt-get -o Acquire::Retries=5 update \
    && apt-get -o Acquire::Retries=5 install -y --no-install-recommends \
        apktool \
        adb \
        aapt \
        apksigner \
        binutils \
        build-essential \
        cmake \
        default-jdk-headless \
        elfutils \
        fastboot \
        file \
        gcc \
        gcc-aarch64-linux-gnu \
        g++ \
        make \
        unzip \
        upx-ucl \
        wget \
        xxd \
        zipalign \
        zip \
    && rm -rf /var/lib/apt/lists/*

RUN set -eux; \
    wget -O /tmp/jadx.zip "https://github.com/skylot/jadx/releases/download/v${JADX_VERSION}/jadx-${JADX_VERSION}.zip"; \
    echo "${JADX_SHA256}  /tmp/jadx.zip" | sha256sum -c -; \
    mkdir -p /opt/jadx; \
    unzip -q /tmp/jadx.zip -d /opt/jadx; \
    chmod +x /opt/jadx/bin/jadx /opt/jadx/bin/jadx-gui; \
    ln -sf /opt/jadx/bin/jadx /usr/local/bin/jadx; \
    ln -sf /opt/jadx/bin/jadx-gui /usr/local/bin/jadx-gui; \
    rm -f /tmp/jadx.zip

RUN set -eux; \
    mkdir -p "${ANDROID_STUBS_ARM64}" "${ANDROID_STUBS_X86_64}"; \
    printf '%s\n' \
        'int __android_log_print(int prio, const char *tag, const char *fmt, ...) { return 0; }' \
        'int __android_log_write(int prio, const char *tag, const char *text) { return 0; }' \
        'int __android_log_vprint(int prio, const char *tag, const char *fmt, void *args) { return 0; }' \
        > /tmp/liblog_stub.c; \
    printf '%s\n' \
        'void *AAssetManager_fromJava(void *env, void *assetManager) { return (void *)0; }' \
        'void *AAssetManager_open(void *mgr, const char *filename, int mode) { return (void *)0; }' \
        'int AAsset_read(void *asset, void *buf, unsigned long count) { return -1; }' \
        'int AAsset_seek(void *asset, int offset, int whence) { return -1; }' \
        'long AAsset_getLength(void *asset) { return 0; }' \
        'long AAsset_getLength64(void *asset) { return 0; }' \
        'long AAsset_getRemainingLength(void *asset) { return 0; }' \
        'long AAsset_getRemainingLength64(void *asset) { return 0; }' \
        'const void *AAsset_getBuffer(void *asset) { return (void *)0; }' \
        'void AAsset_close(void *asset) {}' \
        > /tmp/libandroid_stub.c; \
    gcc -shared -fPIC -nostdlib -Wl,-soname,liblog.so -o "${ANDROID_STUBS_X86_64}/liblog.so" /tmp/liblog_stub.c; \
    gcc -shared -fPIC -nostdlib -Wl,-soname,libandroid.so -o "${ANDROID_STUBS_X86_64}/libandroid.so" /tmp/libandroid_stub.c; \
    aarch64-linux-gnu-gcc -shared -fPIC -nostdlib -Wl,-soname,liblog.so -o "${ANDROID_STUBS_ARM64}/liblog.so" /tmp/liblog_stub.c; \
    aarch64-linux-gnu-gcc -shared -fPIC -nostdlib -Wl,-soname,libandroid.so -o "${ANDROID_STUBS_ARM64}/libandroid.so" /tmp/libandroid_stub.c; \
    rm -f /tmp/liblog_stub.c /tmp/libandroid_stub.c

RUN python -m pip install --no-cache-dir \
        angr \
        androguard \
        angrop \
        capstone \
        frida-tools \
        keystone-engine \
        lief \
        objection \
        qiling \
        r2pipe \
        unicorn \
        uncompyle6 \
        z3-solver

WORKDIR /workspace

CMD ["python", "/opt/ctf_agent/agent.py"]
