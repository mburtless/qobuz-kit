.PHONY: test run

test:
	go test ./...

run:
	go run -buildvcs=false ./cmd/qobuz-kit $(ARGS)
