build:
	go build -o sb
cp:
	cp sb ~/.local/bin/

install: build cp