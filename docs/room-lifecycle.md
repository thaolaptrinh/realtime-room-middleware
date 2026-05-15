# Room Lifecycle

> Placeholder — to be written during Milestone 2.

## Contents

- Logical room ID vs room instance ID
- Room creation and assignment
- Join flow
- Leave flow
- Reconnect flow
- Idle room cleanup
- Overflow room behavior
- Room state ownership model

## Hard Rules

- Do not migrate live rooms.
- Room loop is the only writer of room state.
