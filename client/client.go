package gitmediaclient

import (
	".."
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/cheggaaa/pb"
	"io"
	"io/ioutil"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
)

const (
	gitMediaType      = "application/vnd.git-media"
	gitMediaMetaType  = gitMediaType + "+json; charset=utf-8"
	gitMediaHeader    = "--git-media."
	gitBoundaryLength = 61
)

func Options(filehash string) error {
	oid := filepath.Base(filehash)
	_, err := os.Stat(filehash)
	if err != nil {
		return err
	}

	req, creds, err := clientRequest("OPTIONS", oid)
	if err != nil {
		return err
	}

	_, err = doRequest(req, creds)
	if err != nil {
		return err
	}

	return nil
}

func Put(filehash, filename string) error {
	if filename == "" {
		filename = filehash
	}

	oid := filepath.Base(filehash)
	stat, err := os.Stat(filehash)
	if err != nil {
		return err
	}

	file, err := os.Open(filehash)
	if err != nil {
		return err
	}

	req, creds, err := clientRequest("PUT", oid)
	if err != nil {
		return err
	}

	bar := pb.StartNew(int(stat.Size()))
	bar.SetUnits(pb.U_BYTES)
	bar.Start()

	req.Header.Set("Content-Type", gitMediaType)
	req.Header.Set("Accept", gitMediaMetaType)
	req.Body = ioutil.NopCloser(bar.NewProxyReader(file))
	req.ContentLength = stat.Size()

	fmt.Printf("Sending %s\n", filename)

	_, err = doRequest(req, creds)
	if err != nil {
		return err
	}

	return nil
}

func Get(filename string) (io.ReadCloser, error) {
	oid := filepath.Base(filename)
	if stat, err := os.Stat(filename); err != nil || stat == nil {
		req, creds, err := clientRequest("GET", oid)
		if err != nil {
			return nil, err
		}

		req.Header.Set("Accept", gitMediaType)
		res, err := doRequest(req, creds)

		if err != nil {
			return nil, err
		}

		contentType := res.Header.Get("Content-Type")
		if contentType == "" {
			return nil, errors.New("Invalid Content-Type")
		}

		if ok, err := validateMediaHeader(contentType, res.Body); !ok {
			return nil, err
		}

		return res.Body, nil
	}

	return os.Open(filename)
}

func validateMediaHeader(contentType string, reader io.Reader) (bool, error) {
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false, errors.New("Invalid Media Type")
	}

	if mediaType != gitMediaType {
		return false, errors.New("Invalid Media Type")
	}

	givenHeader, ok := params["header"]
	if !ok {
		return false, errors.New("Invalid header")
	}

	fullGivenHeader := "--" + givenHeader + "\n"

	header := make([]byte, len(fullGivenHeader))
	_, err = io.ReadAtLeast(reader, header, len(fullGivenHeader))
	if err != nil {
		return false, err
	}

	if string(header) != fullGivenHeader {
		return false, errors.New("Invalid header")
	}

	return true, nil
}

func doRequest(req *http.Request, creds Creds) (*http.Response, error) {
	res, err := http.DefaultClient.Do(req)

	if err == nil {
		if res.StatusCode > 299 {
			execCreds(creds, "reject")

			apierr := &Error{}
			dec := json.NewDecoder(res.Body)
			if err := dec.Decode(apierr); err != nil {
				return res, err
			}

			return res, apierr
		}
	} else {
		execCreds(creds, "approve")
	}

	return res, err
}

func clientRequest(method, oid string) (*http.Request, Creds, error) {
	u := ObjectUrl(oid)
	req, err := http.NewRequest(method, u.String(), nil)
	if err == nil {
		creds, err := credentials(u)
		if err != nil {
			return req, nil, err
		}

		token := fmt.Sprintf("%s:%s", creds["username"], creds["password"])
		auth := "Basic " + base64.URLEncoding.EncodeToString([]byte(token))
		req.Header.Set("Authorization", auth)
		return req, creds, nil
	}

	return req, nil, err
}

func ObjectUrl(oid string) *url.URL {
	c := gitmedia.Config
	u, _ := url.Parse(c.Endpoint())
	u.Path = filepath.Join(u.Path, "/objects/"+oid)
	return u
}

type Error struct {
	Message   string `json:"message"`
	RequestId string `json:"request_id,omitempty"`
}

func (e *Error) Error() string {
	return e.Message
}
