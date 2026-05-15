# Protocol Specification

> Placeholder — to be written during Milestone 1.

## Contents

- MessagePack envelope format
- Protocol versioning policy
- Client → Server message types
- Server → Client message types
- Wire format for each message
- Backward compatibility rules

## Hard Rules

- No wire-format change without updating this document.
- No removal of Unity-used fields without explicit migration.
- Every packet has a protocol version.
