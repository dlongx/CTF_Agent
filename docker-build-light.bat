@echo off
setlocal

set "ROOT=%~dp0"
if "%ROOT:~-1%"=="\" set "ROOT=%ROOT:~0,-1%"
set "LOG_DIR=%ROOT%\data"
set "LATEST_LOG=%LOG_DIR%\docker-build-light.log"
set "REBUILD_BASE=0"
set "TARGET=all"
set "APT_MIRROR=%CTF_AGENT_APT_MIRROR%"
set "APT_SECURITY_MIRROR=%CTF_AGENT_APT_SECURITY_MIRROR%"
if "%APT_MIRROR%"=="" set "APT_MIRROR=http://mirrors.tuna.tsinghua.edu.cn/debian"
if "%APT_SECURITY_MIRROR%"=="" set "APT_SECURITY_MIRROR=http://mirrors.tuna.tsinghua.edu.cn/debian-security"

:parse_args
if "%~1"=="" goto :args_done
if /I "%~1"=="--rebuild-base" (
    set "REBUILD_BASE=1"
    shift
    goto :parse_args
)
if /I "%~1"=="--mirror" (
    if "%~2"=="" goto :usage
    call :set_mirror "%~2" || goto :usage
    shift
    shift
    goto :parse_args
)
if /I "%~1"=="--apt-mirror" (
    if "%~2"=="" goto :usage
    set "APT_MIRROR=%~2"
    shift
    shift
    goto :parse_args
)
if /I "%~1"=="--apt-security-mirror" (
    if "%~2"=="" goto :usage
    set "APT_SECURITY_MIRROR=%~2"
    shift
    shift
    goto :parse_args
)
set "TARGET=%~1"
shift
goto :parse_args

:args_done

if not exist "%LOG_DIR%" mkdir "%LOG_DIR%"
for /f "delims=" %%I in ('powershell -NoProfile -Command "Get-Date -Format yyyyMMdd-HHmmss-fff"') do set "RUN_ID=%%I"
if "%RUN_ID%"=="" set "RUN_ID=%RANDOM%"
set "LOG=%LOG_DIR%\docker-build-light-%RUN_ID%-%RANDOM%.log"

echo Building lightweight CTF Agent images...
echo Log file: %LOG%
echo Target: %TARGET%
echo APT mirror: %APT_MIRROR%
echo APT security mirror: %APT_SECURITY_MIRROR%
echo.

(
    echo ============================================================
    echo Build started: %DATE% %TIME%
    echo Root: %ROOT%
    echo Rebuild base: %REBUILD_BASE%
    echo Target: %TARGET%
    echo APT mirror: %APT_MIRROR%
    echo APT security mirror: %APT_SECURITY_MIRROR%
    echo ============================================================
) > "%LOG%"

if /I "%TARGET%"=="help" goto :usage
if /I "%TARGET%"=="verify" (
    call :check_docker || goto :failed
    call :verify_images || goto :failed
    goto :success
)
if /I not "%TARGET%"=="all" if /I not "%TARGET%"=="each" if /I not "%TARGET%"=="base" if /I not "%TARGET%"=="opencode" if /I not "%TARGET%"=="web" if /I not "%TARGET%"=="pwn" if /I not "%TARGET%"=="crypto" if /I not "%TARGET%"=="reverse" if /I not "%TARGET%"=="forensics" if /I not "%TARGET%"=="misc" goto :unknown_target
call :check_docker || goto :failed
if /I "%TARGET%"=="each" (
    call :build_image ctf-agent-base:latest "%ROOT%\docker\agent-base" || goto :failed
    call :build_image ctf-agent-opencode:latest "%ROOT%\docker\opencode-agent" || goto :failed
    call :build_image ctf-agent-web:latest "%ROOT%\docker\web-agent" || goto :failed
    call :build_image ctf-agent-pwn:latest "%ROOT%\docker\pwn-agent" || goto :failed
    call :build_image ctf-agent-crypto:latest "%ROOT%\docker\crypto-agent" || goto :failed
    call :build_image ctf-agent-reverse:latest "%ROOT%\docker\reverse-agent" || goto :failed
    call :build_image ctf-agent-forensics:latest "%ROOT%\docker\forensics-agent" || goto :failed
    call :build_image ctf-agent-misc:latest "%ROOT%\docker\misc-agent" || goto :failed
    goto :success
)
if /I "%TARGET%"=="base" (
    call :build_image ctf-agent-base:latest "%ROOT%\docker\agent-base" || goto :failed
    goto :success
)
if /I "%TARGET%"=="opencode" (
    call :ensure_base || goto :failed
    call :build_image ctf-agent-opencode:latest "%ROOT%\docker\opencode-agent" || goto :failed
    goto :success
)
call :ensure_opencode || goto :failed

if /I "%TARGET%"=="web" (
    call :build_image ctf-agent-web:latest "%ROOT%\docker\web-agent" || goto :failed
    goto :success
)
if /I "%TARGET%"=="pwn" (
    call :build_image ctf-agent-pwn:latest "%ROOT%\docker\pwn-agent" || goto :failed
    goto :success
)
if /I "%TARGET%"=="crypto" (
    call :build_image ctf-agent-crypto:latest "%ROOT%\docker\crypto-agent" || goto :failed
    goto :success
)
if /I "%TARGET%"=="reverse" (
    call :build_image ctf-agent-reverse:latest "%ROOT%\docker\reverse-agent" || goto :failed
    goto :success
)
if /I "%TARGET%"=="forensics" (
    call :build_image ctf-agent-forensics:latest "%ROOT%\docker\forensics-agent" || goto :failed
    goto :success
)
if /I "%TARGET%"=="misc" (
    call :build_image ctf-agent-misc:latest "%ROOT%\docker\misc-agent" || goto :failed
    goto :success
)
if /I not "%TARGET%"=="all" goto :unknown_target

call :build_image ctf-agent-web:latest "%ROOT%\docker\web-agent" || goto :failed
call :build_image ctf-agent-pwn:latest "%ROOT%\docker\pwn-agent" || goto :failed
call :build_image ctf-agent-crypto:latest "%ROOT%\docker\crypto-agent" || goto :failed
call :build_image ctf-agent-reverse:latest "%ROOT%\docker\reverse-agent" || goto :failed
call :build_image ctf-agent-forensics:latest "%ROOT%\docker\forensics-agent" || goto :failed
call :build_image ctf-agent-misc:latest "%ROOT%\docker\misc-agent" || goto :failed
goto :success

:ensure_base
if "%REBUILD_BASE%"=="1" (
    call :build_image ctf-agent-base:latest "%ROOT%\docker\agent-base" || exit /b 1
) else (
    docker image inspect ctf-agent-base:latest >nul 2>nul
    if errorlevel 1 (
        call :build_image ctf-agent-base:latest "%ROOT%\docker\agent-base" || exit /b 1
    ) else (
        echo Reusing existing image ctf-agent-base:latest
        echo Reusing existing image ctf-agent-base:latest>>"%LOG%"
    )
)
exit /b 0

:ensure_opencode
call :ensure_base || exit /b 1
if "%REBUILD_BASE%"=="1" (
    call :build_image ctf-agent-opencode:latest "%ROOT%\docker\opencode-agent" || exit /b 1
) else (
    docker image inspect ctf-agent-opencode:latest >nul 2>nul
    if errorlevel 1 (
        call :build_image ctf-agent-opencode:latest "%ROOT%\docker\opencode-agent" || exit /b 1
    ) else (
        echo Reusing existing image ctf-agent-opencode:latest
        echo Reusing existing image ctf-agent-opencode:latest>>"%LOG%"
    )
)
exit /b 0

:success
echo.
echo Build completed.
echo Build finished successfully: %DATE% %TIME%>>"%LOG%"
copy /Y "%LOG%" "%LATEST_LOG%" >nul 2>nul

endlocal
exit /b 0

:verify_images
call :verify_image ctf-agent-base:latest "requests,bs4,cryptography,lxml,pwn,Crypto,z3,httpx,dns.resolver,numpy,PIL,scapy,sympy,magic" || exit /b 1
call :verify_commands ctf-agent-base:latest "bash curl wget unzip 7z xz file rg python gcc g++ make cmake clang lldb gdb gdbserver strace ltrace nc socat nmap tcpdump tmux jq" || exit /b 1
call :verify_image ctf-agent-opencode:latest "requests,bs4,cryptography,lxml,pwn,Crypto,z3,httpx,dns.resolver,numpy,PIL,scapy,sympy,magic" || exit /b 1
call :verify_image ctf-agent-web:latest "jwt,jose,flask_unsign,websockets,aiohttp,requests_toolbelt,websocket" || exit /b 1
call :verify_commands ctf-agent-web:latest "sqlmap dirb gobuster nikto whatweb wafw00f mitmproxy tshark whois go ping traceroute" || exit /b 1
call :verify_image ctf-agent-misc:latest "pyzbar,qrcode,dnslib,pymodbus,stegpy,scipy,cv2,pytesseract,pylibdmtx,pysstv" || exit /b 1
call :verify_commands ctf-agent-misc:latest "ffmpeg sox zbarimg qrencode exiftool pngcheck steghide outguess tshark 7z unrar-free binwalk tesseract dmtxread ping traceroute" || exit /b 1
call :verify_image ctf-agent-crypto:latest "gmpy2,fpylll,ecdsa,py_ecc,libnum,owiener,primefac" || exit /b 1
call :verify_commands ctf-agent-crypto:latest "openssl john hashcat gp" || exit /b 1
call :verify_image ctf-agent-pwn:latest "pwn,capstone,keystone,unicorn,ropper,LibcSearcher" || exit /b 1
call :verify_commands ctf-agent-pwn:latest "checksec gdb gdb-multiarch gdbserver strace ltrace file readelf objdump eu-readelf xxd nc socat nmap patchelf qemu-x86_64 qemu-system-x86_64 cpio busybox" || exit /b 1
call :verify_image ctf-agent-reverse:latest "angr,lief,qiling,frida_tools,capstone,unicorn,androguard,angrop,objection,r2pipe,scipy,cv2,pytesseract,pylibdmtx,pysstv" || exit /b 1
call :verify_commands ctf-agent-reverse:latest "apktool jadx adb fastboot aapt apksigner zipalign gcc g++ aarch64-linux-gnu-gcc readelf objdump eu-readelf file xxd unzip zip upx wine wine64 qemu-i386 qemu-x86_64 r2 radare2 tesseract ping traceroute" || exit /b 1
call :verify_image ctf-agent-forensics:latest "volatility3,construct,pyshark,hachoir,oletools,stegpy,scipy,cv2,pytesseract,pylibdmtx,pysstv" || exit /b 1
call :verify_commands ctf-agent-forensics:latest "binwalk foremost exiftool pngcheck zbarimg qrencode steghide outguess ffmpeg sox convert 7z unrar-free john tshark tcpdump pcapfix pdfinfo fls zsteg tesseract dmtxread ping traceroute" || exit /b 1
exit /b 0

:verify_image
set "IMAGE=%~1"
set "MODULES=%~2"
echo [verify] %IMAGE%
echo.>>"%LOG%"
echo [verify] %IMAGE%>>"%LOG%"
docker run --rm "%IMAGE%" python -c "import importlib; mods='%MODULES%'.split(','); [importlib.import_module(m) for m in mods if m]; print('ok')" >>"%LOG%" 2>&1
if errorlevel 1 (
    echo [failed] %IMAGE%
    echo [failed] %IMAGE%>>"%LOG%"
    exit /b 1
)
echo [ok] %IMAGE%
echo [ok] %IMAGE%>>"%LOG%"
exit /b 0

:verify_commands
set "IMAGE=%~1"
set "COMMANDS=%~2"
echo [verify commands] %IMAGE%
echo.>>"%LOG%"
echo [verify commands] %IMAGE%>>"%LOG%"
docker run --rm "%IMAGE%" sh -lc "for cmd in %COMMANDS%; do command -v $cmd >/dev/null || exit 1; done; echo ok" >>"%LOG%" 2>&1
if errorlevel 1 (
    echo [failed] %IMAGE%
    echo [failed] %IMAGE%>>"%LOG%"
    exit /b 1
)
echo [ok] %IMAGE%
echo [ok] %IMAGE%>>"%LOG%"
exit /b 0

:build_image
set "IMAGE=%~1"
set "CONTEXT=%~2"
echo [build] %IMAGE%
echo.>>"%LOG%"
echo [build] %IMAGE%>>"%LOG%"
docker build --build-arg "APT_MIRROR=%APT_MIRROR%" --build-arg "APT_SECURITY_MIRROR=%APT_SECURITY_MIRROR%" -t "%IMAGE%" "%CONTEXT%" >>"%LOG%" 2>&1
if errorlevel 1 (
    echo [failed] %IMAGE%
    echo [failed] %IMAGE%>>"%LOG%"
    exit /b 1
)
echo [ok] %IMAGE%
echo [ok] %IMAGE%>>"%LOG%"
exit /b 0

:unknown_target
echo Unknown target: %TARGET%
echo Unknown target: %TARGET%>>"%LOG%"
goto :usage

:check_docker
docker version >nul 2>>"%LOG%"
if errorlevel 1 (
    echo Docker Desktop is not running or the Linux engine is unavailable.
    echo Please start Docker Desktop, wait until it says "Engine running", then run this script again.
    echo If Docker Desktop is already open, run: docker context use desktop-linux
    echo [failed] Docker daemon unavailable>>"%LOG%"
    echo Please start Docker Desktop and wait for the Linux engine.>>"%LOG%"
    exit /b 1
)
exit /b 0

:set_mirror
set "MIRROR_PRESET=%~1"
if /I "%MIRROR_PRESET%"=="ustc" (
    set "APT_MIRROR=http://mirrors.ustc.edu.cn/debian"
    set "APT_SECURITY_MIRROR=http://mirrors.ustc.edu.cn/debian-security"
    exit /b 0
)
if /I "%MIRROR_PRESET%"=="tuna" (
    set "APT_MIRROR=http://mirrors.tuna.tsinghua.edu.cn/debian"
    set "APT_SECURITY_MIRROR=http://mirrors.tuna.tsinghua.edu.cn/debian-security"
    exit /b 0
)
if /I "%MIRROR_PRESET%"=="aliyun" (
    set "APT_MIRROR=http://mirrors.aliyun.com/debian"
    set "APT_SECURITY_MIRROR=http://mirrors.aliyun.com/debian-security"
    exit /b 0
)
if /I "%MIRROR_PRESET%"=="debian" (
    set "APT_MIRROR=http://deb.debian.org/debian"
    set "APT_SECURITY_MIRROR=http://deb.debian.org/debian-security"
    exit /b 0
)
echo Unknown mirror preset: %MIRROR_PRESET%
exit /b 1

:usage
echo.
echo Usage:
echo   docker-build-light.bat [all^|each^|base^|opencode^|web^|pwn^|crypto^|reverse^|forensics^|misc^|verify] [--rebuild-base] [--mirror ustc^|tuna^|aliyun^|debian]
echo.
echo Examples:
echo   docker-build-light.bat each
echo   docker-build-light.bat forensics --mirror tuna
echo   docker-build-light.bat reverse --mirror tuna
echo   docker-build-light.bat crypto
echo   docker-build-light.bat web
echo   docker-build-light.bat all
echo   docker-build-light.bat opencode --rebuild-base
echo   docker-build-light.bat verify
echo.
endlocal
exit /b 1

:failed
echo.
echo Build failed. Full log: %LOG%
echo.
echo Last 120 log lines:
echo ------------------------------------------------------------
powershell -NoProfile -ExecutionPolicy Bypass -Command "if (Test-Path $env:LOG) { Get-Content -LiteralPath $env:LOG -Tail 120 }"
echo ------------------------------------------------------------
echo.
echo Press any key to close this window.
pause >nul
copy /Y "%LOG%" "%LATEST_LOG%" >nul 2>nul
endlocal
exit /b 1
