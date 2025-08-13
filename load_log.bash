#!/bin/bash

APP_NAME="$(basename "$0")"

if [ "$1" = "" ] || [ "$2" = "" ]; then
	cat <<EOT
USAGE: ${APP_NAME} SQLITE_DB LOG_FILE

This script uses "logToSQL" that translate the EPrints log into
a SQLite3 database and table. 

EXAMPLE

  ${APP_NAME} thesis_log.db /var/log/apache2/thesis-ssl_access.log

EOT
	exit 1
fi

DB_NAME="$1"
LOG_NAME="$2"
TABLE="$(basename "$DB_NAME" ".db")"

logToSQL "${TABLE}" "${LOG_NAME}" | sqlite3 "${DB_NAME}"

### cat <<SQL | sqlite3 "$1"
### SELECT
###     SUBSTR(ip, 1, INSTR(SUBSTR(ip, 1, INSTR(ip, '.', 1, 2) + 1), '.', 1, 2) - 1) || '.0.0/16' AS network,
###     COUNT(*) AS visits
### FROM
###     ${TABLE}
### GROUP BY
###     network
### ORDER BY
###     visits DESC;
### SQL
### 
