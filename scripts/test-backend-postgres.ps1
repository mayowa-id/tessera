param(
  [string]$PostgresBin = 'C:\Program Files\PostgreSQL\18\bin',
  [ValidateRange(1024, 65535)][int]$Port = 15432,
  [switch]$CheckMigrations
)

$ErrorActionPreference = 'Stop'
$workspacePath = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).Path
$cachePath = Join-Path $workspacePath '.cache'
$testPath = Join-Path $cachePath ('postgres-test-' + [guid]::NewGuid().ToString('N'))
$previousConnection = $env:TEST_DATABASE_URL
$created = $false
$testExitCode = 1

foreach ($binary in @('initdb.exe', 'pg_ctl.exe', 'pg_isready.exe')) {
  if (!(Test-Path -LiteralPath (Join-Path $PostgresBin $binary))) { throw "Missing PostgreSQL binary: $binary in $PostgresBin" }
}
$goCommand = Get-Command go -ErrorAction SilentlyContinue
$goExecutable = if ($goCommand) { $goCommand.Source } else { 'C:\Program Files\Go\bin\go.exe' }
if (!(Test-Path -LiteralPath $goExecutable)) { throw 'Install Go or add it to PATH first.' }
& (Join-Path $PostgresBin 'pg_isready.exe') -h 127.0.0.1 -p $Port -t 1 *> $null
if ($LASTEXITCODE -eq 0) { throw "Port $Port already serves PostgreSQL. Choose another -Port; this script does not reuse existing databases." }

New-Item -ItemType Directory -Path $cachePath -Force | Out-Null
try {
  # Trust is restricted to this disposable loopback instance containing only test fixtures.
  & (Join-Path $PostgresBin 'initdb.exe') -D $testPath -U tessera_test -A trust -E UTF8 --locale=C
  if ($LASTEXITCODE -ne 0) { throw 'Could not initialize the test cluster.' }
  $created = $true
  & (Join-Path $PostgresBin 'pg_ctl.exe') -D $testPath -l (Join-Path $testPath 'server.log') -o "-h 127.0.0.1 -p $Port" -w start
  if ($LASTEXITCODE -ne 0) { throw 'Could not start the test cluster.' }
  $env:TEST_DATABASE_URL = "postgres://tessera_test@127.0.0.1:$Port/postgres?sslmode=disable"
  Push-Location (Join-Path $workspacePath 'backend')
  try {
    if ($CheckMigrations) {
      & $goExecutable run github.com/pressly/goose/v3/cmd/goose@v3.24.3 -dir db/migrations validate
      if ($LASTEXITCODE -ne 0) { throw 'Migration validation failed.' }
      foreach ($migrationCommand in @('up', 'down', 'up')) {
        & $goExecutable run github.com/pressly/goose/v3/cmd/goose@v3.24.3 -dir db/migrations postgres $env:TEST_DATABASE_URL $migrationCommand
        if ($LASTEXITCODE -ne 0) { throw "Migration $migrationCommand failed." }
      }
    }
    & $goExecutable test ./... -count=1
    $testExitCode = $LASTEXITCODE
  } finally { Pop-Location }
} finally {
  $env:TEST_DATABASE_URL = $previousConnection
  if ($created) {
    & (Join-Path $PostgresBin 'pg_ctl.exe') -D $testPath status *> $null
    if ($LASTEXITCODE -eq 0) {
      & (Join-Path $PostgresBin 'pg_ctl.exe') -D $testPath -m fast -w stop
      if ($LASTEXITCODE -ne 0) { throw "Could not stop test database. Data retained at $testPath" }
    }
  }
  if ($created) {
    $resolvedTestPath = (Resolve-Path -LiteralPath $testPath).Path
    $expectedPrefix = $cachePath + [IO.Path]::DirectorySeparatorChar + 'postgres-test-'
    if (!$resolvedTestPath.StartsWith($expectedPrefix, [StringComparison]::OrdinalIgnoreCase)) { throw 'Refusing cleanup outside the test cache.' }
    Remove-Item -LiteralPath $resolvedTestPath -Recurse -Force
  }
}
exit $testExitCode
