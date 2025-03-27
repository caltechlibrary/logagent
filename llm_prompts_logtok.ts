I am creating a TypeScript module called `logparse.ts`. It will run under Deno 2.2. It has an exported function called `parseLogLine`. The function takes two parameters. The first is `text` of type `string` this is the line to be parsed. The second is `verbose` of type `boolean`. If `verbose` is true then the tokenization process should display each token found.

Tokens are delimited by a space character or the end of the `text` string.  A token starts with a non-space character.
If the token starts with a `[` then the token ends with a `]` is encountered.  If a token starts with a `"` then it ends when a matching `"` is encountered. In this type of token a `"` can be escaped if it is proceeded by a single `\` character.

You should also create a file named `logparse_test.ts` which uses `Deno.test` to test for correctness and use asserts from the `@std/assert` module.

---

Change the line

```typescript
import { assertEquals } from "https://deno.land/std@0.192.0/assert/mod.ts";
```

To

```typescript
import { assertEquals } from "@std/assert";
```

---

Rename the function `parseLogLine` to `parse`. 

---

Add a test that uses the following lines to be parsed.

```
192.168.1.1 - - [01/Jan/2023:00:00:01 +0000] "GET /file HTTP/1.1" 200 1024 "-" "agent1"
192.168.1.2 - - [01/Jan/2023:01:00:02 +0000] "GET /file HTTP/1.1" 200 1024 "-" "agent2"
192.168.1.1 - - [01/Jan/2023:00:00:03 +0000] "GET /other HTTP/1.1" 200 1024 "-" "agent1"
.43.120.29 - - [25/Mar/2025:15:50:00 +0000] "GET /cgi/export/eprint/16341/RDFXML/caltechthesis-eprint-16341.rdf HTTP/1.1" 500 624
192.43.120.29 - - [25/Mar/2025:15:56:07 +0000] "GET /cgi/export/eprint/8736/RDFXML/caltechthesis-eprint-8736.rdf HTTP/1.1" 500 624
192.43.120.29 - - [25/Mar/2025:17:01:55 +0000] "GET /cgi/export/eprint/14094/RDFXML/caltechthesis-eprint-14094.rdf HTTP/1.1" 500 624
192.210.147.121 - - [25/Mar/2025:00:00:14 +0000] "GET /records/dn9cj-ehd72/files/knee_train_lmdb.zip?download=1 HTTP/1.1" 302 480 "-" "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/64.0.3608.7 Safari/537.36"
192.249.74.136 - - [25/Mar/2025:00:00:20 +0000] "GET /records/rd2f5-dwc23/files/Lev2:metadata.json?download=1 HTTP/1.1" 302 473 "-" "Mozilla/5.0 (Linux; Android 6.0.1; Nexus 5X Build/MMB29P) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/134.0.6998.165 Mobile Safari/537.36 (compatible; GoogleOther)"
192.242.234.100 - - [25/Mar/2025:00:00:25 +0000] "HEAD /records/dn9cj-ehd72/files/knee_train_lmdb.zip?download=1 HTTP/1.1" 302 0 "-" "Mozilla/5.0 (Windows NT 6.1; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/75.0.2514.118 Safari/537.36"
192.242.234.100 - - [25/Mar/2025:00:00:26 +0000] "GET /records/dn9cj-ehd72/files/knee_train_lmdb.zip?download=1 HTTP/1.1" 302 480 "-" "Mozilla/5.0 (Windows NT 6.1; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/75.0.2514.118 Safari/537.36"
192.210.12.248 - - [25/Mar/2025:00:00:44 +0000] "HEAD /records/dn9cj-ehd72/files/knee_train_lmdb.zip?download=1 HTTP/1.1" 302 0 "-" "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/64.0.3769.163 Safari/537.36"
192.210.12.248 - - [25/Mar/2025:00:00:44 +0000] "GET /records/dn9cj-ehd72/files/knee_train_lmdb.zip?download=1 HTTP/1.1" 302 480 "-" "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/64.0.3769.163 Safari/537.36"
192.89.148.175 - - [25/Mar/2025:00:00:50 +0000] "GET /records/fpwk2-f1188/preview/20200820_3_YFP0003.tif HTTP/1.1" 200 385 "-" "Mozilla/5.0 (X11; U; Linux i686; ru; rv:1.9.1.3) Gecko/20091020 Ubuntu/10.04 (lucid) Firefox/4.0.1"
292.86.79.235 - - [25/Mar/2025:00:00:54 +0000] "GET /records/mzrjq-6wc02 HTTP/1.1" 200 10053 "-" "lychee/0.18.1"
192.86.79.235 - - [25/Mar/2025:00:00:54 +0000] "GET /records/nyy15-4j048 HTTP/1.1" 200 10008 "-" "lychee/0.18.1"
192.210.176.195 - - [25/Mar/2025:00:00:55 +0000] "HEAD /records/dn9cj-ehd72/files/knee_train_lmdb.zip?download=1 HTTP/1.1" 302 0 "-" "Mozilla/5.0 (Windows NT 6.1; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/78.0.3649.113 Safari/537.36"
```

---

Rename the `parse` function to `tokenize`.

---

In `logparse_test.ts` the following is not correct.

```typescript
        const expectedResults = [
            ['192.168.1.1', '-', '-', '[01/Jan/2023:00:00:01', '+0000]', '"GET', '/file', 'HTTP/1.1"', '200', '1024', '"-"', '"agent1"'],
            ['192.168.1.2', '-', '-', '[01/Jan/2023:01:00:02', '+0000]', '"GET', '/file', 'HTTP/1.1"', '200', '1024', '"-"', '"agent2"'],
            ['192.168.1.1', '-', '-', '[01/Jan/2023:00:00:03', '+0000]', '"GET', '/other', 'HTTP/1.1"', '200', '1024', '"-"', '"agent1"'],
            ['.43.120.29', '-', '-', '[25/Mar/2025:15:50:00', '+0000]', '"GET', '/cgi/export/eprint/16341/RDFXML/caltechthesis-eprint-16341.rdf', 'HTTP/1.1"', '500', '624'],
            ['192.43.120.29', '-', '-', '[25/Mar/2025:15:56:07', '+0000]', '"GET', '/cgi/export/eprint/8736/RDFXML/caltechthesis-eprint-8736.rdf', 'HTTP/1.1"', '500', '624'],
            ['192.43.120.29', '-', '-', '[25/Mar/2025:17:01:55', '+0000]', '"GET', '/cgi/export/eprint/14094/RDFXML/caltechthesis-eprint-14094.rdf', 'HTTP/1.1"', '500', '624'],
            ['192.210.147.121', '-', '-', '[25/Mar/2025:00:00:14', '+0000]', '"GET', '/records/dn9cj-ehd72/files/knee_train_lmdb.zip?download=1', 'HTTP/1.1"', '302', '480', '"-"', '"Mozilla/5.0', '(Windows', 'NT', '10.0;', 'Win64;', 'x64)', 'AppleWebKit/537.36', '(KHTML,', 'like', 'Gecko)', 'Chrome/64.0.3608.7', 'Safari/537.36"'],
            ['192.249.74.136', '-', '-', '[25/Mar/2025:00:00:20', '+0000]', '"GET', '/records/rd2f5-dwc23/files/Lev2:metadata.json?download=1', 'HTTP/1.1"', '302', '473', '"-"', '"Mozilla/5.0', '(Linux;', 'Android', '6.0.1;', 'Nexus', '5X', 'Build/MMB29P)', 'AppleWebKit/537.36', '(KHTML,', 'like', 'Gecko)', 'Chrome/134.0.6998.165', 'Mobile', 'Safari/537.36', '(compatible;', 'GoogleOther)"'],
            ['192.242.234.100', '-', '-', '[25/Mar/2025:00:00:25', '+0000]', '"HEAD', '/records/dn9cj-ehd72/files/knee_train_lmdb.zip?download=1', 'HTTP/1.1"', '302', '0', '"-"', '"Mozilla/5.0', '(Windows', 'NT', '6.1;', 'Win64;', 'x64)', 'AppleWebKit/537.36', '(KHTML,', 'like', 'Gecko)', 'Chrome/75.0.2514.118', 'Safari/537.36"'],
            ['192.242.234.100', '-', '-', '[25/Mar/2025:00:00:26', '+0000]', '"GET', '/records/dn9cj-ehd72/files/knee_train_lmdb.zip?download=1', 'HTTP/1.1"', '302', '480', '"-"', '"Mozilla/5.0', '(Windows', 'NT', '6.1;', 'Win64;', 'x64)', 'AppleWebKit/537.36', '(KHTML,', 'like', 'Gecko)', 'Chrome/75.0.2514.118', 'Safari/537.36"'],
            ['192.210.12.248', '-', '-', '[25/Mar/2025:00:00:44', '+0000]', '"HEAD', '/records/dn9cj-ehd72/files/knee_train_lmdb.zip?download=1', 'HTTP/1.1"', '302', '0', '"-"', '"Mozilla/5.0', '(Windows', 'NT', '10.0;', 'Win64;', 'x64)', 'AppleWebKit/537.36', '(KHTML,', 'like', 'Gecko)', 'Chrome/64.0.3769.163', 'Safari/537.36"'],
            ['192.210.12.248', '-', '-', '[25/Mar/2025:00:00:44', '+0000]', '"GET', '/records/dn9cj-ehd72/files/knee_train_lmdb.zip?download=1', 'HTTP/1.1"', '302', '480', '"-"', '"Mozilla/5.0', '(Windows', 'NT', '10.0;', 'Win64;', 'x64)', 'AppleWebKit/537.36', '(KHTML,', 'like', 'Gecko)', 'Chrome/64.0.3769.163', 'Safari/537.36"'],
            ['192.89.148.175', '-', '-', '[25/Mar/2025:00:00:50', '+0000]', '"GET', '/records/fpwk2-f1188/preview/20200820_3_YFP0003.tif', 'HTTP/1.1"', '200', '385', '"-"', '"Mozilla/5.0', '(X11;', 'U;', 'Linux', 'i686;', 'ru;', 'rv:1.9.1.3)', 'Gecko/20091020', 'Ubuntu/10.04', '(lucid)', 'Firefox/4.0.1"'],
            ['292.86.79.235', '-', '-', '[25/Mar/2025:00:00:54', '+0000]', '"GET', '/records/mzrjq-6wc02', 'HTTP/1.1"', '200', '10053', '"-"', '"lychee/0.18.1"'],
            ['192.86.79.235', '-', '-', '[25/Mar/2025:00:00:54', '+0000]', '"GET', '/records/nyy15-4j048', 'HTTP/1.1"', '200', '10008', '"-"', '"lychee/0.18.1"'],
            ['192.210.176.195', '-', '-', '[25/Mar/2025:00:00:55', '+0000]', '"HEAD', '/records/dn9cj-ehd72/files/knee_train_lmdb.zip?download=1', 'HTTP/1.1"', '302', '0', '"-"', '"Mozilla/5.0', '(Windows', 'NT', '6.1;', 'Win64;', 'x64)', 'AppleWebKit/537.36', '(KHTML,', 'like', 'Gecko)', 'Chrome/78.0.3649.113', 'Safari/537.36"']
        ];
```

It should be

```typescript
        const expectedResults = [
            ['192.168.1.1', '-', '-', '[01/Jan/2023:00:00:01 +0000]', '"GET', '/file', 'HTTP/1.1"', '200', '1024', '"-"', '"agent1"'],
            ['192.168.1.2', '-', '-', '[01/Jan/2023:01:00:02 +0000]', '"GET', '/file', 'HTTP/1.1"', '200', '1024', '"-"', '"agent2"'],
            ['192.168.1.1', '-', '-', '[01/Jan/2023:00:00:03 +0000]', '"GET', '/other', 'HTTP/1.1"', '200', '1024', '"-"', '"agent1"'],
            ['.43.120.29', '-', '-', '[25/Mar/2025:15:50:00 +0000]', '"GET', '/cgi/export/eprint/16341/RDFXML/caltechthesis-eprint-16341.rdf', 'HTTP/1.1"', '500', '624'],
            ['192.43.120.29', '-', '-', '[25/Mar/2025:15:56:07 +0000]', '"GET', '/cgi/export/eprint/8736/RDFXML/caltechthesis-eprint-8736.rdf', 'HTTP/1.1"', '500', '624'],
            ['192.43.120.29', '-', '-', '[25/Mar/2025:17:01:55 +0000]', '"GET', '/cgi/export/eprint/14094/RDFXML/caltechthesis-eprint-14094.rdf', 'HTTP/1.1"', '500', '624'],
            ['192.210.147.121', '-', '-', '[25/Mar/2025:00:00:14 +0000]', '"GET', '/records/dn9cj-ehd72/files/knee_train_lmdb.zip?download=1', 'HTTP/1.1"', '302', '480', '"-"', '"Mozilla/5.0', '(Windows', 'NT', '10.0;', 'Win64;', 'x64)', 'AppleWebKit/537.36', '(KHTML,', 'like', 'Gecko)', 'Chrome/64.0.3608.7', 'Safari/537.36"'],
            ['192.249.74.136', '-', '-', '[25/Mar/2025:00:00:20 +0000]', '"GET', '/records/rd2f5-dwc23/files/Lev2:metadata.json?download=1', 'HTTP/1.1"', '302', '473', '"-"', '"Mozilla/5.0', '(Linux;', 'Android', '6.0.1;', 'Nexus', '5X', 'Build/MMB29P)', 'AppleWebKit/537.36', '(KHTML,', 'like', 'Gecko)', 'Chrome/134.0.6998.165', 'Mobile', 'Safari/537.36', '(compatible;', 'GoogleOther)"'],
            ['192.242.234.100', '-', '-', '[25/Mar/2025:00:00:25 +0000]', '"HEAD', '/records/dn9cj-ehd72/files/knee_train_lmdb.zip?download=1', 'HTTP/1.1"', '302', '0', '"-"', '"Mozilla/5.0', '(Windows', 'NT', '6.1;', 'Win64;', 'x64)', 'AppleWebKit/537.36', '(KHTML,', 'like', 'Gecko)', 'Chrome/75.0.2514.118', 'Safari/537.36"'],
            ['192.242.234.100', '-', '-', '[25/Mar/2025:00:00:26 +0000]', '"GET', '/records/dn9cj-ehd72/files/knee_train_lmdb.zip?download=1', 'HTTP/1.1"', '302', '480', '"-"', '"Mozilla/5.0', '(Windows', 'NT', '6.1;', 'Win64;', 'x64)', 'AppleWebKit/537.36', '(KHTML,', 'like', 'Gecko)', 'Chrome/75.0.2514.118', 'Safari/537.36"'],
            ['192.210.12.248', '-', '-', '[25/Mar/2025:00:00:44 +0000]', '"HEAD', '/records/dn9cj-ehd72/files/knee_train_lmdb.zip?download=1', 'HTTP/1.1"', '302', '0', '"-"', '"Mozilla/5.0', '(Windows', 'NT', '10.0;', 'Win64;', 'x64)', 'AppleWebKit/537.36', '(KHTML,', 'like', 'Gecko)', 'Chrome/64.0.3769.163', 'Safari/537.36"'],
            ['192.210.12.248', '-', '-', '[25/Mar/2025:00:00:44 +0000]', '"GET', '/records/dn9cj-ehd72/files/knee_train_lmdb.zip?download=1', 'HTTP/1.1"', '302', '480', '"-"', '"Mozilla/5.0', '(Windows', 'NT', '10.0;', 'Win64;', 'x64)', 'AppleWebKit/537.36', '(KHTML,', 'like', 'Gecko)', 'Chrome/64.0.3769.163', 'Safari/537.36"'],
            ['192.89.148.175', '-', '-', '[25/Mar/2025:00:00:50 +0000]', '"GET', '/records/fpwk2-f1188/preview/20200820_3_YFP0003.tif', 'HTTP/1.1"', '200', '385', '"-"', '"Mozilla/5.0', '(X11;', 'U;', 'Linux', 'i686;', 'ru;', 'rv:1.9.1.3)', 'Gecko/20091020', 'Ubuntu/10.04', '(lucid)', 'Firefox/4.0.1"'],
            ['292.86.79.235', '-', '-', '[25/Mar/2025:00:00:54 +0000]', '"GET', '/records/mzrjq-6wc02', 'HTTP/1.1"', '200', '10053', '"-"', '"lychee/0.18.1"'],
            ['192.86.79.235', '-', '-', '[25/Mar/2025:00:00:54 +0000]', '"GET', '/records/nyy15-4j048', 'HTTP/1.1"', '200', '10008', '"-"', '"lychee/0.18.1"'],
            ['192.210.176.195', '-', '-', '[25/Mar/2025:00:00:55 +0000]', '"HEAD', '/records/dn9cj-ehd72/files/knee_train_lmdb.zip?download=1', 'HTTP/1.1"', '302', '0', '"-"', '"Mozilla/5.0', '(Windows', 'NT', '6.1;', 'Win64;', 'x64)', 'AppleWebKit/537.36', '(KHTML,', 'like', 'Gecko)', 'Chrome/78.0.3649.113', 'Safari/537.36"']
        ];
```

---

In the `expectedResults` array elements like

```
            ['192.168.1.1', '-', '-', '[01/Jan/2023:00:00:01 +0000]', '"GET', '/file', 'HTTP/1.1"', '200', '1024', '"-"', '"agent1"'],
```

should me

```
            ['192.168.1.1', '-', '-', '[01/Jan/2023:00:00:01 +0000]', '"GET /file HTTP/1.1"', '200', '1024', '"-"', '"agent1"'],
```

---

When I run `deno test logparse_test.ts` I get the following error

```
tokenize ... log lines => ./logparse_test.ts:43:13
error: AssertionError: Values are not equal.


    [Diff] Actual / Expected


    [
      "192.168.1.1",
      "-",
      "-",
      "[01/Jan/2023:00:00:01 +0000]",
+     '"GET',
+     "/file",
+     'HTTP/1.1"',
-     '"GET /file HTTP/1.1"',
      "200",
      "1024",
      '"-"',
      '"agent1"',
    ]

  throw new AssertionError(message);
        ^
    at assertEquals (https://jsr.io/@std/assert/1.0.11/equals.ts:64:9)
    at file:///Users/rsdoiel/src/github.com/caltechlibrary/logagent/logparse_test.ts:84:13
    at innerWrapped (ext:cli/40_test.js:180:11)
    at exitSanitizer (ext:cli/40_test.js:96:33)
    at Object.outerWrapped [as fn] (ext:cli/40_test.js:123:20)
    at TestContext.step (ext:cli/40_test.js:481:37)
    at file:///Users/rsdoiel/src/github.com/caltechlibrary/logagent/logparse_test.ts:43:13
```

---

Use the following for `logparse_test.ts`.

```typescript
// Import the function to be tested
import { tokenize } from './logparse.ts';

// Import assertions from the standard library
import { assertEquals } from "@std/assert";

// Test suite for tokenize function
Deno.test("tokenize", async (t) => {
    await t.step("simple tokens", () => {
        const result = tokenize("token1 token2 token3", false);
        assertEquals(result, ["token1", "token2", "token3"]);
    });

    await t.step("tokens with brackets", () => {
        const result = tokenize("[token1] [token2] token3", false);
        assertEquals(result, ["[token1]", "[token2]", "token3"]);
    });

    await t.step("tokens with quotes", () => {
        const result = tokenize("\"token1\" \"token2\" token3", false);
        assertEquals(result, ["\"token1\"", "\"token2\"", "token3"]);
    });

    await t.step("tokens with escaped quotes", () => {
        const result = tokenize("\"token1\\\"token2\" token3", false);
        assertEquals(result, ["\"token1\\\"token2\"", "token3"]);
    });

    await t.step("verbose mode", () => {
        // Redirect console.log to capture verbose output
        const logOutput: string[] = [];
        const originalLog = console.log;
        console.log = (message) => logOutput.push(message);

        const result = tokenize("token1 token2", true);
        assertEquals(result, ["token1", "token2"]);
        assertEquals(logOutput, ["Token found: token1", "Token found: token2"]);

        // Restore original console.log
        console.log = originalLog;
    });

    await t.step("log lines", () => {
        const logLines = [
            '192.168.1.1 - - [01/Jan/2023:00:00:01 +0000] "GET /file HTTP/1.1" 200 1024 "-" "agent1"',
            '192.168.1.2 - - [01/Jan/2023:01:00:02 +0000] "GET /file HTTP/1.1" 200 1024 "-" "agent2"',
            '192.168.1.1 - - [01/Jan/2023:00:00:03 +0000] "GET /other HTTP/1.1" 200 1024 "-" "agent1"',
            '192.43.120.29 - - [25/Mar/2025:15:50:00 +0000] "GET /cgi/export/eprint/16341/RDFXML/caltechthesis-eprint-16341.rdf HTTP/1.1" 500 624',
            '192.43.120.29 - - [25/Mar/2025:15:56:07 +0000] "GET /cgi/export/eprint/8736/RDFXML/caltechthesis-eprint-8736.rdf HTTP/1.1" 500 624',
            '192.43.120.29 - - [25/Mar/2025:17:01:55 +0000] "GET /cgi/export/eprint/14094/RDFXML/caltechthesis-eprint-14094.rdf HTTP/1.1" 500 624',
            '192.210.147.121 - - [25/Mar/2025:00:00:14 +0000] "GET /records/dn9cj-ehd72/files/knee_train_lmdb.zip?download=1 HTTP/1.1" 302 480 "-" "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/64.0.3608.7 Safari/537.36"',
            '192.249.74.136 - - [25/Mar/2025:00:00:20 +0000] "GET /records/rd2f5-dwc23/files/Lev2:metadata.json?download=1 HTTP/1.1" 302 473 "-" "Mozilla/5.0 (Linux; Android 6.0.1; Nexus 5X Build/MMB29P) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/134.0.6998.165 Mobile Safari/537.36 (compatible; GoogleOther)"',
            '192.242.234.100 - - [25/Mar/2025:00:00:25 +0000] "HEAD /records/dn9cj-ehd72/files/knee_train_lmdb.zip?download=1 HTTP/1.1" 302 0 "-" "Mozilla/5.0 (Windows NT 6.1; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/75.0.2514.118 Safari/537.36"',
            '192.242.234.100 - - [25/Mar/2025:00:00:26 +0000] "GET /records/dn9cj-ehd72/files/knee_train_lmdb.zip?download=1 HTTP/1.1" 302 480 "-" "Mozilla/5.0 (Windows NT 6.1; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/75.0.2514.118 Safari/537.36"',
            '192.210.12.248 - - [25/Mar/2025:00:00:44 +0000] "HEAD /records/dn9cj-ehd72/files/knee_train_lmdb.zip?download=1 HTTP/1.1" 302 0 "-" "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/64.0.3769.163 Safari/537.36"',
            '192.210.12.248 - - [25/Mar/2025:00:00:44 +0000] "GET /records/dn9cj-ehd72/files/knee_train_lmdb.zip?download=1 HTTP/1.1" 302 480 "-" "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/64.0.3769.163 Safari/537.36"',
            '192.89.148.175 - - [25/Mar/2025:00:00:50 +0000] "GET /records/fpwk2-f1188/preview/20200820_3_YFP0003.tif HTTP/1.1" 200 385 "-" "Mozilla/5.0 (X11; U; Linux i686; ru; rv:1.9.1.3) Gecko/20091020 Ubuntu/10.04 (lucid) Firefox/4.0.1"',
            '192.86.79.235 - - [25/Mar/2025:00:00:54 +0000] "GET /records/mzrjq-6wc02 HTTP/1.1" 200 10053 "-" "lychee/0.18.1"',
            '192.86.79.235 - - [25/Mar/2025:00:00:54 +0000] "GET /records/nyy15-4j048 HTTP/1.1" 200 10008 "-" "lychee/0.18.1"',
            '192.210.176.195 - - [25/Mar/2025:00:00:55 +0000] "HEAD /records/dn9cj-ehd72/files/knee_train_lmdb.zip?download=1 HTTP/1.1" 302 0 "-" "Mozilla/5.0 (Windows NT 6.1; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/78.0.3649.113 Safari/537.36"'
        ];

        const expectedResults = [
          ['192.168.1.1', '-', '-', '[01/Jan/2023:00:00:01 +0000]', '"GET /file HTTP/1.1"', '200', '1024', '"-"', '"agent1"'],
          ['192.168.1.2', '-', '-', '[01/Jan/2023:01:00:02 +0000]', '"GET /file HTTP/1.1"', '200', '1024', '"-"', '"agent2"'],
          ['192.168.1.1', '-', '-', '[01/Jan/2023:00:00:03 +0000]', '"GET /other HTTP/1.1"', '200', '1024', '"-"', '"agent1"'],
          ['192.43.120.29', '-', '-', '[25/Mar/2025:15:50:00 +0000]', '"GET /cgi/export/eprint/16341/RDFXML/caltechthesis-eprint-16341.rdf HTTP/1.1"', '500', '624'],
          ['192.43.120.29', '-', '-', '[25/Mar/2025:15:56:07 +0000]', '"GET /cgi/export/eprint/8736/RDFXML/caltechthesis-eprint-8736.rdf HTTP/1.1"', '500', '624'],
          ['192.43.120.29', '-', '-', '[25/Mar/2025:17:01:55 +0000]', '"GET /cgi/export/eprint/14094/RDFXML/caltechthesis-eprint-14094.rdf HTTP/1.1"', '500', '624'],
          ['192.210.147.121', '-', '-', '[25/Mar/2025:00:00:14 +0000]', '"GET /records/dn9cj-ehd72/files/knee_train_lmdb.zip?download=1 HTTP/1.1"', '302', '480', '"-"', '"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/64.0.3608.7 Safari/537.36"'],
          ['192.249.74.136', '-', '-', '[25/Mar/2025:00:00:20 +0000]', '"GET /records/rd2f5-dwc23/files/Lev2:metadata.json?download=1 HTTP/1.1"', '302', '473', '"-"', '"Mozilla/5.0 (Linux; Android 6.0.1; Nexus 5X Build/MMB29P) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/134.0.6998.165 Mobile Safari/537.36 (compatible; GoogleOther)"'],
          ['192.242.234.100', '-', '-', '[25/Mar/2025:00:00:25 +0000]', '"HEAD /records/dn9cj-ehd72/files/knee_train_lmdb.zip?download=1 HTTP/1.1"', '302', '0', '"-"', '"Mozilla/5.0 (Windows NT 6.1; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/75.0.2514.118 Safari/537.36"'],
          ['192.242.234.100', '-', '-', '[25/Mar/2025:00:00:26 +0000]', '"GET /records/dn9cj-ehd72/files/knee_train_lmdb.zip?download=1 HTTP/1.1"', '302', '480', '"-"', '"Mozilla/5.0 (Windows NT 6.1; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/75.0.2514.118 Safari/537.36"'],
          ['192.210.12.248', '-', '-', '[25/Mar/2025:00:00:44 +0000]', '"HEAD /records/dn9cj-ehd72/files/knee_train_lmdb.zip?download=1 HTTP/1.1"', '302', '0', '"-"', '"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/64.0.3769.163 Safari/537.36"'],
          ['192.210.12.248', '-', '-', '[25/Mar/2025:00:00:44 +0000]', '"GET /records/dn9cj-ehd72/files/knee_train_lmdb.zip?download=1 HTTP/1.1"', '302', '480', '"-"', '"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/64.0.3769.163 Safari/537.36"'],
          ['192.89.148.175', '-', '-', '[25/Mar/2025:00:00:50 +0000]', '"GET /records/fpwk2-f1188/preview/20200820_3_YFP0003.tif HTTP/1.1"', '200', '385', '"-"', '"Mozilla/5.0 (X11; U; Linux i686; ru; rv:1.9.1.3) Gecko/20091020 Ubuntu/10.04 (lucid) Firefox/4.0.1"'],
          ['192.86.79.235', '-', '-', '[25/Mar/2025:00:00:54 +0000]', '"GET /records/mzrjq-6wc02 HTTP/1.1"', '200', '10053', '"-"', '"lychee/0.18.1"'],
          ['192.86.79.235', '-', '-', '[25/Mar/2025:00:00:54 +0000]', '"GET /records/nyy15-4j048 HTTP/1.1"', '200', '10008', '"-"', '"lychee/0.18.1"'],
          ['192.210.176.195', '-', '-', '[25/Mar/2025:00:00:55 +0000]', '"HEAD /records/dn9cj-ehd72/files/knee_train_lmdb.zip?download=1 HTTP/1.1"', '302', '0', '"-"', '"Mozilla/5.0 (Windows NT 6.1; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/78.0.3649.113 Safari/537.36"']
      ];
      
        for (let i = 0; i < logLines.length; i++) {
            const result = tokenize(logLines[i], false);
            assertEquals(result, expectedResults[i], `expected ${JSON.stringify(result, null, 2)}, got ${JSON.stringify(expectedResults[i], null, 2)}`);
        }
    });
});
```

---

Use the following code for `logparse.ts`

```typescript
// Function to parse a log line into tokens
function tokenize(text: string, verbose: boolean): string[] {
    const tokens: string[] = [];
    let i = 0;
    const len = text.length;

    while (i < len) {
        // Skip leading spaces
        while (i < len && text[i] === ' ') {
            i++;
        }
        if (i >= len) {
            break;
        }

        // Determine the type of token
        if (text[i] === '[') {
            // Token is enclosed in square brackets
            let end = i + 1;
            while (end < len && text[end] !== ']') {
                end++;
            }
            if (end < len) {
                tokens.push(text.slice(i, end + 1));
                i = end + 1;
            }
        } else if (text[i] === '"') {
            // Token is enclosed in double quotes
            let end = i + 1;
            while (end < len) {
                // Skip escaped characters
                if (text[end] === '\\') {
                    end += 2;
                    if (end >= len) {
                        break;
                    }
                }
                if (text[end] === '"') {
                    break;
                } else {
                    end++;
                }
            }
            tokens.push(text.slice(i, end + 1));
            i = end + 1;
        } else {
            // Token is a regular word
            let end = i + 1;
            while (end < len && text[end] !== ' ') {
                end++;
            }
            tokens.push(text.slice(i, end));
            i = end;
        }

        // Log the token if verbose is true
        if (verbose) {
            console.log(\`Token found: ${tokens[tokens.length - 1]}\`);
        }
    }

    return tokens;
}

// Export the function
export { tokenize };
```

---

Rename `logparse.ts` to `logtok.ts` and rename `logparse_test.ts` to `logtok_test.ts`.
