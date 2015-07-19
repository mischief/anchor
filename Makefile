P=anchor
SRC=github.com
USER?=mischief

all:	docker

docker:	bin/$(P)
	docker build -t "$(USER)/$(P):latest" .

bin/$(P):	main.go
	docker run --rm -v ${PWD}:/go/src/$(SRC)/$(USER)/$(P) golang:1.4 /bin/bash -c "go list -f '{{range .Imports}}{{printf \"%s\n\" .}}{{end}}' $(SRC)/$(USER)/$(P) | xargs go get -d; CGO_ENABLED=0 go build -v -installsuffix cgo -o /go/src/$(SRC)/$(USER)/$(P)/bin/$(P) $(SRC)/$(USER)/$(P)"

clean:
	docker run --rm -v ${PWD}:/opt busybox rm /opt/bin/$(P)
	docker rmi "$(USER)/$(P):latest"

