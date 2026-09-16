.PHONY: build test vet link

build:
	go build -o bin/herdr-glab ./cmd/herdr-glab

test:
	go test ./...

vet:
	go vet ./...

link: build
	herdr plugin link $(CURDIR)
