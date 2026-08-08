SERVICE_NAME=companion-service
API_PROTO_PATH=api/companion/v1/companion.proto
API_PROTO_DIR=api/companion/v1
SERVICE_DISPLAY_NAME=Companion Service
HTTP_PORT=8131
GRPC_PORT=9131
CONF_PROTO_PATH=internal/conf/conf.proto
RUN_MODE=debug
DEVOPS_TOOLS_DIR := $(shell cd .. && pwd)/devops-tools
include $(DEVOPS_TOOLS_DIR)/Makefile.common
