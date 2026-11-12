<#
.SYNOPSIS
  从 Devin 桌面版读取会话 token，写入仓库根目录的 .env（Windows）。

.EXAMPLE
  powershell -ExecutionPolicy Bypass -File .\scripts\token-windows.ps1

.EXAMPLE
  .\scripts\token-windows.ps1 -Token 'devin-session-token$...'
#>
param([string]$Token = "")

$ErrorActionPreference = 'Stop'
$Root = Split-Path -Parent $PSScriptRoot
$EnvFile = Join-Path $Root '.env'

function Read-DevinToken {
    $db = Join-Path $env:APPDATA 'Devin\User\globalStorage\state.vscdb'
    if (-not (Test-Path -LiteralPath $db)) { Write-Host "  未找到 $db"; return '' }
    if (-not (Get-Command docker -ErrorAction SilentlyContinue)) { Write-Host "  需要 docker 读取 sqlite"; return '' }
    $sqlFile = Join-Path $env:TEMP ('devin-q-' + [guid]::NewGuid().ToString('N') + '.sql')
    $query = 'SELECT json_extract(value,''$.apiKey'') FROM ItemTable WHERE key=''windsurfAuthStatus'';'
    [IO.File]::WriteAllText($sqlFile, $query, (New-Object System.Text.UTF8Encoding($false)))
    try {
        $out = docker run --rm `
            -v "${db}:/db/state.vscdb:ro" `
            -v "${sqlFile}:/db/q.sql:ro" `
            --entrypoint sh alpine:3.22 `
            -c 'apk add --no-cache sqlite >/dev/null 2>&1; sqlite3 /db/state.vscdb < /db/q.sql'
        if ($LASTEXITCODE -ne 0) { return '' }
        return ($out | Out-String).Trim()
    } finally {
        Remove-Item -LiteralPath $sqlFile -ErrorAction SilentlyContinue
    }
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
if (-not $Token.StartsWith('devin-session-token')) {
    Write-Host "警告：token 不以 devin-session-token 开头，可能取错了" -ForegroundColor Yellow
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
Write-Host "    下一步: docker compose up -d --build"
