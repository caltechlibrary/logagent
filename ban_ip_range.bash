#!/bin/bash


#
# This is a script to make it easy to mange ranges of IP address.
# 
# The value after the slash in an IP subnet is called the subnet mask or prefix length. It represents the number of bits in the IP address that define the network portion.
# 
# Here's how to determine the subnet mask value:
# 
# 1. For IPv4 addresses, the standard subnet masks are:
# 
# - /8 - Blocks a Class A network (e.g. 10.0.0.0/8 blocks 10.0.0.0 - 10.255.255.255)
# - /16 - Blocks a Class B network (e.g. 172.16.0.0/16 blocks 172.16.0.0 - 172.16.255.255) 
# - /24 - Blocks a Class C network (e.g. 192.168.1.0/24 blocks 192.168.1.0 - 192.168.1.255)
# 
# 2. The subnet mask value corresponds to the number of 1 bits in the subnet mask. For example:
# 
# - /24 means the subnet mask is 255.255.255.0 (24 one bits)
# - /16 means the subnet mask is 255.255.0.0 (16 one bits)
# - /8 means the subnet mask is 255.0.0.0 (8 one bits)
# 
# 3. The higher the subnet mask value, the smaller the subnet. /24 blocks 256 IP addresses, /16 blocks 65,536 IP addresses, and /8 blocks over 16 million IP addresses.
# 
# So in summary, the value after the slash in an IP subnet represents the number of network bits, which determines how many IP addresses are included in that subnet. This allows you to block an entire range of IP addresses with a single iptables rule.
# 


#sudo iptables -A INPUT -s "${1}" -j DROP
sudo ufw deny from "${1}"
