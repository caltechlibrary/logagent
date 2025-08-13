// spinner_test.ts
import { assert } from "@std/assert";
import { SimpleSpinner } from "./SimpleSpinner.ts";

Deno.test("SimpleSpinner starts and stops correctly", async (t) => {
  const spinner = new SimpleSpinner("Test spinner");

  await t.step("Spinner starts and updates position", async () => {
    let output = "";
    const originalLog = console.log;
    console.log = (msg: string) => { output += msg; };

    spinner.start();
    assert(spinner.isActive() === true);

    // Let the spinner run for a few frames
    await new Promise((resolve) => setTimeout(resolve, 300));
    spinner.stop();

    assert(spinner.isActive() === false);
    assert(output.includes("| Test spinner") || output.includes("/ Test spinner") ||
           output.includes("- Test spinner") || output.includes("\\ Test spinner"));

    // Restore console.log
    console.log = originalLog;
  });

  await t.step("Spinner message can be updated", () => {
    let output = "";
    const originalLog = console.log;
    console.log = (msg: string) => { output += msg; };

    spinner.setMessage("Updated message");
    assert(spinner["message"] === "Updated message");

    // Restore console.log
    console.log = originalLog;
  });

  await t.step("Spinner stops and clears the line", () => {
    let output = "";
    const originalLog = console.log;
    console.log = (msg: string) => { output += msg; };

    spinner.start();
    spinner.stop();
    assert(output.includes("\r\u001B[K"));

    // Restore console.log
    console.log = originalLog;
  });
});
