import { parseArgs } from "@std/cli";
import { licenseText, releaseDate, releaseHash, version } from "./version.ts";
import { fmtHelp, helpText } from "./helptext.ts";
import { LogAgents } from "./logagent.ts";


async function main(): Promise<number> {
  const appName = "logagent";
  const app = parseArgs(Deno.args, {
    alias: {
      help: "h",
      license: "l",
      version: "v",
      dryrun: "d",
    },
    default: {
      help: false,
      version: false,
      license: false,
      dryrun: false,
    },
  });
  const args = app._;

  if (app.help) {
    console.log(fmtHelp(helpText, appName, version, releaseDate, releaseHash));
    Deno.exit(0);
  }
  if (app.license) {
    console.log(licenseText);
    Deno.exit(0);
  }

  if (app.version) {
    console.log(`${appName} ${version} ${releaseDate} ${releaseHash}`);
    Deno.exit(0);
  }
  if (args.length < 1) {
    console.log(`USAGE: ${appName} [OPTIONS] CFG_NAME LOG_NAME`);
    Deno.exit(1);
  }

  let agents = new LogAgents();
  const cfgName: string = (args.length > 0) ? `${args[0]}` : '';
  const logName: string = (args.length > 1) ? `${args[1]}` : '';
  if (cfgName === '' || logName === '') {
    console.log(`USAGE: ${appName} [OPTIONS] CFG_NAME LOG_NAME`);
    Deno.exit(1);  
  }
  //console.log(`cfgName: ${cfgName}, logName: ${logName}`);
  if (app.dryrun) {
    if (await agents.dryRun(cfgName, logName)) {
      return 0;
    }  
  } else {
    if (await agents.apply(cfgName, logName)) {
      return 0;
    }
  }
  return 1;
}

if (import.meta.main) await main();