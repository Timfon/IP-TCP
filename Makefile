all:
	go build -o vrouter ./cmd/vrouter/main.go
	go build -o vhost ./cmd/vhost/main.go

clean:
	rm -fv vrouter vhost
