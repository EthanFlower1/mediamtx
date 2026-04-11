# ---------------------------------------------------------------------------
# sign-windows.ps1 — EV code-signing helper for CI (GitHub Actions).
#
# Supports two modes:
#   1. Azure Key Vault (cloud HSM) via AzureSignTool — preferred for CI.
#   2. Local USB EV token via signtool.exe — for manual release builds.
#
# Required environment variables (Azure):
#   AZURE_VAULT_URI, AZURE_CERT_NAME, AZURE_TENANT_ID,
#   AZURE_CLIENT_ID, AZURE_CLIENT_SECRET
#
# Required environment variables (USB token):
#   CODESIGN_CERT_THUMBPRINT
#
# Optional:
#   CODESIGN_TIMESTAMP_URL (default: http://timestamp.digicert.com)
# ---------------------------------------------------------------------------
param(
    [Parameter(Mandatory)]
    [string[]]$Files,

    [string]$TimestampUrl = $env:CODESIGN_TIMESTAMP_URL
)

if (-not $TimestampUrl) {
    $TimestampUrl = "http://timestamp.digicert.com"
}

function Sign-WithAzure {
    param([string]$FilePath)

    if (-not (Get-Command AzureSignTool -ErrorAction SilentlyContinue)) {
        dotnet tool install --global AzureSignTool 2>$null
    }

    & AzureSignTool sign `
        -kvu $env:AZURE_VAULT_URI `
        -kvc $env:AZURE_CERT_NAME `
        -kvt $env:AZURE_TENANT_ID `
        -kvi $env:AZURE_CLIENT_ID `
        -kvs $env:AZURE_CLIENT_SECRET `
        -tr $TimestampUrl `
        -td sha256 `
        $FilePath

    if ($LASTEXITCODE -ne 0) {
        throw "AzureSignTool failed for $FilePath"
    }
}

function Sign-WithLocal {
    param([string]$FilePath)

    $signtool = Get-ChildItem "C:\Program Files (x86)\Windows Kits\*\bin\*\x64\signtool.exe" -Recurse |
        Sort-Object FullName -Descending |
        Select-Object -First 1

    if (-not $signtool) {
        throw "signtool.exe not found. Install Windows SDK."
    }

    & $signtool.FullName sign `
        /sha1 $env:CODESIGN_CERT_THUMBPRINT `
        /fd sha256 `
        /tr $TimestampUrl `
        /td sha256 `
        /v `
        $FilePath

    if ($LASTEXITCODE -ne 0) {
        throw "signtool failed for $FilePath"
    }
}

# Determine signing method.
$useAzure = -not [string]::IsNullOrEmpty($env:AZURE_VAULT_URI)
$useLocal = -not [string]::IsNullOrEmpty($env:CODESIGN_CERT_THUMBPRINT)

if (-not $useAzure -and -not $useLocal) {
    Write-Error "No signing credentials found. Set AZURE_VAULT_URI or CODESIGN_CERT_THUMBPRINT."
    exit 1
}

foreach ($file in $Files) {
    Write-Host "Signing: $file"
    if ($useAzure) {
        Sign-WithAzure -FilePath $file
    } else {
        Sign-WithLocal -FilePath $file
    }
}

Write-Host "All files signed successfully."
