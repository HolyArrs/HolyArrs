BINARY := holyarrs
export CGO_ENABLED := 0

.PHONY: build test vet docker web portable package-test

build:
	go build -trimpath -ldflags="-s -w" -o bin/$(BINARY) ./cmd/holyarrs

test:
	go test -timeout 25m ./... # internal/api alone runs past Go's 10 min default

vet:
	go vet ./...

docker:
	docker build -t holyarrs:dev .

# web rebuilds the app into web/dist, which the program embeds.
web:
	cd web/app && npm ci && npm run build

# REF is a committed source snapshot; OUTPUT must be a new external directory.
portable:
	python scripts/package.py --ref "$(REF)" --version "$(VERSION)" --output "$(OUTPUT)" --web-node-modules "$(WEB_NODE_MODULES)" --module-cache "$(MODULE_CACHE)"

package-test:
	python -m unittest discover -s scripts -p 'test_packaging.py'
