@echo off
chcp 65001 >nul
setlocal
cd /d "%~dp0"

for /f "delims=" %%v in ('node -p "require('./package.json').version"') do set "APP_VERSION=%%v"

echo ============================================
echo   SakuHentai v%APP_VERSION% 一键打包脚本
echo ============================================

echo.
echo [1/4] 构建前端 (npm run build)...
call npm run build
if errorlevel 1 (
    echo [错误] 前端构建失败
    exit /b 1
)

echo.
echo [2/4] 拷贝前端产物到 backend\webui\dist...
if exist "backend\webui\dist" rmdir /s /q "backend\webui\dist"
mkdir "backend\webui\dist"
xcopy "dist\*" "backend\webui\dist\" /e /i /y >nul

echo.
echo [3/4] 构建后端可执行文件 (go build)...
pushd backend
go build -trimpath -ldflags "-s -w" -o "..\SakuHentai.exe" .
if errorlevel 1 (
    popd
    echo [错误] 后端构建失败
    exit /b 1
)
popd

echo.
echo [4/4] 完成！
echo    生成文件: %cd%\SakuHentai.exe
echo    双击运行后最小化到系统托盘，右键托盘图标可选择「打开界面 / 退出程序」。
echo    若需纯后端运行（无托盘），可执行: SakuHentai.exe --headless
echo.
pause
