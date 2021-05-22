package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/liranpi2/limiter"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
)

var threshold =  flag.Int("threshold", 1, "request limit")
var ttl  = flag.Int("ttl", 2, "time for a request ")
var rateLimiter = limiter.NewUrlRateLimiter(*threshold,*ttl)

type MalformedRequest struct {
	Status int
	Msg    string
}

type Endpoint struct {
	Url string `json:"url""`
}

type LimiterResult struct {
	Block bool `json:"blocked""`
}

func main() {

	//jdata := `{"url" : "http://example.com"}`
	//var endpoint Endpoint
	//json.Unmarshal([]byte(jdata), &endpoint)

	// creat logger
	file, err := os.OpenFile("logs.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)

	if err != nil {
		fmt.Println("init logger failed")
	}
	log.SetOutput(file)

	// init command args
	flag.Parse()
	log.Printf("consumer started with args: threshold: %d, ttl %d", *threshold, *ttl)

	fmt.Println("threshold:", *threshold)
	fmt.Println("ttl:", *ttl)

	// assign handlers
	http.HandleFunc("/", okHandler)
	http.HandleFunc("/report", entryHandler)

	// start listener
	log.Fatal(http.ListenAndServe(":8080", nil))
}


func okHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(""))
}

func entryHandler(w http.ResponseWriter, r *http.Request) {

	// check request method
	if !requestValid(r) {
		w.WriteHeader(405)
		return
	}

	// check request body
	var endpoint Endpoint
	err := parseBody(w,r,&endpoint)
	if err != nil {
		var mr * MalformedRequest
		if errors.As(err, &mr) {
			http.Error(w, mr.Msg, mr.Status)
		} else {
			log.Println(err.Error())
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusBadRequest)
		}
		return
	}

	// log url
	log.Printf("request from url: %s\n", endpoint.Url)

	// find limiter for the specified url
	limiter := rateLimiter.GetLimiter(endpoint.Url)

	log.Printf("limiter: url: %s, limiter: key: %s, bucket: %d, last check: %d", endpoint.Url, limiter.Key, limiter.Bucket, limiter.LastCheck)

	if !limiter.Accept() {
		log.Printf("%s url blocked, bucket is %d", endpoint.Url, limiter.Bucket)

		response := LimiterResult{Block: true}
		prettyJSON, err := json.MarshalIndent(response, "", "    ")
		if err != nil {
			log.Println("Failed to generate json", err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		}

		w.Write([]byte(string(prettyJSON)))
	}
}

func (mr *MalformedRequest) Error() string {
	return mr.Msg
}

func requestValid(r *http.Request) bool  {
	return r.Method == http.MethodPost
}

func parseBody(w http.ResponseWriter, r *http.Request, dst interface{}) error {

	r.Body = http.MaxBytesReader(w, r.Body, 1048576)

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	err := dec.Decode(&dst)
	if err != nil {
		var syntaxError *json.SyntaxError
		var unmarshalTypeError *json.UnmarshalTypeError

		switch {

		case errors.As(err, &syntaxError):
			msg := fmt.Sprintf("malformed json at position %d", syntaxError.Offset)
			return &MalformedRequest{Status: http.StatusBadRequest, Msg: msg}

		case errors.Is(err, io.ErrUnexpectedEOF):
			msg := fmt.Sprintf("malformed json")
			return &MalformedRequest{Status: http.StatusBadRequest, Msg: msg}

		case errors.As(err, &unmarshalTypeError):
			msg := fmt.Sprintf("invalid value for the %q field at position %d", unmarshalTypeError.Field, unmarshalTypeError.Offset)
			return &MalformedRequest{Status: http.StatusBadRequest, Msg: msg}

		case strings.HasPrefix(err.Error(), "unknown json prop"):
			prop := strings.TrimPrefix(err.Error(), "json: unknown prop")
			msg := fmt.Sprintf("unknown prop %s", prop)
			return &MalformedRequest{Status: http.StatusBadRequest, Msg: msg}

		case errors.Is(err, io.EOF):
			msg := "postdata must contain value"
			return &MalformedRequest{Status: http.StatusBadRequest, Msg: msg}

		case err.Error() == "http: request body too large":
			msg := "postdata must not exceed 1MB"
			return &MalformedRequest{Status: http.StatusRequestEntityTooLarge, Msg: msg}

		default:
			return err
		}
	}

	err = dec.Decode(&struct{}{})
	if err != io.EOF {
		msg := "postdata must contain a single JSON object"
		return &MalformedRequest{Status: http.StatusBadRequest, Msg: msg}
	}

	return nil
}



