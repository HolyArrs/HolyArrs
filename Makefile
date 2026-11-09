BINARY := holyarrs
export CGO_ENABLED := 0

.PHONY: ui build test vet docker e2e portable package-test

# ui builds the app into app/frontend/dist, which the program embeds.
ui:
	npm --prefix app/frontend ci
	npm --prefix app/frontend run build

build: ui
	go build -trimpath -ldflags="-s -w" -o bin/$(BINARY) ./app/backend/cmd/holyarrs

test:
	go test -timeout 25m ./... # the api package alone runs past Go's 10 min default

vet:
	go vet ./...

docker:
	docker build -f deploy/Dockerfile -t holyarrs:dev .

e2e:
	npm --prefix app/frontend run e2e

# REF is a committed source snapshot; OUTPUT must be a new external directory.
WEB_NODE_MODULES ?= app/frontend/node_modules
MODULE_CACHE ?= $(shell go env GOMODCACHE)
portable:
	python deploy/release/package.py --ref "$(REF)" --version "$(VERSION)" --output "$(OUTPUT)" --web-node-modules "$(WEB_NODE_MODULES)" --module-cache "$(MODULE_CACHE)"

package-test:
	python -m unittest discover -s deploy/release/tests -p 'test_packaging.py'
