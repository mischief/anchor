P=anchor
SRC=github.com
USER?=mischief

all:	docker

docker:	bin/$(P)
	docker build -t "$(USER)/$(P):latest" .

bin/$(P):	main.go
	docker run --rm -v ${PWD}:/go/src/$(SITE)/$(USER)/$(P) golang:1.4 /bin/bash -c "go list -f '{{range .Imports}}{{printf \"%s\n\" .}}{{end}}' $(SITE)/$(USER)/$(P) | xargs go get -d; CGO_ENABLED=0 go build -v -installsuffix cgo -o /go/src/$(SITE)/$(USER)/$(P)/bin/$(P) $(SITE)/$(USER)/$(P)"

clean:
	docker run --rm -v ${PWD}:/opt busybox rm /opt/bin/$(P)
	docker rmi "$(USER)/$(P):latest"

