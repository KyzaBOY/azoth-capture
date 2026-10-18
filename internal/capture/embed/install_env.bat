@echo off
setlocal EnableDelayedExpansion

net session >nul 2>&1
if %errorlevel% == 0 (
    echo ERROR: Do not run as administrator.
    if not defined APP_LAUNCHED pause
    exit /b 1
)

set "APP_LAUNCHED="
if /I "%~1"=="__APP_LAUNCHED__" set "APP_LAUNCHED=1"

set "ROOT=%~dp0"
if "%ROOT:~-1%"=="\" set "ROOT=%ROOT:~0,-1%"

set "CONFIG_FILE=%ROOT%\config.json"
set "NW_CAPTURE_REPO=https://github.com/Coldzer0/Aeternum-World.git"
:: Pinned upstream commit. The installer checks out exactly this commit, so a
:: later change to the upstream repository can never reach a user machine.
:: To use another one, set "RepoCommit" in config.json ("latest" = follow upstream).
set "NW_CAPTURE_COMMIT=820156dbc44c86c9436af81aa0dba72e94cb636b"

if exist "%CONFIG_FILE%" (
    for /F "usebackq tokens=*" %%a in (`powershell -NoProfile -Command "try { (Get-Content '%CONFIG_FILE%' -Raw | ConvertFrom-Json).RepoUrl } catch { '' }"`) do (
        if not "%%a"=="" set "NW_CAPTURE_REPO=%%a"
    )
    for /F "usebackq tokens=*" %%a in (`powershell -NoProfile -Command "try { (Get-Content '%CONFIG_FILE%' -Raw | ConvertFrom-Json).RepoCommit } catch { '' }"`) do (
        if not "%%a"=="" set "NW_CAPTURE_COMMIT=%%a"
    )
)
if /I "%NW_CAPTURE_COMMIT%"=="latest" set "NW_CAPTURE_COMMIT="

set "EMBEDDED_DIR=%ROOT%\python_embedded"
set "CONTAINED_GIT_DIR=%EMBEDDED_DIR%\git"
set "GIT_EXE=%CONTAINED_GIT_DIR%\bin\git.exe"
set "GIT_EXE_ALT=%CONTAINED_GIT_DIR%\PortableGit\bin\git.exe"
set "CAPTURE_DIR=%ROOT%\Aeternum-World"
set "DOWNLOAD_DIR=%ROOT%\.downloads"
set "REQUIREMENTS=%CAPTURE_DIR%\Tools\nw_capture\requirements.txt"

set "PYTHON_VERSION=3.12.9"
set "PYTHON_URL=https://www.python.org/ftp/python/%PYTHON_VERSION%/python-%PYTHON_VERSION%-embed-amd64.zip"
set "ZIP_FILE=%DOWNLOAD_DIR%\python-%PYTHON_VERSION%-embed-amd64.zip"
set "GET_PIP_URL=https://bootstrap.pypa.io/get-pip.py"
set "GET_PIP_FILE=%DOWNLOAD_DIR%\get-pip.py"
set "PYTHON_EXE=%EMBEDDED_DIR%\python.exe"
set "VERIFY_FILE=%EMBEDDED_DIR%\verify_install.py"

set "GIT_URL=https://github.com/git-for-windows/git/releases/download/v2.45.1.windows.1/PortableGit-2.45.1-64-bit.7z.exe"
set "GIT_FILE=%DOWNLOAD_DIR%\PortableGit-2.45.1-64-bit.7z.exe"

set "GIT_CONFIG_GLOBAL=%CONTAINED_GIT_DIR%\etc\gitconfig"
set "GIT_CONFIG_SYSTEM=%CONTAINED_GIT_DIR%\etc\gitconfig"
set "HOME=%EMBEDDED_DIR%"

set "PS_DL=& { $ProgressPreference='SilentlyContinue'; try { Invoke-WebRequest -Uri $args[0] -OutFile ($args[1]+'.part') -UseBasicParsing; if ((Get-Item ($args[1]+'.part')).Length -lt 10000) { throw 'Downloaded file is too small or empty' }; Move-Item -Force ($args[1]+'.part') $args[1]; exit 0 } catch { Write-Host ('ERROR: ' + $_.Exception.Message) -ForegroundColor Red; Remove-Item -Force ($args[1]+'.part') -ErrorAction SilentlyContinue; exit 1 } }"

echo.
echo ============================================================
echo  Azoth Capture - Environment Setup
echo ============================================================
echo.
echo Working in:
echo   %ROOT%
echo.

if not exist "%DOWNLOAD_DIR%" (
    echo Creating download directory...
    mkdir "%DOWNLOAD_DIR%"
)

if exist "%ROOT%\.downloads\*.part" (
    echo Previous download was interrupted. Cleaning partial downloads...
    del /Q "%ROOT%\.downloads\*.part" >nul 2>&1
)

if exist "%PYTHON_EXE%" (
    echo python_embedded already exists. Skipping Python download.
    goto :GitSection
)

echo Installing embeddable Python %PYTHON_VERSION%...

if exist "%EMBEDDED_DIR%" (
    echo Removing incomplete python_embedded folder...
    rmdir /S /Q "%EMBEDDED_DIR%"
)

if exist "%ZIP_FILE%" (
    echo Using cached Python ZIP.
) else (
    echo Downloading official Windows embeddable Python...
    powershell -NoProfile -Command "%PS_DL%" '%PYTHON_URL%' '%ZIP_FILE%'
    if errorlevel 1 (
        echo Failed to download Python embeddable ZIP.
        if not defined APP_LAUNCHED pause
        exit /b 1
    )
)

echo Extracting Python...
mkdir "%EMBEDDED_DIR%"
powershell -NoProfile -Command "try { Expand-Archive -Path '%ZIP_FILE%' -DestinationPath '%EMBEDDED_DIR%' -Force; exit 0 } catch { Write-Host 'ERROR:' $_.Exception.Message -ForegroundColor Red; exit 1 }"
if errorlevel 1 (
    echo Failed to extract Python ZIP.
    if not defined APP_LAUNCHED pause
    exit /b 1
)

echo Configuring embedded Python...
for %%f in ("%EMBEDDED_DIR%\*._pth") do (
    >"%%f.tmp" (
        for /F "usebackq delims=" %%l in ("%%f") do (
            echo %%l
        )
        echo .
        echo import site
    )
    move /Y "%%f.tmp" "%%f" >nul
)

if not exist "%GET_PIP_FILE%" (
    echo Downloading pip installer...
    powershell -NoProfile -Command "%PS_DL%" '%GET_PIP_URL%' '%GET_PIP_FILE%'
    if errorlevel 1 (
        echo Failed to download get-pip.py.
        if not defined APP_LAUNCHED pause
        exit /b 1
    )
)

echo Installing pip...
"%PYTHON_EXE%" "%GET_PIP_FILE%" --no-warn-script-location
if errorlevel 1 (
    echo pip installation failed.
    if not defined APP_LAUNCHED pause
    exit /b 1
)

echo Installing build helpers...
"%PYTHON_EXE%" -m pip install --upgrade setuptools wheel --no-warn-script-location
if errorlevel 1 (
    echo Build helper installation failed.
    if not defined APP_LAUNCHED pause
    exit /b 1
)

:GitSection
if exist "%GIT_EXE%" (
    echo Contained Git already exists. Skipping Git download.
    goto :RepoSection
)

echo Installing contained Git...

if exist "%GIT_FILE%" (
    echo Using cached portable Git installer.
) else (
    echo Downloading portable Git...
    powershell -NoProfile -Command "%PS_DL%" '%GIT_URL%' '%GIT_FILE%'
    if errorlevel 1 (
        echo Failed to download portable Git.
        if not defined APP_LAUNCHED pause
        exit /b 1
    )
)

if exist "%CONTAINED_GIT_DIR%" rmdir /S /Q "%CONTAINED_GIT_DIR%"
echo Extracting portable Git...
mkdir "%CONTAINED_GIT_DIR%"
"%GIT_FILE%" -y -o"%CONTAINED_GIT_DIR%"
if errorlevel 1 (
    echo Portable Git self-extractor reported an error.
    if not defined APP_LAUNCHED pause
    exit /b 1
)

if exist "%GIT_EXE_ALT%" (
    if not exist "%GIT_EXE%" (
        xcopy /E /I /Y "%CONTAINED_GIT_DIR%\PortableGit\*" "%CONTAINED_GIT_DIR%"
        rmdir /S /Q "%CONTAINED_GIT_DIR%\PortableGit"
    )
)

if not exist "%GIT_EXE%" (
    echo Portable Git extraction failed.
    if not defined APP_LAUNCHED pause
    exit /b 1
)

:RepoSection
if exist "%CAPTURE_DIR%\Tools\nw_capture\nw_capture.py" (
    echo Checking for repository updates...

    if not exist "%CAPTURE_DIR%\.git\info\exclude" (
        mkdir "%CAPTURE_DIR%\.git\info" 2>nul
    )
    findstr /I /C:"Tools/nw_capture/captures" "%CAPTURE_DIR%\.git\info\exclude" >nul 2>&1
    if errorlevel 1 (
        echo Tools/nw_capture/captures >> "%CAPTURE_DIR%\.git\info\exclude"
        echo Tools/nw_capture/captures/ >> "%CAPTURE_DIR%\.git\info\exclude"
    )

    pushd "%CAPTURE_DIR%"
    if defined NW_CAPTURE_COMMIT (
        call :EnsurePinned
        if errorlevel 1 (
            popd
            if not defined APP_LAUNCHED pause
            exit /b 1
        )
        popd
        goto :RequirementsSection
    )
    "%GIT_EXE%" fetch origin
    if errorlevel 1 (
        echo Failed to check for updates. Continuing with current version.
        popd
        goto :RequirementsSection
    )

    set "LOCAL_HEAD="
    set "REMOTE_HEAD="
    for /F "usebackq tokens=*" %%h in (`""%GIT_EXE%" rev-parse HEAD"`) do set "LOCAL_HEAD=%%h"
    "%GIT_EXE%" rev-parse "@{u}" >nul 2>&1
    if errorlevel 1 (
        popd
        goto :RequirementsSection
    )
    for /F "usebackq tokens=*" %%h in (`""%GIT_EXE%" rev-parse "@{u}""`) do set "REMOTE_HEAD=%%h"

    if not defined LOCAL_HEAD ( popd & goto :RequirementsSection )
    if not defined REMOTE_HEAD ( popd & goto :RequirementsSection )

    if /I not "%LOCAL_HEAD%"=="%REMOTE_HEAD%" (
        echo Update found. Pulling latest changes...
        "%GIT_EXE%" pull --ff-only
        if errorlevel 1 (
            "%GIT_EXE%" stash push -m "nw_capture_auto_stash" -u
            "%GIT_EXE%" pull --ff-only
            "%GIT_EXE%" stash pop >nul 2>&1
        )
        echo Repository updated successfully.
    ) else (
        echo Repository is already up to date.
    )
    popd
    goto :RequirementsSection
)

echo Cloning Aeternum-World repository...

if exist "%ROOT%\nw_capture" (
    rmdir /S /Q "%ROOT%\nw_capture"
)

if exist "%CAPTURE_DIR%" (
    if not exist "%CAPTURE_DIR%\.git\HEAD" (
        rmdir /S /Q "%CAPTURE_DIR%"
    )
)

if not exist "%CAPTURE_DIR%" (
    "%GIT_EXE%" clone "%NW_CAPTURE_REPO%" "%CAPTURE_DIR%"
    if errorlevel 1 (
        echo Failed to clone repository.
        if not defined APP_LAUNCHED pause
        exit /b 1
    )
)

if defined NW_CAPTURE_COMMIT (
    pushd "%CAPTURE_DIR%"
    call :EnsurePinned
    if errorlevel 1 (
        popd
        if not defined APP_LAUNCHED pause
        exit /b 1
    )
    popd
)

if not exist "%CAPTURE_DIR%\.git\info\exclude" (
    mkdir "%CAPTURE_DIR%\.git\info" 2>nul
)
findstr /I /C:"Tools/nw_capture/captures" "%CAPTURE_DIR%\.git\info\exclude" >nul 2>&1
if errorlevel 1 (
    echo Tools/nw_capture/captures >> "%CAPTURE_DIR%\.git\info\exclude"
    echo Tools/nw_capture/captures/ >> "%CAPTURE_DIR%\.git\info\exclude"
)

if not exist "%CAPTURE_DIR%\Tools\nw_capture\nw_capture.py" (
    echo nw_capture.py was not found in the expected location.
    if not defined APP_LAUNCHED pause
    exit /b 1
)

echo Repository setup complete.

:RequirementsSection
if not exist "%REQUIREMENTS%" (
    echo ERROR: requirements.txt not found.
    if not defined APP_LAUNCHED pause
    exit /b 1
)

echo Installing Python packages...
"%PYTHON_EXE%" -m pip install -r "%REQUIREMENTS%" --no-warn-script-location
if errorlevel 1 (
    echo Package installation failed.
    if not defined APP_LAUNCHED pause
    exit /b 1
)

echo Verifying installed packages...
(
    echo import sys
    echo import frida
    echo import cryptography
    echo import lz4
    echo import websockets
    echo print^('python      :', sys.executable^)
    echo print^('version     :', sys.version^)
    echo print^('frida       :', frida.__version__^)
    echo print^('cryptography:', cryptography.__version__^)
    echo print^('lz4         :', lz4.__version__^)
    echo print^('websockets  :', websockets.__version__^)
) > "%VERIFY_FILE%"

"%PYTHON_EXE%" "%VERIFY_FILE%"
if errorlevel 1 (
    echo Verification failed.
    del "%VERIFY_FILE%" >nul 2>&1
    if not defined APP_LAUNCHED pause
    exit /b 1
)
del "%VERIFY_FILE%" >nul 2>&1

echo.
echo ============================================================
echo  Environment setup complete!
echo ============================================================

if not defined APP_LAUNCHED (
    pause
)

exit /b 0

:: Put the working tree (current directory) on NW_CAPTURE_COMMIT.
:EnsurePinned
set "CUR_HEAD="
for /F "usebackq tokens=*" %%h in (`""%GIT_EXE%" rev-parse HEAD"`) do set "CUR_HEAD=%%h"
if /I "%CUR_HEAD%"=="%NW_CAPTURE_COMMIT%" (
    echo Capture tools are at the pinned commit.
    exit /b 0
)
echo Moving the capture tools to the pinned commit %NW_CAPTURE_COMMIT:~0,10%...
"%GIT_EXE%" fetch origin
"%GIT_EXE%" checkout --quiet --force --detach %NW_CAPTURE_COMMIT%
if errorlevel 1 (
    echo Failed to check out the pinned commit %NW_CAPTURE_COMMIT%.
    exit /b 1
)
exit /b 0
