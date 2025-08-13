#!/bin/bash

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
