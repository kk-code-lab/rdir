#!/usr/bin/env pwsh
<#
Helper to mirror common Make targets in PowerShell.
Usage: ./scripts/make.ps1 <target>
Targets: build, install, test, test-coverage, test-race, bench-fuzzy, bench-fuzzy-prof,
         pprof-fuzzy-cpu, pprof-fuzzy-mem, clean, fmt, lint, check, run, help.
Requires: Go in PATH; golangci-lint for lint.
#>

[CmdletBinding()]
param(
    [Parameter(Position = 0)]
    [ValidateSet('build', 'install', 'test', 'test-coverage', 'test-race', 'bench-fuzzy', 'bench-fuzzy-prof', 'pprof-fuzzy-cpu', 'pprof-fuzzy-mem', 'clean', 'fmt', 'lint', 'check', 'run', 'help')]
    [string]$Target = 'help'
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot '..')
Push-Location $repoRoot
try {
    $binaryName = 'rdir'
    $buildDir = 'build'
    $profileDir = Join-Path $buildDir 'profiles'
    $exeSuffix = $IsWindows ? '.exe' : ''
    $binPath = Join-Path $buildDir "$binaryName$exeSuffix"
    $mainEntry = './cmd/rdir'
    $internalPackages = './internal/...'

    if (-not $env:RDIR_DEBUG_LOG) {
        $env:RDIR_DEBUG_LOG = '1'
    }

    function Get-GitCommit {
        try {
            $result = git rev-parse --short HEAD 2>$null
            if ($result) { return $result.Trim() }
        } catch {
        }
        return 'unknown'
    }

    $buildFlag = "-X github.com/kk-code-lab/rdir/internal/app.BuildCommit=$(Get-GitCommit)"

    function Invoke-CommandChecked {
        param([Parameter(Mandatory)][string]$Description, [Parameter(Mandatory)][scriptblock]$Action)
        Write-Host $Description
        & $Action
    }

    function Invoke-BenchProfiles {
        New-Item -ItemType Directory -Force -Path $profileDir | Out-Null
        go test $internalPackages -run='^$' -bench Fuzzy -benchmem -cpuprofile (Join-Path $profileDir 'fuzzy.cpu.pprof') -memprofile (Join-Path $profileDir 'fuzzy.mem.pprof')
        Write-Host "`nProfiles written to:"
        Write-Host "  $(Join-Path $profileDir 'fuzzy.cpu.pprof')"
        Write-Host "  $(Join-Path $profileDir 'fuzzy.mem.pprof')"
    }

    switch ($Target) {
        'build' {
            Invoke-CommandChecked -Description "Building $binaryName..." -Action { go build -ldflags $buildFlag -o $binPath $mainEntry }
        }
        'install' {
            Invoke-CommandChecked -Description "Installing $binaryName to GOBIN (or GOPATH/bin)..." -Action { go install -ldflags $buildFlag $mainEntry }
        }
        'test' {
            Invoke-CommandChecked -Description 'Running tests...' -Action { go test $internalPackages }
        }
        'test-coverage' {
            Invoke-CommandChecked -Description 'Running tests with coverage...' -Action { go test $internalPackages -cover }
        }
        'test-race' {
            Invoke-CommandChecked -Description 'Running tests with race detector...' -Action { go test $internalPackages -race }
        }
        'bench-fuzzy' {
            Invoke-CommandChecked -Description 'Running fuzzy matching benchmarks...' -Action { go test $internalPackages -bench Fuzzy -benchmem }
        }
        'bench-fuzzy-prof' {
            Invoke-CommandChecked -Description 'Running fuzzy benchmarks with CPU/memory profiles...' -Action { Invoke-BenchProfiles }
        }
        'pprof-fuzzy-cpu' {
            $cpuProfile = Join-Path $profileDir 'fuzzy.cpu.pprof'
            if (-not (Test-Path $cpuProfile)) { throw "CPU profile not found. Run './scripts/make.ps1 bench-fuzzy-prof' first." }
            Invoke-CommandChecked -Description 'Top hot paths (cpu):' -Action { go tool pprof -top $cpuProfile }
        }
        'pprof-fuzzy-mem' {
            $memProfile = Join-Path $profileDir 'fuzzy.mem.pprof'
            if (-not (Test-Path $memProfile)) { throw "Memory profile not found. Run './scripts/make.ps1 bench-fuzzy-prof' first." }
            Invoke-CommandChecked -Description 'Top allocators (mem):' -Action { go tool pprof -top $memProfile }
        }
        'clean' {
            Write-Host 'Cleaning build artifacts...'
            Remove-Item -Force -ErrorAction SilentlyContinue $binPath
        }
        'fmt' {
            Invoke-CommandChecked -Description 'Formatting code...' -Action { go fmt ./... }
        }
        'lint' {
            Invoke-CommandChecked -Description 'Linting code (requires golangci-lint)...' -Action { golangci-lint run ./... }
        }
        'check' {
            Invoke-CommandChecked -Description 'Linting code (requires golangci-lint)...' -Action { golangci-lint run ./... }
            Invoke-CommandChecked -Description "Building $binaryName..." -Action { go build -ldflags $buildFlag -o $binPath $mainEntry }
            Invoke-CommandChecked -Description 'Type-checking tests (no execution)...' -Action { go test -run '^$' $internalPackages }
        }
        'run' {
            Invoke-CommandChecked -Description "Building $binaryName..." -Action { go build -ldflags $buildFlag -o $binPath $mainEntry }
            Invoke-CommandChecked -Description "Running $binaryName..." -Action { & $binPath }
        }
        Default {
            Write-Host 'rdir - Terminal file manager in Go'
            Write-Host ''
            Write-Host 'Available targets:'
            Write-Host '  build              - Build the binary to build/rdir'
            Write-Host '  install            - Install the binary to GOBIN (or GOPATH/bin)'
            Write-Host '  test               - Run all tests'
            Write-Host '  test-coverage      - Run tests with coverage report'
            Write-Host '  test-race          - Run tests with race detector'
            Write-Host '  bench-fuzzy        - Run fuzzy matching benchmarks'
            Write-Host '  bench-fuzzy-prof   - Run fuzzy benchmarks and capture CPU/memory profiles'
            Write-Host '  pprof-fuzzy-cpu    - Show hottest stack traces from the captured CPU profile'
            Write-Host '  pprof-fuzzy-mem    - Show top allocators from the captured memory profile'
            Write-Host '  check              - Lint, build, and compile tests without running them'
            Write-Host '  run                - Build and run the application'
            Write-Host '  clean              - Remove build artifacts'
            Write-Host '  fmt                - Format code with go fmt'
            Write-Host '  lint               - Run linter (requires golangci-lint)'
            Write-Host '  help               - Show this help message'
        }
    }
}
finally {
    Pop-Location
}
