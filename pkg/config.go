package pkg

type clientConfig struct {
	// Number of worker goroutines publishing log events
	Workers int `config:"workers" validate:"min=1"`
	// Max number of events in a batch to send to a single client
	BatchSize int `config:"batch_size" validate:"min=1"`
	// Max number of retries for single batch of events
	RetryLimit int `config:"retry_limit"`
	// Schema WebSocket Schema
	Schema string `config:"schema"`
	// Addr WebSocket Addr
	Addr string `config:"addr"`
	// Path WebSocket Path
	Path string `config:"path"`
	// PingInterval WebSocket PingInterval
	PingInterval int `config:"ping_interval"`
	MaxLen       int `config:"max_len"`
}
