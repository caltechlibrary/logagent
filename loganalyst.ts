import { parseArgs } from "@std/cli/parse-args";
import * as yaml from "@std/yaml";
import { processLines } from './readLines.ts';
import { tokenize, tokenizeUnquote } from './logtok.ts';
import { licenseText, releaseDate, releaseHash, version } from "./version.ts";
import { fmtHelp, helpTextLogAnalyst as helpText } from "./helptext.ts";
import { Database } from "@db/sqlite";
import { LogAgents } from "./logagent.ts";

interface LogEntry {
  ip: string;
  timestamp: string;
  reqMethod: string;
  reqPath: string;
  status: string;
  agent: string;
}

interface AnalysisResult {
  ipCounts?: Record<string, number>;
  agentCounts?: Record<string, number>;
  pathCounts?: Record<string, number>;
  statusCounts?: Record<string, number>;
  endpointCounts?: Record<string, Record<string, number>>;
  timePeriodCounts?: Record<string, number>;
  subnetCounts?: Record<string, number>;
}

const schema = `
DROP TABLE IF EXISTS log_entries;
CREATE TABLE log_entries (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  ip TEXT NOT NULL,
  timestamp TEXT NOT NULL,
  reqMethod TEXT NOT NULL,
  reqPath TEXT NOT NULL,
  status TEXT NOT NULL,
  agent TEXT NOT NULL
);
`;

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
    const period = entry.timestamp.slice(0, 14);
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

export function analyzeSubnets(entries: LogEntry[], subnetMask: number = 24): Record<string, number> {
  const subnetCounts: Record<string, number> = {};
  for (const entry of entries) {
    const subnet = getSubnet(entry.ip, subnetMask);
    subnetCounts[subnet] = (subnetCounts[subnet] || 0) + 1;
  }
  return subnetCounts;
}

function getSubnet(ip: string, subnetMask: number): string {
  const octets = ip.split('.').map(Number);
  if (subnetMask === 24) {
    return `${octets[0]}.${octets[1]}.${octets[2]}.0/24`;
  } else if (subnetMask === 16) {
    return `${octets[0]}.${octets[1]}.0.0/16`;
  }
  return ip;
}

export function summarizeAnalysis(entries: LogEntry[]): AnalysisResult {
  return {
    ipCounts: analyzeIPAddresses(entries),
    agentCounts: analyzeUserAgents(entries),
    pathCounts: analyzePaths(entries),
    statusCounts: analyzeStatusCodes(entries),
    timePeriodCounts: analyzeTimePeriods(entries),
    endpointCounts: endPointAnalyser(entries),
    subnetCounts: analyzeSubnets(entries),
  };
}

export function parseLogLine(line: string, verbose: boolean): LogEntry | null {
  const parts: string[] = tokenize(line, verbose);
  if (parts.length < 5) {
    console.error(`invalid log line "${line}" -> tokenized as ${JSON.stringify(parts)}`);
    return null;
  }
  const ip = parts[0];
  const timestamp = parts[3].slice(1, -1);
  const reqParts = tokenizeUnquote(parts[4], verbose);
  const reqMethod = reqParts[0] || 'missing_in_log';
  const reqPath = reqParts[1] || 'missing_in_log';
  const status = parts[5] || '';
  let agent = parts[7]?.slice(1, -1) || 'missing_in_log';
  if (agent === '-') {
    agent = parts[8]?.slice(1, -1) || 'missing_in_log';
  }
  if (verbose) {
    console.log(`Parsed entry: IP=${ip}, Timestamp=${timestamp}, Method=${reqMethod}, Path=${reqPath}, Status=${status}, Agent=${agent}`);
  }
  return { ip, timestamp, reqMethod, reqPath, status, agent };
}

async function storeLogsInDB(dbPath: string, entries: LogEntry[]) {
  const db = new Database(dbPath);
  await db.exec(schema);
  for (const entry of entries) {
    await db.exec(
      `INSERT INTO log_entries (ip, timestamp, reqMethod, reqPath, status, agent) VALUES (?, ?, ?, ?, ?, ?)`,
      [entry.ip, entry.timestamp, entry.reqMethod, entry.reqPath, entry.status, entry.agent],
    );
  }
  db.close();
}

async function runCmd(s: string): Promise<boolean> {
  const args: string[] = parseParams(s);
  const cmd: string | undefined = args.shift();
  if (!cmd) return false;
  const command = new Deno.Command(cmd, { args });
  const { code, stderr } = await command.output();
  if (code !== 0) {
    const eout = new TextDecoder().decode(stderr);
    console.error(eout);
    return false;
  }
  return true;
}

function parseParams(input: string): string[] {
  const params: string[] = [];
  let currentParam = '';
  let inQuotes = false;
  for (const char of input) {
    if (char === '"') inQuotes = !inQuotes;
    else if (char === ' ' && !inQuotes) {
      if (currentParam.trim()) params.push(currentParam.trim());
      currentParam = '';
    } else currentParam += char;
  }
  if (currentParam.trim()) params.push(currentParam.trim());
  return params;
}

export async function analyzeLogs(
  inputStream: ReadableStream<Uint8Array>,
  outputStream: WritableStream<Uint8Array>,
  verbose: boolean = false,
  customRegex?: RegExp,
  analysisTypes: string[] = [],
  outputFormat: 'json' | 'yaml' | 'csv' = 'json',
  dbPath: string = '',
  actionConfig: string = '',
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

  if (dbPath) {
    await storeLogsInDB(dbPath, fileEntries);
  }

  const results = summarizeAnalysis(fileEntries);

  const filteredResults: AnalysisResult = {};
  if (analysisTypes.length === 0 || analysisTypes.includes('summary')) {
    Object.assign(filteredResults, results);
  }
  if (analysisTypes.includes('ip')) {
    filteredResults.ipCounts = results.ipCounts;
  }
  if (analysisTypes.includes('agent')) {
    filteredResults.agentCounts = results.agentCounts;
  }
  if (analysisTypes.includes('path')) {
    filteredResults.pathCounts = results.pathCounts;
  }
  if (analysisTypes.includes('time')) {
    filteredResults.timePeriodCounts = results.timePeriodCounts;
  }
  if (analysisTypes.includes('endpoint')) {
    filteredResults.endpointCounts = results.endpointCounts;
  }
  if (analysisTypes.includes('status')) {
    filteredResults.statusCounts = results.statusCounts;
  }
  if (analysisTypes.includes('subnet')) {
    filteredResults.subnetCounts = results.subnetCounts;
  }

  let output = '';
  switch (outputFormat) {
    case 'json':
      output = JSON.stringify(filteredResults, null, 2);
      break;
    case 'yaml':
      output = yaml.stringify(filteredResults);
      break;
    case 'csv':
      output = Object.entries(filteredResults)
        .map(([key, value]) => `${key}\n${Object.entries(value as object).map(([k, v]) => `${k},${v}`).join('\n')}`)
        .join('\n\n');
      break;
  }

  const writer = outputStream.getWriter();
  await writer.write(new TextEncoder().encode(output));
  await writer.close();

  if (actionConfig && results.subnetCounts) {
    const agents = new LogAgents();
    await agents.loadConfig(actionConfig);
    for (const [subnet, count] of Object.entries(results.subnetCounts)) {
      if (count > 100) {
        for (const agent of agents.agents) {
          if (agent.tag === 'swarm') {
            const cmd = agent.action.replaceAll('{ipaddress}', subnet);
            console.log(`Blocking subnet ${subnet} with command: ${cmd}`);
            await runCmd(cmd);
          }
        }
      }
    }
  }
}

async function main() {
  const appName = 'loganalyst';
  const args = parseArgs(Deno.args, {
    alias: {
      help: 'h',
      license: 'l',
      version: 'v',
      verbose: 'V',
      db: 'd',
      subnet: 's',
      action: 'a',
      regex: 'r',
      analysis: 'a',
      output: 'o',
    },
    default: {
      help: false,
      license: false,
      version: false,
      verbose: false,
      db: '',
      subnet: false,
      action: '',
      regex: '',
      analysis: 'summary',
      output: 'json',
    },
  });

  if (args.help) {
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

  const customRegex = args.regex ? new RegExp(args.regex) : undefined;
  const analysisTypes = args.analysis.split(',');
  if (args.subnet) analysisTypes.push('subnet');

  const outputFormat = args.output as 'json' | 'yaml' | 'csv';

  await analyzeLogs(
    Deno.stdin.readable,
    Deno.stdout.writable,
    args.verbose,
    customRegex,
    analysisTypes,
    outputFormat,
    args.db,
    args.action,
  );
}

if (import.meta.main) {
  main();
}
