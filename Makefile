.PHONY: clean test

test:
	go test -race -v ./...

clean:
	rm -rf dist/
