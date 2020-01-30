.PHONY: jepsen test-all list

clean:
	go clean .

jepsen: main.go
	go build -o jepsen main.go

test-all:
	go test -timeout=12h .

list:
	go test -list=.
