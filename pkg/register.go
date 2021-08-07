package pkg

import (
	"github.com/elastic/beats/v7/libbeat/beat"
	"github.com/elastic/beats/v7/libbeat/common"
	"github.com/elastic/beats/v7/libbeat/outputs"
)

func init() {
	outputs.RegisterType("websocket", newWsOutput)
}

func newWsOutput(_ outputs.IndexManager, _ beat.Info, stats outputs.Observer, cfg *common.Config) (outputs.Group, error) {
	config := clientConfig{}
	// 卸载配置，将配置用于初始化WebSocket客户端
	if err := cfg.Unpack(&config); err != nil {
		return outputs.Fail(err)
	}
	clients := make([]outputs.NetworkClient, config.Workers)
	for i := 0; i < config.Workers; i++ {
		clients[i] = &wsClient{
			stats:        stats,
			Schema:       config.Schema,
			Host:         config.Addr,
			Path:         config.Path,
			PingInterval: config.PingInterval,
			MaxLen:       config.MaxLen,
		}
	}

	return outputs.SuccessNet(true, config.BatchSize, config.RetryLimit, clients)
}
