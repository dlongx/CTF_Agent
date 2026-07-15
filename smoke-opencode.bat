@echo off
setlocal EnableExtensions EnableDelayedExpansion

set "BASE_URL=%CTF_AGENT_SMOKE_URL%"
if "%BASE_URL%"=="" set "BASE_URL=http://127.0.0.1:8000"
set "SMOKE_TOKEN=%CTF_AGENT_SMOKE_TOKEN%"
if "%SMOKE_TOKEN%"=="" set "SMOKE_TOKEN=%CTF_AGENT_ACCESS_TOKEN%"

set "TASK_NAME=smoke-opencode-%RANDOM%"
set "WORK_DIR=%TEMP%\ctf-agent-smoke-%RANDOM%"
set "ATTACHMENT=%WORK_DIR%\challenge.txt"

mkdir "%WORK_DIR%" >nul 2>nul
if errorlevel 1 (
  echo failed to create smoke work dir
  exit /b 1
)

curl.exe -sS -f -H "Authorization: Bearer %SMOKE_TOKEN%" "%BASE_URL%/api/settings/provider" -o "%WORK_DIR%\provider.json"
if errorlevel 1 (
  echo [smoke] provider settings query failed
  exit /b 1
)
for /f "usebackq delims=" %%P in (`powershell -NoProfile -Command "$p=Get-Content -Raw -Encoding UTF8 '%WORK_DIR%\provider.json' | ConvertFrom-Json; $p.active_format"`) do set "PROVIDER_FORMAT=%%P"
if "%PROVIDER_FORMAT%"=="" (
  echo [smoke] active provider is unavailable
  exit /b 1
)
> "%WORK_DIR%\provider-test.json" echo {"format":"%PROVIDER_FORMAT%"}
for /f "usebackq delims=" %%H in (`curl.exe -sS -H "Authorization: Bearer %SMOKE_TOKEN%" -H "Content-Type: application/json" -X POST --data-binary "@%WORK_DIR%\provider-test.json" -o "%WORK_DIR%\provider-test-result.json" -w "%%{http_code}" "%BASE_URL%/api/settings/provider/test"`) do set "PROVIDER_HTTP=%%H"
if not "%PROVIDER_HTTP%"=="200" (
  echo [smoke] provider preflight failed HTTP %PROVIDER_HTTP%
  type "%WORK_DIR%\provider-test-result.json"
  exit /b 1
)

> "%ATTACHMENT%" echo The flag is flag{ctf_agent_smoke_ok}
>> "%ATTACHMENT%" echo Please output the exact flag.

echo [smoke] submitting %TASK_NAME% to %BASE_URL%
curl.exe -sS -f -X POST "%BASE_URL%/api/tasks" ^
  -H "Authorization: Bearer %SMOKE_TOKEN%" ^
  -F "name=%TASK_NAME%" ^
  -F "type=misc" ^
  -F "description=Read the attached text file and return the exact flag. Then write the required Chinese WP." ^
  -F "attachments=@%ATTACHMENT%" ^
  -o "%WORK_DIR%\submit.json"
if errorlevel 1 (
  echo [smoke] submit failed
  exit /b 1
)

for /f "usebackq delims=" %%I in (`powershell -NoProfile -Command "$p=Get-Content -Raw -Encoding UTF8 '%WORK_DIR%\submit.json' | ConvertFrom-Json; $p.id"`) do set "TASK_ID=%%I"
if "%TASK_ID%"=="" (
  echo [smoke] could not parse task id
  type "%WORK_DIR%\submit.json"
  exit /b 1
)

echo [smoke] task_id=%TASK_ID%
for /f "delims=" %%T in ('powershell -NoProfile -Command "[DateTimeOffset]::UtcNow.AddMinutes(10).ToUnixTimeSeconds()"') do set "DEADLINE=%%T"

:poll
for /f "delims=" %%T in ('powershell -NoProfile -Command "[DateTimeOffset]::UtcNow.ToUnixTimeSeconds()"') do set "NOW=%%T"
if !NOW! GEQ !DEADLINE! goto timed_out
curl.exe -sS -f -H "Authorization: Bearer %SMOKE_TOKEN%" "%BASE_URL%/api/tasks/%TASK_ID%" -o "%WORK_DIR%\task.json"
if errorlevel 1 (
  echo [smoke] task query failed
  call :stop_task
  exit /b 1
)
for /f "usebackq delims=" %%S in (`powershell -NoProfile -Command "$p=Get-Content -Raw -Encoding UTF8 '%WORK_DIR%\task.json' | ConvertFrom-Json; $p.status"`) do set "TASK_STATUS=%%S"
for /f "usebackq delims=" %%F in (`powershell -NoProfile -Command "$p=Get-Content -Raw -Encoding UTF8 '%WORK_DIR%\task.json' | ConvertFrom-Json; $p.flag"`) do set "TASK_FLAG=%%F"
echo [smoke] status=!TASK_STATUS! flag=!TASK_FLAG!
if "!TASK_STATUS!"=="solved" goto solved
if "!TASK_STATUS!"=="failed" goto failed
powershell -NoProfile -Command "Start-Sleep -Seconds 5"
goto poll

:timed_out
echo [smoke] timed out waiting for task
call :stop_task
exit /b 1

:solved
if not "%TASK_FLAG%"=="flag{ctf_agent_smoke_ok}" (
  echo [smoke] unexpected flag: %TASK_FLAG%
  exit /b 1
)
curl.exe -sS -f -H "Authorization: Bearer %SMOKE_TOKEN%" "%BASE_URL%/api/tasks/%TASK_ID%/writeup" -o "%WORK_DIR%\writeup.md"
if errorlevel 1 (
  echo [smoke] writeup download failed
  exit /b 1
)
echo [smoke] passed. writeup=%WORK_DIR%\writeup.md
exit /b 0

:failed
echo [smoke] task failed
curl.exe -sS -H "Authorization: Bearer %SMOKE_TOKEN%" "%BASE_URL%/api/tasks/%TASK_ID%/logs?tail=12000"
call :stop_task
exit /b 1

:stop_task
if "%TASK_ID%"=="" exit /b 0
curl.exe -sS -H "Authorization: Bearer %SMOKE_TOKEN%" -X POST "%BASE_URL%/api/tasks/%TASK_ID%/stop" >nul 2>nul
exit /b 0
