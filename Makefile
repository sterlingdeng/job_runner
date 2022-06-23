buildtest:
	GOOS=linux GOARCH=amd64 go build -o ./bin/utility/cmd ./cmd/utility_process
	GOOS=linux GOARCH=amd64 go build -o ./bin/server ./cmd/server
	GOOS=linux GOARCH=amd64 go build -o ./bin/client ./cmd/client

buildrace:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=1 go build -race -o ./bin/testing/testcmd ./cmd/testing

generate:
	find . -type f -name "*.proto" -exec protoc --go_out="paths=source_relative,plugins=grpc:." "{}" ";"
