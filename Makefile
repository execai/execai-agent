.PHONY: build run chat test tidy

BIN := bin/execai

build:
	@mkdir -p bin
	go build -o $(BIN) ./cmd/execai

run: build
	$(BIN) $(ARGS)

chat: build
	$(BIN) chat

test:
	go test ./...

tidy:
	go mod tidy
