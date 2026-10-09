importScripts('wasm_exec.js');

const go = new Go();

// Load the headless worker WASM binary
WebAssembly.instantiateStreaming(fetch('worker.wasm'), go.importObject).then((result) => {
    go.run(result.instance);
}).catch(console.error);

self.onmessage = function(e) {
    if (e.data.id !== undefined) {
        if (self.inkanimWorkerProcess) {
            // inkanimWorkerProcess is registered by Go during InitWorker()
            // It will synchronously process the image and return the quantized palette bytes.
            const resultBuf = self.inkanimWorkerProcess(
                new Uint8Array(e.data.rgbaBuf),
                e.data.width,
                e.data.height,
                e.data.palette,
                e.data.alphaThreshold,
                e.data.dither
            );
            
            self.postMessage({
                id: e.data.id,
                palettedBuf: resultBuf.buffer
            }, [resultBuf.buffer]);
        }
    }
};
