run:
	go run .

build:
	go build -o sb .

install:
	go build -o sb . && mv sb ~/go/bin/sb
