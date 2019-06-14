package bee

import (
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

const Version = `0.0.5`

const DEFAULT_MULTICLIENT_HEALTHCHECK_TIMEOUT = (time.Duration(10) * time.Second)

type RequestBodyType int

const (
	BodyRaw RequestBodyType = iota
	BodyXml
	BodyJson
	BodyForm
)

type MultiClient struct {
	Addresses                []string
	HealthChecks             bool
	HealthCheckPath          string
	HealthCheckMethod        string
	HealthCheckBody          io.Reader
	HealthCheckMatch         string
	HealthCheckTimeout       time.Duration
	RequestQueryStrings      map[string]interface{}
	RequestHeaders           map[string]interface{}
	RetryLimit               int
	DefaultBodyType          RequestBodyType
	PreRequestHooks          []PreRequestHook
	LatePreRequestHooks      []PreRequestHook
	ImmediatePreRequestHooks []ImmediatePreRequestHook
	healthyAddresses         []int
	checkLock                sync.Mutex
	active                   bool
	client                   *http.Client
}

func NewMultiClient(addresses ...string) *MultiClient {
	return &MultiClient{
		Addresses:                addresses,
		HealthCheckMethod:        `GET`,
		HealthCheckTimeout:       DEFAULT_MULTICLIENT_HEALTHCHECK_TIMEOUT,
		RequestQueryStrings:      make(map[string]interface{}),
		RequestHeaders:           make(map[string]interface{}),
		RetryLimit:               1,
		DefaultBodyType:          BodyJson,
		PreRequestHooks:          make([]PreRequestHook, 0),
		LatePreRequestHooks:      make([]PreRequestHook, 0),
		ImmediatePreRequestHooks: make([]ImmediatePreRequestHook, 0),
		active:                   true,
	}
}

func (self *MultiClient) SetAddresses(addresses ...string) {
	self.Addresses = addresses
}

func (self *MultiClient) SetHealthCheckPath(path string) {
	self.HealthCheckPath = path
}

func (self *MultiClient) SetHealthCheckTimeout(timeout time.Duration) {
	self.HealthCheckTimeout = timeout
}

func (self *MultiClient) SetRetryLimit(n int) {
	self.RetryLimit = n
}

func (self *MultiClient) SetDefaultBodyType(t RequestBodyType) {
	self.DefaultBodyType = t
}

func (self *MultiClient) SetClient(client *http.Client) {
	self.client = client
}

func (self *MultiClient) Resume() {
	self.active = true
}

func (self *MultiClient) Suspend() {
	self.active = false
	self.CheckAll()
}

func (self *MultiClient) IsActive() bool {
	return self.active
}

func (self *MultiClient) IsHealthy(address string) bool {
	// if the healthcheck path is not set, then simply attempt a TCP socket
	// connection to the address and return whether that was successful or not
	if self.HealthCheckPath == `` {
		socketAddress := address
		parts := strings.Split(socketAddress, `://`)

		if len(parts) == 2 {
			socketAddress = parts[1]
		}

		if conn, err := net.DialTimeout(`tcp`, socketAddress, self.HealthCheckTimeout); err == nil {
			defer conn.Close()
			return true
		}
	} else {
		if request, err := NewClientRequest(self.HealthCheckMethod, self.HealthCheckPath, self.HealthCheckBody, BodyRaw); err == nil {
			var response string

			if _, err := request.Perform(&response, nil); err == nil {
				if ok, err := regexp.MatchString(self.HealthCheckMatch, response); err == nil && ok {
					return true
				}
			}
		}
	}

	return false
}

func (self *MultiClient) GetHealthyAddresses() []string {
	self.checkLock.Lock()
	defer self.checkLock.Unlock()

	addresses := make([]string, len(self.healthyAddresses))

	for i, id := range self.healthyAddresses {
		addresses[i] = self.Addresses[id]
	}

	return addresses
}

func (self *MultiClient) GetRandomHealthyAddress() (string, error) {
	// if we have health checks enabled, only select from known healthy addresses
	if self.HealthChecks {
		if len(self.healthyAddresses) == 0 {
			return ``, fmt.Errorf("No healthy addresses found")
		}

		randId := self.healthyAddresses[rand.Intn(len(self.healthyAddresses))]

		if randId < len(self.Addresses) {
			return self.Addresses[randId], nil
		} else {
			return ``, fmt.Errorf("No healthy addresses found")
		}
	} else {
		// otherwise, just pick a random address
		if len(self.Addresses) == 0 {
			return ``, fmt.Errorf("No addresses found")
		}

		randAddr := self.Addresses[rand.Intn(len(self.Addresses))]

		return randAddr, nil
	}
}

func (self *MultiClient) checkConnect(minSuccessfulAddresses int) error {
	self.checkLock.Lock()
	defer self.checkLock.Unlock()

	if self.IsActive() {
		var successfulChecks int

		self.healthyAddresses = make([]int, 0)

		for i, address := range self.Addresses {
			if self.IsHealthy(address) {
				successfulChecks += 1
				self.healthyAddresses = append(self.healthyAddresses, i)
			}

			if successfulChecks >= minSuccessfulAddresses {
				break
			}
		}

		if successfulChecks < minSuccessfulAddresses {
			return fmt.Errorf("Not enough healthy addresses configured to meet requested minimum: want %d, have %d",
				minSuccessfulAddresses, successfulChecks)
		}

		return nil
	} else {
		self.healthyAddresses = nil
		return fmt.Errorf("Client is not active")
	}
}

func (self *MultiClient) CheckOne() error {
	return self.checkConnect(1)
}

func (self *MultiClient) CheckN(n int) error {
	return self.checkConnect(n)
}

func (self *MultiClient) CheckQuorum() error {
	return self.checkConnect(int(len(self.Addresses)/2) + 1)
}

func (self *MultiClient) CheckAll() error {
	return self.checkConnect(len(self.Addresses))
}

func (self *MultiClient) Request(method string, path string, payload interface{}, output interface{}, failure interface{}, preRequestHooks ...PreRequestHook) (*http.Response, error) {
	var lastErr error

	if request, err := NewClientRequest(method, path, payload, self.DefaultBodyType); err == nil {
		request.Client = self.client

		for i := 0; i < self.RetryLimit; i++ {
			// get a random healthy address or fail out
			if address, err := self.GetRandomHealthyAddress(); err == nil {
				request.SetBaseUrl(address)

				for k, v := range self.RequestQueryStrings {
					request.QuerySet(k, v)
				}

				for k, v := range self.RequestHeaders {
					request.HeaderSet(k, v)
				}

				preRequestHooks = append(self.PreRequestHooks, preRequestHooks...)
				preRequestHooks = append(preRequestHooks, self.LatePreRequestHooks...)

				request.PreRequestHooks = preRequestHooks
				request.ImmediatePreRequestHooks = self.ImmediatePreRequestHooks

				if response, err := request.Perform(output, failure); err == nil {
					return response, nil
				} else {
					lastErr = err
				}
			} else {
				return nil, err
			}
		}

		if lastErr != nil {
			return nil, lastErr
		} else {
			return nil, fmt.Errorf("Exceeded retry limit for request")
		}
	} else {
		return nil, err
	}
}

func (self *MultiClient) QuerySet(key string, value interface{}) {
	self.RequestQueryStrings[key] = value
}

func (self *MultiClient) HeaderSet(key string, value interface{}) {
	self.RequestHeaders[key] = value
}
