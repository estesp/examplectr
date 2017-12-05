.PHONY: binary binary2 static static2 clean

# Target to build a dynamically linked binary
binary:
	go build -o examplectr examplectr.go

binary2:
	go build -o examplectr2 examplectr-advanced.go utils.go

# Target to build a statically linked binary
static:
	GO_EXTLINK_ENABLED=0 CGO_ENABLED=0 go build \
	   -ldflags "-w -extldflags -static" \
	   -tags netgo -installsuffix netgo \
	   -o examplectr examplectr.go

static2:
	GO_EXTLINK_ENABLED=0 CGO_ENABLED=0 go build \
	   -ldflags "-w -extldflags -static" \
	   -tags netgo -installsuffix netgo \
	   -o examplectr2 examplectr-advanced.go utils.go

clean:
	rm -f examplectr

