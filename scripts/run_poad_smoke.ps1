# Run the standalone 80-payment strict PoAD completion smoke test on Windows.
# Preserves the default 10k experiment and archives any previous smoke run.
$repoRoot = Split-Path -Parent $PSScriptRoot
Set-Location $repoRoot

$stamp = Get-Date -Format "yyyyMMdd_HHmmss"
$diag = Join-Path $repoRoot "diagnostics"
New-Item -ItemType Directory -Path $diag -Force | Out-Null

$results = Join-Path $repoRoot "exp\poad-smoke-results"
$log = Join-Path $diag "poad_smoke_$stamp.log"
$exitFile = Join-Path $diag "poad_smoke_exit_$stamp.txt"

Write-Host "Building Go packages before touching prior smoke output..."
& go build ./...
if ($LASTEXITCODE -ne 0) {
    Write-Error "Go build failed; previous experiment results were preserved"
    exit 1
}

if (Test-Path $results) {
    $archive = Join-Path $diag "poad_smoke_backup_$stamp"
    Move-Item -LiteralPath $results -Destination $archive
    Write-Host "Previous smoke results archived at $archive"
}

Write-Host "Strict PoAD smoke: 80 local payments, four shards, max 600s."
Write-Host "Console log: $log"

# Use cmd to combine native stdout/stderr consistently in Windows PowerShell 5.1.
cmd /c "go run ./cmd/agentemu -config agentEmuSmokeConfig.yaml 2>&1" |
    Tee-Object -FilePath $log

$chainExit = $LASTEXITCODE
"AgentEmulator exit code: $chainExit" | Set-Content -Path $exitFile
Write-Host "AgentEmulator exit code: $chainExit"

$pythonCommand = $null
$pythonFlags = @()
if (Get-Command python -ErrorAction SilentlyContinue) {
    $pythonCommand = "python"
} elseif (Get-Command py -ErrorAction SilentlyContinue) {
    $pythonCommand = "py"
    $pythonFlags = @("-3")
}

if (-not $pythonCommand) {
    Write-Error "Python is required for smoke completion verification; see $log"
    exit 1
}

if (Test-Path (Join-Path $results "round_001")) {
    # Generate the same thesis metrics as the normal launcher, even when the
    # AgentEmulator exits nonzero (partial diagnostics are still useful).
    & $pythonCommand @pythonFlags .\scripts\analyze_agentemu_results.py $results
    if ($LASTEXITCODE -ne 0) {
        Write-Warning "Thesis metrics could not be generated"
    }

    & $pythonCommand @pythonFlags .\scripts\verify_poad_smoke.py $results
    $verifyExit = $LASTEXITCODE
} else {
    Write-Warning "No round_001 directory; inspect the startup log"
    $verifyExit = 1
}

Write-Host "Results: $results"
Write-Host "Console: $log"
Write-Host "Exit status: $exitFile"

if ($chainExit -ne 0 -or $verifyExit -ne 0) {
    Write-Error "PoAD smoke INCOMPLETE: see smoke_validation.json and console log"
    exit 1
}

Write-Host "PoAD smoke PASSED: 80 payment hashes have ACCEPT decisions and committed block roots; four shards completed."
exit 0
