param(
  [Parameter(Mandatory=$true)][string]$WorkspacePath,
  [Parameter(Mandatory=$true)][string]$ArtifactsPath,
  [Parameter(Mandatory=$true)][string]$GoExecutable
)
$ErrorActionPreference='Stop'
if (!$env:TEST_DATABASE_URL) { throw 'Run through test-backend-postgres.ps1 -BrowserSmoke so the database is isolated.' }
$nodeCommand=Get-Command node -ErrorAction SilentlyContinue
$nodeExecutable=if ($nodeCommand) { $nodeCommand.Source } else { 'C:\Program Files\nodejs\node.exe' }
function Get-TestPort {
  $listener=New-Object System.Net.Sockets.TcpListener([System.Net.IPAddress]::Loopback,0)
  $listener.Start()
  try { return $listener.LocalEndpoint.Port } finally { $listener.Stop() }
}
function Wait-TestUrl([string]$Url) {
  $deadline=[DateTime]::UtcNow.AddSeconds(120)
  while ([DateTime]::UtcNow -lt $deadline) {
    try { $response=Invoke-WebRequest -UseBasicParsing -TimeoutSec 2 -Uri $Url; if ($response.StatusCode -eq 200) { return } } catch {}
    Start-Sleep -Milliseconds 500
  }
  throw "Test server did not become ready: $Url"
}
$names=@('DATABASE_URL','APP_ORIGIN','HTTP_ADDR','PORT','VERCEL','SESSION_COOKIE_SECURE','TESSERA_ALLOW_DEMO_SEED','DEMO_PASSWORD','E2E_BASE_URL','E2E_PASSWORD')
$previous=@{};foreach ($name in $names) { $previous[$name]=[Environment]::GetEnvironmentVariable($name,'Process') }
$apiProcess=$null;$frontendProcess=$null
try {
  $apiPort=Get-TestPort;$frontendPort=Get-TestPort
  while ($apiPort -eq $frontendPort) { $frontendPort=Get-TestPort }
  $env:DATABASE_URL=$env:TEST_DATABASE_URL
  $env:APP_ORIGIN="http://127.0.0.1:$frontendPort"
  $env:HTTP_ADDR="127.0.0.1:$apiPort"
  $env:PORT='';$env:VERCEL='';$env:SESSION_COOKIE_SECURE='false';$env:TESSERA_ALLOW_DEMO_SEED='true'
  $env:DEMO_PASSWORD=[guid]::NewGuid().ToString('N')
  $env:E2E_PASSWORD=$env:DEMO_PASSWORD;$env:E2E_BASE_URL=$env:APP_ORIGIN
  $binary=Join-Path $ArtifactsPath 'tessera-browser-api.exe'
  Push-Location (Join-Path $WorkspacePath 'backend')
  try {
    & $GoExecutable build -o $binary ./cmd/api
    if ($LASTEXITCODE -ne 0) { throw 'Could not build browser-test API.' }
    & $GoExecutable run ./cmd/seed
    if ($LASTEXITCODE -ne 0) { throw 'Could not seed browser-test database.' }
  } finally { Pop-Location }
  $proxy=Join-Path $ArtifactsPath 'browser-proxy.json'
  $proxyContent=@{ '/api/**'=@{target="http://127.0.0.1:$apiPort";secure=$false;changeOrigin=$true} } | ConvertTo-Json -Depth 4
  [IO.File]::WriteAllText($proxy,$proxyContent,(New-Object Text.UTF8Encoding($false)))
  $relativeProxy='../.cache/' + (Split-Path -Leaf $ArtifactsPath) + '/browser-proxy.json'
  $apiProcess=Start-Process -FilePath $binary -WindowStyle Hidden -PassThru -RedirectStandardOutput (Join-Path $ArtifactsPath 'api.log') -RedirectStandardError (Join-Path $ArtifactsPath 'api-error.log')
  $frontendProcess=Start-Process -FilePath $nodeExecutable -ArgumentList @('node_modules/@angular/cli/bin/ng.js','serve','--host','127.0.0.1','--port',"$frontendPort",'--proxy-config',$relativeProxy) -WorkingDirectory (Join-Path $WorkspacePath 'frontend') -WindowStyle Hidden -PassThru -RedirectStandardOutput (Join-Path $ArtifactsPath 'frontend.log') -RedirectStandardError (Join-Path $ArtifactsPath 'frontend-error.log')
  Wait-TestUrl "$env:APP_ORIGIN/"
  Wait-TestUrl "http://127.0.0.1:$apiPort/api/readyz"
  Push-Location (Join-Path $WorkspacePath 'frontend')
  try { & $nodeExecutable e2e/publication.smoke.mjs; if ($LASTEXITCODE -ne 0) { throw 'Publication browser smoke failed.' } } finally { Pop-Location }
} finally {
  foreach ($process in @($frontendProcess,$apiProcess)) { if ($process) { Stop-Process -Id $process.Id -ErrorAction SilentlyContinue } }
  $diagnostics=Join-Path $WorkspacePath '.cache\publication-browser'
  New-Item -ItemType Directory -Path $diagnostics -Force | Out-Null
  foreach ($log in @('api.log','api-error.log','frontend.log','frontend-error.log')) {
    $source=Join-Path $ArtifactsPath $log
    if (Test-Path -LiteralPath $source) { Copy-Item -LiteralPath $source -Destination (Join-Path $diagnostics $log) -Force }
  }
  foreach ($name in $names) { [Environment]::SetEnvironmentVariable($name,$previous[$name],'Process') }
}
