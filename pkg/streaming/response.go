package streaming

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/Azure/go-amqp"
)

type ReaderProtocol struct {
	FrameLen          uint32
	CommandID         uint16
	Key               uint16
	Version           uint16
	CorrelationId     uint32
	ResponseCode      uint16
	PublishID         uint8
	PublishingIdCount uint64
}

func (client *Client) handleResponse() {
	buffer := bufio.NewReader(client.socket.connection)
	for  {
		readerProtocol := &ReaderProtocol{}
		frameLen, err := ReadUInt(buffer)
		if err != nil {
			fmt.Printf("Socket Error %s \n", err)
			_ = client.Close()
			break
		}
		readerProtocol.FrameLen = frameLen
		readerProtocol.CommandID = UShortExtractResponseCode(ReadUShort(buffer))
		readerProtocol.Version = ReadUShort(buffer)

		switch readerProtocol.CommandID {

		case CommandPeerProperties:
			{
				client.handlePeerProperties(readerProtocol, buffer)
			}
		case CommandSaslHandshake:
			{
				client.handleSaslHandshakeResponse(readerProtocol, buffer)
			}
		case CommandTune:
			{
				client.handleTune(buffer)
			}
		case CommandOpen, CommandDeclarePublisher,
			CommandDeletePublisher, CommandDeleteStream,
			CommandCreateStream, CommandSaslAuthenticate, CommandSubscribe:
			{
				client.handleGenericResponse(readerProtocol, buffer)
			}

		case CommandPublishConfirm:
			{
				client.handleConfirm(readerProtocol, buffer)
			}
		case CommandDeliver:
			{
				client.handleDeliver(buffer)

			}
		case CommandCredit:
			{
			client.CreditNotificationFrameHandler(readerProtocol, buffer)
			}
		case CommandHeartbeat:
			{
				fmt.Printf("CommandHeartbeat %d buff:%d \n", readerProtocol.CommandID, buffer.Buffered())

			}
		default:
			{
				fmt.Printf("dont CommandID %d buff:%d \n", readerProtocol.CommandID, buffer.Buffered())
				break
			}
		}
	}

}

func (client *Client) handleSaslHandshakeResponse(streamingRes *ReaderProtocol, r *bufio.Reader) interface{} {
	streamingRes.CorrelationId,_ = ReadUInt(r)
	streamingRes.ResponseCode = UShortExtractResponseCode(ReadUShort(r))
	mechanismsCount,_ := ReadUInt(r)
	var mechanisms []string
	for i := 0; i < int(mechanismsCount); i++ {
		mechanism := ReadString(r)
		mechanisms = append(mechanisms, mechanism)
	}

	res, err := client.responses.GetById(streamingRes.CorrelationId)
	if err != nil {
		// TODO handle response
		return err
	}
	res.data <- mechanisms

	return mechanisms
}

func (client *Client) handlePeerProperties(readProtocol *ReaderProtocol, r *bufio.Reader) {
	readProtocol.CorrelationId,_ = ReadUInt(r)
	readProtocol.ResponseCode = UShortExtractResponseCode(ReadUShort(r))
	if readProtocol.ResponseCode != 1 {
		fmt.Printf("Errr ResponseCode: %d ", readProtocol.ResponseCode)
	}
	serverPropertiesCount,_ := ReadUInt(r)
	serverProperties := make(map[string]string)

	for i := 0; i < int(serverPropertiesCount); i++ {
		key := ReadString(r)
		value := ReadString(r)
		serverProperties[key] = value
	}
	res, err := client.responses.GetById(readProtocol.CorrelationId)
	if err != nil {
		// TODO handle response
		return
	}
	res.code <- Code{id: readProtocol.ResponseCode}

}

func (client *Client) handleTune(r *bufio.Reader) interface{} {

	serverMaxFrameSize,_ := ReadUInt(r)
	serverHeartbeat,_ := ReadUInt(r)

	maxFrameSize := serverMaxFrameSize
	heartbeat := serverHeartbeat

	length := 2 + 2 + 4 + 4
	var b = bytes.NewBuffer(make([]byte, 0, length+4))
	WriteInt(b, length)
	WriteUShort(b, UShortEncodeResponseCode(CommandTune))
	WriteShort(b, Version1)
	WriteUInt(b, maxFrameSize)
	WriteUInt(b, heartbeat)
	res, err := client.responses.GetByName("tune")
	if err != nil {
		// TODO handle response
		return err
	}
	res.data <- b.Bytes()
	return b.Bytes()

}

func (client *Client) handleGenericResponse(readProtocol *ReaderProtocol, r *bufio.Reader) {
	readProtocol.CorrelationId,_ = ReadUInt(r)
	readProtocol.ResponseCode = UShortExtractResponseCode(ReadUShort(r))
	res, err := client.responses.GetById(readProtocol.CorrelationId)
	if err != nil {
		// TODO handle readProtocol
		return
	}
	res.code <- Code{id: readProtocol.ResponseCode}
}

func (client *Client) handleConfirm(readProtocol *ReaderProtocol, r *bufio.Reader) interface{} {
	readProtocol.PublishID = ReadByte(r)
	//readProtocol.PublishingIdCount = ReadIntFromReader(testClient.reader)
	publishingIdCount,_ := ReadUInt(r)
	//var _publishingId int64
	for publishingIdCount != 0 {
		ReadInt64(r)
		publishingIdCount--
	}

	return 0

}

func (client *Client) handleDeliver(r *bufio.Reader) {

	subscriptionId := ReadByte(r)
	_ = ReadByte(r)
	_ = ReadByte(r)
	_ = ReadUShort(r)
	numRecords,_ := ReadUInt(r)
	_ = ReadInt64(r)
	_ = ReadInt64(r)
	_ = ReadInt64(r)
	_,_ = ReadUInt(r)
	_,_ = ReadUInt(r)
	_,_ = ReadUInt(r)
	//fmt.Printf("%d - %d - %d - %d - %d - %d - %d - %d - %d - %d - %d \n", subscriptionId, b, chunkType,
	//		numEntries, numRecords, timestamp, epoch, unsigned, crc, dataLength, trailer)
	client.credit(subscriptionId, 1)
	//messages
	for numRecords != 0 {
		entryType := PeekByte(r)
		if (entryType & 0x80) == 0 {
			sizeMessage,_ := ReadUInt(r)

			arrayMessage := ReadUint8Array(r, sizeMessage)
			msg := amqp.Message{}
			err := msg.UnmarshalBinary(arrayMessage)
			if err != nil {
				fmt.Printf("%s", err)
				//}
			}
			c, _ := client.consumers.GetById(subscriptionId)
			c.response.code <- Code{id: ResponseCodeOk}
			c.response.data <- &msg

		}
		numRecords--
	}

}

func (client *Client) CreditNotificationFrameHandler(readProtocol *ReaderProtocol, r *bufio.Reader) {
	readProtocol.ResponseCode = UShortExtractResponseCode(ReadUShort(r))
	subscriptionId := ReadByte(r)
	fmt.Printf("CreditNotificationFrameHandler %d \n", subscriptionId)
}