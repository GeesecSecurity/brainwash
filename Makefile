.PHONY: build test gui serve run-gui pkg cli

build:
	bash -c 'source scripts/version.sh && GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags="$(brainwash_ldflags)" -o dist/brainwash-cli ./cmd/brainwash-cli'

test:
	go test ./...

serve: build
	./dist/brainwash-cli serve --addr 127.0.0.1:7420

gui:
	bash scripts/package-gui.sh

run-gui: gui
	open dist/brainwash.app

pkg:
	bash scripts/package-pkg.sh

cli:
	bash scripts/package-cli.sh
