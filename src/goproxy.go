package main
import (
  "net/http"
  "log"
  "io"
  "io/ioutil"
  "net/http/cookiejar"
  "fmt"
  "encoding/json"
  "time"
  "flag"
  "strings"
)


type Ephemeric struct {
    Host string
    Port int
    RetrieveTime time.Time
}

var ephemericCache map[string] *Ephemeric;
var authToken string;

func getEphemeric(ephemericHost string) (ephemeric *Ephemeric, err error) {
    ephemeric, ok := ephemericCache[ephemericHost];
    
    if( ok && time.Since( ephemeric.RetrieveTime ) > 10 * time.Minute) {
	delete(ephemericCache, ephemericHost)
	ok = false;
    }
    
    if( !ok ) {
        //log.Println("Cache miss for", request.Host)
    
        apiresp, err := http.Get(fmt.Sprintf("http://ephemeric-api.pharocloud.com/system/ephemerics/%s/start?auth=%s", ephemericHost, authToken))
        if (err != nil) {
          return ephemeric, err
        }

        body, err := ioutil.ReadAll(apiresp.Body)
        if (err != nil) {
          return ephemeric, err
        }

        ephemeric = new(Ephemeric)
        err = json.Unmarshal(body, ephemeric)
        if (err != nil) {
          return ephemeric, err
        }

        ephemeric.RetrieveTime = time.Now()
        ephemericCache[ephemericHost] = ephemeric;
    }
    return ephemeric, err
}

func invalidateEphemeric(ephemericHost string) (ephemeric *Ephemeric, err error) {
	delete(ephemericCache, ephemericHost)
	return getEphemeric(ephemericHost)
}


func proxy(responseWriter http.ResponseWriter, request *http.Request) {
    var ephemeric *Ephemeric;
    var err error;
    ephemericHost := request.Host;
    ephemeric, err = getEphemeric(ephemericHost)
    
    if (err != nil) {
      http.Error(responseWriter, "Error reading from api server", 500)
      return
    }
    
    client := &http.Client{}
    request.RequestURI = ""
    request.Host = ephemeric.Host
    request.URL.Scheme="http"
    request.URL.Host=fmt.Sprintf("%s:%d", ephemeric.Host, ephemeric.Port)
    

    client.Jar, _ = cookiejar.New(nil)
    client.Jar.SetCookies(request.URL, request.Cookies())
    request.Header.Del("Connection")

    //for key, valueArray := range request.Header {
    //  for _, value := range valueArray {
    //    log.Println(key, value)
    //  }
    //}
    //log.Println("--------")

    response, err := client.Do(request)
    for i:=0; err != nil && i < 60; i++ {
        time.Sleep(1000 * time.Millisecond)
	if (i == 10) {
	    ephemeric, err := invalidateEphemeric(ephemericHost)
            if (err != nil) {
	      http.Error(responseWriter, "Error reading from api server", 500)
    	      return
            }
	    
	    request.RequestURI = ""
	    request.Host = ephemeric.Host
	    request.URL.Scheme="http"
	    request.URL.Host=fmt.Sprintf("%s:%d", ephemeric.Host, ephemeric.Port)
	}
	response, err = client.Do(request)
    }
    
    if (err != nil) {
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
    authFile := flag.String("authfile", "authToken", "File which contains system auth token");
    authTokenBytes, err := ioutil.ReadFile(*authFile);

    if (err != nil){
        log.Fatal(err);
    }
    authToken = strings.TrimSpace(string(authTokenBytes)) //todo: rebuild to closure ?

    http.HandleFunc("/", proxy)

    err = http.ListenAndServe(":80", nil);
    if (err != nil){
        log.Fatal(err);
    }
}
