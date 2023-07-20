package importer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gookit/slog"
	"github.com/pkg/errors"
)

type membersResultsWrapper struct {
	MembersResults *MembersResults
	Error          error
}

type Importer struct {
	mailchimpBaseUrl string
	mailchimpApiKey  string
	ometriaEndpoint  string
	ometriaApiKey    string
	stateManager     stateManager
}

type stateManager interface {
	SetCompletedRun(ctx context.Context, timeStamp time.Time, listId string) error
	GetLastRunTimestamp(ctx context.Context, listId string) (string, error)
}

func NewImporter(mailchimpBaseUrl string, mailchimpApiKey string,
	ometriaEndpoint string, ometriaApiKey string, stateManager stateManager) (*Importer, error) {

	if len(mailchimpBaseUrl) == 0 {
		return nil, errors.New("expecting the base url, but it was empty")
	}

	if len(mailchimpApiKey) == 0 {
		return nil, errors.New("expecting the mailchimp api key, but it was empty")
	}

	if len(ometriaEndpoint) == 0 {
		return nil, errors.New("expecting the ometria base url, but it was empty")
	}

	if len(ometriaApiKey) == 0 {
		return nil, errors.New("expecting the ometria api key, but it was empty")
	}

	// just in case, to avoid the double slash between the base URL and API path
	mailchimpBaseUrl = strings.TrimSuffix(mailchimpBaseUrl, "/")

	return &Importer{
		mailchimpBaseUrl: mailchimpBaseUrl,
		mailchimpApiKey:  mailchimpApiKey,
		ometriaEndpoint:  ometriaEndpoint,
		ometriaApiKey:    ometriaApiKey,
		stateManager:     stateManager,
	}, nil
}

func (df *Importer) getMemberFieldsForRequest() []string {
	fields := []string{
		"total_items",
		"members.id",
		"members.email_address",
		"members.full_name",
		"members.status",
		"members.merge_fields.FNAME",
		"members.merge_fields.LNAME",
	}
	return fields
}

func (df *Importer) getMembersPage(ctx context.Context, listId string, offset int, count int, sinceLastChanged string) (*MembersResults, error) {

	url := fmt.Sprintf("%v/lists/%v/members?fields=%v&offset=%v&count=%v",
		df.mailchimpBaseUrl,
		listId,
		strings.Join(df.getMemberFieldsForRequest(), ","),
		offset,
		count)

	if len(sinceLastChanged) > 0 {
		url = url + fmt.Sprintf("&since_last_changed=%v", sinceLastChanged)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, errors.Wrap(err, "unable to create the http request")
	}

	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth("anystring", df.mailchimpApiKey)

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "unable to send the http request")
	}
	data, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, errors.Wrap(err, "unable to read the response body")
	}
	defer res.Body.Close()

	var pageResult MembersResults
	err = json.Unmarshal(data, &pageResult)
	if err != nil {
		return nil, errors.Wrap(err, "unable to unmarshal the response data into struct")
	}

	return &pageResult, nil
}

func (imp *Importer) saveMembers(ctx context.Context, members []Member) error {

	jsonMembers, _ := json.MarshalIndent(members, "", "  ")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, imp.ometriaEndpoint, bytes.NewBuffer(jsonMembers))
	if err != nil {
		return errors.Wrap(err, "unable to create the http request")
	}
	req.Header.Add("Authorization", imp.ometriaApiKey)
	req.Header.Add("Content-Type", "application/json")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return errors.Wrap(err, "unable to send the http request")
	}

	if res.StatusCode != 201 {
		return errors.Wrap(err, "post response was not successful")
	}

	return nil
}

func (imp *Importer) SyncList(ctx context.Context, listId string) error {

	const concurrencyLimit int = 9 // max number of concurrent API calls (the mailchimp docs say the maximum allowed is 10)
	const count int = 900          // our page size (max is 1000)

	// save 'now' timestamp - it will be persisted later, and used to sync only the differences
	nowTimestamp := time.Now()

	// get the last run time stamp from redis
	sinceLastChanged, err := imp.stateManager.GetLastRunTimestamp(ctx, listId)
	if err != nil {
		return errors.Wrap(err, "unable to get the last run timestamp from redis (check connectivity)")
	}
	slog.Info(fmt.Sprintf("last run timestamp for list Id %v: %v", listId, sinceLastChanged))

	slog.Info("fetching the first page")

	// fetch the first page
	page1, err := imp.getMembersPage(ctx, listId, 0, count, sinceLastChanged)
	if err != nil {
		return errors.Wrap(err, "unable to get the first page of results")
	}

	// how many records do we have?
	slog.Info(fmt.Sprintf("total items: %v", page1.TotalItems))
	if page1.TotalItems == 0 {
		err = imp.stateManager.SetCompletedRun(ctx, nowTimestamp, listId)
		if err != nil {
			return errors.Wrap(err, "unable to save the last run timestamp")
		}
		slog.Info("didn't find new members - will wait for the next cycle")
		return nil
	}

	// how many pages do we need to fetch?
	var numberOfPages int = (page1.TotalItems / count) + 1
	slog.Info(fmt.Sprintf("page size: %v", count))
	slog.Info(fmt.Sprintf("number of pages (api calls): %v", numberOfPages))

	// save the first page on Ometria
	err = imp.saveMembers(ctx, page1.Members)
	if err != nil {
		return errors.Wrap(err, "unable to save the first page of results")
	}

	// is it only 1 page?
	if numberOfPages == 1 {
		err = imp.stateManager.SetCompletedRun(ctx, nowTimestamp, listId)
		if err != nil {
			return errors.Wrap(err, "unable to save the last run timestamp")
		}
		slog.Info(fmt.Sprintf("number of members processed so far: %v (total is %v)", len(page1.Members), page1.TotalItems))
		return nil // stop here
	}

	// this is a buffered channel that will block at the concurrency limit
	concurrencyLimitChan := make(chan struct{}, concurrencyLimit)

	// this channel will not block and will collect the page of results
	pageResultsChan := make(chan *membersResultsWrapper)

	var wg sync.WaitGroup

	for i := 1; i < numberOfPages; i++ {

		wg.Add(1)

		go func(offset int) {

			// add one to the concurrency limit. When the limit has been reached, it will block until there is more room
			concurrencyLimitChan <- struct{}{}

			// remove one from the concurrency limit and allow another goroutine to start
			defer func() {
				<-concurrencyLimitChan
				wg.Done()
			}()

			pageResult, err := imp.getMembersPage(ctx, listId, offset, count, sinceLastChanged)
			if err == nil {
				// send to ometria
				err = imp.saveMembers(ctx, pageResult.Members)
			}

			// send the page results struct through the results channel (wrapped, so that we can signal errors)
			pageResultsChan <- &membersResultsWrapper{
				MembersResults: pageResult,
				Error:          err,
			}

		}(i * count)

	}

	// this will check if all the pages have been retrieved
	// when that happens it will close the page results channel
	go func() {
		wg.Wait()
		close(pageResultsChan)
	}()

	var numberProcessed int = 0

	// add page 1 to the count (we've already saved it)
	numberProcessed += len(page1.Members)

	for {
		resultsWrapper, ok := <-pageResultsChan
		if !ok {
			// the page results channel was closed
			break
		}

		if resultsWrapper.Error != nil {
			// stop and return the error, if any
			return errors.Wrap(resultsWrapper.Error, "unexpeceted error 😕")
		}

		numberProcessed += len(resultsWrapper.MembersResults.Members)
		slog.Info(fmt.Sprintf("number of members processed so far: %v (total is %v)", numberProcessed, page1.TotalItems))

		// check if we've reached the total expected amount (or more, in case any new record has been added while this is running)
		if numberProcessed >= page1.TotalItems {
			// save the timestamp so next time we can continue from this point
			err = imp.stateManager.SetCompletedRun(ctx, nowTimestamp, listId)
			if err != nil {
				return errors.Wrap(err, "unable to save the last run timestamp")
			}
			return nil
		}
	}
	return nil
}
