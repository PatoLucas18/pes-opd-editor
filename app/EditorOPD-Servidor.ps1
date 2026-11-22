# Servidor local solo para EditorOPD.html (compatible con PowerShell 2.0 / Windows 7+).
# No requiere compilar ni instalar nada. Escucha solo en localhost (puertos 8780-8790).
# Sirve la pagina y una API de archivos: el editor pide abrir un .opd o una carpeta, este servidor muestra el dialogo
# de Windows y despues lee / guarda los archivos de esa carpeta (opd, bin, cfg y texturas png).
# OJO: PowerShell no distingue mayusculas en los nombres de variables.
$ErrorActionPreference = 'Stop'
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$Html = Join-Path $ScriptDir 'EditorOPD.html'
if (-not (Test-Path $Html)) { Write-Host "No encuentro $Html" -ForegroundColor Red; exit 1 }
Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing

$listener = $null
$port = 0
foreach ($tryPort in 8780..8790) {
    try {
        $l = New-Object System.Net.HttpListener
        $l.Prefixes.Add("http://localhost:$tryPort/")
        $l.Start()
        $listener = $l; $port = $tryPort; break
    } catch { }
}
if ($listener -eq $null) { Write-Host 'No se pudo abrir ningun puerto (8780-8790).' -ForegroundColor Red; exit 1 }

$url = "http://localhost:$port/"
Write-Host "Editor OPD funcionando en $url" -ForegroundColor Green
Write-Host 'Cierra esta ventana (o pulsa Ctrl+C) para detener el servidor.'
try { Start-Process $url } catch { Write-Host "Abre manualmente: $url" }

$script:AllowedRoots = @()      # carpetas elegidas con los dialogos: solo ahi se lee y se guarda
$readExt  = @('.opd', '.bin', '.cfg', '.csv', '.png')
$writeExt = @('.opd', '.bin', '.cfg', '.csv')

function Send-Bytes($res, $code, $bytes, $ctype) {
    $res.StatusCode = $code
    $res.ContentType = $ctype
    $res.AddHeader('Cache-Control', 'no-store')
    if ($bytes -eq $null) { $bytes = New-Object byte[] 0 }
    $res.ContentLength64 = $bytes.Length
    if ($bytes.Length -gt 0) { $res.OutputStream.Write($bytes, 0, $bytes.Length) }
}

function Send-Text($res, $code, $text) {
    Send-Bytes $res $code ([Text.Encoding]::UTF8.GetBytes([string]$text)) 'text/plain; charset=utf-8'
}

function Get-Param($reqUrl, $name) {
    $q = $reqUrl.Query
    if (-not $q -or $q.Length -lt 2) { return '' }
    foreach ($part in $q.Substring(1).Split('&')) {
        $eq = $part.IndexOf('=')
        if ($eq -lt 0) { continue }
        $k = [Uri]::UnescapeDataString($part.Substring(0, $eq).Replace('+', ' '))
        if ($k -eq $name) { return [Uri]::UnescapeDataString($part.Substring($eq + 1).Replace('+', ' ')) }
    }
    return ''
}

function Test-Local($req, $portNum) {
    $origin = $req.Headers['Origin']
    $site = $req.Headers['Sec-Fetch-Site']
    if ($origin -and $origin -ne "http://localhost:$portNum" -and $origin -ne "http://127.0.0.1:$portNum") { return $false }
    if ($site -and $site -ne 'same-origin' -and $site -ne 'none') { return $false }
    return $true
}

function Get-Norm($anyPath) {
    $full = [IO.Path]::GetFullPath($anyPath)
    if ($full.Length -gt 3) { $full = $full.TrimEnd('\') }
    return $full.ToLower()
}

function Add-Allowed($folder) {
    $norm = Get-Norm $folder
    if (-not ($script:AllowedRoots -contains $norm)) { $script:AllowedRoots += $norm }
}

function Test-Allowed($anyPath) {
    if (-not $anyPath) { return $false }
    $norm = Get-Norm $anyPath
    foreach ($rootDir in $script:AllowedRoots) {
        if ($norm -eq $rootDir) { return $true }
        if ($norm.StartsWith($rootDir.TrimEnd('\') + '\')) { return $true }
    }
    return $false
}

function Json-Str($s) {
    return '"' + ([string]$s).Replace('\', '\\').Replace('"', '\"') + '"'
}

function Json-List($items) {
    $parts = @()
    foreach ($it in $items) { $parts += (Json-Str $it) }
    return '[' + ($parts -join ',') + ']'
}

function Test-ExtOk($anyPath, $okList) {
    $ext = [IO.Path]::GetExtension($anyPath).ToLower()
    return ($okList -contains $ext)
}

function New-Owner {
    # ventana invisible siempre encima, para que el dialogo no quede detras del navegador
    $f = New-Object System.Windows.Forms.Form
    $f.TopMost = $true
    $f.ShowInTaskbar = $false
    $f.Opacity = 0
    $f.StartPosition = 'CenterScreen'
    $f.Size = New-Object System.Drawing.Size(1, 1)
    $f.Show()
    $f.Activate()
    return $f
}

function Pick-File($startDir) {
    $dlg = New-Object System.Windows.Forms.OpenFileDialog
    $dlg.Title = 'Abrir .opd / scoreboard.bin'
    $dlg.Filter = 'OPD / scoreboard.bin (*.opd;*.bin)|*.opd;*.bin|Todos los archivos (*.*)|*.*'
    if ($startDir -and [IO.Directory]::Exists($startDir)) { $dlg.InitialDirectory = $startDir }
    $owner = New-Owner
    $picked = ''
    try { if ($dlg.ShowDialog($owner) -eq [System.Windows.Forms.DialogResult]::OK) { $picked = $dlg.FileName } }
    finally { $owner.Close(); $owner.Dispose(); $dlg.Dispose() }
    return $picked
}

function Pick-Folder($startDir) {
    $dlg = New-Object System.Windows.Forms.FolderBrowserDialog
    $dlg.Description = 'Carpeta del marcador (la que tiene los .opd y la subcarpeta textures)'
    $dlg.ShowNewFolderButton = $false
    if ($startDir -and [IO.Directory]::Exists($startDir)) { $dlg.SelectedPath = $startDir }
    $owner = New-Owner
    $picked = ''
    try { if ($dlg.ShowDialog($owner) -eq [System.Windows.Forms.DialogResult]::OK) { $picked = $dlg.SelectedPath } }
    finally { $owner.Close(); $owner.Dispose(); $dlg.Dispose() }
    return $picked
}

while ($listener.IsListening) {
    $ctx = $null
    try { $ctx = $listener.GetContext() } catch { break }
    $req = $ctx.Request
    $res = $ctx.Response
    try {
        $route = $req.Url.AbsolutePath
        $argPath = Get-Param $req.Url 'p'
        if (-not (Test-Local $req $port)) {
            Send-Text $res 403 'origen no permitido'
        } elseif ($route -eq '/' -or $route -eq '/index.html' -or $route -eq '/EditorOPD.html') {
            $pageBytes = [IO.File]::ReadAllBytes($Html)   # se lee en cada pedido: si editas el HTML basta con recargar la pagina
            Send-Bytes $res 200 $pageBytes 'text/html; charset=utf-8'
        } elseif ($route -eq '/api/ping') {
            Send-Text $res 200 'editoropd'
        } elseif ($route -eq '/api/pick' -or $route -eq '/api/pickdir') {
            if ($req.HttpMethod -ne 'POST') {
                Send-Text $res 405 'metodo no permitido'
            } else {
                $startDir = Get-Param $req.Url 'start'
                $chosen = ''
                if ($route -eq '/api/pick') {
                    $chosen = Pick-File $startDir
                    if ($chosen) { Add-Allowed ([IO.Path]::GetDirectoryName($chosen)) }
                } else {
                    $chosen = Pick-Folder $startDir
                    if ($chosen) { Add-Allowed $chosen }
                }
                if ($chosen) { Send-Bytes $res 200 ([Text.Encoding]::UTF8.GetBytes('{"path":' + (Json-Str $chosen) + '}')) 'application/json; charset=utf-8' }
                else { Send-Bytes $res 204 $null 'text/plain; charset=utf-8' }
            }
        } elseif ($route -eq '/api/list') {
            if (-not (Test-Allowed $argPath)) {
                Send-Text $res 403 'carpeta no abierta desde el editor'
            } elseif (-not [IO.Directory]::Exists($argPath)) {
                Send-Text $res 404 'no es una carpeta'
            } else {
                $fileNames = @(); $dirNames = @()
                foreach ($f in [IO.Directory]::GetFiles($argPath)) { $fileNames += [IO.Path]::GetFileName($f) }
                foreach ($d in [IO.Directory]::GetDirectories($argPath)) { $dirNames += [IO.Path]::GetFileName($d) }
                $json = '{"files":' + (Json-List $fileNames) + ',"dirs":' + (Json-List $dirNames) + '}'
                Send-Bytes $res 200 ([Text.Encoding]::UTF8.GetBytes($json)) 'application/json; charset=utf-8'
            }
        } elseif ($route -eq '/api/read') {
            if (-not (Test-Allowed $argPath) -or -not (Test-ExtOk $argPath $readExt)) {
                Send-Text $res 403 'archivo no permitido'
            } elseif (-not [IO.File]::Exists($argPath)) {
                Send-Text $res 404 'no existe'
            } else {
                Send-Bytes $res 200 ([IO.File]::ReadAllBytes($argPath)) 'application/octet-stream'
            }
        } elseif ($route -eq '/api/write') {
            if ($req.HttpMethod -ne 'POST') {
                Send-Text $res 405 'metodo no permitido'
            } elseif (-not (Test-Allowed $argPath) -or -not (Test-ExtOk $argPath $writeExt)) {
                Send-Text $res 403 'archivo no permitido'
            } else {
                $ms = New-Object IO.MemoryStream
                $buf = New-Object byte[] 65536
                while (($got = $req.InputStream.Read($buf, 0, $buf.Length)) -gt 0) { $ms.Write($buf, 0, $got) }
                $data = $ms.ToArray()
                if ([IO.File]::Exists($argPath) -and -not [IO.File]::Exists($argPath + '.bak')) {
                    [IO.File]::Copy($argPath, $argPath + '.bak')
                }
                [IO.File]::WriteAllBytes($argPath, $data)
                Send-Bytes $res 204 $null 'text/plain; charset=utf-8'
            }
        } else {
            $res.StatusCode = 404
            $res.ContentLength64 = 0
        }
    } catch {
        Write-Host ("Error en " + $req.Url.AbsolutePath + ": " + $_.Exception.Message) -ForegroundColor Yellow
        try { Send-Text $res 500 $_.Exception.Message } catch { }
    }
    try { $res.OutputStream.Close() } catch { }
}
$listener.Stop()
