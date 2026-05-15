package shared

// ScenarioJoin tests gradual client joins and join storms.
//
// Measures:
//   - Time to complete full join flow per client
//   - Gateway response time
//   - KCP session establishment time
//   - FullSnapshot receipt time
//
// Not yet implemented.
func ScenarioJoin(config ScenarioConfig) (*ScenarioResult, error) {
	// TODO: implement
	return nil, nil
}

// ScenarioJoinStorm tests many clients joining simultaneously.
//
// Not yet implemented.
func ScenarioJoinStorm(config ScenarioConfig) (*ScenarioResult, error) {
	// TODO: implement
	return nil, nil
}
