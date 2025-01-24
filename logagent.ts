// logagent.ts is a TypeScript module
// By R. S. Doiel
import * as yaml from "@std/yaml";
import { parseArgs } from "@std/cli/parse-args";

const reIPAddress = new RegExp('[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+');
export interface LogAgentsInterface {
  loadConfig: (arg0: string) => Promise<boolean>;
}

// LogAgent implements an object that perform action based
// on the contents of a YAML configuration file.
export class LogAgents implements LogAgentsInterface {
  agents: { tag: string; action: string }[] = [];

  // loadConfig, reads the YAML configuration and updates
  // the LogAgent's state.
  //
  // ```
  //  let la = new LogAgent();
  //  if (! await la.loadConfig("badbots.yaml")) {
  //     ... // handle failure to read and proces badbots.yaml
  //  }
  // ```
  async loadConfig(fName: string): Promise<boolean> {
    const src = await Deno.readTextFile(fName);
    if (src === undefined) {
      console.log(`failed to read ${fName}`);
      return false;
    }
    let agentList: { tag: string; action: string }[] = yaml.parse(
      src,
    ) as unknown as { tag: string; action: string }[];
    if (agentList === undefined) {
      return false;
    }
    let i = 0;
    for (let agent of agentList) {
      if (
        agent.tag === undefined || agent.action === undefined ||
        agent.tag.trim() === "" || agent.action.trim() === ""
      ) {
        console.log(
          `agent ${i} is badly formed, ${agent.toString()}, aborting load`,
        );
        return false;
      }
      agent.tag = agent.tag.trim();
      agent.action = agent.action.trim();
      this.agents.push(agent);
      i++;
    }
    return true;
  }

  async dryRun(cfgName: string, logName: string): Promise<boolean> {
    if (await this.loadConfig(cfgName)) {
        const src = await Deno.readTextFile(logName);
        let collected_actions: {[key: string]: string[]} = {};
        for (const line of src.split('\n')) {
            if (line !== '') {
                const ipaddress: string = lineToIPAddress(line);
                for (const agent of this.agents) {
                    if (line.indexOf(agent.tag) > -1) {
                      if (collected_actions[ipaddress] === undefined) {
                        collected_actions[ipaddress] = [];
                      }
                      if (collected_actions[ipaddress].indexOf(agent.action) === -1) {
                        collected_actions[ipaddress].push(agent.action);
                        console.log(`# Agent Tag: ${agent.tag}`);
                        console.log(agent.action.replaceAll('\n', ' ').replaceAll('{ipaddress}', ipaddress));
                      } 
                    }
                }    
            }
        }
        return true;
    }
    return false;
  }

  async apply(fName: string, logName: string): Promise<boolean> {
    if (await this.loadConfig(fName) === false) {
        return false;
    }
    const src = await Deno.readTextFile(logName);
    let collected_actions: {[key: string]: string[]} = {};
    
    for (const line of src.split('\n')) {
        
        const ipaddress: string = lineToIPAddress(line);
        for (const agent of this.agents) {
          let ipaddress = lineToIPAddress(line);
          if (line.indexOf(agent.tag) > -1) {
            if (collected_actions[ipaddress] === undefined) {
              collected_actions[ipaddress] = [];
            }
            if (collected_actions[ipaddress].indexOf(agent.action) === -1) {
              collected_actions[ipaddress].push(agent.action);

              let cmd = agent.action.replaceAll('{ipaddress}', ipaddress);
              console.log(cmd);
              await runCmd(cmd);
            }
          }
        }
    }
    return true;
  }
}

function lineToIPAddress(line: string): string {
    for (let part of line.split(' ')) {
        if (reIPAddress.test(part)) {
            return part;
        }
    }
    return '';
}

function parseParams(input: string): string[] {
    const params: string[] = [];
    let currentParam = '';
    let inQuotes = false;

    for (let i = 0; i < input.length; i++) {
        const char = input[i];

        if (char === '"') {
            inQuotes = !inQuotes;
        } else if (char === ' ' && !inQuotes) {
            if (currentParam.trim() !== '') {
                params.push(currentParam.trim());
                currentParam = '';
            }
        } else {
            currentParam += char;
        }
    }

    if (currentParam.trim() !== '') {
        params.push(currentParam.trim());
    }

    return params;
}
  
async function runCmd(s: string): Promise<boolean> {
    let args: string[] = parseParams(s);
    let cmd: string | undefined = args.shift();
    if (cmd === undefined) {
        return false;
    }
    const command = new Deno.Command(`${cmd}`, {
        args: args
    });
    const { code, stdout, stderr } = await command.output();
    //const out = (new TextDecoder()).decode(stdout);
    //if (out !== undefined) console.log(out);
    if (code === 0) {
        return true
    }
    const eout = (new TextDecoder()).decode(stderr);
    if (eout !== undefined) console.log(eout);
    return false;
}

