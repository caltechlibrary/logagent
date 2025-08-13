#!/bin/bash
cd $HOME/logagent || exit 1
export PATH="$HOME/bin:$PATH"
sudo tail --lines=40000 /var/log/apache2/thesis-ssl_access.log >access.log
sudo tail --lines=40000 /var/log/apache2/authors-ssl_access.log >>access.log
sudo $HOME/bin/logagent badbots.yaml access.log
