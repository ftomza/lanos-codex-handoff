.PHONY: test test-web vet lint build fmt build-obdprobe build-dashboard

test:
	go test ./...

test-web:
	node --test web/model.test.mjs web/feed.test.mjs web/fuel.test.mjs

vet:
	go vet ./...

lint: vet
	@test -z "$$(gofmt -l cmd internal web)" || (gofmt -l cmd internal web; exit 1)

build:
	go build ./...

build-obdprobe:
	go build -o bin/obdprobe ./cmd/obdprobe
	go build -tags=nativeble -o bin/obdprobe-ble ./cmd/obdprobe-ble

build-dashboard:
	go build -o bin/dashboard ./cmd/dashboard
	go build -tags=nativeble -o bin/obdprobe-ble ./cmd/obdprobe-ble

fmt:
	gofmt -w cmd internal web
