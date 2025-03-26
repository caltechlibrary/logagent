import {
    analyzeIPAddresses,
    analyzeUserAgents,
    analyzePaths,
    analyzeTimePeriods,
    summarizeAnalysis,
    analyzeLogs,
    parseLogLine,
    endPointAnalyser
  } from './loganalyst.ts'; // Adjust the path as necessary
  
  import { assertEquals } from "@std/assert";
  
  Deno.test("analyzeIPAddresses counts IP addresses correctly", () => {
    const entries = [
      { ip: "192.168.1.1", timestamp: "01/Jan/2023:00:00:01 +0000", request: "/file", agent: "agent1" },
      { ip: "192.168.1.2", timestamp: "01/Jan/2023:00:00:02 +0000", request: "/file", agent: "agent2" },
      { ip: "192.168.1.1", timestamp: "01/Jan/2023:00:00:03 +0000", request: "/file", agent: "agent1" },
    ];
  
    const result = analyzeIPAddresses(entries);
    assertEquals(result, { "192.168.1.1": 2, "192.168.1.2": 1 });
  });
  
  Deno.test("analyzeUserAgents counts user agents correctly", () => {
    const entries = [
      { ip: "192.168.1.1", timestamp: "01/Jan/2023:00:00:01 +0000", request: "/file", agent: "agent1" },
      { ip: "192.168.1.2", timestamp: "01/Jan/2023:00:00:02 +0000", request: "/file", agent: "agent2" },
      { ip: "192.168.1.1", timestamp: "01/Jan/2023:00:00:03 +0000", request: "/file", agent: "agent1" },
    ];
  
    const result = analyzeUserAgents(entries);
    assertEquals(result, { "agent1": 2, "agent2": 1 });
  });
  
  Deno.test("analyzePaths counts paths correctly", () => {
    const entries = [
      { ip: "192.168.1.1", timestamp: "01/Jan/2023:00:00:01 +0000", request: "/file", agent: "agent1" },
      { ip: "192.168.1.2", timestamp: "01/Jan/2023:00:00:02 +0000", request: "/file", agent: "agent2" },
      { ip: "192.168.1.1", timestamp: "01/Jan/2023:00:00:03 +0000", request: "/other", agent: "agent1" },
    ];
  
    const result = analyzePaths(entries);
    assertEquals(result, { "/file": 2, "/other": 1 });
  });
  
  Deno.test("analyzeTimePeriods counts time periods correctly", () => {
    const entries = [
      { ip: "192.168.1.1", timestamp: "01/Jan/2023:00:00:01 +0000", request: "/file", agent: "agent1" },
      { ip: "192.168.1.2", timestamp: "01/Jan/2023:01:00:02 +0000", request: "/file", agent: "agent2" },
      { ip: "192.168.1.1", timestamp: "01/Jan/2023:00:00:03 +0000", request: "/file", agent: "agent1" },
    ];
  
    const result = analyzeTimePeriods(entries);
    assertEquals(result, { "01/Jan/2023:00": 2, "01/Jan/2023:01": 1 });
  });
  
  Deno.test("endPointAnalyser analyzes endpoints correctly", () => {
    const entries = [
      { ip: "192.168.1.1", timestamp: "01/Jan/2023:00:00:01 +0000", request: "GET /file", agent: "agent1" },
      { ip: "192.168.1.2", timestamp: "01/Jan/2023:00:00:02 +0000", request: "POST /file", agent: "agent2" },
      { ip: "192.168.1.1", timestamp: "01/Jan/2023:00:00:03 +0000", request: "GET /other", agent: "agent1" },
    ];
  
    const result = endPointAnalyser(entries);
    assertEquals(result, {
      "/file": { "GET": 1, "POST": 1 },
      "/other": { "GET": 1 },
    });
  });
  
  Deno.test("summarizeAnalysis aggregates all analyses correctly", () => {
    const entries = [
      { ip: "192.168.1.1", timestamp: "01/Jan/2023:00:00:01 +0000", request: "GET /file", agent: "agent1" },
      { ip: "192.168.1.2", timestamp: "01/Jan/2023:01:00:02 +0000", request: "POST /file", agent: "agent2" },
      { ip: "192.168.1.1", timestamp: "01/Jan/2023:00:00:03 +0000", request: "GET /other", agent: "agent1" },
    ];
  
    const result = summarizeAnalysis(entries);
    assertEquals(result, {
      ipCounts: { "192.168.1.1": 2, "192.168.1.2": 1 },
      agentCounts: { "agent1": 2, "agent2": 1 },
      pathCounts: { "/file": 2, "/other": 1 },
      timePeriodCounts: { "01/Jan/2023:00": 2, "01/Jan/2023:01": 1 },
      endpointCounts: {
        "/file": { "GET": 1, "POST": 1 },
        "/other": { "GET": 1 },
      },
    });
  });
  
  Deno.test("parseLogLine parses valid log lines correctly", () => {
    const validLines = [
      '8.210.147.121 - - [25/Mar/2025:00:00:14 +0000] "GET /records/dn9cj-ehd72/files/knee_train_lmdb.zip?download=1 HTTP/1.1" 302 480 "-" "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/64.0.3608.7 Safari/537.36"',
      '66.249.74.136 - - [25/Mar/2025:00:00:20 +0000] "GET /records/rd2f5-dwc23/files/Lev2:metadata.json?download=1 HTTP/1.1" 302 473 "-" "Mozilla/5.0 (Linux; Android 6.0.1; Nexus 5X Build/MMB29P) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/134.0.6998.165 Mobile Safari/537.36 (compatible; GoogleOther)"',
      '47.242.234.100 - - [25/Mar/2025:00:00:25 +0000] "HEAD /records/dn9cj-ehd72/files/knee_train_lmdb.zip?download=1 HTTP/1.1" 302 0 "-" "Mozilla/5.0 (Windows NT 6.1; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/75.0.2514.118 Safari/537.36"',
    ];
  
    const expectedResults = [
      {
        ip: "8.210.147.121",
        timestamp: "25/Mar/2025:00:00:14 +0000",
        request: "GET /records/dn9cj-ehd72/files/knee_train_lmdb.zip?download=1",
        agent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/64.0.3608.7 Safari/537.36",
      },
      {
        ip: "66.249.74.136",
        timestamp: "25/Mar/2025:00:00:20 +0000",
        request: "GET /records/rd2f5-dwc23/files/Lev2:metadata.json?download=1",
        agent: "Mozilla/5.0 (Linux; Android 6.0.1; Nexus 5X Build/MMB29P) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/134.0.6998.165 Mobile Safari/537.36 (compatible; GoogleOther)",
      },
      {
        ip: "47.242.234.100",
        timestamp: "25/Mar/2025:00:00:25 +0000",
        request: "HEAD /records/dn9cj-ehd72/files/knee_train_lmdb.zip?download=1",
        agent: "Mozilla/5.0 (Windows NT 6.1; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/75.0.2514.118 Safari/537.36",
      },
    ];
  
    validLines.forEach((line, index) => {
      const result = parseLogLine(line);
      assertEquals(result, expectedResults[index]);
    });
  });
  
  Deno.test("parseLogLine returns null for invalid log lines", () => {
    const invalidLines = [
      'Invalid log line format',
      '8.210.147.121 - - [25/Mar/2025:00:00:14 +0000] "GET /records/dn9cj-ehd72/files/knee_train_lmdb.zip?download=1 HTTP/1.1" 302 "-"',
    ];
  
    invalidLines.forEach(line => {
      const result = parseLogLine(line);
      assertEquals(result, null);
    });
  });
  
  Deno.test("analyzeLogs processes logs and outputs correctly without regex filter", async () => {
    const mockInput = [
      '192.168.1.1 - - [01/Jan/2023:00:00:01 +0000] "GET /file HTTP/1.1" 200 1024 "-" "agent1"',
      '192.168.1.2 - - [01/Jan/2023:01:00:02 +0000] "GET /file HTTP/1.1" 200 1024 "-" "agent2"',
      '192.168.1.1 - - [01/Jan/2023:00:00:03 +0000] "GET /other HTTP/1.1" 200 1024 "-" "agent1"',
    ];
  
    const mockData = new TextEncoder().encode(mockInput.join("\n") + "\n");
    const mockStream = new ReadableStream({
      start(controller) {
        controller.enqueue(mockData);
        controller.close();
      }
    });
  
    const outputChunks: Uint8Array[] = [];
    const mockOutput = new WritableStream({
      write(chunk) {
        outputChunks.push(chunk);
        return Promise.resolve();
      },
    });
  
    await analyzeLogs(mockStream, mockOutput, false, undefined, ['summary'], 'json');
  
    const output = new TextDecoder().decode(new Uint8Array(outputChunks.flat().reduce((acc, chunk) => {
      const temp = new Uint8Array(acc.length + chunk.length);
      temp.set(acc, 0);
      temp.set(chunk, acc.length);
      return temp;
    }, new Uint8Array())));
  
    const expectedOutput = JSON.stringify({
      ipCounts: { "192.168.1.1": 2, "192.168.1.2": 1 },
      agentCounts: { "agent1": 2, "agent2": 1 },
      pathCounts: { "/file": 2, "/other": 1 },
      timePeriodCounts: { "01/Jan/2023:00": 2, "01/Jan/2023:01": 1 },
      endpointCounts: {
        "/file": { "GET": 2 },
        "/other": { "GET": 1 },
      },
    }, null, 2);
  
    assertEquals(output.trim(), expectedOutput);
  });
  
  Deno.test("analyzeLogs processes logs and outputs correctly with regex filter", async () => {
    const mockInput = [
      '192.168.1.1 - - [01/Jan/2023:00:00:01 +0000] "GET /file HTTP/1.1" 200 1024 "-" "agent1"',
      '192.168.1.2 - - [01/Jan/2023:01:00:02 +0000] "GET /file HTTP/1.1" 200 1024 "-" "agent2"',
      '192.168.1.1 - - [01/Jan/2023:00:00:03 +0000] "GET /other HTTP/1.1" 200 1024 "-" "agent1"',
    ];
  
    const mockData = new TextEncoder().encode(mockInput.join("\n") + "\n");
    const mockStream = new ReadableStream({
      start(controller) {
        controller.enqueue(mockData);
        controller.close();
      }
    });
  
    const outputChunks: Uint8Array[] = [];
    const mockOutput = new WritableStream({
      write(chunk) {
        outputChunks.push(chunk);
        return Promise.resolve();
      },
    });
  
    await analyzeLogs(mockStream, mockOutput, false, /\/file/, ['summary'], 'json');
  
    const output = new TextDecoder().decode(new Uint8Array(outputChunks.flat().reduce((acc, chunk) => {
      const temp = new Uint8Array(acc.length + chunk.length);
      temp.set(acc, 0);
      temp.set(chunk, acc.length);
      return temp;
    }, new Uint8Array())));
  
    const expectedOutput = JSON.stringify({
      ipCounts: { "192.168.1.1": 1, "192.168.1.2": 1 },
      agentCounts: { "agent1": 1, "agent2": 1 },
      pathCounts: { "/file": 2 },
      timePeriodCounts: { "01/Jan/2023:00": 1, "01/Jan/2023:01": 1 },
      endpointCounts: {
        "/file": { "GET": 2 },
      },
    }, null, 2);
  
    assertEquals(output.trim(), expectedOutput);
  });
  