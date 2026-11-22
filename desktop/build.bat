@echo off
setlocal
cd /d "%~dp0"
where go >nul 2>&1
if errorlevel 1 (
  echo No encuentro Go en PATH. Instala Go 1.26 o posterior.
  pause
  exit /b 1
)
rem Copia app\EditorOPD.html a esta carpeta (el programa lo incrusta al compilar).
copy /y "..\app\EditorOPD.html" "." >nul
if not exist "EditorOPD.html" (
  echo No encuentro app\EditorOPD.html.
  pause
  exit /b 1
)
go mod tidy
if errorlevel 1 (
  echo Fallo la descarga o preparacion de dependencias.
  pause
  exit /b 1
)
go build -trimpath -ldflags="-H=windowsgui -s -w" -o "..\app\EditorOPD.exe" .
if errorlevel 1 (
  echo No se pudo compilar EditorOPD.exe.
  pause
  exit /b 1
)
echo Compilado: ..\app\EditorOPD.exe
pause
