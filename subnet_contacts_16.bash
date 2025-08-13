#!/bin/bash

APP_NAME="$(basename "$0")"

if [ "$1" = "" ]; then
	cat <<EOT
USAGE: ${APP_NAME} SQLITE_DB

This script runs a SQL query to identify the networks that have
accessed the web service.

EXAMPLE

  ${APP_NAME} thesis_log.db

EOT
	exit 1
fi

DB_NAME="$1"
TABLE="$(basename "$DB_NAME" ".db")"

#logToSQL "${TABLE}" "${LOG_NAME}" | sqlite3 "${DB_NAME}"

cat <<SQL | sqlite3 "${DB_NAME}"
WITH networks AS (
    SELECT
        SUBSTR(ip, 1,
            INSTR(ip, '.') +
            INSTR(SUBSTR(ip, INSTR(ip, '.') + 1), '.')
                - 1) || '.0.0/16' AS subnet
    FROM
	${TABLE}
)
SELECT
    subnet,
    COUNT(*) AS visits
FROM
    networks
GROUP BY
    subnet
ORDER BY
    visits DESC;
SQL

