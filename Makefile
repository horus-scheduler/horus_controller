
TARGET_CC=horus-central-ctrl
TARGET_LEAF=horus-leaf-ctrl
TARGET_SPINE=horus-spine-ctrl
PKG_NAME=github.com/khaledmdiab/horus_controller
EXEC_PKGS = ctrl_central ctrl_leaf ctrl_spine
CORE_PKGS = $(sort $(dir $(wildcard core/*/)))
FMT := $(shell go fmt)
TIDY := $(shell go mod tidy)

all: clean tidy fmt proto
	@echo ""
	@echo "- Building..."
	mkdir -p bin
	go build -o bin/${TARGET_CC} -race -v central_ctrl.go
	go build -o bin/${TARGET_LEAF} -race -v leaf_ctrl.go
	go build -o bin/${TARGET_SPINE} -race -v spine_ctrl.go

tidy:
	@echo "-- Output of go mod tidy:"
	@echo $(TIDY)
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
	go build -o bin/dummy-client -race -v dummy_client.go

proto:
	@echo "-- Generating Protocol Buffers:"
	protoc  --go_out=plugins=grpc:. ./protobuf/message.proto
	@echo ""
	@echo "---------------------------------------------"

clean_proto:
	rm -rf ./protobuf/*.pb.go

clean: clean_proto
	rm -rf bin/${TARGET_CC}
	rm -rf bin/${TARGET_LEAF}
	rm -rf bin/${TARGET_SPINE}
