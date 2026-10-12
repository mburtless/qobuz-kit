.PHONY: test run govulncheck

test:
	go test -race ./...

govulncheck:
	go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...

run:
	go run -buildvcs=false ./cmd/qobuz-kit $(ARGS)
