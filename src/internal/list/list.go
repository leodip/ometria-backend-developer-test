package list

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/pkg/errors"
	"github.com/spf13/viper"
)

type ListsResults struct {
	Lists []List `json:"lists"`
}

type List struct {
	Id    string `json:"id"`
	Name  string `json:"name"`
	Stats Stats  `json:"stats"`
}

type Stats struct {
	MemberCount   int `json:"member_count"`
	TotalContacts int `json:"total_contacts"`
}

func GetAllLists() ([]List, error) {
	// this will return all lists associated with the API KEY
	url := fmt.Sprintf("%v/lists?count=1000&include_total_contacts=true", viper.GetString("Mailchimp.BaseUrl"))

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, errors.Wrap(err, "unable to create the http request")
	}

	req.SetBasicAuth("anystring", viper.GetString("Mailchimp.ApiKey"))

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "unable to send the http request")
	}
	data, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, errors.Wrap(err, "unable to read the response body")
	}
	defer res.Body.Close()

	var pageResult ListsResults
	err = json.Unmarshal(data, &pageResult)
	if err != nil {
		return nil, errors.Wrap(err, "unable to unmarshal the response data into struct")
	}
	return pageResult.Lists, nil
}
