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
            console.log(`Token found: ${tokens[tokens.length - 1]}`);
        }
    }

    return tokens;
}

// tokenizeUnquote takes a string and unquotes it if necessary before
// calleding tokenize to tokenize the string.
function tokenizeUnquote(text: string, verbose: boolean): string [] {
    let tokens: string[] = [];
    if (text.startsWith('"') && text.endsWith('"')) {
        tokens = tokenize(text.slice(1,-1), verbose);
    } else {
        tokens = tokenize(text, verbose);
    }
    return tokens;
}

// Export the function
export { tokenize, tokenizeUnquote };
