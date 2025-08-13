#!/bin/bash
cd $HOME/logagent || exit 1
export PATH="$HOME/bin:$PATH"
sudo $HOME/bin/logagent badbots.yaml /var/log/apache2/thesis-ssl_access.log
sudo $HOME/bin/logagent badbots.yaml /var/log/apache2/authors-ssl_access.log
