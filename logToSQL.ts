const schema = `
--
-- This is s the schema for ad-hoc log analysis
--
DROP TABLE IF EXISTS apache_logs;
CREATE TABLE apache_logs (
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

interface LogEntry {
  ip: string;
  timestamp: string;
  method: string;
  path: string;
  status: string;
}

function parseLogLine(line: string): LogEntry | null {
  // Example line:
  // 1.2.3.4 - - [12/Aug/2025:16:22:28 +0000] "GET /3526/1/SuyuThesisNewStyle.pdf HTTP/1.1" 200 7743174
  const logRegex =
    /^([\d.]+)\s.*\s\[(.*?)\]\s"\s*(\w+)\s+([^ ]+).*?"\s(\d{3})/;
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

export async function processLogFile(logFilePath: string): Promise<string> {
  const file = await Deno.readTextFile(logFilePath);
  const lines = file.split("\n").filter((line) => line.trim() !== "");
  const stmts: string[] = [];
  stmts.push(schema);

  for (const line of lines) {
    const entry = parseLogLine(line);
    if (entry) {
      const { ip, timestamp, method, path, status } = entry;
      const sql = `
        INSERT INTO apache_logs (ip, timestamp, method, path, status)
        VALUES ('${ip}', '${timestamp}', '${method}', '${path}', '${status}');
      `;
      stmts.push(sql);
    }
  }
  return stmts.join('\n');
}

async function main() {
  const logFilePath = Deno.args[0];
  if (!logFilePath) {
    console.error("Usage: deno run --allow-read apache_log_to_sql.ts <apache-log-file>");
    Deno.exit(1);
  }
  console.log(await processLogFile(logFilePath));
}

if (import.meta.main) {
    await main().catch(console.error);
}
