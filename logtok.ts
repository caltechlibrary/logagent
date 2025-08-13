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

// Improved tokenizeUnquote function
function tokenizeUnquote(text: string, verbose: boolean): string[] {
    // If the text is enclosed in quotes, remove them
    if (text.startsWith('"') && text.endsWith('"')) {
        const unquoted = text.slice(1, -1);
        // Special case: if this looks like an HTTP request line, split on first space
        const requestMatch = unquoted.match(/^([A-Z]+)\s+([^\s]+)/);
        if (requestMatch) {
            // Split into method and path
            return [requestMatch[1], requestMatch[2]];
        } else {
            // Otherwise, just return the unquoted text as a single token
            return [unquoted];
        }
    } else {
        // Not quoted, so just tokenize as before
        return tokenize(text, verbose);
    }
}

// Export the functions
export { tokenize, tokenizeUnquote };
