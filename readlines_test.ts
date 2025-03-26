import { processLines, readLines } from './readLines.ts'; // Adjust the path as necessary
import { assertEquals } from "@std/assert";

Deno.test("readLines reads all lines from standard input", async () => {
  // Mock input data
  const mockInput = ["line1", "line2", "line3"];
  const mockData = new TextEncoder().encode(mockInput.join("\n") + "\n");

  // Create a mock ReadableStream
  const mockStream = new ReadableStream({
    start(controller) {
      controller.enqueue(mockData);
      controller.close();
    }
  });

  // Call the readLines function with the mock stream
  const lines = await readLines(mockStream);

  // Assertions
  assertEquals(lines.length, mockInput.length);
  assertEquals(lines, mockInput);
});

Deno.test("processLines processes each line with the given function", async () => {
  // Mock input data
  const mockInput = ["line1", "line2", "line3"];
  const mockData = new TextEncoder().encode(mockInput.join("\n") + "\n");

  // Create a mock ReadableStream
  const mockStream = new ReadableStream({
    start(controller) {
      controller.enqueue(mockData);
      controller.close();
    }
  });

  // Track processed lines
  const processedLines: string[] = [];

  // Call the processLines function with the mock stream
  await processLines(mockStream, (line) => {
    processedLines.push(line);
  });

  // Assertions
  assertEquals(processedLines.length, mockInput.length);
  assertEquals(processedLines, mockInput);
});
