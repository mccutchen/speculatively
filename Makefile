test: *.go
	go test .

testcover: *.go
	go test -cover .

testrace: *.go
	go test -race .
