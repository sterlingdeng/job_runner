buildtest:
	GOOS=linux GOARCH=amd64  go build -o ./bin/testing/testcmd ./cmd/testing
	GOOS=linux GOARCH=amd64 go build -o ./bin/utility/cmd ./cmd/utility_process

buildrace:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=1 go build -race -o ./bin/testing/testcmd ./cmd/testing
