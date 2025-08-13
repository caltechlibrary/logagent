#!/bin/bash

# Function to check IP bans
check_ip_bans() {
    # Read from standard input
    while IFS= read -r line; do
        # Split the line into IP and count
        ip=$(echo "$line" | cut -d'|' -f1)
        count=$(echo "$line" | cut -d'|' -f2)

        # Check if count is greater than 500, if so check ban status
        ban_status=""
        if [ "$count" -gt 500 ]; then
            # Check if the IP is banned using iptables
            banned=$(sudo iptables -L -n | grep "$ip")
            if [ -z "$banned" ]; then
                ban_status="|not banned"
            else
                ban_status="|banned"
            fi
        fi

        # Print the original line plus ban status if applicable
        echo "$line$ban_status"
    done
}

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

cat <<SQL | sqlite3 "${DB_NAME}" | check_ip_bans
WITH networks AS (
    SELECT 
      ip
    FROM
	${TABLE}
)
SELECT
    ip,
    COUNT(*) AS visits
FROM
    networks
GROUP BY
    ip
ORDER BY
    visits DESC
LIMIT 25;
SQL

