all: tmpl/bindata.go public/bindata.go
	go build

public/bindata.go: $(shell find public)
	mkdir -p public
	go-bindata -o public/bindata.go -pkg public public/...

tmpl/bindata.go: $(shell find templates)
	mkdir -p tmpl
	go-bindata -o tmpl/bindata.go -pkg tmpl templates/...
