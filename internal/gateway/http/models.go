package http

import "time"

// JoinRequest is the JSON body for POST /join.
type JoinRequest struct {
	UserID                string `json:"user_id"`
	LogicalRoomID         string `json:"logical_room_id"`
	ClientProtocolVersion uint16 `json:"client_protocol_version"`
	ClientPlatform        string `json:"client_platform"`
	RequestedTransport    string `json:"requested_transport,omitempty"`
}

// JoinResponse is the JSON body returned on successful join.
type JoinResponse struct {
	RoomInstanceID  string    `json:"room_instance_id"`
	GameNodeAddr    string    `json:"game_node_addr"`
	ProtocolVersion uint16    `json:"protocol_version"`
	SessionToken    string    `json:"session_token"`
	Transport       string    `json:"transport"`
	KCPAddr         string    `json:"kcp_addr,omitempty"`
	WebSocketURL    string    `json:"websocket_url,omitempty"`
	ExpiresAt       time.Time `json:"expires_at"`
}

// ErrorResponse is the JSON body for error responses.
type ErrorResponse struct {
	Error string `json:"error"`
	Code  string `json:"code,omitempty"`
}
