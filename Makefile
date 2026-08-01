# re2-wasm Makefile
#
# Targets:
#   make wasm      - Build web/re2.wasm + copy wasm_exec.js (default)
#   make serve     - Serve web/ on http://localhost:8080 for local testing
#   make test      - Run host unit tests
#   make vet       - Run go vet on both host and js/wasm targets
#   make clean     - Remove build artifacts
#   make check     - vet + test + wasm (the gate CI runs)
#   make fmt       - Format all Go source
#   make tidy      - Run go mod tidy
#
# The Makefile is intentionally portable: only POSIX sh, Go, and Python 3 are
# required. There is no C/C++ toolchain dependency in Phase 1; CMakeLists.txt
# exists for the Phase 3 native-asset-converter pipeline.

# ---- Tool discovery -------------------------------------------------------

GO          ?= go
GOFMT       ?= $(GO) fmt
GOVET       ?= $(GO) vet
PYTHON      ?= python3
HTTP_SERVER ?= $(PYTHON) -m http.server

# ---- Paths ----------------------------------------------------------------

ROOT      := $(abspath $(dir $(lastword $(MAKEFILE_LIST))))
WEB_DIR   := $(ROOT)/web
WASM_OUT  := $(WEB_DIR)/re2.wasm
WASM_EXEC_SRC := $(shell $(GO) env GOROOT)/lib/wasm/wasm_exec.js
WASM_EXEC_DST := $(WEB_DIR)/wasm_exec.js

# ---- Go flags -------------------------------------------------------------

# -trimpath keeps the binary reproducible across build machines; -ldflags
# "-s -w" strips DWARF + symbol table for a smaller wasm payload.
GOFLAGS   ?= -trimpath
LDFLAGS   ?= -s -w
WASM_GO   := GOOS=js GOARCH=wasm $(GO) build $(GOFLAGS) -ldflags="$(LDFLAGS)"

# ---- Phony targets --------------------------------------------------------

.PHONY: all wasm serve test vet wasm-vet clean check fmt tidy help \
        copy-wasm-exec

all: wasm

help:
	@echo "re2-wasm targets:"
	@echo "  make wasm    - Build web/re2.wasm and copy wasm_exec.js"
	@echo "  make serve   - Serve web/ on http://localhost:8080"
	@echo "  make test    - Run host unit tests"
	@echo "  make vet     - Run go vet on host AND js/wasm"
	@echo "  make check   - vet + test + wasm (CI gate)"
	@echo "  make fmt     - Format Go source"
	@echo "  make tidy    - go mod tidy"
	@echo "  make clean   - Remove build artifacts"

# ---- Build ----------------------------------------------------------------

wasm: $(WASM_OUT) $(WASM_EXEC_DST)

$(WASM_OUT): $(shell find $(ROOT)/cmd $(ROOT)/engine $(ROOT)/renderer \
                       $(ROOT)/audio $(ROOT)/input $(ROOT)/assets \
                       $(ROOT)/filesystem $(ROOT)/saves $(ROOT)/ui \
                       $(ROOT)/wasm -name '*.go') \
              $(ROOT)/go.mod
	@mkdir -p $(WEB_DIR)
	@echo ">> building web/re2.wasm"
	@$(WASM_GO) -o $(WASM_OUT) ./cmd/re2-wasm
	@ls -lh $(WASM_OUT) | awk '{print "   ", $$5, $$9}'

$(WASM_EXEC_DST): $(WASM_EXEC_SRC)
	@mkdir -p $(WEB_DIR)
	@echo ">> copying wasm_exec.js"
	@cp $(WASM_EXEC_SRC) $(WASM_EXEC_DST)

# ---- Tests ----------------------------------------------------------------

test:
	@echo ">> running host unit tests"
	@$(GO) test -count=1 ./...

vet: wasm-vet
	@echo ">> vet (host)"
	@$(GO) vet ./...

wasm-vet:
	@echo ">> vet (js/wasm)"
	@GOOS=js GOARCH=wasm $(GO) vet ./...

# ---- Dev server -----------------------------------------------------------

serve: wasm
	@echo ">> serving $(WEB_DIR) on http://localhost:8080"
	@cd $(WEB_DIR) && $(HTTP_SERVER) 8080

# ---- Maintenance ----------------------------------------------------------

fmt:
	@$(GOFMT) ./...

tidy:
	@$(GO) mod tidy

clean:
	@rm -f $(WASM_OUT) $(WASM_EXEC_DST)
	@find $(ROOT) -name '*.test' -delete
	@find $(ROOT) -name '*.out'   -delete

# ---- CI gate --------------------------------------------------------------

check: fmt vet test wasm
	@echo ">> all gates passed"
