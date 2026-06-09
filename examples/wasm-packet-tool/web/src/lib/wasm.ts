// Re-export everything from the generated interface file.
// Interfaces, MeshcoreWasm, RouteTypes and PayloadTypes all live in wasm.gen.ts.
export * from "./wasm.gen";
export type { ErrResult, MeshcoreWasm } from "./wasm.gen";

import type { MeshcoreWasm, ErrResult } from "./wasm.gen";

declare class Go {
  importObject: WebAssembly.Imports;
  run(instance: WebAssembly.Instance): Promise<void>;
}

declare global {
  interface Window {
    meshcore: MeshcoreWasm;
  }
}

// ── runtime helpers ───────────────────────────────────────────────────────────

export function isError(v: object): v is ErrResult {
  return "error" in v;
}

export function fmtTimestamp(unix: number): string {
  return new Date(unix * 1000).toLocaleString();
}

// ── WASM loader ───────────────────────────────────────────────────────────────

let wasmReady: Promise<MeshcoreWasm>;

async function loadWasmExec(): Promise<void> {
  if (typeof (window as unknown as Record<string, unknown>)["Go"] !== "undefined") return;
  await new Promise<void>((resolve, reject) => {
    const s = document.createElement("script");
    s.src = "/wasm_exec.js";
    s.onload = () => resolve();
    s.onerror = () => reject(new Error("Failed to load /wasm_exec.js — run `make wasm` first"));
    document.head.appendChild(s);
  });
}

export function loadWasm(): Promise<MeshcoreWasm> {
  if (wasmReady) return wasmReady;
  wasmReady = (async () => {
    await loadWasmExec();
    const go = new Go();
    let instance: WebAssembly.Instance;
    try {
      const result = await WebAssembly.instantiateStreaming(fetch("/meshcore.wasm"), go.importObject);
      instance = result.instance;
    } catch {
      const buf = await fetch("/meshcore.wasm").then((r) => r.arrayBuffer());
      const result = await WebAssembly.instantiate(buf, go.importObject);
      instance = result.instance;
    }
    go.run(instance);
    await new Promise<void>((resolve) => {
      const id = setInterval(() => {
        if (window.meshcore) { clearInterval(id); resolve(); }
      }, 10);
    });
    return window.meshcore;
  })();
  return wasmReady;
}
