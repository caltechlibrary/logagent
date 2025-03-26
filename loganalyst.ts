import { parseArgs } from "@std/cli/parse-args";
import { processLines } from "./readLines.ts"; // Adjust the path as necessary
import * as yaml from "@std/yaml";
import { licenseText, releaseDate, releaseHash, version } from "./version.ts";
import { fmtHelp, helpTextLogAnalyst as helpText } from "./helptext.ts";

interface LogEntry {
  ip: string;
  timestamp: string; // Changed to string
  method: string; // New attribute for HTTP method
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

export function analyzeIPAddresses(
  entries: LogEntry[],
): Record<string, number> {
  const ipCounts: Record<string, number> = {};
  entries.forEach((entry) => {
    ipCounts[entry.ip] = (ipCounts[entry.ip] || 0) + 1;
  });
  return ipCounts;
}

export function analyzeUserAgents(entries: LogEntry[]): Record<string, number> {
  const agentCounts: Record<string, number> = {};
  entries.forEach((entry) => {
    agentCounts[entry.agent] = (agentCounts[entry.agent] || 0) + 1;
  });
  return agentCounts;
}

export function analyzePaths(entries: LogEntry[]): Record<string, number> {
  const pathCounts: Record<string, number> = {};
  entries.forEach((entry) => {
    pathCounts[entry.request] = (pathCounts[entry.request] || 0) + 1;
  });
  return pathCounts;
}

export function analyzeTimePeriods(
  entries: LogEntry[],
): Record<string, number> {
  const timePeriodCounts: Record<string, number> = {};
  entries.forEach((entry) => {
    const period = entry.timestamp.slice(0, 14); // Truncate to DD/MMM/YYYY:HH
    timePeriodCounts[period] = (timePeriodCounts[period] || 0) + 1;
  });
  return timePeriodCounts;
}

export function endPointAnalyser(
  entries: LogEntry[],
): Record<string, Record<string, number>> {
  const endpointCounts: Record<string, Record<string, number>> = {};
  entries.forEach((entry) => {
    if (!endpointCounts[entry.request]) {
      endpointCounts[entry.request] = {};
    }
    endpointCounts[entry.request][entry.method] =
      (endpointCounts[entry.request][entry.method] || 0) + 1;
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
  const parts = line.split(" ");
  if (parts.length < 12) {
    console.error(`Invalid log line format: ${line}`);
    return null;
  }
  const ip = parts[0];
  const timestamp = parts[3].slice(1, -1); // Remove brackets
  const method = parts[5].replace(/"/g, ""); // Remove quotes from method
  const request = parts[6].replace(/"/g, ""); // Remove quotes from request
  const agent = parts.slice(11).join(" ").replace(/"/g, ""); // Remove quotes from agent

  console.log(
    `Parsed entry: IP=${ip}, Timestamp=${timestamp}, Method=${method}, Request=${request}, Agent=${agent}`,
  );

  return { ip, timestamp, method, request, agent };
}

export async function analyzeLogs(
  inputStream: ReadableStream<Uint8Array>,
  outputStream: WritableStream<Uint8Array>,
  verbose: boolean = false,
  customRegex?: RegExp,
  analysisTypes: string[] = [],
  outputFormat: "json" | "yaml" | "csv" = "json",
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

  if (analysisTypes.length === 0 || analysisTypes.includes("summary")) {
    Object.assign(results, summarizeAnalysis(fileEntries));
  }

  if (analysisTypes.includes("ip")) {
    results.ipCounts = analyzeIPAddresses(fileEntries);
  }

  if (analysisTypes.includes("agent")) {
    results.agentCounts = analyzeUserAgents(fileEntries);
  }

  if (analysisTypes.includes("path")) {
    results.pathCounts = analyzePaths(fileEntries);
  }

  if (analysisTypes.includes("time")) {
    results.timePeriodCounts = analyzeTimePeriods(fileEntries);
  }

  if (analysisTypes.includes("endpoint")) {
    results.endpointCounts = endPointAnalyser(fileEntries);
  }

  let output = "";
  switch (outputFormat) {
    case "json":
      output = JSON.stringify(results, null, 2);
      break;
    case "yaml":
      output = yaml.stringify(results);
      break;
    case "csv":
      output = Object.entries(results)
        .map(([key, value]) =>
          `${key}\n${
            Object.entries(value as object).map(([k, v]) => `${k},${v}`).join(
              "\n",
            )
          }`
        )
        .join("\n\n");
      break;
  }

  const writer = outputStream.getWriter();
  await writer.write(new TextEncoder().encode(output));
  await writer.close();
}

async function main() {
  const appName = "loganalyst";
  const args = parseArgs(Deno.args);

  if (args.h || args.help) {
    console.log(fmtHelp(helpText, appName, version, releaseDate, releaseHash));
    Deno.exit(0);
  }
  if (args.l || args.license) {
    console.log(licenseText);
    Deno.exit(0);
  }

  if (args.v || args.version) {
    console.log(`${appName} ${version} ${releaseDate} ${releaseHash}`);
    Deno.exit(0);
  }

  const customRegex = args.r || args.regexp
    ? new RegExp(args.r || args.regexp)
    : undefined;
  let analysisTypes = (args.a || args.analysis || "summary").split(",");

  if (args.ips) {
    analysisTypes = ["ip"];
  }

  if (args.endpoint) {
    analysisTypes = ["endpoint"];
  }

  const outputFormat = args.yaml
    ? "yaml"
    : (args.o || args.output || "json") as "json" | "yaml" | "csv";

  await analyzeLogs(
    Deno.stdin.readable,
    Deno.stdout.writable,
    args.verbose,
    customRegex,
    analysisTypes,
    outputFormat,
  );
}

if (import.meta.main) {
  main();
}
