package run

// Execute runs one deploy-unit invocation. Real phases arrive in later tasks.
func Execute(o Options) int {
	if err := o.Validate(); err != nil {
		println("error:", err.Error())
		return 2
	}
	return 0
}
