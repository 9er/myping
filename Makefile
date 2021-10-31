all: dependencies build

dependencies:
	go get -d ./...

build:
	go build src/myping.go

setcap:
	setcap cap_net_raw+ep myping

clean:
	$(RM) myping
