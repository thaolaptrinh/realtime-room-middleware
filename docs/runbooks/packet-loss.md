# Runbook: Packet Loss / UDP Connectivity

> Placeholder.

## Symptoms

- KCP retransmits high
- Clients report lag or desync

## Actions

1. Confirm firewall allows UDP :9000.
2. Run KCP smoke test.
3. Check server logs for receive errors.
4. Check packet receive counters.
5. Confirm Gateway returns correct node address.
