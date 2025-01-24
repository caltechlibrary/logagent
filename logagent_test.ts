// logagent_test.ts tests the TypeScript module logagent.ts
// By R. S. Doiel, <rsdoiel@caltech.edu>

import { assertEquals, assertNotEquals } from "@std/assert";
import { LogAgents } from "./logagent.ts";

async function testLoadConfig() {
  const cfgName = "testdata/badbots.yaml";
  let agents = new LogAgents();
  assertNotEquals(
    agents,
    undefined,
    `let la = new LogAgents(); // failed to create LogAgents object`,
  );
  assertEquals(
    await agents.loadConfig(cfgName),
    true,
    `expected true, got false for la.loadConfig(${cfgName})`,
  );
}

async function testDryRun() {
  const cfgName = "testdata/badbots.yaml";
  const logName = "testdata/access.log";

  let agents = new LogAgents();
  assertEquals(
    await agents.dryRun(cfgName, logName),
    true,
    `expected true, got false for la.dryRun(${cfgName}, ${logName})`,
  );
}

async function testApply() {
  const cfgName = "testdata/echobots.yaml";
  const logName = "testdata/access.log";

  let agents = new LogAgents();
  assertEquals(
    await agents.apply(cfgName, logName),
    true,
    `expected true, got false for la.apply(${cfgName}, ${logName})`,
  );
}


if (import.meta.main) {
  await testLoadConfig();
  await testDryRun();
  await testApply();
} else {
  Deno.test("testLoadConfig", testLoadConfig);
  Deno.test("testDryRun", testDryRun);
  Deno.test("testApply", testApply);
}
