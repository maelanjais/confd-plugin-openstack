.PHONY: build clean test run-once run-watch

BINARY    := bin/confd-plugin-openstack
CONFD     := ../confd/bin/confd
CONFDIR   := ./confd

build:
	@echo "Building plugin..."
	@mkdir -p bin
	@go build -buildvcs=false -o $(BINARY) ./cmd/confd-plugin-openstack
	@echo "Done: $(BINARY)"

clean:
	@rm -rf bin/

test:
	@go test -v ./pkg/backends/openstack/...

# One-time run: generate config files from OpenStack metadata and exit
run-once: build
	$(CONFD) plugin \
		--plugin-path "./$(BINARY)" \
		--confdir $(CONFDIR) \
		--onetime

# Watch mode: confd blocks and updates config on every VM metadata change
run-watch: build
	$(CONFD) plugin \
		--plugin-path "./$(BINARY)" \
		--confdir $(CONFDIR) \
		--watch