package bee

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"github.com/dghubble/sling"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
)

type MultiClientRequest struct {
	BaseUrl     string
	BodyType    RequestBodyType
	Method      string
	Path        string
	RequestBody interface{}
}

func NewClientRequest(method string, path string, payload interface{}, payloadType RequestBodyType) (*MultiClientRequest, error) {
	mcRequest := &MultiClientRequest{
		BodyType:    payloadType,
		Method:      method,
		Path:        path,
		RequestBody: payload,
	}

	return mcRequest, nil
}

func (self *MultiClientRequest) SetBaseUrl(base string) {
	self.BaseUrl = strings.TrimSuffix(base, `/`) + `/`
}

func (self *MultiClientRequest) Perform(success interface{}, failure interface{}) (*http.Response, error) {
	request := sling.New()

	request.Base(self.BaseUrl)

	switch self.Method {
	case `GET`:
		request.Get(self.Path)
	case `POST`:
		request.Post(self.Path)
	case `PUT`:
		request.Put(self.Path)
	case `DELETE`:
		request.Delete(self.Path)
	case `HEAD`:
		request.Head(self.Path)
	case `PATCH`:
		request.Patch(self.Path)
	default:
		return nil, fmt.Errorf("Unsupported HTTP method '%s'", self.Method)
	}

	if self.RequestBody != nil {
		switch self.BodyType {
		case BodyJson:
			request.BodyJSON(self.RequestBody)

			// data, _ := json.MarshalIndent(self.RequestBody, ``, `  `)
			// fmt.Printf("[MC]: %s %s%s body:%s\n", self.Method, self.BaseUrl, self.Path, string(data[:]))

		case BodyForm:
			request.BodyForm(self.RequestBody)
		case BodyRaw, BodyXml:
			var reader io.Reader

			switch self.RequestBody.(type) {
			case io.Reader:
				reader = self.RequestBody.(io.Reader)
			case string:
				reader = bytes.NewBufferString(self.RequestBody.(string))
			case []byte:
				reader = bytes.NewBuffer(self.RequestBody.([]byte))
			}

			switch self.BodyType {
			case BodyXml:
				if input, err := ioutil.ReadAll(reader); err == nil {
					if data, err := xml.Marshal(input); err == nil {
						request.Body(bytes.NewBuffer(data))
					} else {
						return nil, err
					}
				} else {
					return nil, err
				}

			default:
				request.Body(reader)
			}
		}
	}

	return request.Receive(success, failure)
}
