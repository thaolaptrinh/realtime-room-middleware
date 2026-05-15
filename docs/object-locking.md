# Object Synchronization and Locking

> Placeholder — to be written during Milestone 4.

## Contents

- Object state model
- Command queue + lease TTL consistency model
- Lock flow
- Refresh flow
- Release flow
- Expiration flow
- Disconnect release flow
- Lock configuration
- ObjectDelta format

## Hard Rules

- Server-authoritative command queue + lease lock.
- No optimistic locking.
- Locks must expire on TTL.
- Disconnect releases all user locks.
