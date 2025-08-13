const appName: string = "logToSQL";

const helpText: string = `
USAGE: ${appName} TABLE_NAME LOG_FILE_PATH

This program takes the log file and generates a SQLite3 
database and table containing the logged content. This can then
be queried and reported on via standard SQL.

From here you can see which subnets are causing the problem and
deal with them.
`;

const schema:string = `
--
-- This is s the schema for ad-hoc log analysis
--
DROP TABLE IF EXISTS {tableName};
CREATE TABLE {tableName} (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  ip TEXT NOT NULL,
  timestamp TEXT NOT NULL,
  method TEXT NOT NULL,
  path TEXT NOT NULL,
  status TEXT NOT NULL
);

--
-- Here is the data as a SQL insert run
--

`;

function createSchema(tableName: string, scheme: string): string {
  return scheme.replaceAll("{tableName}", tableName);
}

interface LogEntry {
  ip: string;
  timestamp: string;
  method: string;
  path: string;
  status: string;
}

function parseLogLine(line: string): LogEntry | null {
  // Example line:
  // 52.255.111.116 - - [12/Aug/2025:16:22:28 +0000] "GET /3526/1/SuyuThesisNewStyle.pdf HTTP/1.1" 200 7743174
  const logRegex = /^([\d.]+)\s.*\s\[(.*?)\]\s"\s*(\w+)\s+([^ ]+).*?"\s(\d{3})/;
  const match = line.match(logRegex);

  if (!match) {
    console.error(`Failed to parse line: ${line}`);
    return null;
  }

  return {
    ip: match[1],
    timestamp: match[2],
    method: match[3],
    path: match[4],
    status: match[5],
  };
}

async function processLogFile(tableName: string, logFilePath: string) {
  const file = await Deno.readTextFile(logFilePath);
  const lines = file.split("\n").filter((line) => line.trim() !== "");

  for (const line of lines) {
    const entry = parseLogLine(line);
    if (entry) {
      const { ip, timestamp, method, path, status } = entry;
      const sql = `
        INSERT INTO ${tableName} (ip, timestamp, method, path, status)
        VALUES ('${ip}', '${timestamp}', '${method}', '${path.replace(/'/g, "''")}', '${status}');
      `;
      console.log(sql);
    }
  }
}

async function main(): Promise<number> {
  const args = Deno.args;
  const tableName = args.shift() || "";
  const logFilePath = args.shift() || "";
  if (tableName === "" || logFilePath === "") {
    console.log(helpText);
    console.log(`
${appName} requires a table name and log file path.
`);
    return 1;
  }
  console.log(createSchema(tableName, schema));
  await processLogFile(tableName, logFilePath);
  return 0;
}

if (import.meta.main) {
  const exitCode = await main();
  Deno.exit(exitCode);
}
