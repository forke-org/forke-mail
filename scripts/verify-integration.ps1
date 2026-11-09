$ErrorActionPreference = 'Stop'
$expectedAdvik = if ($env:VERIFY_EXPECTED_ADVIK) { $env:VERIFY_EXPECTED_ADVIK } else { 'a7d4f0c626369a3241aa168d241cdb2499e0f08d' }
$expectedDiya = if ($env:VERIFY_EXPECTED_DIYA) { $env:VERIFY_EXPECTED_DIYA } else { 'f704c79db0313f15b81c86986653fb284accaf94' }
$env:VERIFY_EXPECTED_ADVIK = $expectedAdvik
$env:VERIFY_EXPECTED_DIYA = $expectedDiya
Push-Location (Join-Path $PSScriptRoot '..')
try {
    $goBinary = if ($env:GO_BIN) { $env:GO_BIN } else { (Get-Command go -ErrorAction SilentlyContinue).Source }
    if (-not $goBinary) { $goBinary = Join-Path $PSScriptRoot '../../tools/go/bin/go.exe' }
    if (-not (Test-Path -LiteralPath $goBinary)) { throw 'Install Go 1.26+ or set GO_BIN' }
    $env:PATH = (Split-Path $goBinary -Parent) + [IO.Path]::PathSeparator + $env:PATH
    $env:CGO_ENABLED = '0'
    if (-not $env:GOMAXPROCS) { $env:GOMAXPROCS = '1' }
    if (-not $env:GOFLAGS) { $env:GOFLAGS = '-p=1' }
    if (-not $env:GOMEMLIMIT) { $env:GOMEMLIMIT = '256MiB' }
    if (-not $env:GOGC) { $env:GOGC = '20' }
    & $goBinary run ./cmd/verifyintegration
    $verifyExit = $LASTEXITCODE
} finally {
    Pop-Location
}
exit $verifyExit
