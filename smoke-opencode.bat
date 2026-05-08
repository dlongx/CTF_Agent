@echo off
setlocal EnableExtensions EnableDelayedExpansion

set "BASE_URL=%CTF_AGENT_SMOKE_URL%"
if "%BASE_URL%"=="" set "BASE_URL=http://127.0.0.1:8000"

set "TASK_NAME=smoke-opencode-%RANDOM%"
set "WORK_DIR=%TEMP%\ctf-agent-smoke-%RANDOM%"
set "ATTACHMENT=%WORK_DIR%\challenge.txt"

mkdir "%WORK_DIR%" >nul 2>nul
if errorlevel 1 (
  echo failed to create smoke work dir
  exit /b 1
)

> "%ATTACHMENT%" echo The flag is flag{ctf_agent_smoke_ok}
>> "%ATTACHMENT%" echo Please output the exact flag.

echo [smoke] submitting %TASK_NAME% to %BASE_URL%
curl.exe -sS -f -X POST "%BASE_URL%/api/tasks" ^
  -F "name=%TASK_NAME%" ^
  -F "type=misc" ^
  -F "description=Read the attached text file and return the exact flag. Then write the required Chinese WP." ^
  -F "attachments=@%ATTACHMENT%" ^
  -o "%WORK_DIR%\submit.json"
if errorlevel 1 (
  echo [smoke] submit failed
  exit /b 1
)

for /f "usebackq delims=" %%I in (`powershell -NoProfile -Command "$p=Get-Content -Raw '%WORK_DIR%\submit.json' | ConvertFrom-Json; $p.id"`) do set "TASK_ID=%%I"
if "%TASK_ID%"=="" (
  echo [smoke] could not parse task id
  type "%WORK_DIR%\submit.json"
  exit /b 1
)

echo [smoke] task_id=%TASK_ID%
for /l %%I in (1,1,120) do (
  curl.exe -sS -f "%BASE_URL%/api/tasks/%TASK_ID%" -o "%WORK_DIR%\task.json"
  if errorlevel 1 (
    echo [smoke] task query failed
    exit /b 1
  )
  for /f "usebackq delims=" %%S in (`powershell -NoProfile -Command "$p=Get-Content -Raw '%WORK_DIR%\task.json' | ConvertFrom-Json; $p.status"`) do set "TASK_STATUS=%%S"
  for /f "usebackq delims=" %%F in (`powershell -NoProfile -Command "$p=Get-Content -Raw '%WORK_DIR%\task.json' | ConvertFrom-Json; $p.flag"`) do set "TASK_FLAG=%%F"
  echo [smoke] status=!TASK_STATUS! flag=!TASK_FLAG!
  if "!TASK_STATUS!"=="solved" goto solved
  if "!TASK_STATUS!"=="failed" goto failed
  timeout /t 5 /nobreak >nul
)

echo [smoke] timed out waiting for task
exit /b 1

:solved
if not "%TASK_FLAG%"=="flag{ctf_agent_smoke_ok}" (
  echo [smoke] unexpected flag: %TASK_FLAG%
  exit /b 1
)
curl.exe -sS -f "%BASE_URL%/api/tasks/%TASK_ID%/writeup" -o "%WORK_DIR%\writeup.md"
if errorlevel 1 (
  echo [smoke] writeup download failed
  exit /b 1
)
echo [smoke] passed. writeup=%WORK_DIR%\writeup.md
exit /b 0

:failed
echo [smoke] task failed
curl.exe -sS "%BASE_URL%/api/tasks/%TASK_ID%/logs?tail=12000"
exit /b 1
