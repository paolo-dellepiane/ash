$version = Get-Content .\version -Raw
$now = Get-Date -AsUTC -Format o
$sha1 = (git rev-parse HEAD).Trim()
go build -ldflags "-s -w -X main.version=$version -X main.sha1ver=$sha1 -X main.buildTime=$now" && 7z a ash.zip ash.exe ash.config.json utils\*