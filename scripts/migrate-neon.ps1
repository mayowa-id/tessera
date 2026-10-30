$ErrorActionPreference = 'Stop'
$workspacePath = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).Path
$goCommand = Get-Command go -ErrorAction SilentlyContinue
$goExecutable = if ($goCommand) { $goCommand.Source } else { 'C:\Program Files\Go\bin\go.exe' }
if (!(Test-Path -LiteralPath $goExecutable)) { throw 'Install Go for Windows before running migrations.' }

$secret = Read-Host 'Paste the Neon direct PostgreSQL URL (with TLS)' -AsSecureString
$connection = [System.Net.NetworkCredential]::new('', $secret).Password
$connectionUri = $null
if (![Uri]::TryCreate($connection, [UriKind]::Absolute, [ref]$connectionUri) -or $connectionUri.Scheme -notin @('postgres', 'postgresql')) {
  throw 'Use the direct PostgreSQL connection URL from Neon.'
}
if ($connection -notmatch '[?&]sslmode=(require|verify-ca|verify-full)(&|$)') {
  throw 'Keep sslmode=require (or verify-ca/verify-full) in the connection URL.'
}

$previousConnection = $env:GOOSE_DBSTRING
$previousDriver = $env:GOOSE_DRIVER
try {
  $env:GOOSE_DBSTRING = $connection
  $env:GOOSE_DRIVER = 'postgres'
  Push-Location (Join-Path $workspacePath 'backend')
  try {
    foreach ($migrationCommand in @('status', 'up', 'version')) {
      & $goExecutable run github.com/pressly/goose/v3/cmd/goose@v3.24.3 -env none -dir db/migrations $migrationCommand
      if ($LASTEXITCODE -ne 0) { throw "Migration command '$migrationCommand' failed. No success has been confirmed." }
    }
  } finally { Pop-Location }
} finally {
  $env:GOOSE_DBSTRING = $previousConnection
  $env:GOOSE_DRIVER = $previousDriver
  $connection = $null
  $secret.Dispose()
}
