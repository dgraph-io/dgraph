.PHONY: jepsen test list

clean:
	go clean .

jepsen: main.go
	go build -o jepsen main.go

test:
	go test .

list:
	go test -list=.
