package types

// SystemInfo struct to hold system related information
type SystemInfo struct {
	SystemName       string
	SystemVersion    string
	NodeName         string
	SystemProperties map[string]interface{}
}
