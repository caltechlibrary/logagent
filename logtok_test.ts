// Import the function to be tested
import { tokenize, tokenizeUnquote } from './logtok.ts';

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

Deno.test("tokenize requests", async (t) => {
    await t.step("tokenizeUnquote", () => {
        const testRequests: string[] = [ 
            '"GET /other HTTP/1.1"' 
        ];
        const expectedResults: string[][] = [
            [ "GET", "/other", "HTTP/1.1" ]
        ];
        for (let i = 0; i < testRequests.length; i++) {
            const result = tokenizeUnquote(testRequests[i], true);
            assertEquals(result, expectedResults[i], `expected ${JSON.stringify(result, null, 2)}, got ${JSON.stringify(expectedResults[i], null, 2)}`);
        }        
    });
});
