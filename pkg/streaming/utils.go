package streaming

import (
	"fmt"
	"log"
	"time"
)

func uShortExtractResponseCode(code uint16) uint16 {
	return code & 0b0111_1111_1111_1111
}

//func UIntExtractResponseCode(code int32) int32 {
//	return code & 0b0111_1111_1111_1111
//}

func uShortEncodeResponseCode(code uint16) uint16 {
	return code | 0b1000_0000_0000_0000
}

func waitCodeWithDefaultTimeOut(response *Response) error {
	return waitCodeWithTimeOut(response, defaultSocketCallTimeout)
}
func waitCodeWithTimeOut(response *Response, timeout time.Duration) error {
	select {
	case code := <-response.code:
		if code.id != responseCodeOk {
			return fmt.Errorf("code error: %s", lookErrorCode(code.id))
		}
		return nil
	case <-time.After(timeout):
		WARN("timeout waiting Code, operation:%d", response.correlationid)
		return fmt.Errorf("timeout waiting Code, operation:%d ", response.correlationid)
	}
}

// logging

func INFO(message string, v ...interface{}) {
	log.Printf(fmt.Sprintf("[INFO] - %s", message), v...)
}

func ERROR(message string, v ...interface{}) {
	log.Printf(fmt.Sprintf("[ERROR] - %s", message), v...)
}

func DEBUG(message string, v ...interface{}) {
	log.Printf(fmt.Sprintf("[DEBUG] - %s", message), v...)
}

func WARN(message string, v ...interface{}) {
	log.Printf(fmt.Sprintf("[WARN] - %s", message), v...)
}
