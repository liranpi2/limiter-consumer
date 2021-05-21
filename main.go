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
	url string
}

type LimiterResult struct {
	Block bool `json:"blocked""`
}

func main() {

	// set logger
	//l4g.AddFilter("file", l4g.FINE, l4g.NewFileLogWriter("errors.log", false))

	// init command args
	flag.Parse()

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

	var endpoint Endpoint
	err := json.NewDecoder(r.Body).Decode(&endpoint)
	// read postdata
	//err := decodeJSONBody(w,r,&endpoint)

	// validate
	if err != nil {
		var mr * MalformedRequest
		if errors.As(err, &mr) {
			http.Error(w, mr.Msg, mr.Status)
		} else {
			log.Println(err.Error())
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		}
		return
	}

	// find limiter for the specified url
	limiter := rateLimiter.GetLimiter(endpoint.url)
	if !limiter.Accept(endpoint.url) {
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

func decodeJSONBody(w http.ResponseWriter, r *http.Request, dst interface{}) error {

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



