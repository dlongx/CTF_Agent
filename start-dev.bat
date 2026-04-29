@echo off
setlocal

set "ROOT=%~dp0"
if "%ROOT:~-1%"=="\" set "ROOT=%ROOT:~0,-1%"
set "CHECK_ONLY=0"
set "RESTART_PORTS=0"

:parse_args
if "%~1"=="" goto args_done
if /I "%~1"=="--check" set "CHECK_ONLY=1"
if /I "%~1"=="--restart" set "RESTART_PORTS=1"
shift
goto parse_args

:args_done

if exist "%ROOT%\opencode.env" (
    for /f "usebackq eol=# tokens=1,* delims==" %%A in ("%ROOT%\opencode.env") do (
        if not "%%A"=="" set "%%A=%%B"
    )
)

if not defined CTF_AGENT_DOCKER_IMAGE set "CTF_AGENT_DOCKER_IMAGE=ctf-agent-opencode:latest"
if not defined CTF_AGENT_MAX_CONTAINERS set "CTF_AGENT_MAX_CONTAINERS=4"
if not defined CTF_AGENT_PIDS_LIMIT set "CTF_AGENT_PIDS_LIMIT=1024"
if not defined CTF_AGENT_SKILLS_DIR set "CTF_AGENT_SKILLS_DIR=runtime\opencode\skills"
if not defined CTF_AGENT_OPENCODE_WEB_ENABLED set "CTF_AGENT_OPENCODE_WEB_ENABLED=true"
if not defined CTF_AGENT_OPENCODE_BIND_IP set "CTF_AGENT_OPENCODE_BIND_IP=127.0.0.1"
if not defined CTF_AGENT_OPENCODE_PUBLIC_BASE_URL set "CTF_AGENT_OPENCODE_PUBLIC_BASE_URL=http://127.0.0.1"

echo Starting Go CTF Agent development services...
echo.
echo Web UI: http://127.0.0.1:8000
echo Docker image: %CTF_AGENT_DOCKER_IMAGE%
echo Max containers: %CTF_AGENT_MAX_CONTAINERS%
echo Skills dir: %CTF_AGENT_SKILLS_DIR%
echo OpenCode web: %CTF_AGENT_OPENCODE_WEB_ENABLED% bind=%CTF_AGENT_OPENCODE_BIND_IP% public=%CTF_AGENT_OPENCODE_PUBLIC_BASE_URL%
echo OpenCode provider: %OPENCODE_PROVIDER_ID%
echo OpenCode model: %OPENCODE_MODEL%
echo.

if "%CHECK_ONLY%"=="1" (
    endlocal
    exit /b 0
)

if "%RESTART_PORTS%"=="1" (
    echo Cleaning existing development processes on port 8000...
    for /f "tokens=5" %%P in ('netstat -ano ^| findstr /R /C:":8000 .*LISTENING"') do (
        taskkill /F /PID %%P >nul 2>nul
    )
    echo.
)

start "" /D "%ROOT%" cmd /k "go run ./cmd/go-server"

echo Waiting for backend health check...
for /l %%I in (1,1,60) do (
    curl -fsS http://127.0.0.1:8000/health >nul 2>nul
    if not errorlevel 1 goto backend_ready
    timeout /t 1 /nobreak >nul
)
echo [WARN] Web server did not become healthy within 60 seconds. Check the backend window for errors.
endlocal
exit /b 1

:backend_ready
echo Web server is healthy.

endlocal
