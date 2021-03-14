package stream

import (
	"bufio"
	"bytes"
	"net"
	"net/url"
	"sync"
)

type TuneState struct {
	requestedMaxFrameSize int
	requestedHeartbeat    int
}

type ClientProperties struct {
	items map[string]string
}

type Client struct {
	socket           net.Conn
	clientProperties ClientProperties
	tuneState        TuneState
	writer           *bufio.Writer
	reader           *bufio.Reader
	mutexWrite       *sync.Mutex
	producers        *Producers
	responses        *Responses
	consumers        *Consumers
}

func NewStreamingClient() *Client {
	client := &Client{
		mutexWrite: &sync.Mutex{},
		producers:  NewProducers(),
		responses:  NewResponses(),
		consumers:  NewConsumers(),
	}
	return client
}

func (client *Client) Connect(addr string) error {
	u, err := url.Parse(addr)
	if err != nil {
		return err
	}
	host, port := u.Hostname(), u.Port()
	if port == "" {
		port = "5551"
	}

	client.tuneState.requestedHeartbeat = 60
	client.tuneState.requestedMaxFrameSize = 1048576
	client.clientProperties.items = make(map[string]string)
	connection, err2 := net.Dial("tcp", net.JoinHostPort(host, port))
	if err2 != nil {
		return err2
	}
	client.socket = connection
	client.writer = bufio.NewWriter(client.socket)
	go client.handleResponse(connection)
	_, err2 = client.peerProperties()

	if err2 != nil {
		return err2
	}
	pwd, _ := u.User.Password()
	err2 = client.authenticate(u.User.Username(), pwd)
	if err2 != nil {
		return err2
	}
	vhost := "/"
	if len(u.Path) > 1 {
		vhost, _ = url.QueryUnescape(u.Path[1:])
	}
	_, err2 = client.open(vhost)
	if err2 != nil {
		return err2
	}

	return nil
}

func (client *Client) CreateStream(stream string) (*Code, error) {

	resp := client.responses.New()
	length := 2 + 2 + 4 + 2 + len(stream) + 4
	correlationId := resp.subId
	arguments := make(map[string]string)
	arguments["queue-leader-locator"] = "least-leaders"
	for key, element := range arguments {
		length = length + 2 + len(key) + 2 + len(element)
	}
	var b = bytes.NewBuffer(make([]byte, 0, length))
	WriteInt(b, length)
	WriteShort(b, CommandCreateStream)
	WriteShort(b, Version1)
	WriteInt(b, correlationId)
	WriteString(b, stream)
	WriteInt(b, len(arguments))

	for key, element := range arguments {
		WriteString(b, key)
		WriteString(b, element)
	}

	err := client.writeAndFlush(b.Bytes())
	if err != nil {
		return nil, err
	}
	code := <-resp.code
	err = client.responses.RemoveById(correlationId)
	if err != nil {
		return nil, err
	}
	return &code, nil
}

func (client *Client) peerProperties() (*Code, error) {
	clientPropertiesSize := 4 // size of the map, always there

	client.clientProperties.items["connection_name"] = "rabbitmq-stream-locator"
	client.clientProperties.items["product"] = "RabbitMQ Stream"
	client.clientProperties.items["copyright"] = "Copyright (c) 2021 VMware, Inc. or its affiliates."
	client.clientProperties.items["information"] = "Licensed under the MPL 2.0. See https://www.rabbitmq.com/"
	client.clientProperties.items["version"] = "0.1.0"
	client.clientProperties.items["platform"] = "Golang"
	for key, element := range client.clientProperties.items {
		clientPropertiesSize = clientPropertiesSize + 2 + len(key) + 2 + len(element)
	}

	length := 2 + 2 + 4 + clientPropertiesSize
	resp := client.responses.New()
	correlationId := resp.subId
	var b = bytes.NewBuffer(make([]byte, 0, length+4))

	WriteInt(b, length)
	WriteShort(b, CommandPeerProperties)
	WriteShort(b, Version1)
	WriteInt(b, correlationId)
	WriteInt(b, len(client.clientProperties.items))

	for key, element := range client.clientProperties.items {
		WriteString(b, key)
		WriteString(b, element)
	}

	err := client.writeAndFlush(b.Bytes())
	if err != nil {
		return nil, err
	}
	code := <-resp.code
	err = client.responses.RemoveById(correlationId)
	if err != nil {
		return nil, err
	}
	return &code, nil
}

func (client *Client) authenticate(user string, password string) error {

	saslMechanisms := client.getSaslMechanisms()
	saslMechanism := ""
	for i := 0; i < len(saslMechanisms); i++ {
		if saslMechanisms[i] == "PLAIN" {
			saslMechanism = "PLAIN"
		}
	}
	response := UnicodeNull + user + UnicodeNull + password
	saslResponse := []byte(response)
	return client.sendSaslAuthenticate(saslMechanism, saslResponse)
}

func (client *Client) getSaslMechanisms() []string {
	length := 2 + 2 + 4
	resp := client.responses.New()
	correlationId := resp.subId
	var b = bytes.NewBuffer(make([]byte, 0, length+4))
	WriteInt(b, length)
	WriteShort(b, CommandSaslHandshake)
	WriteShort(b, Version1)
	WriteInt(b, correlationId)
	client.writeAndFlush(b.Bytes())
	data := <-resp.dataString
	err := client.responses.RemoveById(correlationId)
	if err != nil {
		return nil
	}
	return data

}

func (client *Client) sendSaslAuthenticate(saslMechanism string, challengeResponse []byte) error {
	length := 2 + 2 + 4 + 2 + len(saslMechanism) + 4 + len(challengeResponse)
	resp := client.responses.New()
	respTune := client.responses.NewWitName("tune")
	correlationId := resp.subId
	var b = bytes.NewBuffer(make([]byte, 0, length+4))
	WriteInt(b, length)
	WriteShort(b, CommandSaslAuthenticate)
	WriteShort(b, Version1)
	WriteInt(b, correlationId)
	WriteString(b, saslMechanism)
	WriteInt(b, len(challengeResponse))
	b.Write(challengeResponse)
	err := client.writeAndFlush(b.Bytes())
	if err != nil {
		return err
	}

	<-resp.code
	err = client.responses.RemoveById(correlationId)
	if err != nil {
		return err
	}

	// double read for TUNE
	tuneData := <-respTune.dataBytes
	err = client.responses.RemoveByName("tune")
	if err != nil {
		return err
	}

	return client.writeAndFlush(tuneData)
}

func (client *Client) open(virtualHost string) (*Code, error) {
	length := 2 + 2 + 4 + 2 + len(virtualHost)
	resp := client.responses.New()
	correlationId := resp.subId
	var b = bytes.NewBuffer(make([]byte, 0, length+4))
	WriteInt(b, length)
	WriteShort(b, CommandOpen)
	WriteShort(b, Version1)
	WriteInt(b, correlationId)
	WriteString(b, virtualHost)
	err := client.writeAndFlush(b.Bytes())
	if err != nil {
		return nil, err
	}
	code := <-resp.code
	err = client.responses.RemoveById(correlationId)
	if err != nil {
		return nil, err
	}
	return &code, nil
}

func (client *Client) DeleteStream(stream string) (*Code, error) {
	length := 2 + 2 + 4 + 2 + len(stream)
	resp := client.responses.New()
	correlationId := resp.subId
	var b = bytes.NewBuffer(make([]byte, 0, length+4))
	WriteInt(b, length)
	WriteShort(b, CommandDeleteStream)
	WriteShort(b, Version1)
	WriteInt(b, correlationId)
	WriteString(b, stream)
	err := client.writeAndFlush(b.Bytes())
	if err != nil {
		return nil, err
	}
	code := <-resp.code
	err = client.responses.RemoveById(correlationId)
	if err != nil {
		return nil, err
	}
	return &code, nil
}

func (client *Client) writeAndFlush(buffer []byte) error {
	client.mutexWrite.Lock()
	_, err := client.writer.Write(buffer)
	if err != nil {
		return err
	}
	err = client.writer.Flush()
	if err != nil {
		return err
	}
	client.mutexWrite.Unlock()
	return nil
}

//Consumers
