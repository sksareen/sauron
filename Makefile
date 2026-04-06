VERSION ?= 0.1.0

build:
	go build -ldflags "-X main.version=$(VERSION)" -o sauron ./cmd/sauron

install-local: build
	sudo cp sauron /usr/local/bin/sauron
	sauron install
	sauron start

release:
	mkdir -p dist
	GOOS=darwin GOARCH=arm64 go build -ldflags "-X main.version=$(VERSION)" -o dist/sauron-darwin-arm64 ./cmd/sauron
	GOOS=darwin GOARCH=amd64 go build -ldflags "-X main.version=$(VERSION)" -o dist/sauron-darwin-amd64 ./cmd/sauron

clean:
	rm -f sauron
	rm -rf dist/

.PHONY: build install-local release clean
