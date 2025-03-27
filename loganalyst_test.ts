import {
  analyzeIPAddresses,
  analyzeUserAgents,
  analyzePaths,
  analyzeTimePeriods,
  analyzeStatusCodes,
  summarizeAnalysis,
  analyzeLogs,
  parseLogLine,
  endPointAnalyser
} from './loganalyst.ts'; // Adjust the path as necessary

import { assertEquals } from "@std/assert";

Deno.test("analyzeIPAddresses counts IP addresses correctly", () => {
  const entries = [
    { ip: "192.168.1.1", timestamp: "01/Jan/2023:00:00:01 +0000", reqMethod: "GET", reqPath: "/file", status: "200", agent: "agent1" },
    { ip: "192.168.1.2", timestamp: "01/Jan/2023:00:00:02 +0000", reqMethod: "GET", reqPath: "/file", status: "200", agent: "agent2" },
    { ip: "192.168.1.1", timestamp: "01/Jan/2023:00:00:03 +0000", reqMethod: "GET", reqPath: "/file", status: "200", agent: "agent1" },
  ];

  const result = analyzeIPAddresses(entries);
  assertEquals(result, { "192.168.1.1": 2, "192.168.1.2": 1 });
});

Deno.test("analyzeUserAgents counts user agents correctly", () => {
  const entries = [
    { ip: "192.168.1.1", timestamp: "01/Jan/2023:00:00:01 +0000", reqMethod: "GET", reqPath: "/file", status: "200", agent: "agent1" },
    { ip: "192.168.1.2", timestamp: "01/Jan/2023:00:00:02 +0000", reqMethod: "GET", reqPath: "/file", status: "200", agent: "agent2" },
    { ip: "192.168.1.1", timestamp: "01/Jan/2023:00:00:03 +0000", reqMethod: "GET", reqPath: "/file", status: "200", agent: "agent1" },
  ];

  const result = analyzeUserAgents(entries);
  assertEquals(result, { "agent1": 2, "agent2": 1 });
});

Deno.test("analyzePaths counts paths correctly", () => {
  const entries = [
    { ip: "192.168.1.1", timestamp: "01/Jan/2023:00:00:01 +0000", reqMethod: "GET", reqPath: "/file", status: "200", agent: "agent1" },
    { ip: "192.168.1.2", timestamp: "01/Jan/2023:00:00:02 +0000", reqMethod: "GET", reqPath: "/file", status: "200", agent: "agent2" },
    { ip: "192.168.1.1", timestamp: "01/Jan/2023:00:00:03 +0000", reqMethod: "GET", reqPath: "/other", status: "200", agent: "agent1" },
  ];

  const result = analyzePaths(entries);
  assertEquals(result, { "/file": 2, "/other": 1 });
});

Deno.test("analyzeTimePeriods counts time periods correctly", () => {
  const entries = [
    { ip: "192.168.1.1", timestamp: "01/Jan/2023:00:00:01 +0000", reqMethod: "GET", reqPath: "/file", status: "200", agent: "agent1" },
    { ip: "192.168.1.2", timestamp: "01/Jan/2023:01:00:02 +0000", reqMethod: "GET", reqPath: "/file", status: "200", agent: "agent2" },
    { ip: "192.168.1.1", timestamp: "01/Jan/2023:00:00:03 +0000", reqMethod: "GET", reqPath: "/file", status: "200", agent: "agent1" },
  ];

  const result = analyzeTimePeriods(entries);
  assertEquals(result, { "01/Jan/2023:00": 2, "01/Jan/2023:01": 1 });
});

Deno.test("analyzeStatusCodes counts status codes correctly", () => {
  const entries = [
    { ip: "192.168.1.1", timestamp: "01/Jan/2023:00:00:01 +0000", reqMethod: "GET", reqPath: "/file", status: "200", agent: "agent1" },
    { ip: "192.168.1.2", timestamp: "01/Jan/2023:00:00:02 +0000", reqMethod: "GET", reqPath: "/file", status: "404", agent: "agent2" },
    { ip: "192.168.1.1", timestamp: "01/Jan/2023:00:00:03 +0000", reqMethod: "GET", reqPath: "/other", status: "500", agent: "agent1" },
  ];

  const result = analyzeStatusCodes(entries);
  assertEquals(result, { "200": 1, "404": 1, "500": 1 });
});

Deno.test("endPointAnalyser analyzes endpoints correctly", () => {
  const entries = [
    { ip: "192.168.1.1", timestamp: "01/Jan/2023:00:00:01 +0000", reqMethod: "GET", reqPath: "/file", status: "200", agent: "agent1" },
    { ip: "192.168.1.2", timestamp: "01/Jan/2023:00:00:02 +0000", reqMethod: "POST", reqPath: "/file", status: "200", agent: "agent2" },
    { ip: "192.168.1.1", timestamp: "01/Jan/2023:00:00:03 +0000", reqMethod: "GET", reqPath: "/other", status: "200", agent: "agent1" },
  ];

  const result = endPointAnalyser(entries);
  assertEquals(result, {
    "/file": { "GET": 1, "POST": 1 },
    "/other": { "GET": 1 },
  });
});

Deno.test("summarizeAnalysis aggregates all analyses correctly", () => {
  const entries = [
    { ip: "192.168.1.1", timestamp: "01/Jan/2023:00:00:01 +0000", reqMethod: "GET", reqPath: "/file", status: "200", agent: "agent1" },
    { ip: "192.168.1.2", timestamp: "01/Jan/2023:01:00:02 +0000", reqMethod: "POST", reqPath: "/file", status: "404", agent: "agent2" },
    { ip: "192.168.1.1", timestamp: "01/Jan/2023:00:00:03 +0000", reqMethod: "GET", reqPath: "/other", status: "500", agent: "agent1" },
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
    statusCounts: { "200": 1, "404": 1, "500": 1 },
  });
});

Deno.test("parseLogLine parses valid log lines correctly", () => {
  const validLines = [
    '20.43.120.29 - - [25/Mar/2025:15:50:00 +0000] "GET /cgi/export/eprint/16341/RDFXML/caltechthesis-eprint-16341.rdf HTTP/1.1" 500 624',
    '20.43.120.29 - - [25/Mar/2025:15:56:07 +0000] "GET /cgi/export/eprint/8736/RDFXML/caltechthesis-eprint-8736.rdf HTTP/1.1" 500 624',
    '20.43.120.29 - - [25/Mar/2025:17:01:55 +0000] "GET /cgi/export/eprint/14094/RDFXML/caltechthesis-eprint-14094.rdf HTTP/1.1" 500 624',
  ];

  const expectedResults = [
    {
      ip: "20.43.120.29",
      timestamp: "25/Mar/2025:15:50:00 +0000",
      reqMethod: "GET",
      reqPath: "/cgi/export/eprint/16341/RDFXML/caltechthesis-eprint-16341.rdf",
      status: "500",
      agent: "missing_in_log",
    },
    {
      ip: "20.43.120.29",
      timestamp: "25/Mar/2025:15:56:07 +0000",
      reqMethod: "GET",
      reqPath: "/cgi/export/eprint/8736/RDFXML/caltechthesis-eprint-8736.rdf",
      status: "500",
      agent: "missing_in_log",
    },
    {
      ip: "20.43.120.29",
      timestamp: "25/Mar/2025:17:01:55 +0000",
      reqMethod: "GET",
      reqPath: "/cgi/export/eprint/14094/RDFXML/caltechthesis-eprint-14094.rdf",
      status: "500",
      agent: "missing_in_log",
    },
  ];

  validLines.forEach((line, index) => {
    const result = parseLogLine(line, false);
    assertEquals(result, expectedResults[index]);
  });
});

Deno.test("parseLogLine returns null for invalid log lines", () => {
  const invalidLines = [
    'Invalid log line format',
    '20.43.120.29 - - [25/Mar/2025:15:50:00 +0000] "GET /cgi/export/eprint/16341/RDFXML/caltechthesis-eprint-16341.rdf HTTP/1.1" 500',
  ];

  const expectedResults = [
    true,
    false
  ]

  for (let i = 0; i < invalidLines.length; i++ ) {
    const result = (parseLogLine(invalidLines[i], false) === null);
    assertEquals(result, expectedResults[i], `expected null, got ${JSON.stringify(result)}`);
  }
});

Deno.test("analyzeLogs processes logs and outputs correctly without regex filter", async () => {
  const mockInput = [
    '192.168.1.1 - - [01/Jan/2023:00:00:01 +0000] "GET /file HTTP/1.1" 200 1024 "agent1"',
    '192.168.1.2 - - [01/Jan/2023:01:00:02 +0000] "GET /file HTTP/1.1" 200 1024 "agent2"',
    '192.168.1.1 - - [01/Jan/2023:00:00:03 +0000] "GET /other HTTP/1.1" 200 1024 "agent1"',
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
    statusCounts: { "200": 3 },
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
    statusCounts: { "200": 2 },
    timePeriodCounts: { "01/Jan/2023:00": 1, "01/Jan/2023:01": 1 },
    endpointCounts: {
      "/file": { "GET": 2 },
    },
  }, null, 2);

  assertEquals(output.trim(), expectedOutput);
});
