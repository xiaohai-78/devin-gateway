<#
.SYNOPSIS
  从 Devin 桌面版读取会话 token，写入仓库根目录的 .env（Windows）。

.DESCRIPTION
  依次尝试以下方式读取本地状态库，任一成功即停止，都不需要 Docker：
    1. 本机 sqlite3.exe（winget install SQLite.SQLite）
    2. Python 3 自带的 sqlite3 模块（py / python / python3）
    3. Docker（一次性 alpine 容器）
  都没有时，按提示手动传入 -Token。

.EXAMPLE
  powershell -ExecutionPolicy Bypass -File .\scripts\token-windows.ps1

.EXAMPLE
  .\scripts\token-windows.ps1 -Token 'devin-session-token$...'
#>
param([string]$Token = "")

$ErrorActionPreference = 'Stop'
$Root = Split-Path -Parent $PSScriptRoot
$EnvFile = Join-Path $Root '.env'
$Prefix = 'devin-session-token'

# 注意：传给外部程序的参数里不要出现双引号，Windows PowerShell 5.1 会把它们拆坏。
$Query = 'SELECT json_extract(value,''$.apiKey'') FROM ItemTable WHERE key=''windsurfAuthStatus'';'
$PyCode = 'import sqlite3,json,sys;' +
    'r=sqlite3.connect(sys.argv[1]).execute(''SELECT value FROM ItemTable WHERE key=?'',(''windsurfAuthStatus'',)).fetchone();' +
    'print(json.loads(r[0]).get(''apiKey'','''') if r else '''')'

# 运行外部命令，成功且输出像 token 时返回它，否则返回空串。
function Invoke-Reader {
    param([string]$Name, [scriptblock]$Run)
    try {
        $out = & $Run 2>$null
        if ($LASTEXITCODE -ne 0) { return '' }
        $value = ($out | Out-String).Trim()
        if ($value.StartsWith($Prefix)) {
            Write-Host "  通过 $Name 读取成功"
            return $value
        }
    } catch { }
    return ''
}

function Read-DevinToken {
    $db = Join-Path $env:APPDATA 'Devin\User\globalStorage\state.vscdb'
    if (-not (Test-Path -LiteralPath $db)) { Write-Host "  未找到 $db（Devin 桌面版未安装或未登录）"; return '' }

    # 1. sqlite3.exe
    if (Get-Command sqlite3 -ErrorAction SilentlyContinue) {
        $t = Invoke-Reader 'sqlite3' { sqlite3 -readonly $db $Query }
        if ($t) { return $t }
    }

    # 2. Python 3。python.exe 可能是应用商店的占位程序，所以逐个试并校验输出。
    $pythons = @(
        @{ Cmd = 'py'; Args = @('-3') },
        @{ Cmd = 'python'; Args = @() },
        @{ Cmd = 'python3'; Args = @() }
    )
    foreach ($py in $pythons) {
        if (-not (Get-Command $py.Cmd -ErrorAction SilentlyContinue)) { continue }
        $cmd = $py.Cmd; $pre = $py.Args
        $t = Invoke-Reader $cmd { & $cmd @pre -c $PyCode $db }
        if ($t) { return $t }
    }

    # 3. Docker
    if (Get-Command docker -ErrorAction SilentlyContinue) {
        $sqlFile = Join-Path $env:TEMP ('devin-q-' + [guid]::NewGuid().ToString('N') + '.sql')
        [IO.File]::WriteAllText($sqlFile, $Query, (New-Object System.Text.UTF8Encoding($false)))
        try {
            $t = Invoke-Reader 'docker' {
                docker run --rm `
                    -v "${db}:/db/state.vscdb:ro" `
                    -v "${sqlFile}:/db/q.sql:ro" `
                    --entrypoint sh alpine:3.22 `
                    -c 'apk add --no-cache sqlite >/dev/null 2>&1; sqlite3 /db/state.vscdb < /db/q.sql'
            }
            if ($t) { return $t }
        } finally {
            Remove-Item -LiteralPath $sqlFile -ErrorAction SilentlyContinue
        }
    }

    Write-Host "  找不到可用的读取方式。任选其一后重试：" -ForegroundColor Yellow
    Write-Host "    winget install SQLite.SQLite      # 装 sqlite3"
    Write-Host "    winget install Python.Python.3.12 # 或装 Python"
    return ''
}

if ([string]::IsNullOrWhiteSpace($Token)) { $Token = $env:DEVIN_TOKEN }
if ([string]::IsNullOrWhiteSpace($Token)) {
    Write-Host "==> 读取 Devin 本地状态"
    $Token = Read-DevinToken
}
if ([string]::IsNullOrWhiteSpace($Token)) {
    Write-Host "没有读到 token。用 -Token 'devin-session-token`$...' 传入。" -ForegroundColor Red
    exit 1
}
if (-not $Token.StartsWith($Prefix)) {
    Write-Host "警告：token 不以 $Prefix 开头，可能取错了" -ForegroundColor Yellow
}

# 保留 .env 中除 DEVIN_TOKEN 外的行
$lines = @()
if (Test-Path -LiteralPath $EnvFile) {
    $lines = @(Get-Content -LiteralPath $EnvFile | Where-Object { $_ -notmatch '^DEVIN_TOKEN=' })
}
if ($lines.Count -eq 0) { $lines = @('PORT=18089', 'AUTH_API_KEY=') }
$lines += "DEVIN_TOKEN='$Token'"

# 用 LF 写入、无 BOM；token 用单引号包住，避免 Compose 展开 $
$text = ($lines -join "`n") + "`n"
[IO.File]::WriteAllText($EnvFile, $text, (New-Object System.Text.UTF8Encoding($false)))

Write-Host ("==> 已写入 {0}" -f $EnvFile) -ForegroundColor Green
Write-Host ("    token: {0}…（长度 {1}）" -f $Token.Substring(0, [Math]::Min(22, $Token.Length)), $Token.Length)
Write-Host "    下一步（二选一）："
Write-Host "      Docker:  docker compose up -d"
Write-Host "      二进制:  .\scripts\start-windows.ps1   # 读取 .env 后启动 devin-gateway.exe"
