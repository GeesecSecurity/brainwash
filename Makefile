.PHONY: build test gui serve run-gui pkg

build:
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -o dist/brainwash-cli ./cmd/brainwash-cli

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
