BINARY_NAME=omimporter

build:
	rm -f ./bin/${BINARY_NAME}
	go build -v -o ./bin/${BINARY_NAME} ./cmd/omimporter/main.go
	cp ./configs/config.json ./bin/

run: build
	./bin/${BINARY_NAME}