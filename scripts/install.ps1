# Install the latest codehop release on Windows.
#   irm https://raw.githubusercontent.com/ardacoskun/codehop/main/scripts/install.ps1 | iex
# Environment: CODEHOP_VERSION (e.g. v0.1.0, default latest),
#              CODEHOP_INSTALL_DIR (default %LOCALAPPDATA%\Programs\codehop).
# Runs in its own scope: `irm | iex` would otherwise leave
# $ErrorActionPreference and helper variables set in the caller's session.
& {
    $ErrorActionPreference = 'Stop'
    # Windows PowerShell 5.1 may default to TLS 1.0/1.1, which GitHub rejects.
    [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12

    $repo = 'ardacoskun/codehop'
    $binDir = if ($env:CODEHOP_INSTALL_DIR) { $env:CODEHOP_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA 'Programs\codehop' }
    $arch = switch ($env:PROCESSOR_ARCHITECTURE) {
        'AMD64' { 'amd64' }
        'ARM64' { 'arm64' }
        default { throw "Unsupported architecture $($env:PROCESSOR_ARCHITECTURE)" }
    }

    $version = $env:CODEHOP_VERSION
    if (-not $version) {
        $version = (Invoke-RestMethod "https://api.github.com/repos/$repo/releases/latest").tag_name
    }
    if ($version -notmatch '^v\d+\.\d+\.\d+([-+][0-9A-Za-z.-]+)?$') { throw "Invalid version '$version' (expected e.g. v0.1.0)" }

    $archive = "codehop_$($version.TrimStart('v'))_windows_$arch.zip"
    $base = "https://github.com/$repo/releases/download/$version"
    $tmp = Join-Path ([System.IO.Path]::GetTempPath()) ("codehop-" + [guid]::NewGuid())
    New-Item -ItemType Directory -Path $tmp | Out-Null
    try {
        Write-Host "Downloading codehop $version (windows/$arch)..."
        Invoke-WebRequest -UseBasicParsing "$base/$archive" -OutFile (Join-Path $tmp $archive)
        Invoke-WebRequest -UseBasicParsing "$base/checksums.txt" -OutFile (Join-Path $tmp 'checksums.txt')

        $line = Get-Content (Join-Path $tmp 'checksums.txt') | Where-Object { $_ -match " $([regex]::Escape($archive))$" }
        if (-not $line) { throw "No checksum for $archive" }
        $expected = ($line -split '\s+')[0]
        $actual = (Get-FileHash (Join-Path $tmp $archive) -Algorithm SHA256).Hash.ToLower()
        if ($expected -ne $actual) { throw "Checksum mismatch for $archive" }

        Expand-Archive (Join-Path $tmp $archive) -DestinationPath $tmp -Force
        New-Item -ItemType Directory -Path $binDir -Force | Out-Null
        Copy-Item (Join-Path $tmp 'codehop.exe') (Join-Path $binDir 'codehop.exe') -Force
    } finally {
        Remove-Item $tmp -Recurse -Force -ErrorAction SilentlyContinue
    }
    Write-Host "Installed codehop $version to $binDir\codehop.exe"
    Write-Host "Verify provenance (optional): gh attestation verify `"$binDir\codehop.exe`" --repo $repo"

    # Read the user PATH unexpanded so %VAR% entries survive, and keep its
    # REG_EXPAND_SZ type when writing it back.
    $envKey = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey('Environment', $true)
    try {
        $rawPath = [string]$envKey.GetValue('Path', '', [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames)
        $entries = @($rawPath -split ';' | Where-Object { $_ })
        if ($entries -notcontains $binDir) {
            $envKey.SetValue('Path', (($entries + $binDir) -join ';'), [Microsoft.Win32.RegistryValueKind]::ExpandString)
            # Setting any user variable broadcasts WM_SETTINGCHANGE so new
            # terminals see the updated PATH.
            [Environment]::SetEnvironmentVariable('CODEHOP_INSTALL_TMP', '1', 'User')
            [Environment]::SetEnvironmentVariable('CODEHOP_INSTALL_TMP', $null, 'User')
            Write-Host "Added $binDir to your user PATH (open a new terminal)."
        }
    } finally {
        $envKey.Close()
    }
    Write-Host 'Next: add to $PROFILE:  Invoke-Expression (& { (codehop init powershell | Out-String) })'
}
