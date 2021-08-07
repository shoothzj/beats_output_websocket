# 工程搭建

## 引入对`beat`的依赖

```bash
go get github.com/elastic/beats/v7
```

## 定义在filebeat中的配置文件

`filebeat`通常以配置文件的方式加载插件。让我们定义一下必须的配置，就像`elasticsearch`中的连接地址等等一样。

```yaml
output.websocket:
  # worker
  # 用于工作的websocket客户端数量
  workers: 1
  # 日志批量的最大大小
  batch_size: 1
  # 重试的最大次数，0代表不重试
  retry_limit: 1
  # conn
  # ws/wss
  schema: "ws"
  # websocket连接地址
  addr: "localhost:8080"
  # websocket路径
  path: "/echo"
  # websocket心跳间隔，用于保活
  ping_interval: 30
```

### go文件中的配置

```go
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
}
```



## 初始化加载插件

### 加载插件

在某个init函数中注册插件

```go
func init() {
	outputs.RegisterType("websocket", newWsOutput)
}
```

在`newWsOutput`中卸载配置，并提供配置给`WebSocket`客户端

```go
func newWsOutput(_ outputs.IndexManager, _ beat.Info, stats outputs.Observer, cfg *common.Config) (outputs.Group, error) {
	config := clientConfig{}
	// 卸载配置，将配置用于初始化WebSocket客户端
	if err := cfg.Unpack(&config); err != nil {
		return outputs.Fail(err)
	}
	clients := make([]outputs.NetworkClient, config.Workers)
	for i := 0; i < config.Workers; i++ {
		clients[i] = &wsClient{
			stats:  stats,
			Schema: config.Schema,
			Host:   config.Addr,
			Path:   config.Path,
			PingInterval: config.PingInterval,
		}
	}

	return outputs.SuccessNet(true, config.BatchSize, config.RetryLimit, clients)
}
```

## 初始化`WebSocket`客户端

`WebSocket`客户端不仅仅是一个`WebSocket`客户端，而且还需要实现`filebeat`中的`NetworkClient`接口，接下来，让我们来关注接口中的每一个方法的作用及实现

### String()接口

`String`作为客户端的名字，用来标识日志以及指标。是最简单的一个接口

```go
func (w *wsClient) String() string {
	return "websocket"
}
```

### Connect()接口

`Connect`用来初始化客户端

```go
func (w *wsClient) Connect() error {
	u := url.URL{Scheme: w.Schema, Host: w.Host, Path: w.Path}
	dial, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err == nil {
		w.conn = dial
		ticker := time.NewTicker(time.Duration(w.PingInterval) * time.Second)
		go func() {
			for range ticker.C {
				w.conn.WriteMessage(websocket.PingMessage, nil)
			}
		}()
	} else {
		time.Sleep(10 * time.Second)
	}
	return err
}
```

注意，这里初始化失败，需要`Sleep`一段时间，否则，filebeat会一直重试。这绝非是你想要的。或许对于场景来说，退避重试可能会更好

### Close()接口

关闭客户端，也是很简单的接口

```go
func (w *wsClient) Close() error {
	return w.conn.Close()
}
```

### Publish()接口

```go
func (w *wsClient) Publish(_ context.Context, batch publisher.Batch) error {
	events := batch.Events()
	// 记录这批日志
	w.stats.NewBatch(len(events))
	failEvents, err := w.PublishEvents(events)
	if err != nil {
		// 如果发送正常，则ACK
		batch.ACK()
	} else {
		// 发送失败，则重试。受RetryLimit的限制
		batch.RetryEvents(failEvents)
	}
	return err
}

func (w *wsClient) PublishEvents(events []publisher.Event) ([]publisher.Event, error) {
	for i, event := range events {
		err := w.publishEvent(&event)
		if err != nil {
			// 如果单条消息发送失败，则将剩余的消息直接重试
			return events[i:], err
		}
	}
	return nil, nil
}

func (w *wsClient) publishEvent(event *publisher.Event) error {
	bytes, err := encode(&event.Content)
	if err != nil {
		// 如果编码失败，就不重试了，重试也不会成功
		// encode error, don't retry.
		// consider being success
		return nil
	}
	err = w.conn.WriteMessage(websocket.TextMessage, bytes)
	if err != nil {
		// 写入WebSocket Server失败
		return err
	}
	return nil
}
```

### 编码

编码的逻辑因人而异，事实上，这可能是大家最大的差异所在。这里只是做一个简单地例子

```go
type LogOutput struct {
	Timestamp time.Time `json:"timestamp"`
	Message   string    `json:"message"`
}

func encode(event *beat.Event) ([]byte, error) {
	logOutput := &LogOutput{}
	value, err := event.Fields.GetValue("message")
	if err != nil {
		return nil, err
	}
	logOutput.Timestamp = event.Timestamp
	logOutput.Message = value.(string)
	return json.Marshal(logOutput)
}
```

### 最后是我们的`wsclient`

```go
type wsClient struct {
	// construct field
	Schema       string
	Host         string
	Path         string
	PingInterval int

	stats outputs.Observer
	conn  *websocket.Conn
}
```

## 添加额外的功能：大包丢弃

你可能会想保护你的`WebSocket`服务器，避免接收到超级大的日志。我们可以在配置项中添加一个配置

maxLen用来限制日志长度，超过maxLen的日志直接丢弃。为什么不使用`filebeat`中的`max_bytes`？

因为`filebeat`中`max_bytes`的默认行为是截断，截断的日志在某些场景下不如丢弃。（比如，日志是json格式，截断后格式无法解析）

### 配置中添加maxLen

```yaml
  max_len: 1024
```

省略掉那些重复的添加结构体，读取`max_len`在encode的时候忽略掉

```go
	s := value.(string)
	if len(s) >= w.MaxLen {
		return nil, err
	}
```

# 参考文献

- https://www.elastic.co/guide/en/beats/filebeat/current/filebeat-reference-yml.html
- https://www.fullstory.com/blog/writing-a-filebeat-output-plugin
- https://github.com/raboof/beats-output-http

