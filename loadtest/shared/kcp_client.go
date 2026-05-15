package shared

// LoadTestClient simulates a Unity client connecting to the game server.
// It performs HTTP /join, opens a KCP session, and sends/receives
// MessagePack-encoded protocol messages.
//
// Not yet implemented. Requires internal/protocol and internal/transport/kcp.
type LoadTestClient struct {
	// TODO: KCP connection
	// TODO: session token
	// TODO: room instance ID
	// TODO: packet stats collector
}

// Connect performs the full join flow: HTTP /join then KCP connect.
func (c *LoadTestClient) Connect(gatewayAddr, roomID, userID string) error {
	// TODO: implement
	return nil
}

// SendMovement sends a PlayerTransform message.
func (c *LoadTestClient) SendMovement(x, y, rotation float32) error {
	// TODO: implement
	return nil
}

// SendLockRequest sends an ObjectCommand to acquire a lock.
func (c *LoadTestClient) SendLockRequest(objectID string) error {
	// TODO: implement
	return nil
}

// Close shuts down the KCP session.
func (c *LoadTestClient) Close() error {
	// TODO: implement
	return nil
}
