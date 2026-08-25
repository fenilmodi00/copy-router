# Set Build.io config vars for app `router` (Supabase + Pub/Sub disabled).
# Requires: BUILDIO_API_TOKEN (Account → API token), and DATABASE_URL in env or -DatabaseUrl.
#
# Usage:
#   $env:BUILDIO_API_TOKEN = '...'
#   $env:DATABASE_URL = 'postgresql://postgres.REF:PASS@aws-0-ap-northeast-1.pooler.supabase.com:5432/postgres'
#   .\scripts\buildio-set-safe-config.ps1
#
# Optional: -EnvFile .env.production.local to pull AIAND_API_KEY and friends (never prints values).

param(
  [string]$App = "router",
  [string]$ApiBase = "https://app.build.io/api/v1",
  [string]$DatabaseUrl = $env:DATABASE_URL,
  [string]$EnvFile = "",
  [switch]$DryRun
)

$ErrorActionPreference = "Stop"
$token = $env:BUILDIO_API_TOKEN
if (-not $token) { throw "Set BUILDIO_API_TOKEN (Build.io Account → API token)" }
if (-not $DatabaseUrl) { throw "Set DATABASE_URL or pass -DatabaseUrl (Supabase session pooler :5432)" }

function Read-EnvFile([string]$path) {
  $map = @{}
  if (-not $path -or -not (Test-Path $path)) { return $map }
  Get-Content $path | ForEach-Object {
    $line = $_.Trim()
    if ($line -match '^\s*#' -or $line -eq '') { return }
    if ($line -match '^(?:export\s+)?([A-Za-z_][A-Za-z0-9_]*)\s*[:=]\s*(.*)$') {
      $k = $Matches[1]
      $v = $Matches[2].Trim().Trim('"').Trim("'")
      $map[$k] = $v
    }
  }
  return $map
}

$fromFile = Read-EnvFile $EnvFile
function Pick([string]$key, [string]$fallback = $null) {
  if ($fromFile.ContainsKey($key) -and $fromFile[$key]) { return $fromFile[$key] }
  if (Test-Path "env:$key") {
    $v = (Get-Item "env:$key").Value
    if ($v) { return $v }
  }
  return $fallback
}

$payload = [ordered]@{
  DATABASE_URL            = $DatabaseUrl
  PUBSUB_DISABLED         = "true"
  SERVER_REPLICAS         = "1"
  PORT                    = (Pick "PORT" "8080")
  ROUTER_DEPLOYMENT_MODE  = (Pick "ROUTER_DEPLOYMENT_MODE" "selfhosted")
  ROUTER_CLUSTER_VERSION  = (Pick "ROUTER_CLUSTER_VERSION" "v0.76")
  LOG_LEVEL               = (Pick "LOG_LEVEL" "info")
  OTEL_SERVICE_NAME       = (Pick "OTEL_SERVICE_NAME" "router")
}

foreach ($k in @(
  "AIAND_API_KEY", "AIAND_API_URL", "ROUTER_ADMIN_PASSWORD",
  "EXTERNAL_KEY_ENCRYPTION_KEY", "ROUTER_FEEDBACK_LINK_SECRET", "ROUTER_FEEDBACK_BASE_URL"
)) {
  $v = Pick $k
  if ($v) { $payload[$k] = $v }
}

# Explicitly clear crash-causing half-config if present on the app.
$clear = @("PUBSUB_PROJECT_ID", "PUBSUB_EMULATOR_HOST", "PUBSUB_TOPIC_ROUTER_INVALIDATION", "PUBSUB_SUBSCRIPTION_ROUTER_INVALIDATION")

Write-Host "App=$App keys=$($payload.Keys -join ',') clear=$($clear -join ',')"
if ($DryRun) { Write-Host "DryRun: no API calls"; exit 0 }

$headers = @{
  Authorization = "Bearer $token"
  "Content-Type" = "application/json"
  Accept = "application/json"
}

$body = ($payload | ConvertTo-Json -Compress)
Invoke-RestMethod -Method Patch -Uri "$ApiBase/apps/$App/config-vars" -Headers $headers -Body $body | Out-Null
Write-Host "Patched config vars"

foreach ($k in $clear) {
  try {
    Invoke-RestMethod -Method Delete -Uri "$ApiBase/apps/$App/config-vars/$k" -Headers $headers | Out-Null
    Write-Host "Cleared $k"
  } catch {
    # 404 = already absent
  }
}

Write-Host "Done. Redeploy or restart dynos, then: curl -skSf https://router-568f5bb0.onbld.com/health"
