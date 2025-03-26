import { parseArgs } from "@std/cli/parse-args";
import { processLines } from './readLines.ts'; // Adjust the path as necessary
import * as yaml from "@std/yaml";

interface LogEntry {
  ip: string;
  timestamp: string; // Changed to string
  request: string;
  agent: string;
}

interface AnalysisResult {
  ipCounts?: Record<string, number>;
  agentCounts?: Record<string, number>;
  pathCounts?: Record<string, number>;
  timePeriodCounts?: Record<string, number>;
  endpointCounts?: Record<string, Record<string, number>>;
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
    pathCounts[entry.request] = (pathCounts[entry.request] || 0) + 1;
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

export function endPointAnalyser(entries: LogEntry[]): Record<string, Record<string, number>> {
  const endpointCounts: Record<string, Record<string, number>> = {};
  entries.forEach(entry => {
    const [method, path] = entry.request.split(' ');
    if (!endpointCounts[path]) {
      endpointCounts[path] = {};
    }
    endpointCounts[path][method] = (endpointCounts[path][method] || 0) + 1;
  });
  return endpointCounts;
}

export function summarizeAnalysis(entries: LogEntry[]): AnalysisResult {
  return {
    ipCounts: analyzeIPAddresses(entries),
    agentCounts: analyzeUserAgents(entries),
    pathCounts: analyzePaths(entries),
    timePeriodCounts: analyzeTimePeriods(entries),
    endpointCounts: endPointAnalyser(entries),
  };
}

export function parseLogLine(line: string): LogEntry | null {
  const parts = line.split(' ');
  if (parts.length < 12) {
    console.error(`Invalid log line format: ${line}`);
    return null;
  }
  const ip = parts[0];
  const timestamp = parts[3].slice(1, -1); // Remove brackets
  const request = parts[5] + ' ' + parts[6];
  const agent = parts.slice(11).join(' ');

  console.log(`Parsed entry: IP=${ip}, Timestamp=${timestamp}, Request=${request}, Agent=${agent}`);

  return { ip, timestamp, request, agent };
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
      const entry = parseLogLine(line);
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

function displayHelp() {
  const helpText = `
# Log Analyst

Analyze NginX log files to identify entries matching a specified regex.

## Usage

\`\`\`
loganalyst.ts [options]
\`\`\`

## Options

-h, --help           Display this help message
--verbose            Display lines as they are being processed
-r, --regexp         Specify a custom regex pattern to match log entries
-a, --analysis       Specify analysis types (comma-separated): summary, ip, agent, path, time, endpoint
-o, --output         Specify output format: json, yaml, csv (default: json)
--ips                Shortcut for --analysis ip
--yaml               Shortcut for --output yaml

## Example

To analyze log entries with default summary analysis:

\`\`\`
deno run --allow-read loganalyst.ts < logfile.log
\`\`\`

To analyze log entries with custom regex and specific analyses:

\`\`\`
deno run --allow-read loganalyst.ts --regexp ".*\\.zip" --analysis ip,agent --output yaml < logfile.log
\`\`\`

**Note:** When specifying a regex pattern, make sure to escape special characters properly. For example, use \`\\.\` to match a literal dot.

  `;
  console.log(helpText);
}

async function main() {
  const args = parseArgs(Deno.args);

  if (args.h || args.help) {
    displayHelp();
  } else {
    const customRegex = args.r || args.regexp ? new RegExp(args.r || args.regexp) : undefined;
    let analysisTypes = (args.a || args.analysis || 'summary').split(',');

    if (args.ips) {
      analysisTypes = ['ip'];
    }

    const outputFormat = args.yaml ? 'yaml' : (args.o || args.output || 'json') as 'json' | 'yaml' | 'csv';

    await analyzeLogs(Deno.stdin.readable, Deno.stdout.writable, args.verbose, customRegex, analysisTypes, outputFormat);
  }
}

if (import.meta.main) {
  main();
}
