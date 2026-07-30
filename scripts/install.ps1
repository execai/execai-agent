# Installs execai from public prod-bucket in Yandex Object Storage.
#   irm https://storage.yandexcloud.net/execai-agent-prod/execai/R5/latest/install.ps1 | iex
#   $env:PREFIX='execai/R5/42'; iex (irm ...)
#
# All user-facing strings are ASCII on purpose: Windows PowerShell 5.1
# doesn't handle UTF-8 correctly when a script comes over `iwr | iex`
# without a BOM, so Cyrillic would print as '?'. Comments can stay in
# Russian for maintainers.
$ErrorActionPreference = 'Stop'

$Bucket     = if ($env:BUCKET)      { $env:BUCKET }      else { 'https://storage.yandexcloud.net/execai-agent-prod' }
$Prefix     = if ($env:PREFIX)      { $env:PREFIX }      else { 'execai/R5/latest' }
$InstallDir = if ($env:INSTALL_DIR) { $env:INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA 'execai' }

if (-not [Environment]::Is64BitOperatingSystem) {
    throw "64-bit Windows required"
}
# Определяем архитектуру: amd64 (Intel/AMD) или arm64 (Copilot+ PCs, ARM).
$archName = if ($env:PROCESSOR_ARCHITECTURE) { $env:PROCESSOR_ARCHITECTURE } else { 'AMD64' }
if ($env:PROCESSOR_ARCHITEW6432) { $archName = $env:PROCESSOR_ARCHITEW6432 }
switch -Regex ($archName.ToUpper()) {
    'ARM64'   { $archive = 'execai-windows-arm64.zip'; break }
    default   { $archive = 'execai-windows-amd64.zip' }
}
$base = "$Bucket/$Prefix"

# === Windows Defender exclusion ===
# Defender иногда флаггит неподписанные Go-бинари как malware. Скачивание
# может тихо провалиться либо exe будет удалён после установки. Пытаемся
# добавить $InstallDir в исключения ЗАРАНЕЕ — до скачивания.
#
# Требует админ. Три сценария:
#   1) Скрипт уже запущен от Admin → просто выполняем.
#   2) Не Admin → запускаем UAC-повышенный дочерний powershell только для
#      этой одной команды (юзер увидит одно окно UAC — короче чем каждый раз
#      объяснять руками).
#   3) Юзер отказался от UAC / Add-MpPreference недоступен (Defender выключен
#      или Windows Server без AV) → печатаем предупреждение и продолжаем.
function Add-DefenderExclusion {
    param([string]$Path)

    if (-not (Get-Command Add-MpPreference -ErrorAction SilentlyContinue)) {
        Write-Host "==> Add-MpPreference not available, skipping Defender exclusion"
        return
    }

    try {
        $existing = (Get-MpPreference -ErrorAction Stop).ExclusionPath
        if ($existing -and ($existing -contains $Path)) {
            Write-Host "==> Defender exclusion already set for $Path"
            return
        }
    } catch { }

    $isAdmin = ([Security.Principal.WindowsPrincipal] `
                [Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole(
                [Security.Principal.WindowsBuiltInRole]::Administrator)

    if ($isAdmin) {
        try {
            Add-MpPreference -ExclusionPath $Path -ErrorAction Stop
            Write-Host "==> Added $Path to Defender exclusion"
        } catch {
            Write-Host "==> Failed to add exclusion: $($_.Exception.Message)"
        }
    } else {
        Write-Host "==> Asking for admin to add $Path to Defender exclusion..."
        try {
            $psExe = (Get-Command powershell).Source
            $cmd = "Add-MpPreference -ExclusionPath '$Path'"
            Start-Process $psExe -Verb RunAs -WindowStyle Hidden `
                -ArgumentList '-NoProfile','-Command',$cmd -Wait -ErrorAction Stop
            Write-Host "==> Defender exclusion added"
        } catch {
            Write-Host ""
            Write-Host "!  Skipped Defender exclusion (user declined UAC or Defender unavailable)."
            Write-Host "   If execai.exe gets removed after install, run from elevated PowerShell:"
            Write-Host "       Add-MpPreference -ExclusionPath `"$Path`""
            Write-Host ""
        }
    }
}

Add-DefenderExclusion -Path $InstallDir

$tmp = New-Item -ItemType Directory -Path (Join-Path $env:TEMP "execai-install-$(Get-Random)")
try {
    Write-Host "==> Downloading $base/$archive"
    Invoke-WebRequest -UseBasicParsing -Uri "$base/$archive"      -OutFile (Join-Path $tmp $archive)
    Invoke-WebRequest -UseBasicParsing -Uri "$base/SHA256SUMS"    -OutFile (Join-Path $tmp 'SHA256SUMS')

    Write-Host "==> Verifying SHA256"
    $expected = (Get-Content (Join-Path $tmp 'SHA256SUMS') |
                   Select-String -SimpleMatch $archive |
                   ForEach-Object { ($_ -split '\s+')[0] })
    $actual = (Get-FileHash -Algorithm SHA256 (Join-Path $tmp $archive)).Hash.ToLower()
    if ($expected -ne $actual) {
        throw "SHA256 mismatch: expected=$expected actual=$actual"
    }

    Expand-Archive -Path (Join-Path $tmp $archive) -DestinationPath $tmp -Force
    if (-not (Test-Path $InstallDir)) { New-Item -ItemType Directory -Path $InstallDir | Out-Null }
    $exeName = if ($archive -like '*arm64*') { 'execai-windows-arm64.exe' } else { 'execai-windows-amd64.exe' }
    Move-Item -Force (Join-Path $tmp $exeName) (Join-Path $InstallDir 'execai.exe')

    # Снимаем 'Zone.Identifier' (пометка "скачано из интернета") — иначе
    # SmartScreen дополнительно предупреждает при каждом запуске.
    try {
        Unblock-File -Path (Join-Path $InstallDir 'execai.exe') -ErrorAction Stop
    } catch { }

    Write-Host "==> Installed: $InstallDir\execai.exe"

    # === Remove stale execai.exe copies from PATH ===
    # Same fix as Linux install.sh: if there were older execai.exe copies in
    # PATH (in different directories), they'd shadow the fresh one and the
    # user would still get the old version. Sweep them.
    $freshFull = (Join-Path $InstallDir 'execai.exe').ToLowerInvariant()
    foreach ($d in ($env:Path -split ';')) {
        if (-not $d) { continue }
        $p = Join-Path $d 'execai.exe'
        if (-not (Test-Path $p)) { continue }
        if ($p.ToLowerInvariant() -eq $freshFull) { continue }
        try {
            Remove-Item -Path $p -Force -ErrorAction Stop
            Write-Host "==> Removed stale copy: $p"
        } catch {
            Write-Host "!  Found stale copy $p — no permission to remove. Delete manually."
        }
    }

    & (Join-Path $InstallDir 'execai.exe') version

    # === PATH ===
    # Добавляем в User PATH если ещё нет. На Windows User-PATH хранится в реестре
    # (HKCU\Environment\Path) и пересылается в новые процессы. Текущая сессия
    # подхватит изменение только после рестарта shell — поэтому также правим
    # $env:Path локально, чтобы 'execai' заработал прямо сейчас.
    $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    if (-not $userPath) { $userPath = '' }
    $parts = $userPath -split ';' | Where-Object { $_ -ne '' }
    if ($parts -notcontains $InstallDir) {
        $newPath = if ($userPath) { "$userPath;$InstallDir" } else { $InstallDir }
        [Environment]::SetEnvironmentVariable('Path', $newPath, 'User')
        Write-Host "==> Added $InstallDir to User PATH (via registry)"
    }
    if (($env:Path -split ';') -notcontains $InstallDir) {
        $env:Path = "$env:Path;$InstallDir"
        Write-Host "==> Added $InstallDir to PATH for current session"
    }

    Write-Host ""
    Write-Host "Done. Run:"
    Write-Host "    execai"
    Write-Host "(no args = TUI chat; first run opens browser for confirmation)"
    Write-Host ""
    Write-Host "TIP: use Windows Terminal (not old cmd.exe / conhost) —"
    Write-Host "the TUI (bubbletea) renders correctly only in modern terminals."
} finally {
    Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
}
