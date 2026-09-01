@echo off
rem ==============================================================================
rem 👑 Toron Edge Gateway — Windows Auto-Installer & Service Manager
rem Supports Windows AMD64 and ARM64 architectures
rem ==============================================================================

setlocal enabledelayedexpansion

set TORON_VERSION=1.5.0
if exist "%~dp0VERSION" (
    set /p TORON_VERSION=<"%~dp0VERSION"
)
set INSTALL_BIN_DIR=C:\Program Files\Toron
set INSTALL_CONF_DIR=C:\ProgramData\Toron
set INSTALL_LOG_DIR=C:\ProgramData\Toron\logs
set SERVICE_NAME=Toron

echo ==============================================================================
echo  👑 Toron Web Server ^& Edge Gateway — Windows Installer (v%TORON_VERSION%)
echo ==============================================================================

rem Check for Administrator Privileges
net session >nul 2>&1
if %errorlevel% neq 0 (
    echo [ERROR] This installer must be run as Administrator!
    echo         Please right-click install.bat and select "Run as administrator".
    pause
    exit /b 1
)

rem Check for Uninstallation Flag
if "%1"=="/uninstall" goto UNINSTALL
if "%1"=="-u" goto UNINSTALL
if "%1"=="--uninstall" goto UNINSTALL

rem Auto-Detect Architecture
set ARCH=amd64
if /i "%PROCESSOR_ARCHITECTURE%"=="ARM64" set ARCH=arm64
if /i "%PROCESSOR_ARCHITEW6432%"=="ARM64" set ARCH=arm64

set TARGET_EXECUTABLE=toron-windows-%ARCH%.exe

echo [INFO] Detected Architecture: %ARCH%
echo [INFO] Target Binary Name: %TARGET_EXECUTABLE%

rem Step 1: Locate or Compile Binary
set BINARY_SOURCE=

if exist "%~dp0bin\%TARGET_EXECUTABLE%" (
    set BINARY_SOURCE=%~dp0bin\%TARGET_EXECUTABLE%
) else if exist "%~dp0bin\toron-windows-amd64.exe" (
    set BINARY_SOURCE=%~dp0bin\toron-windows-amd64.exe
) else if exist "%~dp0toron.exe" (
    set BINARY_SOURCE=%~dp0toron.exe
)

if "%BINARY_SOURCE%"=="" (
    where go >nul 2>&1
    if %errorlevel% equ 0 (
        echo [INFO] Local binary not found. Compiling %TARGET_EXECUTABLE% with Go...
        if not exist "%~dp0bin" mkdir "%~dp0bin"
        set CGO_ENABLED=0
        set GOOS=windows
        set GOARCH=%ARCH%
        go build -o "%~dp0bin\%TARGET_EXECUTABLE%" "%~dp0cmd\toron"
        set BINARY_SOURCE=%~dp0bin\%TARGET_EXECUTABLE%
    ) else (
        echo [ERROR] Could not find %TARGET_EXECUTABLE% in .\bin\ and Go is not installed to compile it.
        pause
        exit /b 1
    )
)

echo [SUCCESS] Found executable binary: %BINARY_SOURCE%

rem Step 2: Install Binary to C:\Program Files\Toron
echo [INFO] Installing binary to %INSTALL_BIN_DIR%\toron.exe...
if not exist "%INSTALL_BIN_DIR%" mkdir "%INSTALL_BIN_DIR%"
copy /Y "%BINARY_SOURCE%" "%INSTALL_BIN_DIR%\toron.exe" >nul
echo [SUCCESS] Binary installed to %INSTALL_BIN_DIR%\toron.exe

rem Step 3: Set Up Configuration in C:\ProgramData\Toron
echo [INFO] Setting up configuration directory at %INSTALL_CONF_DIR%...
if not exist "%INSTALL_CONF_DIR%" mkdir "%INSTALL_CONF_DIR%"
if not exist "%INSTALL_CONF_DIR%\public" mkdir "%INSTALL_CONF_DIR%\public"
if not exist "%INSTALL_LOG_DIR%" mkdir "%INSTALL_LOG_DIR%"

if exist "%~dp0config.yaml" (
    if not exist "%INSTALL_CONF_DIR%\config.yaml" (
        copy /Y "%~dp0config.yaml" "%INSTALL_CONF_DIR%\config.yaml" >nul
        echo [SUCCESS] Installed %INSTALL_CONF_DIR%\config.yaml
    ) else (
        echo [WARN] Existing %INSTALL_CONF_DIR%\config.yaml preserved.
    )
)

if exist "%~dp0routes.yaml" (
    if not exist "%INSTALL_CONF_DIR%\routes.yaml" (
        copy /Y "%~dp0routes.yaml" "%INSTALL_CONF_DIR%\routes.yaml" >nul
        echo [SUCCESS] Installed %INSTALL_CONF_DIR%\routes.yaml
    ) else (
        echo [WARN] Existing %INSTALL_CONF_DIR%\routes.yaml preserved.
    )
)

if exist "%~dp0public" (
    xcopy /E /I /Y "%~dp0public" "%INSTALL_CONF_DIR%\public" >nul
    echo [SUCCESS] Installed static dashboard UI assets to %INSTALL_CONF_DIR%\public\
)

rem Step 4: Install Windows Service (sc.exe)
echo [INFO] Registering Windows Service '%SERVICE_NAME%'...
sc query %SERVICE_NAME% >nul 2>&1
if %errorlevel% equ 0 (
    echo [INFO] Stopping existing %SERVICE_NAME% service...
    sc stop %SERVICE_NAME% >nul 2>&1
    timeout /t 2 >nul
    sc delete %SERVICE_NAME% >nul 2>&1
    timeout /t 1 >nul
)

sc create %SERVICE_NAME% binPath= "\"%INSTALL_BIN_DIR%\toron.exe\" -config \"%INSTALL_CONF_DIR%\config.yaml\"" start= auto DisplayName= "Toron Edge Gateway & Web Control Center" >nul
if %errorlevel% equ 0 (
    echo [SUCCESS] Windows Service '%SERVICE_NAME%' created successfully.
    echo [INFO] Starting %SERVICE_NAME% service...
    sc start %SERVICE_NAME% >nul 2>&1
    echo [SUCCESS] %SERVICE_NAME% service started!
) else (
    echo [WARN] Sc create failed. You can run Toron manually using: "%INSTALL_BIN_DIR%\toron.exe" -config "%INSTALL_CONF_DIR%\config.yaml"
)

echo.
echo ==============================================================================
echo  🎉 Toron Edge Gateway Windows Installation Completed Successfully!
echo ==============================================================================
echo  • Binary Location:     %INSTALL_BIN_DIR%\toron.exe
echo  • Config Directory:    %INSTALL_CONF_DIR%\ (config.yaml, routes.yaml)
echo  • Web Dashboard:       http://localhost:8080/internal/dashboard/
echo  • Service Management:  sc query Toron ^| sc stop Toron ^| sc start Toron
echo  • Uninstallation:      install.bat /uninstall
echo.
pause
exit /b 0

:UNINSTALL
echo [WARN] Starting uninstallation of Toron Edge Gateway...
sc query %SERVICE_NAME% >nul 2>&1
if %errorlevel% equ 0 (
    echo [INFO] Stopping and removing Windows Service '%SERVICE_NAME%'...
    sc stop %SERVICE_NAME% >nul 2>&1
    timeout /t 2 >nul
    sc delete %SERVICE_NAME% >nul 2>&1
)

if exist "%INSTALL_BIN_DIR%" (
    rmdir /S /Q "%INSTALL_BIN_DIR%"
    echo [INFO] Removed %INSTALL_BIN_DIR%
)

if exist "%INSTALL_CONF_DIR%" (
    rmdir /S /Q "%INSTALL_CONF_DIR%"
    echo [INFO] Removed %INSTALL_CONF_DIR%
)

echo [SUCCESS] Toron Edge Gateway uninstallation complete!
pause
exit /b 0
