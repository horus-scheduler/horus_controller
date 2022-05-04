
TARGET_CC=horus-central-ctrl
TARGET_LEAF=horus-leaf-ctrl
TARGET_SPINE=horus-spine-ctrl
PKG_NAME=github.com/khaledmdiab/horus_controller
EXEC_PKGS = ctrl_central ctrl_leaf ctrl_spine
CORE_PKGS = $(sort $(dir $(wildcard core/*/)))
FMT := $(shell go fmt)
MODULES := $(shell go mod tidy)

all: clean modules fmt proto
	@echo ""
	@echo "- Building..."
	mkdir -p bin
	go build -o bin/${TARGET_CC} -race -v cmd/centralized/main.go
	go build -o bin/${TARGET_LEAF} -race -v cmd/leaf/main.go
	go build -o bin/${TARGET_SPINE} -race -v cmd/spine/main.go

modules:
	@echo "-- Output of go mod modules:"
	@echo $(MODULES)
	@echo ""
	@echo "---------------------------------------------"

fmt:
	@echo "-- Output of go fmt:"
	@for pkg in ${EXEC_PKGS} ; do \
		go fmt ${PKG_NAME}/$$pkg ; \
    	done
	@for pkg in ${CORE_PKGS} ; do \
                go fmt ${PKG_NAME}/$$pkg ; \
        done
	@echo ""
	@echo "---------------------------------------------"

dummy_client:
	go build -o bin/dummy-client -race -v cmd/dummy/main.go

proto:
	@echo "-- Generating Protocol Buffers:"
	protoc -I=./protobuf --go_out=./protobuf --go-grpc_out=./protobuf ./protobuf/message.proto
	@echo ""
	@echo "---------------------------------------------"

clean_proto:
	rm -rf ./protobuf/*.pb.go

clean: clean_proto
	rm -rf bin/${TARGET_CC}
	rm -rf bin/${TARGET_LEAF}
	rm -rf bin/${TARGET_SPINE}
