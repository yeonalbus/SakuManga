@echo off
chcp 65001 >nul
setlocal
cd /d "%~dp0"

rem ─────────────────────────────────────────────────────────────
rem  SakuManga Docker 镜像一键脚本（本地手动构建/推送）
rem
rem  用法：
rem    build-docker.bat        本地构建（仅当前架构，载入本地镜像，用于测试）
rem    build-docker.bat push   多架构构建（amd64 + arm64）并推送到 GHCR
rem
rem  说明：不强依赖本机 Docker —— 打 tag 或手动触发 GitHub Actions
rem        也会自动构建并推送镜像（见 .github/workflows/docker-publish.yml）。
rem ─────────────────────────────────────────────────────────────

where docker >nul 2>nul
if errorlevel 1 (
    echo [错误] 未检测到 docker 命令，请先安装并启动 Docker Desktop。
    echo        若不想在本机装 Docker，可在 GitHub 仓库的 Actions 页面手动触发
    echo        「Docker Image ^(GHCR^)」工作流，由 GitHub 的机器替你构建。
    pause
    exit /b 1
)

for /f "delims=" %%v in ('node -p "require('./package.json').version"') do set "APP_VERSION=%%v"
set "IMAGE=ghcr.io/yeonalbus/sakumanga"

for /f %%i in ('powershell -NoProfile -Command "Get-Date -Format yyyyMMddHHmm"') do set "TS=%%i"
for /f %%i in ('git rev-parse --short HEAD 2^>nul') do set "GITSHA=%%i"
if not defined GITSHA set "GITSHA=unknown"
set "BUILD_ID=%TS%-g%GITSHA%"

echo ============================================
echo   SakuManga v%APP_VERSION% Docker 镜像
echo ============================================
echo   镜像名  : %IMAGE%
echo   构建标识: %BUILD_ID%
echo.

if /i "%~1"=="push" goto push

echo [模式 1] 本地构建（仅当前架构，载入本地镜像供测试）
echo   需要多架构并推送 GHCR 请执行：build-docker.bat push
echo.
docker buildx build --platform linux/amd64 ^
  --build-arg BUILD_ID=%BUILD_ID% ^
  -t sakumanga:%APP_VERSION% -t sakumanga:test --load .
if errorlevel 1 goto fail

echo.
echo 完成：本地镜像 sakumanga:%APP_VERSION%（别名 sakumanga:test）
echo 试运行：
echo   docker run --rm -p 8081:8081 -v sakumanga-test:/app sakumanga:%APP_VERSION%
echo   然后浏览器访问 http://127.0.0.1:8081 （首次登录 admin / admin123）
goto end

:push
echo [模式 2] 构建多架构（linux/amd64 + linux/arm64）并推送到 GHCR
echo   前置条件：已执行  docker login ghcr.io -u ^<你的GitHub用户名^>
echo.
docker buildx build --platform linux/amd64,linux/arm64 ^
  --build-arg BUILD_ID=%BUILD_ID% ^
  -t %IMAGE%:%APP_VERSION% -t %IMAGE%:latest --push .
if errorlevel 1 goto fail

echo.
echo 完成：已推送 %IMAGE%:%APP_VERSION% 与 %IMAGE%:latest
echo 用户拉取：docker pull %IMAGE%:%APP_VERSION%
goto end

:fail
echo.
echo [错误] 构建失败，请查看上方 docker 输出。
:end
echo.
pause
