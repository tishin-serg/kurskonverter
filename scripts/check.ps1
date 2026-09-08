param([ValidateSet('test','race','lint','coverage','vuln','build')][string]$Task='test')
$ErrorActionPreference='Stop'
$env:GOPATH="$PSScriptRoot\..\.cache\go"
$env:GOMODCACHE="$PSScriptRoot\..\.cache\mod"
$env:GOCACHE="$PSScriptRoot\..\.cache\build"
$env:STATICCHECK_CACHE="$PSScriptRoot\..\.cache\staticcheck"
switch ($Task) {
  test { go test ./... }
  race { go test -race ./... }
  lint {
    $unformatted=gofmt -l cmd internal
    if ($unformatted) { throw 'Run gofmt -w cmd internal' }
    go vet ./...
    if ($LASTEXITCODE) { exit $LASTEXITCODE }
    go run honnef.co/go/tools/cmd/staticcheck@v0.8.1 ./...
  }
  coverage { go test '-coverprofile=coverage.out' ./...; if ($LASTEXITCODE) { exit $LASTEXITCODE }; go tool cover '-func=coverage.out' }
  vuln { go run golang.org/x/vuln/cmd/govulncheck@v1.7.0 ./... }
  build { $env:CGO_ENABLED='0'; go build -trimpath -o bin/bot.exe ./cmd/bot }
}
exit $LASTEXITCODE
