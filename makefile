proto:
	protoc --go_out=. --go-grpc_out=. ./pkg/pb/*.proto

server:
	go run cmd/main.go

service:
	sudo git pull origin
	go build -o user-svc cmd/main.go
	nohup ./user-svc &
	pgrep -l user-svc