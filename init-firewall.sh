#!/bin/bash
set -euo pipefail

# iptables-based network allowlist for the dev container.
# Replaces the squid proxy approach with hard isolation that cannot
# be bypassed by unsetting env vars or using raw sockets.

# Save Docker DNS rules (127.0.0.11) before flushing
DOCKER_DNS_RULES=$(iptables-save -t nat | grep "127\.0\.0\.11" || true)

# Flush existing rules
iptables -F
iptables -X
iptables -t nat -F
iptables -t nat -X
iptables -t mangle -F
iptables -t mangle -X
ipset destroy allowed-domains 2>/dev/null || true

# Restore Docker internal DNS resolution
if [ -n "$DOCKER_DNS_RULES" ]; then
    iptables -t nat -N DOCKER_OUTPUT 2>/dev/null || true
    iptables -t nat -N DOCKER_POSTROUTING 2>/dev/null || true
    echo "$DOCKER_DNS_RULES" | xargs -L 1 iptables -t nat
fi

# Allow DNS
iptables -A OUTPUT -p udp --dport 53 -j ACCEPT
iptables -A INPUT -p udp --sport 53 -j ACCEPT

# Allow localhost
iptables -A INPUT -i lo -j ACCEPT
iptables -A OUTPUT -o lo -j ACCEPT

# Allow private networks (PurpleAir sensors, Docker host, etc.)
for net in 10.0.0.0/8 172.16.0.0/12 192.168.0.0/16; do
    iptables -A OUTPUT -d "$net" -j ACCEPT
    iptables -A INPUT -s "$net" -j ACCEPT
done

# Create ipset for allowed internet IPs
ipset create allowed-domains hash:net

# Resolve and add allowed domains
ALLOWED_DOMAINS=(
    # App APIs
    home.sensibo.com
    # Anthropic / Claude
    api.anthropic.com
    claude.ai
    claude.com
    statsig.anthropic.com
    # OpenAI / ChatGPT
    api.openai.com
    chatgpt.com
    # Feature flags and telemetry
    statsig.com
    events.statsig.com
    api.datadoghq.com
)

for domain in "${ALLOWED_DOMAINS[@]}"; do
    ips=$(dig +noall +answer A "$domain" 2>/dev/null | awk '$4 == "A" {print $5}')
    if [ -n "$ips" ]; then
        while read -r ip; do
            if [[ "$ip" =~ ^[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}$ ]]; then
                ipset add allowed-domains "$ip" 2>/dev/null || true
                echo "  $domain -> $ip"
            fi
        done <<< "$ips"
    else
        echo "  WARNING: could not resolve $domain"
    fi
done

# Set default DROP policies
iptables -P INPUT DROP
iptables -P FORWARD DROP
iptables -P OUTPUT DROP

# Allow established connections
iptables -A INPUT -m state --state ESTABLISHED,RELATED -j ACCEPT
iptables -A OUTPUT -m state --state ESTABLISHED,RELATED -j ACCEPT

# Allow only traffic to resolved allowed IPs
iptables -A OUTPUT -m set --match-set allowed-domains dst -j ACCEPT

# Reject everything else with immediate feedback (not silent drop)
iptables -A OUTPUT -j REJECT --reject-with icmp-admin-prohibited

echo "Firewall configured (${#ALLOWED_DOMAINS[@]} domains resolved)"
