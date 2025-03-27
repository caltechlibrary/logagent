import { parseArgs } from "@std/cli/parse-args";
import * as yaml from "@std/yaml";
import { processLines } from './readLines.ts';
import { tokenize, tokenizeUnquote } from './logtok.ts';
import { licenseText, releaseDate, releaseHash, version } from "./version.ts";
import { fmtHelp, helpTextLogAnalyst as helpText } from "./helptext.ts";


interface LogEntry {
  ip: string; // IP address of requestor
  timestamp: string; // Timestamp of entry
  reqMethod: string; // HTTP request method
  reqPath: string; // HTTP request path
  status: string; // New attribute for HTTP status
  agent: string;
}

interface AnalysisResult {
  ipCounts?: Record<string, number>;
  agentCounts?: Record<string, number>;
  pathCounts?: Record<string, number>;
  statusCounts?: Record<string, number>;
  endpointCounts?: Record<string, Record<string, number>>;
  timePeriodCounts?: Record<string, number>;
}

export function analyzeIPAddresses(entries: LogEntry[]): Record<string, number> {
  const ipCounts: Record<string, number> = {};
  entries.forEach(entry => {
    ipCounts[entry.ip] = (ipCounts[entry.ip] || 0) + 1;
  });
  return ipCounts;
}

export function analyzeUserAgents(entries: LogEntry[]): Record<string, number> {
  const agentCounts: Record<string, number> = {};
  entries.forEach(entry => {
    agentCounts[entry.agent] = (agentCounts[entry.agent] || 0) + 1;
  });
  return agentCounts;
}

export function analyzePaths(entries: LogEntry[]): Record<string, number> {
  const pathCounts: Record<string, number> = {};
  entries.forEach(entry => {
    pathCounts[entry.reqPath] = (pathCounts[entry.reqPath] || 0) + 1;
  });
  return pathCounts;
}

export function analyzeTimePeriods(entries: LogEntry[]): Record<string, number> {
  const timePeriodCounts: Record<string, number> = {};
  entries.forEach(entry => {
    const period = entry.timestamp.slice(0, 14); // Truncate to DD/MMM/YYYY:HH
    timePeriodCounts[period] = (timePeriodCounts[period] || 0) + 1;
  });
  return timePeriodCounts;
}

export function analyzeStatusCodes(entries: LogEntry[]): Record<string, number> {
  const statusCounts: Record<string, number> = {};
  entries.forEach(entry => {
    statusCounts[entry.status] = (statusCounts[entry.status] || 0) + 1;
  });
  return statusCounts;
}

export function endPointAnalyser(entries: LogEntry[]): Record<string, Record<string, number>> {
  const endpointCounts: Record<string, Record<string, number>> = {};
  entries.forEach(entry => {
    if (!endpointCounts[entry.reqPath]) {
      endpointCounts[entry.reqPath] = {};
    }
    endpointCounts[entry.reqPath][entry.reqMethod] = (endpointCounts[entry.reqPath][entry.reqMethod] || 0) + 1;
  });
  return endpointCounts;
}

export function summarizeAnalysis(entries: LogEntry[]): AnalysisResult {
  return {
    ipCounts: analyzeIPAddresses(entries),
    agentCounts: analyzeUserAgents(entries),
    pathCounts: analyzePaths(entries),
    statusCounts: analyzeStatusCodes(entries),
    timePeriodCounts: analyzeTimePeriods(entries),
    endpointCounts: endPointAnalyser(entries),
  };
}

// parseLogLine tokenizes the log entry then does a best guess of the entry structure
// base on logs used by CaltechLibrary for NginX and Apache2.
export function parseLogLine(line: string, verbose: boolean): LogEntry | null {
  const parts: string[] = tokenize(line, verbose);
  let currentToken = '';
  let inQuotes = false;
  let inBrackets = false;

  if (parts.length < 5) {
    console.error(`invalid log line "${line}" -> tokenized as ${JSON.stringify(parts)}`);
    return null;
  }
  const ip = parts[0]; // Get IP address making requst
  const timestamp = parts[3].slice(1, -1); // Get the timestamp, strip the square backets
  const request = parts[4]; // Get the full quoted request
  const reqParts = tokenizeUnquote(parts[4], verbose); // raw request string holding reqMethod, path and protocol
  const reqMethod = parts.length > 0 ? reqParts[0] : 'missing_in_log'; // Extract reqMethod
  const reqPath = parts.length > 1 ? reqParts[1] : 'missing_in_log'; // Extract the URL path
  const status = parts.length > 5 ? parts[5] : ''; // Extract status code
  let agent = 'missing_in_log';
  agent = parts.length > 7 ? parts[7].slice(1,-1) : 'missing_in_log'; // Remove quotes from agent or set to 'missing_in_log'
  // Make sure have the agent token, sometimes "-" creaps in the before the agent token.
  if (agent === '-') {
    agent = parts.length > 8 ? parts[8].slice(1,-1) : 'missing_in_log'; // Remove quotes from agent or set to 'missing_in_log'
  }

  if (verbose) {
    console.log(`Parsed entry: IP=${ip}, Timestamp=${timestamp}, Method=${reqMethod}, Path=${reqPath}, Status=${status}, Agent=${agent}`);
  }

  return { ip, timestamp, reqMethod, reqPath, status, agent };
}

export async function analyzeLogs(
  inputStream: ReadableStream<Uint8Array>,
  outputStream: WritableStream<Uint8Array>,
  verbose: boolean = false,
  customRegex?: RegExp,
  analysisTypes: string[] = [],
  outputFormat: 'json' | 'yaml' | 'csv' = 'json'
) {
  const fileEntries: LogEntry[] = [];

  await processLines(inputStream, (line) => {
    if (verbose) {
      console.log(`Processing line: ${line}`);
    }

    if (!customRegex || customRegex.exec(line)) {
      const entry = parseLogLine(line, verbose);
      if (entry) {
        fileEntries.push(entry);
      }
    }
  });

  const results: AnalysisResult = {};

  if (analysisTypes.length === 0 || analysisTypes.includes('summary')) {
    Object.assign(results, summarizeAnalysis(fileEntries));
  }

  if (analysisTypes.includes('ip')) {
    results.ipCounts = analyzeIPAddresses(fileEntries);
  }

  if (analysisTypes.includes('agent')) {
    results.agentCounts = analyzeUserAgents(fileEntries);
  }

  if (analysisTypes.includes('path')) {
    results.pathCounts = analyzePaths(fileEntries);
  }

  if (analysisTypes.includes('time')) {
    results.timePeriodCounts = analyzeTimePeriods(fileEntries);
  }

  if (analysisTypes.includes('endpoint')) {
    results.endpointCounts = endPointAnalyser(fileEntries);
  }

  if (analysisTypes.includes('status')) {
    results.statusCounts = analyzeStatusCodes(fileEntries);
  }

  let output = '';
  switch (outputFormat) {
    case 'json':
      output = JSON.stringify(results, null, 2);
      break;
    case 'yaml':
      output = yaml.stringify(results);
      break;
    case 'csv':
      output = Object.entries(results)
        .map(([key, value]) => `${key}\n${Object.entries(value as object).map(([k, v]) => `${k},${v}`).join('\n')}`)
        .join('\n\n');
      break;
  }

  const writer = outputStream.getWriter();
  await writer.write(new TextEncoder().encode(output));
  await writer.close();
}

async function main() {
  const appName = 'loganalyst'
  const args = parseArgs(Deno.args);

  if (args.h || args.help) {
    console.log(fmtHelp(helpText, appName, version, releaseDate, releaseHash));
    Deno.exit(0);
  }
  if (args.license) {
    console.log(licenseText);
    Deno.exit(0);
  }

  if (args.version) {
    console.log(`${appName} ${version} ${releaseDate} ${releaseHash}`);
    Deno.exit(0);
  }


  const customRegex = args.r || args.regexp ? new RegExp(args.r || args.regexp) : undefined;
    let analysisTypes = (args.a || args.analysis || 'summary').split(',');

    if (args.ips) {
      analysisTypes = ['ip'];
    }

    if (args.endpoint) {
      analysisTypes = ['endpoint'];
    }

    if (args.status) {
      analysisTypes = ['status'];
    }

    const outputFormat = args.yaml ? 'yaml' : (args.o || args.output || 'json') as 'json' | 'yaml' | 'csv';

    await analyzeLogs(Deno.stdin.readable, Deno.stdout.writable, args.verbose, customRegex, analysisTypes, outputFormat);
}

if (import.meta.main) {
  main();
}
