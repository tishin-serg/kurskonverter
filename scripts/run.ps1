param([switch]$CheckBybit, [switch]$CheckRoutes)
$ErrorActionPreference='Stop'
Set-Location -LiteralPath "$PSScriptRoot\.."
# Data only: never execute .env as PowerShell or print its values.
if (Test-Path -LiteralPath '.env') {
  foreach ($line in Get-Content -LiteralPath '.env') {
    $entry=$line.Trim()
    if (!$entry -or $entry.StartsWith('#')) { continue }
    if ($entry -notmatch '^([A-Z][A-Z0-9_]*)=(.*)$') { throw 'Invalid .env line; expected NAME=value' }
    $name=$Matches[1]; $value=$Matches[2].Trim()
    if ($value.Length -ge 2 -and (($value.StartsWith('"') -and $value.EndsWith('"')) -or ($value.StartsWith("'") -and $value.EndsWith("'")))) { $value=$value.Substring(1,$value.Length-2) }
    [Environment]::SetEnvironmentVariable($name,$value,'Process')
  }
}
if (!(Test-Path -LiteralPath 'bin/bot.exe')) { throw 'Build first: ./scripts/check.ps1 build' }
if ($CheckRoutes) { & ./bin/bot.exe check-routes } elseif ($CheckBybit) { & ./bin/bot.exe check-bybit } else { & ./bin/bot.exe }
exit $LASTEXITCODE
