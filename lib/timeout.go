package lib

import (
	"errors"
	"reflect"
	"time"
)

// TimeoutError is returned when the operation times out
var TimeoutError = errors.New("operation timed out")

func DoWorkWithTimeout(fn interface{}, params []interface{}, timeout time.Duration) (interface{}, error) {
	resultChannel := make(chan interface{}, 1)
	errorChannel := make(chan error, 1)

	go func() {
		fnVal := reflect.ValueOf(fn)
		paramsVal := make([]reflect.Value, len(params))

		for i, param := range params {
			paramsVal[i] = reflect.ValueOf(param)
		}

		resVal := fnVal.Call(paramsVal)
		var err error
		var res interface{}

		// Convert returned values to interface{} and error
		if len(resVal) > 0 {
			res = resVal[0].Interface()
		}
		if len(resVal) > 1 {
			err, _ = resVal[1].Interface().(error)
		}

		resultChannel <- res
		errorChannel <- err
	}()

	select {
	case res := <-resultChannel:
		return res, <-errorChannel
	case <-time.After(timeout):
		return nil, TimeoutError
	}
}
