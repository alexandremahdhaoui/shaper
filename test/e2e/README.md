# IPXER E2E tests

The goal of this document is to clarify intents regarding e2e tests
and describe how to achieve them and what's they're testing.

## Helpers

```
# Check tcp dump.
sudo tcpdump -n -i e2e0br0
sudo tcpdump -n -i e2e0tap0

sudo dhclient -v e2e0br0
sudo dhclient -v e2e0tap0
sudo dhclient -v -s 172.16.0.255 e2e0tap0

curl -XGET --interface e2e0tap0 172.16.0.1
ping -I e2e0tap0 172.16.0.1
```

