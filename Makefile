BINARY := holyarrs
export CGO_ENABLED := 0

.PHONY: build test vet docker web

build:
	go build -trimpath -ldflags="-s -w" -o bin/$(BINARY) ./cmd/holyarrs

test:
	go test ./...

vet:
	go vet ./...

docker:
	docker build -t holyarrs:dev .

# web rebuilds the app into web/dist, which the program embeds.
web:
	cd web/app && npm ci && npm run build
