# Build the production router image and push to Docker Hub for build.io deployment.
#
# Usage:
#   .\build_and_push.ps1 -DockerHubUser YOUR_USERNAME
#   .\build_and_push.ps1 -DockerHubUser YOUR_USERNAME -Tag v1 -Push
#
# Prerequisites:
#   - Docker Desktop running
#   - docker login (for push)
#   - Run from repo root (d:\router)

param(
    [Parameter(Mandatory = $true)]
    [string]$DockerHubUser,

    [string]$Tag = "latest",
    [switch]$Push,
    [string]$Platform = "linux/amd64"
)

$ErrorActionPreference = "Stop"
Set-Location $PSScriptRoot

$image = "${DockerHubUser}/weave-router:${Tag}"
$sha = (git rev-parse HEAD 2>$null)
if (-not $sha) { $sha = "unknown" }
$buildTime = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")

Write-Host "Building $image" -ForegroundColor Cyan
Write-Host "  ROUTER_SHA=$sha"
Write-Host "  ROUTER_BUILD_TIME=$buildTime"
Write-Host "  Platform=$Platform"
Write-Host ""

docker build `
    -f Dockerfile.buildio `
    -t $image `
    --platform $Platform `
    --build-arg "ROUTER_SHA=$sha" `
    --build-arg "ROUTER_BUILD_TIME=$buildTime" `
    --build-arg "ROUTER_PR=buildio" `
    --build-arg "TARGETARCH=amd64" `
    .

if ($LASTEXITCODE -ne 0) {
    Write-Error "Docker build failed"
}

Write-Host ""
Write-Host "Build succeeded: $image" -ForegroundColor Green

if ($Push) {
    Write-Host "Pushing to Docker Hub..." -ForegroundColor Cyan
    docker push $image
    if ($LASTEXITCODE -ne 0) {
        Write-Error "Docker push failed — run 'docker login' first"
    }
    Write-Host "Push succeeded." -ForegroundColor Green
} else {
    Write-Host "Skipping push (pass -Push to upload)." -ForegroundColor Yellow
}

Write-Host ""
Write-Host "=== Next: deploy on build.io ===" -ForegroundColor Cyan
Write-Host "1. Dashboard: https://www.build.io/dashboard -> app 'router' -> Deploy"
Write-Host "   Container image: $image"
Write-Host ""
Write-Host "2. Or CLI:"
Write-Host "   bld config:set CONTAINER_IMAGE=$image --app router"
Write-Host "   bld deploy router"
Write-Host ""
Write-Host "3. Set env vars from .env.production.local (see BUILDIO_DEPLOYMENT_GUIDE.md)"
