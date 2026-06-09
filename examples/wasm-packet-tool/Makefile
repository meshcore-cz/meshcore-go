.PHONY: generate wasm dev build clean

WASM_OUT    := web/public/meshcore.wasm
WASM_EXEC   := web/public/wasm_exec.js
TS_GEN      := web/src/lib/wasm.gen.ts

## generate: regenerate TypeScript interfaces from meshpkt.Ops
generate:
	go run ./cmd/gen-ts -out $(TS_GEN)
	@echo "Generated $(TS_GEN)"

## wasm: generate TS interfaces, compile Go → WebAssembly, copy wasm_exec.js
wasm: generate
	GOOS=js GOARCH=wasm go build -o $(WASM_OUT) ./wasm
	cp "$$(go env GOROOT)/lib/wasm/wasm_exec.js" $(WASM_EXEC)
	@echo "Built $(WASM_OUT) ($$(du -sh $(WASM_OUT) | cut -f1))"

## dev: build WASM then start Vite dev server
dev: wasm
	cd web && npm install && npm run dev

## build: build WASM then produce a static Vite bundle
build: wasm
	cd web && npm install && npm run build

## clean: remove build artifacts
clean:
	rm -f $(WASM_OUT) $(WASM_EXEC)
	rm -rf web/dist web/node_modules

## help: show this message
help:
	@grep -E '^##' Makefile | sed 's/## //'
