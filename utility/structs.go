package utility

// Global config for all clients
// TODO: Implement individual configs for individual clients
type ListenerConfig struct {
	CfgWriteWait int
	CfgPongWait int
	CfgMaxMessageSize int64
	CfgPingPeriod int
	CfgAflZip string
	CfgAflLocation string
	CfgDataDir string
	CfgQueueFilesDir string
	CfgQueueZip string
}

type CommChannels struct {
	SendSignal chan []byte
	ReceiveSignal chan []byte
	UnregSignal chan []byte
}

type ClientInfo struct {
	LogicalCoresNum int
	OperatingSystem	string
}