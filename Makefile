.PHONY: test test-web vet lint build fmt

test:
	go test ./...

test-web:
	node --test web/model.test.mjs web/feed.test.mjs

vet:
	go vet ./...

lint: vet
	@test -z "$$(gofmt -l cmd internal web)" || (gofmt -l cmd internal web; exit 1)

build:
	go build ./...

fmt:
	gofmt -w cmd internal web
