.PHONY: clean test

lambda-extention-client: go.* *.go
	go build -o $@ cmd/lambda-extention-client/main.go

clean:
	rm -rf lambda-extention-client dist/

test:
	go test -v ./...

install:
	go install github.com/fujiwara/lambda-extention-client/cmd/lambda-extention-client

dist:
	goreleaser build --snapshot --rm-dist
