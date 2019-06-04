.PHONY: test, vet, build

test:
	go test ./...

vet:
	go vet ./...

build: test vet
	docker build . -t f3api
