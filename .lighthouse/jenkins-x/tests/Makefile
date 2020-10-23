NAME := jx3-pipeline-catalog
GO := GO111MODULE=on go

BUILDFLAGS :=

TESTFLAGS ?= -v

TESTFLAGS += -timeout 2h

ifdef DEBUG
BUILDFLAGS += -gcflags "all=-N -l" $(BUILDFLAGS)
endif


.PHONY: fmt
fmt:
	@FORMATTED=`$(GO) fmt $(PACKAGE_DIRS)`
	@([[ ! -z "$(FORMATTED)" ]] && printf "Fixed unformatted files:\n$(FORMATTED)") || true

.PHONY: test
build:
	$(GO) build $(BUILDFLAGS) ./test/...

.PHONY: test
test:
	$(GO) test $(TESTFLAGS) ./...

