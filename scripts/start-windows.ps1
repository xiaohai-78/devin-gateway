<#
.SYNOPSIS
  读取 .env 并在前台启动 devin-gateway.exe（Windows，不需要 Docker）。

.DESCRIPTION
  .env 里的变量只注入给本进程和网关子进程，不写入用户/系统环境变量，
  所以不需要 setx，也不会让其他程序读到 token。已经设置的环境变量优先，不会被 .env 覆盖。
  工作目录切到仓库根目录，网关会读取那里的 config.yaml（如果有）。

  适合交给任务计划程序 / NSSM 做常驻：网关退出时本脚本以相同退出码退出。

.EXAMPLE
  powershell -ExecutionPolicy Bypass -File .\scripts\start-windows.ps1

.EXAMPLE
  .\scripts\start-windows.ps1 -Exe C:\tools\devin-gateway.exe -GatewayArgs '-check'
#>
param(
    [string]$Exe = '',
    [string]$EnvFile = '',
    [string[]]$GatewayArgs = @()
)

$ErrorActionPreference = 'Stop'
$Root = Split-Path -Parent $PSScriptRoot
if (-not $EnvFile) { $EnvFile = Join-Path $Root '.env' }
if (-not $Exe) {
    $Exe = Join-Path $Root 'devin-gateway.exe'
    if (-not (Test-Path -LiteralPath $Exe)) {
        # 非 Windows 上调试脚本时用无后缀的二进制
        $plain = Join-Path $Root 'devin-gateway'
        if (Test-Path -LiteralPath $plain) { $Exe = $plain }
    }
}

if (-not (Test-Path -LiteralPath $Exe)) {
    Write-Host "找不到网关程序：$Exe（把 devin-gateway.exe 放到仓库根目录，或用 -Exe 指定）" -ForegroundColor Red
    exit 1
}

if (Test-Path -LiteralPath $EnvFile) {
    foreach ($line in Get-Content -LiteralPath $EnvFile) {
        $trimmed = $line.Trim()
        if ($trimmed -eq '' -or $trimmed.StartsWith('#')) { continue }
        $eq = $trimmed.IndexOf('=')
        if ($eq -lt 1) { continue }
        $name = $trimmed.Substring(0, $eq).Trim()
        $value = $trimmed.Substring($eq + 1).Trim()
        # 去掉成对的单/双引号；不做任何 $ 展开，token 原样保留
        if ($value.Length -ge 2 -and (
                ($value.StartsWith("'") -and $value.EndsWith("'")) -or
                ($value.StartsWith('"') -and $value.EndsWith('"')))) {
            $value = $value.Substring(1, $value.Length - 2)
        }
        if ([string]::IsNullOrEmpty([Environment]::GetEnvironmentVariable($name, 'Process'))) {
            [Environment]::SetEnvironmentVariable($name, $value, 'Process')
        }
    }
} elseif ([string]::IsNullOrEmpty($env:DEVIN_TOKEN)) {
    Write-Host "缺少 $EnvFile，且没有设置 DEVIN_TOKEN。先运行 .\scripts\token-windows.ps1" -ForegroundColor Red
    exit 1
}

if ([string]::IsNullOrEmpty($env:DEVIN_TOKEN)) {
    Write-Host "DEVIN_TOKEN 为空。先运行 .\scripts\token-windows.ps1" -ForegroundColor Red
    exit 1
}

Set-Location -LiteralPath $Root
& $Exe @GatewayArgs
exit $LASTEXITCODE
