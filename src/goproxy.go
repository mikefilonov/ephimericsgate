package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"time"
)

type Ephemeric struct {
	Host         string
	Port         int
	RetrieveTime time.Time
}

var ephemericCache map[string]*Ephemeric
var authToken string

var LimitExceededError = errors.New("Active ephemerics limit exceeded")

func getEphemeric(ephemericHost string) (ephemeric *Ephemeric, err error) {
	ephemeric, ok := ephemericCache[ephemericHost]

	if ok && time.Since(ephemeric.RetrieveTime) > 10*time.Minute {
		delete(ephemericCache, ephemericHost)
		ok = false
	}

	if !ok {
		//log.Println("Cache miss for", request.Host)
		cmdstring := fmt.Sprintf("https://www.pharocloud.com/api/ephemerics/system/ephemerics/%s/start?auth=%s", ephemericHost, authToken)
		//log.Println(cmdstring)
		apiresp, err := http.Get(cmdstring)
		if err != nil {
			return ephemeric, err
		}

		body, err := ioutil.ReadAll(apiresp.Body)
		if err != nil {
			return ephemeric, err
		}
		//log.Println(string(body))

		if string(body) == `"Limit exceeded"` {
			return ephemeric, LimitExceededError
		}

		ephemeric = new(Ephemeric)
		err = json.Unmarshal(body, ephemeric)
		if err != nil {
			return ephemeric, err
		}

		ephemeric.RetrieveTime = time.Now()
		ephemericCache[ephemericHost] = ephemeric
	}
	return ephemeric, err
}

func invalidateEphemeric(ephemericHost string) (ephemeric *Ephemeric, err error) {
	delete(ephemericCache, ephemericHost)
	return getEphemeric(ephemericHost)
}

func proxy(responseWriter http.ResponseWriter, request *http.Request) {
	var ephemeric *Ephemeric
	var err error

	//log.Println("--------", request)

	ephemericHost := request.Host
	ephemeric, err = getEphemeric(ephemericHost)

	if err != nil {
		switch err {
		case LimitExceededError:
			http.Error(responseWriter, err.Error(), 500)
			return
		default:
			http.Error(responseWriter, "Error reading from api server", 500)
			return
		}
	}

	client := &http.Client{}
	request.RequestURI = ""
	request.Host = ephemeric.Host
	request.URL.Scheme = "http"
	request.URL.Host = fmt.Sprintf("%s:%d", ephemeric.Host, ephemeric.Port)

	client.Jar, _ = cookiejar.New(nil)
	client.Jar.SetCookies(request.URL, request.Cookies())
	request.Header.Del("Connection")

	//for key, valueArray := range request.Header {
	//  for _, value := range valueArray {
	//    log.Println(key, value)
	//  }
	//}
	//log.Println("--------", request)

	response, err := client.Do(request)
	for i := 0; err != nil && i < 60; i++ {
		time.Sleep(1000 * time.Millisecond)
		if i == 10 {
			ephemeric, err := invalidateEphemeric(ephemericHost)
			if err != nil {
				//http.Error(responseWriter, "Error reading from api server", 500)
				http.Error(responseWriter, err.Error(), 500)
				return
			}

			request.RequestURI = ""
			request.Host = ephemeric.Host
			request.URL.Scheme = "http"
			request.URL.Host = fmt.Sprintf("%s:%d", ephemeric.Host, ephemeric.Port)
		}
		response, err = client.Do(request)
	}

	if err != nil {
		http.Error(responseWriter, "Connection to 8080 port of ephemeric failed", 500)
		return
	}

	for key, valueArray := range response.Header {
		for _, value := range valueArray {
			responseWriter.Header().Add(key, value)
		}
	}
	responseWriter.WriteHeader(response.StatusCode)

	io.Copy(responseWriter, response.Body)
	response.Body.Close()
}

func main() {
	var err error
	ephemericCache = make(map[string]*Ephemeric)
	authFile := flag.String("authfile", "authToken", "File which contains system auth token")
	authTokenBytes, err := ioutil.ReadFile(*authFile)

	if err != nil {
		log.Fatal(err)
	}
	authToken = strings.TrimSpace(string(authTokenBytes)) //todo: rebuild to closure ?

	http.HandleFunc("/", proxy)

	err = http.ListenAndServe(":80", nil)
	if err != nil {
		log.Fatal(err)
	}
}
