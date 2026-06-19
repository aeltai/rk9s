#!/usr/bin/env bash
# Kubernetes cluster status for Zellij pane
# Shows: context, namespace, node count, pod health
# Usage: Run this in a small Zellij pane above rk9s

set -e

# Colors (ANSI)
CYAN='\033[0;36m'
MAGENTA='\033[0;35m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
RED='\033[0;31m'
DIM='\033[0;90m'
BOLD='\033[1m'
NC='\033[0m'

print_status() {
    local ctx ns nodes pods_ok pods_bad

    ctx=$(kubectl config current-context 2>/dev/null || echo "none")
    ns=$(kubectl config view --minify -o jsonpath='{..namespace}' 2>/dev/null || echo "default")
    [ -z "$ns" ] && ns="default"

    nodes=$(kubectl get nodes --no-headers 2>/dev/null | wc -l | tr -d ' ')
    pods_ok=$(kubectl get pods -A --field-selector=status.phase=Running --no-headers 2>/dev/null | wc -l | tr -d ' ')
    pods_bad=$(kubectl get pods -A --field-selector=status.phase!=Running,status.phase!=Succeeded --no-headers 2>/dev/null | wc -l | tr -d ' ')

    # Clear line and print
    printf "\r\033[K"
    printf "${CYAN}⎈${NC} ${BOLD}%s${NC}" "$ctx"
    printf " ${DIM}│${NC} "
    printf "${MAGENTA}◈${NC} %s" "$ns"
    printf " ${DIM}│${NC} "
    printf "${GREEN}▣${NC} %s nodes" "$nodes"
    printf " ${DIM}│${NC} "
    printf "${GREEN}●${NC} %s pods" "$pods_ok"
    if [ "$pods_bad" -gt 0 ]; then
        printf " ${RED}◉${NC} %s" "$pods_bad"
    fi
    printf " ${DIM}│${NC} "
    printf "${DIM}%s${NC}" "$(date +%H:%M:%S)"
}

# Main loop
while true; do
    print_status
    sleep 5
done
