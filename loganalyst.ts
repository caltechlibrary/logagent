import { parseArgs } from "@std/cli/parse-args";
import * as yaml from "@std/yaml";
import * as path from "@std/path";
import { tokenize, tokenizeUnquote } from "./logtok.ts";
import { licenseText, releaseDate, releaseHash, version } from "./version.ts";
import { fmtHelp, helpTextLogAnalyst } from "./helptext.ts";
import { Database } from "@db/sqlite";
import { LogAgents } from "./logagent.ts";
import { SimpleSpinner as Spinner } from "./SimpleSpinner.ts";

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

const helpMessage = `
loganalyst - Web server log analysis tool
USAGE:
  loganalyst harvest DB_NAME LOGFILE_PATH
  loganalyst analyze DB_NAME [ANALYSIS_TYPE] [OPTIONS]
SUBCOMMANDS:
  harvest    Import log entries into SQLite3 database
  analyze    Run analyses on harvested logs
ANALYSIS TYPES:
  ip        IP address counts
  agent     User agent counts
  path      Request path counts
  time      Time period counts
  endpoint  Endpoint method counts
  status    HTTP status code counts
  subnet    Subnet swarming detection
  summary   All analyses (default)
OPTIONS:
  -h, --help        Show help
  -l, --license     Show license
  -v, --version     Show version
  -V, --verbose     Enable verbose output
  -a, --action=FILE YAML config for blocking actions
  -o, --output=FMT  Output format: json, yaml, or csv (default: json)
`;

function getTableName(dbPath: string): string {
  const base = path.basename(dbPath);
  return base.replace(/\.db$/, "");
}

function createSchema(tableName: string): string {
  return `
DROP TABLE IF EXISTS ${tableName};
CREATE TABLE IF NOT EXISTS ${tableName} (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  ip TEXT NOT NULL,
  timestamp TEXT NOT NULL,
  reqMethod TEXT NOT NULL,
  reqPath TEXT NOT NULL,
  status TEXT NOT NULL,
  agent TEXT NOT NULL
);
`;
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
    pathCounts[entry.reqPath] = (pathCounts[entry.reqPath] || 0) + 1;
  });
  return pathCounts;
}

export function analyzeTimePeriods(
  entries: LogEntry[],
): Record<string, number> {
  const timePeriodCounts: Record<string, number> = {};
  entries.forEach((entry) => {
    const period = entry.timestamp;
    timePeriodCounts[period] = (timePeriodCounts[period] || 0) + 1;
  });
  return timePeriodCounts;
}

export function analyzeStatusCodes(
  entries: LogEntry[],
): Record<string, number> {
  const statusCounts: Record<string, number> = {};
  entries.forEach((entry) => {
    statusCounts[entry.status] = (statusCounts[entry.status] || 0) + 1;
  });
  return statusCounts;
}

export function endPointAnalyser(
  entries: LogEntry[],
): Record<string, Record<string, number>> {
  const endpointCounts: Record<string, Record<string, number>> = {};
  entries.forEach((entry) => {
    if (!endpointCounts[entry.reqPath]) {
      endpointCounts[entry.reqPath] = {};
    }
    endpointCounts[entry.reqPath][entry.reqMethod] =
      (endpointCounts[entry.reqPath][entry.reqMethod] || 0) + 1;
  });
  return endpointCounts;
}

export function analyzeSubnets(
  entries: LogEntry[],
  subnetMask: number = 24,
): Record<string, number> {
  const subnetCounts: Record<string, number> = {};
  for (const entry of entries) {
    const subnet = getSubnet(entry.ip, subnetMask);
    subnetCounts[subnet] = (subnetCounts[subnet] || 0) + 1;
  }
  return subnetCounts;
}

function getSubnet(ip: string, subnetMask: number): string {
  if (typeof ip !== "string") {
    return ip; // or return a default value like "0.0.0.0/0"
  }

  const octets = ip.split(".");
  if (octets.length !== 4) {
    return ip; // or return a default value
  }

  // Ensure all octets are numbers
  const numericOctets = octets.map((octet) => parseInt(octet, 10));
  if (numericOctets.some(isNaN)) {
    return ip; // or return a default value
  }

  if (subnetMask === 24) {
    return `${numericOctets[0]}.${numericOctets[1]}.${numericOctets[2]}.0/24`;
  } else if (subnetMask === 16) {
    return `${numericOctets[0]}.${numericOctets[1]}.0.0/16`;
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

async function logFailedLine(line: string, reason: string): Promise<void> {
  const logFilePath = "failed_lines.log";
  const logMessage = `Failed: ${reason}\nLine: ${line}\n---\n`;
  await Deno.writeTextFile(logFilePath, logMessage, { append: true });
}

export async function parseLogLine(
  line: string,
  verbose: boolean,
): Promise<LogEntry | null> {
  const parts: string[] = tokenize(line, false);
  if (parts.length < 5) {
    console.error(
      `Invalid log line "${line}" -> tokenized as ${JSON.stringify(parts)}`,
    );
    await logFailedLine(line, "Invalid format (too few parts)");
    return null;
  }

  const ip = parts[0];
  // Check if parts[3] exists and is a string
  const timestampPart = parts[3];
  if (!timestampPart) {
    console.error(`Invalid log line "${line}" -> timestamp part is missing`);
    await logFailedLine(line, "Missing timestamp");
    return null;
  }
  const timestamp = timestampPart;

  const reqParts = tokenizeUnquote(parts[4], false);
  const reqMethod = reqParts[0] || "";
  const reqPath = reqParts[1] || "";
  const status = parts[5] || "";

  // Check if parts[7] exists and is a string
  let agent = "-";
  if (
    parts[7] !== undefined && typeof parts[7] === "string"
  ) {
    agent = parts[7];
  }
  if (agent === "-") {
    if (
      parts[8] !== undefined && typeof parts[8] === "string"
    ) {
      agent = parts[8];
    }
  }
  if (verbose) {
    console.log(
      `Parsed entry: IP=${ip}, Timestamp=${timestamp}, Method=${reqMethod}, Path=${reqPath}, Status=${status}, Agent=${agent}`,
    );
  }

  if (reqPath === "" || reqMethod === "" || status === "") {
    console.error(`Failed to parse line: "${line}" (missing required fields)`);
    await logFailedLine(line, "Invalid format (missing required fields)");
    return null;
  }

  return { ip, timestamp, reqMethod, reqPath, status, agent };
}

async function harvestLogs(
  dbPath: string,
  logFilePath: string,
  verbose: boolean = false,
): Promise<void> {
  const tableName = getTableName(dbPath);
  const db = new Database(dbPath);
  let spinner: Spinner | null = null;
  if (!verbose) {
    spinner = new Spinner("Creating database schema...");
    spinner.start();
  }
  await db.exec(createSchema(tableName));
  if (spinner) {
    spinner.stop();
    console.error("✓ Database schema created successfully");
  }
  const file = await Deno.readTextFile(logFilePath);
  const lines = file.split("\n").filter((line) => line.trim() !== "");
  if (!verbose) {
    spinner = new Spinner("Processing log entries...");
    spinner.start();
  }
  let processedCount = 0;
  let insertedCount = 0;
  let skippedCount = 0;
  const totalLines = lines.length;
  for (const line of lines) {
    const entry = await parseLogLine(line, verbose);
    if (entry) {
      try {
        await db.run(
          `INSERT OR IGNORE INTO ${tableName} (ip, timestamp, reqMethod, reqPath, status, agent)
           VALUES (?, ?, ?, ?, ?, ?)`,
          [
            entry.ip,
            entry.timestamp,
            entry.reqMethod,
            entry.reqPath,
            entry.status,
            entry.agent,
          ],
        );
        const changes = db.changes;
        if (changes > 0) {
          insertedCount++;
        } else {
          skippedCount++;
          console.error(`Skipped duplicate entry: "${line}"`);
          await logFailedLine(line, "Duplicate entry");
        }
      } catch (error) {
        console.error(`Error inserting line: "${line}" -> ${error}`);
        await logFailedLine(line, `Database error: ${error}`);
      }
    } else {
      skippedCount++;
      console.error(`Skipped invalid log line: "${line}"`);
      await logFailedLine(line, "Invalid format");
    }
    processedCount++;
    if (spinner && processedCount % 1000 === 0) {
      spinner.setMessage(
        `Processing log entries... (${processedCount}/${totalLines}, inserted: ${insertedCount}, skipped: ${skippedCount})`,
      );
    }
  }
  if (spinner) {
    spinner.stop();
    console.error(
      `✓ Processed ${processedCount} log entries (inserted: ${insertedCount}, skipped: ${skippedCount})`,
    );
  }
  db.close();
  console.log(`Successfully harvested logs from ${logFilePath} into ${dbPath}`);
}

async function getLogEntries(
  db: Database,
  tableName: string,
  verbose: boolean,
): Promise<LogEntry[]> {
  const entries: LogEntry[] = [];
  let spinner: Spinner | null = null;
  if (verbose) {
    spinner = new Spinner("Retrieving log entries from database...");
    spinner.start();
  }
  const rows = await db.prepare(
    `SELECT ip, timestamp, reqMethod, reqPath, status, agent FROM ${tableName}`,
  ).all(1);
  if (spinner) {
    spinner.stop();
    console.error(`✓ Retrieved ${rows.length} log entries from database`);
  }
  if (verbose && rows.length > 0) {
    spinner = new Spinner("Processing database entries...");
    spinner.start();
  }
  for (const row of rows) {
    entries.push({
      ip: row[0] as string,
      timestamp: row[1] as string,
      reqMethod: row[2] as string,
      reqPath: row[3] as string,
      status: row[4] as string,
      agent: row[5] as string,
    });
  }
  if (spinner) {
    spinner.stop();
    console.error(`✓ Processed ${entries.length} database entries`);
  }
  return entries;
}

async function analyzeLogs(
  dbPath: string,
  analysisType: string = "summary",
  outputFormat: "json" | "yaml" | "csv" = "json",
  actionConfig: string = "",
  verbose: boolean = false,
): Promise<void> {
  const tableName = getTableName(dbPath);
  let spinner: Spinner | null = null;
  if (verbose) {
    spinner = new Spinner("Opening database...");
    spinner.start();
  }
  const db = new Database(dbPath);
  if (spinner) {
    spinner.stop();
    console.error("✓ Database opened successfully");
  }
  const entries = await getLogEntries(db, tableName, verbose);
  db.close();
  if (entries.length === 0) {
    console.error(
      "Error: No log entries found in the database. Have you run the harvest command?",
    );
    Deno.exit(1);
  }
  if (verbose) {
    spinner = new Spinner("Analyzing log data...");
    spinner.start();
  }
  const results = summarizeAnalysis(entries);
  if (spinner) {
    spinner.stop();
    console.error("✓ Analysis completed");
  }
  const filteredResults: AnalysisResult = {};
  if (analysisType === "summary" || analysisType === "ip") {
    filteredResults.ipCounts = results.ipCounts;
  }
  if (analysisType === "summary" || analysisType === "agent") {
    filteredResults.agentCounts = results.agentCounts;
  }
  if (analysisType === "summary" || analysisType === "path") {
    filteredResults.pathCounts = results.pathCounts;
  }
  if (analysisType === "summary" || analysisType === "time") {
    filteredResults.timePeriodCounts = results.timePeriodCounts;
  }
  if (analysisType === "summary" || analysisType === "endpoint") {
    filteredResults.endpointCounts = results.endpointCounts;
  }
  if (analysisType === "summary" || analysisType === "status") {
    filteredResults.statusCounts = results.statusCounts;
  }
  if (analysisType === "summary" || analysisType === "subnet") {
    filteredResults.subnetCounts = results.subnetCounts;
  }
  let output = "";
  switch (outputFormat) {
    case "json":
      output = JSON.stringify(filteredResults, null, 2);
      break;
    case "yaml":
      output = yaml.stringify(filteredResults);
      break;
    case "csv":
      output = Object.entries(filteredResults)
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
  console.log(output);
  if (actionConfig && filteredResults.subnetCounts) {
    if (verbose) {
      spinner = new Spinner("Processing actions for swarming subnets...");
      spinner.start();
    }
    const agents = new LogAgents();
    await agents.loadConfig(actionConfig);
    let actionCount = 0;
    for (
      const [subnet, count] of Object.entries(filteredResults.subnetCounts)
    ) {
      if (count > 100) {
        for (const agent of agents.agents) {
          if (agent.tag === "swarm") {
            const cmd = agent.action.replaceAll("{ipaddress}", subnet);
            console.log(`Blocking subnet ${subnet} with command: ${cmd}`);
            await runCmd(cmd);
            actionCount++;
          }
        }
      }
    }
    if (spinner) {
      spinner.stop();
      console.error(`✓ Processed ${actionCount} actions for swarming subnets`);
    }
  }
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
  let currentParam = "";
  let inQuotes = false;
  for (const char of input) {
    if (char === '"') inQuotes = !inQuotes;
    else if (char === " " && !inQuotes) {
      if (currentParam.trim()) params.push(currentParam.trim());
      currentParam = "";
    } else currentParam += char;
  }
  if (currentParam.trim()) params.push(currentParam.trim());
  return params;
}

async function main(): Promise<number> {
  const appName = "loganalyst";
  const args = parseArgs(Deno.args, {
    alias: {
      help: "h",
      license: "l",
      version: "v",
      verbose: "V",
      action: "a",
      output: "o",
    },
    default: {
      help: false,
      license: false,
      version: false,
      verbose: false,
      action: "",
      output: "json",
    },
  });
  if (Deno.args.length === 0) {
    console.log(helpMessage);
    return 1;
  }
  if (args.help) {
    console.log(
      fmtHelp(helpTextLogAnalyst, appName, version, releaseDate, releaseHash),
    );
    return 0;
  }
  if (args.license) {
    console.log(licenseText);
    return 0;
  }
  if (args.version) {
    console.log(`loganalyst ${version} ${releaseDate} ${releaseHash}`);
    return 0;
  }
  const subcommand = Deno.args[0];
  try {
    switch (subcommand) {
      case "harvest": {
        if (Deno.args.length < 3) {
          console.error("Error: Missing arguments for harvest command");
          console.log(helpMessage);
          return 1;
        }
        const dbPath = Deno.args[1];
        const logFilePath = Deno.args[2];
        await harvestLogs(dbPath, logFilePath, args.verbose);
        break;
      }
      case "analyze": {
        if (Deno.args.length < 2) {
          console.error("Error: Missing database path for analyze command");
          console.log(helpMessage);
          return 1;
        }
        const dbPath = Deno.args[1];
        const analysisType = Deno.args[2] || "summary";
        await analyzeLogs(
          dbPath,
          analysisType,
          args.output as "json" | "yaml" | "csv",
          args.action,
          args.verbose,
        );
        break;
      }
      case "help":
        console.log(
          fmtHelp(
            helpTextLogAnalyst,
            appName,
            version,
            releaseDate,
            releaseHash,
          ),
        );
        return 0;
      default:
        console.error(`Error: Unknown subcommand "${subcommand}"`);
        console.log(helpMessage);
        return 1;
    }
  } catch (error) {
    console.error(`Error: ${error}`);
    return 1;
  }
  return 0;
}

if (import.meta.main) {
  Deno.exit(await main());
}
