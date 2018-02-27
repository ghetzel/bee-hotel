package bee

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/dghubble/sling"
	"github.com/ghetzel/go-stockutil/stringutil"
)

type PreRequestHook func(*MultiClientRequest) error          // {}
type ImmediatePreRequestHook func(*http.Request) error       // {}
type ResponseDecoder func(*http.Response, interface{}) error // {}

type MultiClientRequest struct {
	BaseUrl                  string
	BodyType                 RequestBodyType
	Method                   string
	Path                     string
	RequestBody              interface{}
	ResponseProcessor        ResponseDecoder
	Headers                  map[string]interface{}
	QueryString              map[string]interface{}
	PreRequestHooks          []PreRequestHook
	ImmediatePreRequestHooks []ImmediatePreRequestHook
	Client                   *http.Client
}

func NewClientRequest(method string, path string, payload interface{}, payloadType RequestBodyType) (*MultiClientRequest, error) {
	mcRequest := &MultiClientRequest{
		BodyType:                 payloadType,
		Method:                   strings.ToUpper(method),
		Path:                     path,
		RequestBody:              payload,
		QueryString:              make(map[string]interface{}),
		Headers:                  make(map[string]interface{}),
		PreRequestHooks:          make([]PreRequestHook, 0),
		ImmediatePreRequestHooks: make([]ImmediatePreRequestHook, 0),
	}

	mcRequest.ResponseProcessor = mcRequest.DefaultResponseProcessor

	return mcRequest, nil
}

func (self *MultiClientRequest) SetBaseUrl(base string) {
	self.BaseUrl = strings.TrimSuffix(base, `/`)
}

func (self *MultiClientRequest) Perform(success interface{}, failure interface{}) (*http.Response, error) {
	request := sling.New()

	if self.Client != nil {
		request.Client(self.Client)
	}

	request.Base(self.BaseUrl + `/`)

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

	// apply any pre-request hooks
	for _, hook := range self.PreRequestHooks {
		if err := hook(self); err != nil {
			return nil, err
		}
	}

	if httpReq, err := request.Request(); err == nil {
		// apply querystring values
		if len(self.QueryString) > 0 {
			query := httpReq.URL.Query()

			for key, value := range self.QueryString {
				if v, err := stringutil.ToString(value); err == nil {
					query.Set(key, v)
				} else {
					return nil, err
				}
			}

			httpReq.URL.RawQuery = query.Encode()
		}

		// apply header values
		for key, value := range self.Headers {
			if v, err := stringutil.ToString(value); err == nil {
				httpReq.Header.Set(key, v)
			} else {
				return nil, err
			}
		}

		// apply any immediate pre-request hooks
		for _, hook := range self.ImmediatePreRequestHooks {
			if err := hook(httpReq); err != nil {
				return nil, err
			}
		}

		// perform request
		if response, err := http.DefaultClient.Do(httpReq); err == nil {
			if response.StatusCode < 400 {
				return response, self.ResponseProcessor(response, success)
			} else {
				return response, self.ResponseProcessor(response, failure)
			}
		} else {
			return nil, err
		}
	} else {
		return nil, err
	}
}

func (self *MultiClientRequest) DefaultResponseProcessor(response *http.Response, into interface{}) error {
	switch strings.Split(response.Header.Get(`Content-Type`), `;`)[0] {
	case `application/json`, `text/json`:
		return self.DecodeJsonResponse(response, into)
	case `text/xml`:
		return self.DecodeXmlResponse(response, into)
	default:
		return self.DecodeTextResponse(response, into)
	}
}

func (self *MultiClientRequest) DecodeJsonResponse(response *http.Response, into interface{}) error {
	return json.NewDecoder(response.Body).Decode(into)
}

func (self *MultiClientRequest) DecodeXmlResponse(response *http.Response, into interface{}) error {
	return xml.NewDecoder(response.Body).Decode(into)
}

func (self *MultiClientRequest) DecodeTextResponse(response *http.Response, into interface{}) error {
	return self.DecodeXmlResponse(response, into)
}

func (self *MultiClientRequest) QuerySet(key string, value interface{}) {
	self.QueryString[key] = value
}

func (self *MultiClientRequest) HeaderSet(key string, value interface{}) {
	self.Headers[key] = value
}
