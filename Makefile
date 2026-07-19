.PHONY: build test fmt vet bundle-slim bundle-full smoke-papers

build:
	go build ./packages/cli/cmd/latexmk
	go build ./packages/server/cmd/server

fmt:
	gofmt -w packages/cli packages/server

vet:
	go vet ./packages/cli/... ./packages/server/...

test:
	go test ./packages/cli/... ./packages/server/...

smoke-papers:
	bash scripts/smoke-compose-papers.sh

bundle-slim:
	corepack pnpm bundle:slim

bundle-full:
	corepack pnpm bundle:full
