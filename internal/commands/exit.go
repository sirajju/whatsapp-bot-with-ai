package commands

// SetOSExit lets main.go wire os.Exit into the admin Restart command without an import cycle.
func SetOSExit(fn func(int)) { osExit = fn }
