package main
import (
	"net/http"
  "log"
  "io"
  "net/http/cookiejar"
)


func proxy(responseWriter http.ResponseWriter, request *http.Request) {
    client := &http.Client{}
    request.RequestURI = ""
    request.Host = "self.pharocloud.com"
    request.URL.Scheme="http"
    request.URL.Host=request.Host

    client.Jar, _ = cookiejar.New(nil)
    client.Jar.SetCookies(request.URL, request.Cookies())

    response, err := client.Do(request)
    if (err != nil) {
      http.Error(responseWriter, err.Error(), 500)
      return
    }

    for key, valueArray := range response.Header {
      for _, value := range valueArray {
        responseWriter.Header().Add(key, value)
        log.Println(key, value)
      }
    }
    responseWriter.WriteHeader(response.StatusCode)

    io.Copy(responseWriter, response.Body)
    response.Body.Close()
}

func main() {
    http.HandleFunc("/", proxy)

    err := http.ListenAndServe(":8080", nil);
    if (err != nil){
        log.Fatal(err);
    }
}
