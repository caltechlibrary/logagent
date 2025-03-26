export async function readLines(inputStream: ReadableStream<Uint8Array> = Deno.stdin.readable): Promise<string[]> {
    const lines: string[] = [];
    const decoder = new TextDecoder();
    const reader = inputStream.getReader();
    let done = false;
  
    while (!done) {
      const { value, done: readerDone } = await reader.read();
      done = readerDone;
      if (value) {
        const text = decoder.decode(value, { stream: true });
        lines.push(...text.split('\n').filter(line => line.trim() !== ''));
      }
    }
  
    return lines;
  }
  
  export async function processLines(
    inputStream: ReadableStream<Uint8Array>,
    processLine: (line: string) => void
  ): Promise<void> {
    const decoder = new TextDecoder();
    const reader = inputStream.getReader();
    let done = false;
  
    while (!done) {
      const { value, done: readerDone } = await reader.read();
      done = readerDone;
      if (value) {
        const text = decoder.decode(value, { stream: true });
        text.split('\n').forEach(line => {
          if (line.trim() !== '') {
            processLine(line);
          }
        });
      }
    }
  }
  