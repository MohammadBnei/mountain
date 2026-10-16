// AudioWorklet half of stt-capture.js (VENDORED from ukubi-stt web/stt-capture.js WORKLET_SRC, 8c98963).
class Chunker extends AudioWorkletProcessor {
  constructor(options) {
    super();
    this.size = options.processorOptions.chunkSamples;
    this.buf = new Float32Array(this.size);
    this.n = 0;
    this.quanta = 0;
    this.peak = 0;
    this.port.onmessage = (e) => {
      if (e.data === 'flush') {
        // Sent even when n === 0: an empty tail is the close signal, and the
        // server pads it with silence to flush the encoder's own buffer.
        this.port.postMessage({ type: 'tail', samples: this.buf.slice(0, this.n) });
        this.n = 0;
      }
    };
  }
  process(inputs) {
    const ch = inputs[0] && inputs[0][0];
    if (!ch) return true;
    for (let i = 0; i < ch.length; i++) {
      const v = ch[i];
      if (v > this.peak) this.peak = v; else if (-v > this.peak) this.peak = -v;
    }
    // A level reading every 8 quanta (~64ms). Per-quantum would be 125 messages
    // a second to move one float.
    if (++this.quanta >= 8) {
      this.port.postMessage({ type: 'peak', value: this.peak });
      this.quanta = 0; this.peak = 0;
    }
    let i = 0;
    while (i < ch.length) {
      const take = Math.min(ch.length - i, this.size - this.n);
      this.buf.set(ch.subarray(i, i + take), this.n);
      this.n += take; i += take;
      if (this.n === this.size) {
        this.port.postMessage({ type: 'chunk', samples: this.buf.slice(0) });
        this.n = 0;
      }
    }
    return true;
  }
}
registerProcessor('chunker', Chunker);
