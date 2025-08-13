#!/bin/bash
if [ "$1" = "" ]; then
	cat <<EOT
USAGE: ${APP_NAME} IP_ADDRESS

Ban an ip address using fail2ban apache-badbots jail

EOT
fi
sudo fail2ban-client set apache-badbots banip "$1" 
