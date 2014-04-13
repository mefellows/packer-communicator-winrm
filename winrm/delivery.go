package winrm

import (
	"bytes"
	"fmt"
	"github.com/masterzen/xmlpath"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
)

type Deliverable interface {
	Xml() string
}

type HttpError struct {
	StatusCode int
	Status     string
}

func (e *HttpError) Error() string {
	return fmt.Sprintf("[%d] %s", e.StatusCode, e.Status)
}

var ErrHttpAuthenticate = &HttpError{401, "failed to authenticate"}
var ErrHttpNotFound = &HttpError{404, "nothing listening on the endpoint"}

func deliver(endpoint, user, pass string, delivery Deliverable) (io.Reader, error) {
	xml := delivery.Xml()
	if os.Getenv("WINRM_DEBUG") != "" {
		log.Println("winrm: sending", xml)
	}

	request, _ := http.NewRequest("POST", endpoint, bytes.NewBufferString(xml))
	request.SetBasicAuth(user, pass)
	request.Header.Add("Content-Type", "application/soap+xml;charset=UTF-8")

	client := &http.Client{}
	response, err := client.Do(request)

	if err != nil {
		println(err.Error())
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return nil, handleError(response)
	}

	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}

	if os.Getenv("WINRM_DEBUG") != "" {
		log.Println("winrm: receiving", string(body))
	}

	return bytes.NewReader(body), nil
}

func handleError(r *http.Response) error {
	if r.StatusCode == 404 {
		return ErrHttpNotFound
	}
	if r.StatusCode == 401 {
		return ErrHttpAuthenticate
	}

	if h := r.Header.Get("Content-Type"); strings.HasPrefix(h, "application/soap+xml") {
		return handleFault(r)
	}

	return &HttpError{r.StatusCode, r.Status}
}

func handleFault(r *http.Response) error {
	body, _ := ioutil.ReadAll(r.Body)
	if os.Getenv("WINRM_DEBUG") != "" {
		log.Println("winrm: fault", string(body))
	}

	buffer := bytes.NewBuffer(body)
	f := &HttpError{500, "Unparsable SOAP error"}
	root, err := xmlpath.Parse(buffer)

	if err != nil {
		return f
	}

	path := xmlpath.MustCompile("//Fault/Reason/Text")
	if reason, ok := path.String(root); ok {
		f.Status = "FAULT: " + reason
	}

	return f
}
