package pkg

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/elastic/beats/v7/libbeat/beat"
	"github.com/elastic/beats/v7/libbeat/common"
	"github.com/elastic/beats/v7/libbeat/outputs"
	"github.com/elastic/beats/v7/libbeat/publisher"
	"github.com/gorilla/websocket"
	"net/url"
	"reflect"
	"time"
)

type LogOutput struct {
	Timestamp  time.Time `json:"timestamp"`
	Message    string    `json:"message"`
	FileName   string    `json:"file_name"`
	FileOffset int64     `json:"file_offset"`
	Hostname   string    `json:"hostname"`
	IpList     []string  `json:"ip_list"`
	MacList    []string  `json:"mac_list"`
	Arch       string    `json:"arch"`
}

type wsClient struct {
	// construct field
	Schema       string
	Host         string
	Path         string
	PingInterval int
	MaxLen       int

	stats outputs.Observer
	conn  *websocket.Conn
}

func (w *wsClient) String() string {
	return "websocket"
}

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

func (w *wsClient) Close() error {
	return w.conn.Close()
}

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
	bytes, err := w.encode(&event.Content)
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

func (w *wsClient) encode(event *beat.Event) ([]byte, error) {
	log, err := event.Fields.GetValue("log")
	if err != nil {
		return nil, err
	}
	// cast to map
	logMapVal, ok := log.(common.MapStr)
	if !ok {
		return nil, errors.New("cast log meta to map failed")
	}
	value, err := event.Fields.GetValue("message")
	if err != nil {
		return nil, err
	}
	s, ok := value.(string)
	if !ok {
		return nil, errors.New("cast log to str failed")
	}
	if len(s) >= w.MaxLen {
		return nil, err
	}
	hostValue, err := event.Fields.GetValue("host")
	if err == nil {
		// cast to map
		hostMapVal, ok := hostValue.(common.MapStr)
		if !ok {
			return w.encodeInner(event.Timestamp, s, logMapVal, nil)
		}
		return w.encodeInner(event.Timestamp, s, logMapVal, hostMapVal)
	}
	fmt.Println(reflect.TypeOf(log))
	return w.encodeInner(event.Timestamp, s, logMapVal, nil)
}

func (w *wsClient) encodeInner(time time.Time, s string, logMap common.MapStr, hostMap common.MapStr) ([]byte, error) {
	offset, file, err := logMapConvert(logMap)
	if err != nil {
		return nil, err
	}
	ipList, macList, arch, err := hostMapConvert(hostMap)
	if err != nil {
		return nil, err
	}
	logOutput := &LogOutput{}
	logOutput.Timestamp = time
	logOutput.Message = s
	logOutput.FileOffset = offset
	logOutput.FileName = file
	logOutput.IpList = ipList
	logOutput.MacList = macList
	logOutput.Arch = arch
	return json.Marshal(logOutput)
}

func logMapConvert(logMap common.MapStr) (offset int64, file string, err error) {
	offsetVal, err := logMap.GetValue("offset")
	if err != nil {
		return
	}
	offset, ok := offsetVal.(int64)
	if !ok {
		err = errors.New("cast offset to int64 failed")
		return
	}
	fileVal, err := logMap.GetValue("file")
	if err != nil {
		return
	}
	fileMapVal, ok := fileVal.(common.MapStr)
	if !ok {
		err = errors.New("cast file to map failed")
		return
	}
	pathVal, err := fileMapVal.GetValue("path")
	if err != nil {
		return
	}
	file, ok = pathVal.(string)
	if !ok {
		err = errors.New("cast file to string failed")
		return
	}
	return
}

func hostMapConvert(hostMap common.MapStr) (ipList []string, macList []string, arch string, err error) {
	ipListVal, err := hostMap.GetValue("ip")
	if err != nil {
		return
	}
	ipList, ok := ipListVal.([]string)
	if !ok {
		err = errors.New("cast failed")
		return
	}
	macListVal, err := hostMap.GetValue("mac")
	if err != nil {
		return
	}
	macList, ok = macListVal.([]string)
	if !ok {
		err = errors.New("cast failed")
		return
	}
	architectureVal, err := hostMap.GetValue("architecture")
	if err != nil {
		return
	}
	arch, ok = architectureVal.(string)
	if !ok {
		err = errors.New("cast failed")
		return
	}
	return
}
