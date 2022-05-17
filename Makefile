
TARGET_CC=horus-central-ctrl
TARGET_SW=horus-switch-ctrl
PKG_NAME=github.com/khaledmdiab/horus_controller
EXEC_PKGS = centralized switch
CORE_PKGS = $(sort $(dir $(wildcard core/*/)))
FMT := $(shell go fmt)
MODULES := $(shell go mod tidy)

all: clean modules fmt proto build

build:
	@echo ""
	@echo "- Building..."
	mkdir -p bin
	go build -o bin/${TARGET_CC} -race -v cmd/centralized/main.go
	go build -o bin/${TARGET_SW} -race -v cmd/switch/main.go

modules:
	@echo "-- Output of go mod modules:"
	@echo $(MODULES)
	@echo ""
	@echo "---------------------------------------------"

fmt:
	@echo "-- Output of go fmt:"
	@for pkg in ${EXEC_PKGS} ; do \
		go fmt ${PKG_NAME}/cmd/$$pkg ; \
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
	protoc -I=./protobuf --go_out=./ --go-grpc_out=./ ./protobuf/message.proto
	@echo ""
	@echo "---------------------------------------------"

clean_proto:
	rm -rf ./protobuf/*.pb.go

clean: clean_proto
	rm -rf bin/${TARGET_CC}
	rm -rf bin/${TARGET_SW}
