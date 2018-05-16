test: *.go
	go test -race .

testcover: *.go
	go test -cover .

lint: *.go
	gofmt -l -e -d .
	go vet .
	golint -set_exit_status .

testci: lint test
